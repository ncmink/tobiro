package domain

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// hostPattern allows DNS names and IPv4/IPv6 literals only. Anything that could
// be interpreted by a shell (spaces, quotes, ;, &, |, $, backticks) is rejected
// before it can ever reach an exec call.
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]+$`)

// vendorProfilePattern guards the vendor tunnel/profile name that some clients
// (e.g. FortiClient) accept as a CLI argument. Parentheses are allowed because
// macOS VPN services are routinely named like "Client (Prod)"; they are inert
// without a shell, and no command here is ever built through one.
var vendorProfilePattern = regexp.MustCompile(`^[A-Za-z0-9 ._@()-]+$`)

// VPNProfile is a customer/project connection definition.
//
// A profile owns its endpoint and provider, and references credentials by UUID.
// One profile intentionally supports many credentials — a profile is never
// coupled to a single username.
type VPNProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Kind selects which access mechanism this profile describes. An empty
	// value is VPN, so a store written before SSH tunnels existed still loads.
	Kind AccessKind `json:"kind"`

	Provider VPNProvider `json:"provider"`

	// Group buckets connections that belong together — usually one customer or
	// employer. Empty means ungrouped.
	Group string `json:"group"`

	// Requires is the identifier of a connection that has to be up before this
	// one can be reached at all. Empty means this connection stands alone.
	//
	// It exists because a customer's SSH bastion is frequently inside the very
	// network a VPN opens: the tunnel cannot dial 172.16.20.65:22 until the VPN
	// is carrying the route. Recording that relationship lets one Connect bring
	// up the whole path, instead of leaving the engineer to remember the order
	// and read a dial timeout when they get it wrong.
	Requires string `json:"requires,omitempty"`

	Host string `json:"host"`
	Port int    `json:"port"`

	// VendorProfile is the tunnel/profile name already configured inside the
	// native client, for vendors that connect by profile name rather than host.
	VendorProfile string `json:"vendorProfile"`

	// ClientPath optionally pins the vendor executable. Empty means "discover
	// it" via the adapter's detection logic.
	ClientPath string `json:"clientPath"`

	// CredentialIDs preserves credential ordering for the UI. Credentials
	// themselves are stored separately and keyed by UUID.
	CredentialIDs []string `json:"credentialIds"`

	// SSH carries the fields only an SSH tunnel has. It is zero for a VPN
	// profile.
	SSH SSHSettings `json:"ssh"`

	Notes string `json:"notes"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AccessKind reads the profile's kind, treating a missing one as VPN.
func (p VPNProfile) AccessKind() AccessKind { return p.Kind.Normalise() }

// IsSSH reports whether this profile is an SSH tunnel.
func (p VPNProfile) IsSSH() bool { return p.AccessKind() == KindSSH }

// ReadyToConnect reports what is still missing before this connection can be
// raised, or nil when nothing is.
//
// A tunnel is created before it is configured — the server and login are filled
// in on the connection's own page — and port forwards are optional at connect
// time (a session with no -L still reaches the server).
func (p VPNProfile) ReadyToConnect() error {
	if !p.IsSSH() {
		return nil
	}
	if strings.TrimSpace(p.Host) == "" {
		return errors.New("this tunnel has no SSH server yet — add one under SSH tunnel")
	}
	return p.SSH.Validate()
}

// Endpoint renders host:port for display and logging.
func (p VPNProfile) Endpoint() string {
	if p.Host == "" {
		return ""
	}
	if p.Port == 0 {
		return p.Host
	}
	return net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
}

// EndpointDisplay renders the endpoint for the UI. Clients that own their own
// server list have no gateway address, so the provider's stand-in label is
// shown in its place.
func (p VPNProfile) EndpointDisplay() string {
	if p.IsSSH() {
		if user := strings.TrimSpace(p.SSH.User); user != "" {
			return user + "@" + p.Endpoint()
		}
		return p.Endpoint()
	}
	if label := p.Provider.EndpointLabel(); label != "" {
		return label
	}
	return p.Endpoint()
}

// ProfileInput carries user-supplied profile fields from the Add/Edit form.
type ProfileInput struct {
	Name          string      `json:"name"`
	Kind          AccessKind  `json:"kind"`
	Provider      VPNProvider `json:"provider"`
	Group         string      `json:"group"`
	Host          string      `json:"host"`
	Port          int         `json:"port"`
	VendorProfile string      `json:"vendorProfile"`
	ClientPath    string      `json:"clientPath"`
	Notes         string      `json:"notes"`

	// Requires is the connection that has to be up first, by identifier.
	// Whether it exists, and whether pointing at it would form a loop, are
	// questions only the profile service can answer.
	Requires string `json:"requires"`

	// SSH is ignored unless Kind is KindSSH.
	SSH SSHSettings `json:"ssh"`
}

