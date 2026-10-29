package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuthMethod describes how a credential authenticates against the VPN.
//
// Username+password is the common case today, but the domain is deliberately
// not built around it: certificate, SSO and browser flows carry no username at
// all, and adapters advertise what they support through Capabilities.
type AuthMethod string

const (
	AuthPassword    AuthMethod = "password"
	AuthPasswordOTP AuthMethod = "password_otp"
	AuthCertificate AuthMethod = "certificate"
	AuthSSO         AuthMethod = "sso"
	AuthBrowser     AuthMethod = "browser"
	AuthSAML        AuthMethod = "saml"

	// AuthSSHKey is a private key file, with its passphrase — when the key has
	// one — in the OS credential store like any other secret.
	AuthSSHKey AuthMethod = "ssh_key"

	// AuthSSHAgent delegates to the running ssh-agent, so Tobiro never
	// sees key material at all. It is the method to prefer where it works.
	AuthSSHAgent AuthMethod = "ssh_agent"
)

// AuthMethodInfo describes an auth method for presentation in the UI.
type AuthMethodInfo struct {
	ID           AuthMethod `json:"id"`
	DisplayName  string     `json:"displayName"`
	NeedsSecret  bool       `json:"needsSecret"`
	Implemented  bool       `json:"implemented"`
	Descriptions string     `json:"description"`

	// Kinds limits the method to the access kinds it makes sense for, so the
	// account form does not offer a private key for a Forcepoint gateway.
	Kinds []AccessKind `json:"kinds"`
}

// AppliesTo reports whether this method can be used by the given access kind.
func (m AuthMethodInfo) AppliesTo(kind AccessKind) bool {
	kind = kind.Normalise()
	for _, k := range m.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// SupportedAuthMethods lists every method the domain models. Methods that are
// not implemented yet are still enumerated so the UI can show them as coming
// soon rather than the model having to change later.
func SupportedAuthMethods() []AuthMethodInfo {
	both := []AccessKind{KindVPN, KindSSH}
	vpnOnly := []AccessKind{KindVPN}
	sshOnly := []AccessKind{KindSSH}

	return []AuthMethodInfo{
		{ID: AuthPassword, DisplayName: "Username + Password", NeedsSecret: true, Implemented: true,
			Kinds:        both,
			Descriptions: "Standard username and password authentication."},
		{ID: AuthPasswordOTP, DisplayName: "Username + Password + OTP", NeedsSecret: true, Implemented: true,
			Kinds:        vpnOnly,
			Descriptions: "Password plus a one-time code requested during connection."},
		{ID: AuthSSHAgent, DisplayName: "SSH agent", NeedsSecret: false, Implemented: true,
			Kinds:        sshOnly,
			Descriptions: "The running ssh-agent answers the challenge; no key material is read here."},
		{ID: AuthSSHKey, DisplayName: "SSH private key", NeedsSecret: false, Implemented: true,
			Kinds:        sshOnly,
			Descriptions: "A private key file on this machine. Its passphrase, if it has one, goes to the OS credential store."},
		{ID: AuthCertificate, DisplayName: "Certificate", NeedsSecret: false, Implemented: false,
			Kinds:        vpnOnly,
			Descriptions: "Client certificate authentication."},
		{ID: AuthSSO, DisplayName: "SSO", NeedsSecret: false, Implemented: false,
			Kinds:        vpnOnly,
			Descriptions: "Single sign-on through the identity provider."},
		{ID: AuthBrowser, DisplayName: "Browser authentication", NeedsSecret: false, Implemented: false,
			Kinds:        vpnOnly,
			Descriptions: "External browser completes the login."},
		{ID: AuthSAML, DisplayName: "SAML", NeedsSecret: false, Implemented: false,
			Kinds:        vpnOnly,
			Descriptions: "SAML federated authentication."},
	}
}

// Credential is one login identity usable on one VPN profile.
//
// Identity rules (these are load-bearing, not stylistic):
//   - ID is an internal UUID and is the ONLY identity key.
//   - Username is descriptive data. It is NOT unique — "devops" may legitimately
//     exist on many profiles, and even more than once within a single profile.
//   - Label is what a human picks from the UI ("Ming", "DevOps Shared").
//   - The password itself never lives here; SecretRef points at an OS keychain
//     entry and Secret is transient (see below).
type Credential struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`

	Label    string `json:"label"`
	Username string `json:"username"`

	// SecretRef addresses the secret in the OS credential store. It is a
	// reference only — never the secret itself.
	SecretRef string `json:"secretRef"`

	AuthMethod AuthMethod `json:"authMethod"`
	IsDefault  bool       `json:"isDefault"`

	// PrivateKeyPath points at an SSH private key file on this machine. The
	// file is read at connect time and its contents are never copied here; a
	// passphrase, if the key has one, lives in SecretRef like any other secret.
	PrivateKeyPath string `json:"privateKeyPath,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Secret is populated in memory only, for the duration of a single connect
	// call, after being read from the secure store. It is never persisted
	// (json:"-"), never returned to the frontend, and String() masks it so an
	// accidental %v or %+v cannot leak it into a log line.
	Secret string `json:"-"`
}

