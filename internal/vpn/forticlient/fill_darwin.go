//go:build darwin

package forticlient

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"tobiro/internal/domain"
)

const (
	// probeWaitSeconds is how long one probe run keeps scanning before it
	// reports "waiting" and the manager starts another.
	probeWaitSeconds = 12
	// fillWaitSeconds covers the Electron tree briefly dropping the fields
	// between the probe and the fill.
	fillWaitSeconds = 4
	// scriptSlack is added to the in-script wait for osascript startup and a
	// slow accessibility tree.
	scriptSlack   = 15 * time.Second
	osascriptPath = "/usr/bin/osascript"
)

// FillNativeLogin types the stored username and password into FortiClient.
// It never clicks Connect. Paste is not used: FortiClient rejects it.
func (p *Provider) FillNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return nil
	}
	result, err := p.runScript(ctx, credential, false, fillWaitSeconds)
	if err != nil {
		return err
	}
	if result == "filled" {
		return nil
	}
	return errFillWaiting
}

// ProbeNativeLogin reports whether FortiClient's login fields are on screen.
// It never takes focus and never types.
func (p *Provider) ProbeNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return errFillWaiting
	}
	result, err := p.runScript(ctx, credential, true, probeWaitSeconds)
	if err != nil {
		return err
	}
	if result == "ready" {
		return nil
	}
	return errFillWaiting
}

func (p *Provider) runScript(ctx context.Context, credential domain.Credential, probeOnly bool, waitSeconds int) (string, error) {
	if err := validateFillValue("username", credential.Username); err != nil {
		return "", err
	}
	if err := validateFillValue("password", credential.Secret); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(waitSeconds)*time.Second+scriptSlack)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(credential.Username, credential.Secret, probeOnly, waitSeconds))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stderr := strings.TrimSpace(errBuf.String())
	if isAssistiveAccessError(stderr) || (err != nil && isAssistiveAccessError(err.Error())) {
		return "", domain.ErrAssistiveAccess
	}
	if err != nil {
		if stderr == "" {
			stderr = err.Error()
		}
		return "", fmt.Errorf("%w: %s", errFillWaiting, stderr)
	}
	return strings.TrimSpace(out.String()), nil
}