// Normalise trims the input and fills in what the form is allowed to omit, so
// validation and persistence both work from the same values.
func (in ProfileInput) Normalise() ProfileInput {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.VendorProfile = strings.TrimSpace(in.VendorProfile)
	in.ClientPath = strings.TrimSpace(in.ClientPath)
	in.Requires = strings.TrimSpace(in.Requires)
	in.Kind = in.Kind.Normalise()

	if in.Kind == KindSSH {
		// An SSH tunnel has one provider and one usual port, and neither is a
		// choice worth making the form present.
		in.Provider = ProviderSSH
		if in.Port == 0 {
			in.Port = DefaultSSHPort
		}
		in.VendorProfile = ""
		in.ClientPath = ""
		in.SSH = in.SSH.Normalise()
	} else {
		in.SSH = SSHSettings{}
	}
	return in
}

// Validate rejects malformed or unsafe input before it reaches persistence or
// process execution.
func (in ProfileInput) Validate() error {
	in = in.Normalise()

	if in.Name == "" {
		return errors.New("name is required")
	}
	if len(in.Name) > 128 {
		return errors.New("name is too long (max 128 characters)")
	}
	if !IsKnownKind(in.Kind) {
		return fmt.Errorf("unknown connection kind %q", in.Kind)
	}
	if err := ValidateGroup(in.Group); err != nil {
		return err
	}
	if len(in.Notes) > 2000 {
		return errors.New("notes are too long (max 2000 characters)")
	}

	if in.Kind == KindSSH {
		return in.validateSSH()
	}
	return in.validateVPN()
}

// validateSSH checks the fields an SSH tunnel carries here.
//
// The server, the login and the port map are not asked for at this point: they
// are configured on the connection's own page once it exists, the same way its
// accounts are. So this accepts a tunnel that has none of them yet, and only
// insists that whatever is there is well formed.
func (in ProfileInput) validateSSH() error {
	return TunnelInput{Host: in.Host, Port: in.Port, SSH: in.SSH}.Validate()
}

func (in ProfileInput) validateVPN() error {
	if !IsKnownProvider(in.Provider) {
		return fmt.Errorf("unknown VPN provider %q", in.Provider)
	}

	host := in.Host
	if in.Provider.RequiresEndpoint() {
		if host == "" {
			return errors.New("host or IP is required")
		}
		if len(host) > 253 {
			return errors.New("host is too long")
		}
		if !hostPattern.MatchString(host) {
			return fmt.Errorf("host %q contains invalid characters", host)
		}

		if in.Port < 1 || in.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535, got %d", in.Port)
		}
	} else if host != "" {
		if len(host) > 253 {
			return errors.New("host is too long")
		}
		if !hostPattern.MatchString(host) {
			return fmt.Errorf("host %q contains invalid characters", host)
		}
	}

	field := in.Provider.VendorProfileField()
	if vp := in.VendorProfile; vp != "" {
		if len(vp) > 128 {
			return errors.New("vendor profile is too long (max 128 characters)")
		}
		if !vendorProfilePattern.MatchString(vp) {
			return fmt.Errorf("vendor profile %q contains invalid characters", vp)
		}
	} else if field.Required {
		return fmt.Errorf("%s is required", field.Label)
	}

	if cp := in.ClientPath; cp != "" {
		if err := ValidateExecutablePath(cp); err != nil {
			return err
		}
	}
	return nil
}

// ValidateExecutablePath rejects paths that are relative or contain shell
// metacharacters, so an operator-supplied override cannot smuggle arguments
// into a command line.
func ValidateExecutablePath(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return errors.New("executable path is empty")
	}
	if !strings.HasPrefix(p, "/") && !isWindowsAbsolute(p) {
		return fmt.Errorf("executable path %q must be absolute", path)
	}
	if strings.ContainsAny(p, ";&|$`\n\r\"'*?") {
		return fmt.Errorf("executable path %q contains invalid characters", path)
	}
	return nil
}

func isWindowsAbsolute(p string) bool {
	if len(p) < 3 {
		return false
	}
	drive := p[0]
	isLetter := (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
	return isLetter && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// ProfilePreferences remembers per-profile UI choices.
//
// Only the credential UUID is remembered. Storing a username here would break
// the moment two credentials share one, which is explicitly allowed.
type ProfilePreferences struct {
	ProfileID        string `json:"profileId"`
	LastCredentialID string `json:"lastCredentialId"`
}
