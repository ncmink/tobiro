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

var legacyServiceNames = []string{"JANUZ", "Tobiro"}

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
	if err := keyring.Set(serviceName, canary, "probe"); err != nil {
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
func (s *Store) Set(ref string, secret string) error {
	if err := security.ValidateSecretRef(ref); err != nil {
		return err
	}
	if secret == "" {
		return errors.New("refusing to store an empty secret")
	}
	if err := keyring.Set(serviceName, ref, secret); err != nil {
		// The error is wrapped without the secret ever being interpolated.
		return fmt.Errorf("store secret in %s: %w", s.Backend(), err)
	}
	return nil
}

func getWithLegacy(ref string) (string, error) {
	secret, err := keyring.Get(serviceName, ref)
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return "", err
	}
	for _, legacy := range legacyServiceNames {
		secret, err = keyring.Get(legacy, ref)
		if err == nil {
			return secret, nil
		}
		if !errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
	}
	return "", keyring.ErrNotFound
}

// Get returns the secret for ref.
func (s *Store) Get(ref string) (string, error) {
	if err := security.ValidateSecretRef(ref); err != nil {
		return "", err
	}
	secret, err := getWithLegacy(ref)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", domain.ErrSecretNotFound
		}
		return "", fmt.Errorf("read secret from %s: %w", s.Backend(), err)
	}
	return secret, nil
}

// Has reports whether a secret exists without exposing it.
func (s *Store) Has(ref string) bool {
	if err := security.ValidateSecretRef(ref); err != nil {
		return false
	}
	_, err := getWithLegacy(ref)
	return err == nil
}

// Delete removes a secret; missing entries are not an error.
func (s *Store) Delete(ref string) error {
	if err := security.ValidateSecretRef(ref); err != nil {
		return err
	}
	if err := keyring.Delete(serviceName, ref); err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete secret from %s: %w", s.Backend(), err)
		}
	}
	for _, legacy := range legacyServiceNames {
		if err := keyring.Delete(legacy, ref); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete legacy secret from %s: %w", legacy, err)
		}
	}
	return nil
}
