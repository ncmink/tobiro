package security

import (
	"errors"
	"sync"
)

// ErrSecretMissing mirrors domain.ErrSecretNotFound without importing domain,
// keeping this package dependency-free for tests.
var ErrSecretMissing = errors.New("no stored password for this credential")

// MemoryStore keeps secrets in process memory only.
//
// It is the fallback when the OS credential store is unavailable (headless
// Linux, CI) and the store used by tests. Secrets are lost on exit — which is
// the point: they are never written to disk.
type MemoryStore struct {
	mu      sync.RWMutex
	secrets map[string]string
	reason  string
}

// NewMemoryStore creates an in-memory store. The reason explains, in the UI,
// why the OS store was not used.
func NewMemoryStore(reason string) *MemoryStore {
	return &MemoryStore{secrets: make(map[string]string), reason: reason}
}

// Backend names the store.
func (m *MemoryStore) Backend() string { return "In-memory (session only)" }

// Available always reports true, with a warning describing the trade-off.
func (m *MemoryStore) Available() (bool, string) {
	msg := "Secrets are kept in memory for this session only and are lost on exit."
	if m.reason != "" {
		msg = m.reason + " " + msg
	}
	return true, msg
}

// Set stores a secret in memory.
func (m *MemoryStore) Set(ref string, write SecretWrite) error {
	if err := ValidateSecretRef(ref); err != nil {
		return err
	}
	if write.Value == "" {
		return errors.New("refusing to store an empty secret")
	}
	m.mu.Lock()
	m.secrets[ref] = write.Value
	m.mu.Unlock()
	return nil
}

// Get returns a stored secret.
func (m *MemoryStore) Get(ref string) (string, error) {
	m.mu.RLock()
	secret, ok := m.secrets[ref]
	m.mu.RUnlock()
	if !ok {
		return "", ErrSecretMissing
	}
	return secret, nil
}

// Has reports whether a secret exists.
func (m *MemoryStore) Has(ref string) bool {
	m.mu.RLock()
	_, ok := m.secrets[ref]
	m.mu.RUnlock()
	return ok
}

// Delete removes a secret.
func (m *MemoryStore) Delete(ref string) error {
	m.mu.Lock()
	delete(m.secrets, ref)
	m.mu.Unlock()
	return nil
}
