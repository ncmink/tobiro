// Package transfer converts connections to and from a shareable YAML document.
//
// The document is meant to be sent to a colleague, so it holds no secrets and
// no machine-local values: no passwords, no internal UUIDs, no pinned client
// paths. The recipient imports the file and adds their own password.
package transfer

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"tobiro/internal/domain"
)

// Version is the schema version written into every export.
//
// Version 2 added SSH tunnels. A version 1 file still imports: it simply has no
// kind, which reads as VPN.
const Version = 2

// header explains the file to whoever opens it in an editor.
const header = `# Tobiro — connections
#
# Passwords are never included in this file. After importing, open each
# connection and add the password for your own account.
`

// Document is the on-file shape.
type Document struct {
	Version     int          `yaml:"version"`
	ExportedAt  string       `yaml:"exportedAt,omitempty"`
	Connections []Connection `yaml:"connections"`
}

// Connection is one shareable connection definition.
type Connection struct {
	Name string `yaml:"name"`

	// Kind is omitted for a VPN connection, so a file exported by this build
	// still reads the way version 1 files did for the common case.
	Kind string `yaml:"kind,omitempty"`

	Provider      string    `yaml:"provider"`
	Host          string    `yaml:"host,omitempty"`
	Port          int       `yaml:"port,omitempty"`
	VendorProfile string    `yaml:"vendorProfile,omitempty"`
	SSHUser       string    `yaml:"sshUser,omitempty"`
	Forwards      []Forward `yaml:"forwards,omitempty"`

	// Requires names the connection that has to be up first — by name, not by
	// identifier, because the recipient's copy of it has an identifier of its
	// own. Names are unique here, which is what makes that unambiguous.
	Requires string `yaml:"requires,omitempty"`

	Notes    string    `yaml:"notes,omitempty"`
	Accounts []Account `yaml:"accounts,omitempty"`
}

// Forward is one local port an SSH tunnel lends to the customer network.
type Forward struct {
	Name       string `yaml:"name,omitempty"`
	LocalPort  int    `yaml:"localPort"`
	RemoteHost string `yaml:"remoteHost"`
	RemotePort int    `yaml:"remotePort"`
}

// Account is a login identity without its password.
//
// A private key path is machine-local — the recipient's key lives somewhere
// else, under a different home directory — so it is left out for the same
// reason a pinned client path is.
type Account struct {
	Label      string `yaml:"label"`
	Username   string `yaml:"username,omitempty"`
	AuthMethod string `yaml:"authMethod,omitempty"`
	Default    bool   `yaml:"default,omitempty"`
}

// Build assembles a document from profiles and their accounts.
func Build(profiles []domain.VPNProfile, accountsOf func(profileID string) []domain.CredentialView) Document {
	doc := Document{
		Version:     Version,
		ExportedAt:  time.Now().Format(time.RFC3339),
		Connections: make([]Connection, 0, len(profiles)),
	}

	// A requirement is only written when the connection it points at is in the
	// same file. Naming one that is not there would describe an order the
	// recipient cannot follow.
	names := make(map[string]string, len(profiles))
	for _, p := range profiles {
		names[p.ID] = p.Name
	}

	for _, p := range profiles {
		conn := Connection{
			Name:          p.Name,
			Provider:      string(p.Provider),
			Host:          p.Host,
			Port:          p.Port,
			VendorProfile: p.VendorProfile,
			Requires:      names[p.Requires],
			Notes:         p.Notes,
		}
		if p.IsSSH() {
			conn.Kind = string(domain.KindSSH)
			conn.SSHUser = p.SSH.User
			for _, f := range p.SSH.Forwards {
				conn.Forwards = append(conn.Forwards, Forward{
					Name:       f.Name,
					LocalPort:  f.LocalPort,
					RemoteHost: f.RemoteHost,
					RemotePort: f.RemotePort,
				})
			}
		}
		if accountsOf != nil {
			for _, c := range accountsOf(p.ID) {
				conn.Accounts = append(conn.Accounts, Account{
					Label:      c.Label,
					Username:   c.Username,
					AuthMethod: string(c.AuthMethod),
					Default:    c.IsDefault,
				})
			}
		}
		doc.Connections = append(doc.Connections, conn)
	}
	return doc
}

// Marshal renders the document as commented YAML.
func Marshal(doc Document) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(header)

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode connections: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode connections: %w", err)
	}
	return buf.Bytes(), nil
}

// Parse reads a document and rejects anything this build cannot apply.
//
// Unknown keys are an error rather than a silent drop: a typo in a hand-edited
// file should be reported, not turned into a connection missing its host.
func Parse(raw []byte) (Document, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Document{}, errors.New("the file is empty")
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("this does not look like a Tobiro export: %w", err)
	}
	if doc.Version < 1 {
		return Document{}, errors.New("missing or invalid version field")
	}
	if doc.Version > Version {
		return Document{}, fmt.Errorf("the file was written by a newer version (schema %d, this build reads %d)",
			doc.Version, Version)
	}
	if len(doc.Connections) == 0 {
		return Document{}, errors.New("the file contains no connections")
	}
	for i, c := range doc.Connections {
		if strings.TrimSpace(c.Name) == "" {
			return Document{}, fmt.Errorf("connection %d has no name", i+1)
		}
		if err := c.ProfileInput().Validate(); err != nil {
			return Document{}, fmt.Errorf("connection %q: %w", c.Name, err)
		}
		for j, a := range c.Accounts {
			if strings.TrimSpace(a.Label) == "" {
				return Document{}, fmt.Errorf("connection %q: account %d has no label", c.Name, j+1)
			}
			if err := a.CredentialInput().Validate(); err != nil {
				return Document{}, fmt.Errorf("connection %q: %w", c.Name, err)
			}
		}
	}
	return doc, nil
}

// ProfileInput converts to the domain input the profile service validates.
// ClientPath stays empty on purpose: the importing machine detects its own.
func (c Connection) ProfileInput() domain.ProfileInput {
	in := domain.ProfileInput{
		Name:          strings.TrimSpace(c.Name),
		Kind:          domain.AccessKind(strings.TrimSpace(c.Kind)),
		Provider:      domain.VPNProvider(strings.TrimSpace(c.Provider)),
		Host:          strings.TrimSpace(c.Host),
		Port:          c.Port,
		VendorProfile: strings.TrimSpace(c.VendorProfile),
		Notes:         c.Notes,
	}
	in.SSH = domain.SSHSettings{User: strings.TrimSpace(c.SSHUser)}
	for _, f := range c.Forwards {
		in.SSH.Forwards = append(in.SSH.Forwards, domain.PortForward{
			Name:       strings.TrimSpace(f.Name),
			LocalPort:  f.LocalPort,
			RemoteHost: strings.TrimSpace(f.RemoteHost),
			RemotePort: f.RemotePort,
		})
	}
	return in.Normalise()
}

// CredentialInput converts to the domain input, always without a password.
//
// An account that authenticated with a private key arrives without one, because
// the exporter's key path means nothing on the importing machine. It comes in
// as an agent account instead: that is the setup most likely to already work for
// the recipient, and the form shows what it landed on so they can change it.
func (a Account) CredentialInput() domain.CredentialInput {
	method := domain.AuthMethod(strings.TrimSpace(a.AuthMethod))
	if method == domain.AuthSSHKey {
		method = domain.AuthSSHAgent
	}
	return domain.CredentialInput{
		Label:      strings.TrimSpace(a.Label),
		Username:   strings.TrimSpace(a.Username),
		AuthMethod: method,
		IsDefault:  a.Default,
	}
}
