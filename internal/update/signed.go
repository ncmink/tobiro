package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

const signatureSuffix = ".sig"

const maxSignatureBytes = 1024

type Signed struct {
	inner  updater.Provider
	client *http.Client
}

func WithSignatures(inner updater.Provider) *Signed {
	return &Signed{
		inner:  inner,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Signed) Name() string { return s.inner.Name() }

func (s *Signed) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	release, err := s.inner.Check(ctx, req)
	if err != nil || release == nil {
		return release, err
	}

	assetURL, _ := release.Metadata["github.asset.url"].(string)
	if assetURL == "" {
		return nil, errors.New("update: release carries no asset URL to resolve a signature against")
	}

	signature, err := s.fetchSignature(ctx, assetURL+signatureSuffix)
	if err != nil {
		return nil, fmt.Errorf("update: %s is not signed by this project: %w", release.Artifact.Filename, err)
	}

	if release.Verification == nil {
		release.Verification = &updater.Verification{}
	}
	if release.Verification.DigestAlgo == "" {
		release.Verification.DigestAlgo = "sha256"
	}
	release.Verification.SignatureAlgo = "ed25519"
	release.Verification.Signature = signature

	return release, nil
}

func (s *Signed) Download(ctx context.Context, release *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return s.inner.Download(ctx, release, dst, onProgress)
}

func (s *Signed) fetchSignature(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetching signature: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching signature: HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxSignatureBytes))
	if err != nil {
		return nil, fmt.Errorf("reading signature: %w", err)
	}
	return decodeSignature(body)
}

func decodeSignature(body []byte) ([]byte, error) {
	encoded := strings.TrimSpace(string(body))
	if encoded == "" {
		return nil, errors.New("signature file is empty")
	}
	signature, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("signature is not base64: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature is %d bytes, want %d", len(signature), ed25519.SignatureSize)
	}
	return signature, nil
}
