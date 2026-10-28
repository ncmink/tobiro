// Package credential implements credential use cases.
//
// The rule that shapes this whole package: a credential is identified by an
// internal UUID, never by its username. Two credentials may share a username on
// the same profile, and the same username commonly appears across customers.
package credential

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/security"
	"tobiro/internal/store"
)

// Service manages credentials and their secrets.
type Service struct {
	store   *store.Store
	secrets security.SecureCredentialStore
	log     *logging.Logger
}

// NewService creates the credential service.
func NewService(s *store.Store, secrets security.SecureCredentialStore, log *logging.Logger) *Service {
	return &Service{store: s, secrets: secrets, log: log}
}

// ListForProfile returns frontend-safe credential views for a profile, ordered
// by the profile's credential ordering (default first).
func (s *Service) ListForProfile(profileID string) []domain.CredentialView {
	creds := s.rawForProfile(profileID)
	out := make([]domain.CredentialView, 0, len(creds))
	for _, c := range creds {
		out = append(out, c.View(s.secrets.Has(c.SecretRef)))
	}
	return out
}

func (s *Service) rawForProfile(profileID string) []domain.Credential {
	var (
		creds []domain.Credential
		order []string
	)
	s.store.Read(func(d *store.Data) {
		for _, c := range d.Credentials {
			if c.ProfileID == profileID {
				creds = append(creds, c)
			}
		}
		for _, p := range d.Profiles {
			if p.ID == profileID {
				order = append(order, p.CredentialIDs...)
				break
			}
		}
	})

	rank := make(map[string]int, len(order))
	for i, id := range order {
		rank[id] = i
	}
	// Default credential floats to the top; the rest keep profile ordering.
	sortCredentials(creds, rank)
	return creds
}

func sortCredentials(creds []domain.Credential, rank map[string]int) {
	for i := 1; i < len(creds); i++ {
		for j := i; j > 0; j-- {
			if credLess(creds[j], creds[j-1], rank) {
				creds[j], creds[j-1] = creds[j-1], creds[j]
				continue
			}
			break
		}
	}
}

func credLess(a, b domain.Credential, rank map[string]int) bool {
	if a.IsDefault != b.IsDefault {
		return a.IsDefault
	}
	ra, aok := rank[a.ID]
	rb, bok := rank[b.ID]
	switch {
	case aok && bok:
		return ra < rb
	case aok:
		return true
	case bok:
		return false
	default:
		return a.CreatedAt.Before(b.CreatedAt)
	}
}

// Get returns a credential by UUID, without its secret.
func (s *Service) Get(id string) (domain.Credential, error) {
	var (
		found domain.Credential
		ok    bool
	)
	s.store.Read(func(d *store.Data) {
		for _, c := range d.Credentials {
			if c.ID == id {
				found, ok = c, true
				return
			}
		}
	})
	if !ok {
		return domain.Credential{}, fmt.Errorf("credential %s: %w", id, domain.ErrNotFound)
	}
	return found, nil
}

