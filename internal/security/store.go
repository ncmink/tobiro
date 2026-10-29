// Package security defines the secure credential storage abstraction.
//
// Passwords are never written to application data (JSON/YAML/SQLite), never
// held in Pinia or localStorage, never logged and never returned to the
// frontend. Application data stores only a SecretRef pointing at an entry in
// the operating system's credential store.
package security

import (
	"fmt"
	"regexp"
	"strings"
)

// SecureCredentialStore is the contract every backend must satisfy.
//
// Implementations map a reference to an OS-managed secret:
//
//	macOS   -> Keychain
//	Windows -> Credential Manager
//	Linux   -> Secret Service / keyring
// SecretWrite is what Set persists. Comment is optional metadata for the OS
// credential store (Keychain "Comments" on macOS). It must never contain the
// secret itself.
type SecretWrite struct {
	Value   string
	Comment string
}

type SecureCredentialStore interface {
	// Set stores or replaces the secret for ref.
	Set(ref string, write SecretWrite) error

	// Get returns the secret for ref, or domain.ErrSecretNotFound.
	Get(ref string) (string, error)

	// Has reports whether a secret exists, without returning it. The UI uses
	// this for the "Password saved" indicator.
	Has(ref string) bool

	// Delete removes the secret. Deleting a missing secret is not an error.
	Delete(ref string) error

	// Backend names the concrete store, for display in Settings.
	Backend() string

	// Available reports whether the OS store is usable on this machine.
	Available() (bool, string)
}

var refSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// BuildSecretRef composes the reference persisted alongside a credential.
//
// The reference is derived from UUIDs only — never from a username, which is
// not unique and can change.
func BuildSecretRef(profileID, credentialID string) string {
	return fmt.Sprintf("tobiro/%s/%s", profileID, credentialID)
}

// ValidateSecretRef guards against a malformed reference reaching the OS store.
func ValidateSecretRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("secret reference is empty")
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 3 {
		return fmt.Errorf("secret reference %q has an unexpected shape", ref)
	}
	switch parts[0] {
	case "tobiro", "biebie-access", "januz":
	default:
		return fmt.Errorf("secret reference %q has an unexpected shape", ref)
	}
	for _, part := range parts[1:] {
		if !refSegmentPattern.MatchString(part) {
			return fmt.Errorf("secret reference %q contains invalid characters", ref)
		}
	}
	return nil
}