// CredentialView is the shape the frontend is allowed to see. It deliberately
// has no secret field at all, so a password cannot leak through the bindings
// even by accident.
type CredentialView struct {
	ID             string     `json:"id"`
	ProfileID      string     `json:"profileId"`
	Label          string     `json:"label"`
	Username       string     `json:"username"`
	SecretRef      string     `json:"secretRef"`
	AuthMethod     AuthMethod `json:"authMethod"`
	IsDefault      bool       `json:"isDefault"`
	HasPassword    bool       `json:"hasPassword"`
	PrivateKeyPath string     `json:"privateKeyPath,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// View converts to the frontend-safe representation.
func (c Credential) View(hasPassword bool) CredentialView {
	return CredentialView{
		ID:             c.ID,
		ProfileID:      c.ProfileID,
		Label:          c.Label,
		Username:       c.Username,
		SecretRef:      c.SecretRef,
		AuthMethod:     c.AuthMethod,
		IsDefault:      c.IsDefault,
		HasPassword:    hasPassword,
		PrivateKeyPath: c.PrivateKeyPath,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

// String masks the transient secret. Without this, logging a credential with
// %v would print the password.
func (c Credential) String() string {
	secret := "unset"
	if c.Secret != "" {
		secret = "***"
	}
	return fmt.Sprintf("Credential{id=%s profile=%s label=%q username=%q secret=%s}",
		c.ID, c.ProfileID, c.Label, c.Username, secret)
}

// DisplayName is the label, falling back to the username for older records.
func (c Credential) DisplayName() string {
	if strings.TrimSpace(c.Label) != "" {
		return c.Label
	}
	if strings.TrimSpace(c.Username) != "" {
		return c.Username
	}
	return c.ID
}

// RequiresSecret reports whether this credential's auth method needs a stored
// password.
func (c Credential) RequiresSecret() bool {
	for _, m := range SupportedAuthMethods() {
		if m.ID == c.AuthMethod {
			return m.NeedsSecret
		}
	}
	return false
}

// CredentialInput carries user-supplied credential fields. The password is
// separated from the persisted model on purpose: it travels straight to the
// secure store and is never written into application data.
type CredentialInput struct {
	Label      string     `json:"label"`
	Username   string     `json:"username"`
	AuthMethod AuthMethod `json:"authMethod"`
	IsDefault  bool       `json:"isDefault"`

	// PrivateKeyPath is the SSH key file to authenticate with. It is a path,
	// never key material.
	PrivateKeyPath string `json:"privateKeyPath"`

	// Password is transient. On update, an empty value means "keep the
	// existing stored secret".
	Password string `json:"password"`
}

// Validate checks user-supplied credential fields.
func (in CredentialInput) Validate() error {
	if strings.TrimSpace(in.Label) == "" {
		return errors.New("display name is required")
	}
	if len(in.Label) > 128 {
		return errors.New("display name is too long (max 128 characters)")
	}
	if len(in.Username) > 256 {
		return errors.New("username is too long (max 256 characters)")
	}
	if err := ValidatePrivateKeyPath(in.PrivateKeyPath); err != nil {
		return err
	}
	if in.AuthMethod == AuthSSHKey && strings.TrimSpace(in.PrivateKeyPath) == "" {
		return errors.New("a private key file is required for this authentication method")
	}
	if in.AuthMethod == "" {
		return nil
	}
	for _, m := range SupportedAuthMethods() {
		if m.ID == in.AuthMethod {
			if !m.Implemented {
				return fmt.Errorf("authentication method %q is not implemented yet", in.AuthMethod)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown authentication method %q", in.AuthMethod)
}

// ValidatePrivateKeyPath accepts an empty path, an absolute one, or one under
// the user's home directory written with a tilde — which is how every SSH key
// on a developer machine is actually referred to.
func ValidatePrivateKeyPath(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return nil
	}
	if len(p) > 1024 {
		return errors.New("private key path is too long")
	}
	if strings.ContainsAny(p, ";&|$`\n\r\"'*?") {
		return fmt.Errorf("private key path %q contains invalid characters", path)
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "/") || isWindowsAbsolute(p) {
		return nil
	}
	return fmt.Errorf("private key path %q must be absolute or start with ~/", path)
}