// Create adds a credential to a profile and stores its password in the OS
// credential store (never in application data).
func (s *Service) Create(profileID string, in domain.CredentialInput) (domain.CredentialView, error) {
	if err := in.Validate(); err != nil {
		return domain.CredentialView{}, err
	}
	if in.AuthMethod == "" {
		in.AuthMethod = domain.AuthPassword
	}

	now := time.Now()
	cred := domain.Credential{
		ID:             uuid.NewString(),
		ProfileID:      profileID,
		Label:          strings.TrimSpace(in.Label),
		Username:       strings.TrimSpace(in.Username),
		AuthMethod:     in.AuthMethod,
		IsDefault:      in.IsDefault,
		PrivateKeyPath: strings.TrimSpace(in.PrivateKeyPath),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	cred.SecretRef = security.BuildSecretRef(profileID, cred.ID)

	err := s.store.Update(func(d *store.Data) error {
		profileIdx := -1
		for i := range d.Profiles {
			if d.Profiles[i].ID == profileID {
				profileIdx = i
				break
			}
		}
		if profileIdx < 0 {
			return fmt.Errorf("profile %s: %w", profileID, domain.ErrNotFound)
		}

		// First credential on a profile is implicitly the default.
		if len(d.Profiles[profileIdx].CredentialIDs) == 0 {
			cred.IsDefault = true
		}
		if cred.IsDefault {
			clearDefaults(d, profileID)
		}

		d.Credentials = append(d.Credentials, cred)
		d.Profiles[profileIdx].CredentialIDs = append(d.Profiles[profileIdx].CredentialIDs, cred.ID)
		d.Profiles[profileIdx].UpdatedAt = now
		return nil
	})
	if err != nil {
		return domain.CredentialView{}, err
	}

	hasPassword := false
	if in.Password != "" {
		if err := s.secrets.Set(cred.SecretRef, in.Password); err != nil {
			// Metadata is already committed; report the storage failure so the
			// user can retry rather than silently having no password.
			s.log.Error("Could not save password for credential=%s: %v", cred.ID, err)
			return cred.View(false), fmt.Errorf("credential saved, but storing the password failed: %w", err)
		}
		hasPassword = true
	}

	s.log.Info("Added credential=%s label=%q to profile=%s", cred.ID, cred.Label, profileID)
	return cred.View(hasPassword), nil
}

// Update changes credential fields. An empty password keeps the stored secret.
func (s *Service) Update(id string, in domain.CredentialInput) (domain.CredentialView, error) {
	if err := in.Validate(); err != nil {
		return domain.CredentialView{}, err
	}

	var updated domain.Credential
	err := s.store.Update(func(d *store.Data) error {
		idx := -1
		for i := range d.Credentials {
			if d.Credentials[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("credential %s: %w", id, domain.ErrNotFound)
		}

		if in.IsDefault && !d.Credentials[idx].IsDefault {
			clearDefaults(d, d.Credentials[idx].ProfileID)
		}

		d.Credentials[idx].Label = strings.TrimSpace(in.Label)
		d.Credentials[idx].Username = strings.TrimSpace(in.Username)
		if in.AuthMethod != "" {
			d.Credentials[idx].AuthMethod = in.AuthMethod
		}
		d.Credentials[idx].IsDefault = in.IsDefault
		d.Credentials[idx].PrivateKeyPath = strings.TrimSpace(in.PrivateKeyPath)
		d.Credentials[idx].UpdatedAt = time.Now()
		updated = d.Credentials[idx]
		return nil
	})
	if err != nil {
		return domain.CredentialView{}, err
	}

	if in.Password != "" {
		if err := s.secrets.Set(updated.SecretRef, in.Password); err != nil {
			s.log.Error("Could not update password for credential=%s: %v", id, err)
			return updated.View(s.secrets.Has(updated.SecretRef)),
				fmt.Errorf("credential saved, but storing the password failed: %w", err)
		}
	}

	s.log.Info("Updated credential=%s label=%q", updated.ID, updated.Label)
	return updated.View(s.secrets.Has(updated.SecretRef)), nil
}

// Delete removes a credential and its stored secret.
func (s *Service) Delete(id string) error {
	var removed domain.Credential
	err := s.store.Update(func(d *store.Data) error {
		idx := -1
		for i := range d.Credentials {
			if d.Credentials[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("credential %s: %w", id, domain.ErrNotFound)
		}
		removed = d.Credentials[idx]
		d.Credentials = append(d.Credentials[:idx], d.Credentials[idx+1:]...)

		for i := range d.Profiles {
			if d.Profiles[i].ID != removed.ProfileID {
				continue
			}
			d.Profiles[i].CredentialIDs = removeString(d.Profiles[i].CredentialIDs, id)
			// Promote another credential so the profile keeps a default.
			if removed.IsDefault && len(d.Profiles[i].CredentialIDs) > 0 {
				promoteDefault(d, d.Profiles[i].CredentialIDs[0])
			}
			d.Profiles[i].UpdatedAt = time.Now()
			break
		}

		if prefs, ok := d.Preferences[removed.ProfileID]; ok && prefs.LastCredentialID == id {
			prefs.LastCredentialID = ""
			d.Preferences[removed.ProfileID] = prefs
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := s.secrets.Delete(removed.SecretRef); err != nil {
		s.log.Warn("Removed credential=%s but its stored password could not be deleted: %v", id, err)
	}
	s.log.Info("Deleted credential=%s from profile=%s", id, removed.ProfileID)
	return nil
}

// DeleteForProfile removes every credential belonging to a profile, along with
// their secrets. Used when a profile is deleted.
func (s *Service) DeleteForProfile(profileID string) error {
	var removed []domain.Credential
	err := s.store.Update(func(d *store.Data) error {
		kept := d.Credentials[:0]
		for _, c := range d.Credentials {
			if c.ProfileID == profileID {
				removed = append(removed, c)
				continue
			}
			kept = append(kept, c)
		}
		d.Credentials = kept
		return nil
	})
	if err != nil {
		return err
	}
	for _, c := range removed {
		if err := s.secrets.Delete(c.SecretRef); err != nil {
			s.log.Warn("Could not delete stored password for credential=%s: %v", c.ID, err)
		}
	}
	return nil
}

// SetDefault marks one credential as the profile default.
func (s *Service) SetDefault(id string) error {
	return s.store.Update(func(d *store.Data) error {
		for i := range d.Credentials {
			if d.Credentials[i].ID != id {
				continue
			}
			clearDefaults(d, d.Credentials[i].ProfileID)
			d.Credentials[i].IsDefault = true
			d.Credentials[i].UpdatedAt = time.Now()
			return nil
		}
		return fmt.Errorf("credential %s: %w", id, domain.ErrNotFound)
	})
}

// Resolve loads a credential and attaches its secret for a single connect call.
//
// This is the ONLY path that materialises a password, and the result never
// leaves the Go process: it goes straight to the provider adapter. The returned
// credential must not be sent to the frontend.
func (s *Service) Resolve(credentialID string) (domain.Credential, error) {
	cred, err := s.Get(credentialID)
	if err != nil {
		return domain.Credential{}, err
	}
	if !cred.RequiresSecret() {
		// A private key's passphrase is optional in a way a gateway password is
		// not: most keys on a developer machine have none, and demanding one
		// would refuse a credential that works. Attach it when it exists and
		// let the key itself decide whether it was needed.
		if cred.AuthMethod == domain.AuthSSHKey {
			if secret, err := s.secrets.Get(cred.SecretRef); err == nil {
				cred.Secret = secret
			}
		}
		return cred, nil
	}

	secret, err := s.secrets.Get(cred.SecretRef)
	if err != nil {
		if errors.Is(err, domain.ErrSecretNotFound) || errors.Is(err, security.ErrSecretMissing) {
			return domain.Credential{}, fmt.Errorf(
				"no password saved for %q — open the connection and edit the account to add one",
				cred.DisplayName())
		}
		return domain.Credential{}, err
	}
	cred.Secret = secret
	return cred, nil
}

// HasSecret reports whether a password is stored for a credential.
func (s *Service) HasSecret(credentialID string) bool {
	cred, err := s.Get(credentialID)
	if err != nil {
		return false
	}
	return s.secrets.Has(cred.SecretRef)
}

// SelectDefault picks which credential to preselect: the last one used, else
// the profile default, else the only one. Returns "" when there is no choice
// to make.
func (s *Service) SelectDefault(profileID, lastUsedID string) string {
	creds := s.rawForProfile(profileID)
	if len(creds) == 0 {
		return ""
	}
	for _, c := range creds {
		if c.ID == lastUsedID {
			return c.ID
		}
	}
	for _, c := range creds {
		if c.IsDefault {
			return c.ID
		}
	}
	return creds[0].ID
}

func clearDefaults(d *store.Data, profileID string) {
	for i := range d.Credentials {
		if d.Credentials[i].ProfileID == profileID {
			d.Credentials[i].IsDefault = false
		}
	}
}

func promoteDefault(d *store.Data, credentialID string) {
	for i := range d.Credentials {
		if d.Credentials[i].ID == credentialID {
			d.Credentials[i].IsDefault = true
			return
		}
	}
}

func removeString(list []string, value string) []string {
	out := list[:0]
	for _, v := range list {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}
