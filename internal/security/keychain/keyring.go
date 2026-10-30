// Package keychain implements SecureCredentialStore on top of the operating
// system credential store: macOS Keychain, Windows Credential Manager, or the
// freedesktop Secret Service on Linux.
package keychain

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/zalando/go-keyring"

	"tobiro/internal/domain"
	"tobiro/internal/security"
)

// serviceName is the entry Tobiro owns in the OS credential store.
const serviceName = "Tobiro"

// legacyServiceNames are pre-rename Keychain service labels still tried on read.
var legacyServiceNames = []string{"biebie-access", "Biebie Access", "januz"}

// Store is the OS-backed credential store.
type Store struct {
	mu        sync.Mutex
	probed    bool
	available bool
	reason    string
}

// New creates the OS keychain store.
func New() *Store { return &Store{} }

// Backend names the platform store for the Settings screen.
func (s *Store) Backend() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS Keychain"
	case "windows":
		return "Windows Credential Manager"
	case "linux":
		return "Secret Service (libsecret)"
	default:
		return "OS credential store"
	}
}

// Available probes the store once by writing and deleting a canary entry, so
// the UI can warn early (e.g. a headless Linux box with no Secret Service)
// rather than failing at connect time.
func (s *Store) Available() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.probed {
		return s.available, s.reason
	}
	s.probed = true

	const canary = "tobiro/_probe/_probe"
	if err := platformSet(serviceName, canary, security.SecretWrite{Value: "probe"}); err != nil {
		s.available = false
		s.reason = fmt.Sprintf("%s is not accessible: %v", s.Backend(), err)
		return s.available, s.reason
	}
	_ = keyring.Delete(serviceName, canary)

	s.available = true
	s.reason = fmt.Sprintf("%s is available", s.Backend())
	return s.available, s.reason
}

// Set stores or replaces a secret.
func (s *Store) Set(ref string, write security.SecretWrite) error {
	if err := security.ValidateSecretRef(ref); err != nil {
		return err
	}
	if write.Value == "" {
		return errors.New("refusing to store an empty secret")
	}
	if err := platformSet(serviceName, ref, write); err != nil {
		return fmt.Errorf("store secret in %s: %w", s.Backend(), err)
	}
	return nil
}

// Get returns the secret for ref.
func (s *Store) Get(ref string) (string, error) {
	if err := security.ValidateSecretRef(ref); err != nil {
		return "", err
	}
	secret, err := getFromServices(ref)
	if err != nil {
		return "", err
	}
	return secret, nil
}

// Has reports whether a secret exists without exposing it.
func (s *Store) Has(ref string) bool {
	if err := security.ValidateSecretRef(ref); err != nil {
		return false
	}
	_, err := getFromServices(ref)
	return err == nil
}

// Delete removes a secret; missing entries are not an error.
func (s *Store) Delete(ref string) error {
	if err := security.ValidateSecretRef(ref); err != nil {
		return err
	}
	var lastErr error
	for _, svc := range allServiceNames() {
		if err := keyring.Delete(svc, ref); err != nil {
			if errors.Is(err, keyring.ErrNotFound) {
				continue
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return fmt.Errorf("delete secret from %s: %w", s.Backend(), lastErr)
	}
	return nil
}

func allServiceNames() []string {
	out := make([]string, 0, 1+len(legacyServiceNames))
	out = append(out, serviceName)
	out = append(out, legacyServiceNames...)
	return out
}

func getFromServices(ref string) (string, error) {
	var lastErr error
	for _, svc := range allServiceNames() {
		secret, err := keyring.Get(svc, ref)
		if err == nil {
			return secret, nil
		}
		if errors.Is(err, keyring.ErrNotFound) {
			lastErr = domain.ErrSecretNotFound
			continue
		}
		return "", fmt.Errorf("read secret from %s: %w", svc, err)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", domain.ErrSecretNotFound
}
