package domain

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// AccessKind is how a connection reaches a customer network.
//
// VPN and SSH are peers rather than variants of each other. A VPN profile hands
// the machine a route into the network and Tobiro only orchestrates the
// vendor client; an SSH profile opens no route at all and instead lends a
// handful of local ports to addresses that are otherwise unreachable. The two
// share a profile, a credential list and a session, and nothing else.
type AccessKind string

const (
	KindVPN AccessKind = "vpn"
	KindSSH AccessKind = "ssh"
)

// Normalise reads a missing kind as VPN, which is what every profile written
// before SSH tunnels existed is.
func (k AccessKind) Normalise() AccessKind {
	if strings.TrimSpace(string(k)) == "" {
		return KindVPN
	}
	return k
}

// IsKnownKind reports whether the value is one this build can act on.
func IsKnownKind(k AccessKind) bool {
	switch k.Normalise() {
	case KindVPN, KindSSH:
		return true
	default:
		return false
	}
}

// DefaultSSHPort is the port an SSH profile uses when none is given. It is
// nearly always the only port a restrictive firewall leaves open, which is the
// whole reason this access kind exists.
const DefaultSSHPort = 22

// LoopbackAddress is the only address a forward ever listens on.
//
// A forward is a hole into a customer network. Binding it to 0.0.0.0 would
// offer that hole to everything else on the café wifi, so the address is a
// constant here rather than something a profile can set.
const LoopbackAddress = "127.0.0.1"

// sshUserPattern allows the characters a POSIX login name can hold. Anything a
// shell could interpret is rejected, even though no command here is built
// through one.
var sshUserPattern = regexp.MustCompile(`^[A-Za-z0-9._@-]+$`)

// ForwardState is the live status of one forwarded port. It is independent of
// the SSH session: the tunnel can be up while a single remote target refuses.
const (
	ForwardListening = "listening"
	ForwardActive    = "active"
	ForwardFailed    = "failed"
)

// PortForward lends one local port to an address reachable from the SSH server.
//
// It is the `-L localPort:remoteHost:remotePort` of an ssh command line, with
// the bind address fixed to loopback. RemoteHost is resolved on the server, so
// "localhost" means the server itself — which is how a Kubernetes API server
// listening only on its own loopback is reached at all.
//
// State, Active, LastError and RemoteOK are runtime-only. They appear on a
// live session so the UI can tell a listening port from one that cannot reach
// the far side. They are never written onto a saved profile: Normalise copies
// only the four configuration fields.
type PortForward struct {
	Name       string `json:"name"`
	LocalPort  int    `json:"localPort"`
	RemoteHost string `json:"remoteHost"`
	RemotePort int    `json:"remotePort"`

	State     string `json:"state,omitempty"`
	Active    int    `json:"active,omitempty"`
	LastError string `json:"lastError,omitempty"`
	RemoteOK  *bool  `json:"remoteOk,omitempty"`
}

// Local renders the loopback address this forward listens on.
func (f PortForward) Local() string {
	return net.JoinHostPort(LoopbackAddress, strconv.Itoa(f.LocalPort))
}

// Remote renders the address the SSH server dials on our behalf.
func (f PortForward) Remote() string {
	return net.JoinHostPort(f.RemoteHost, strconv.Itoa(f.RemotePort))
}

// Label is what to call this forward in the UI and in logs.
func (f PortForward) Label() string {
	if name := strings.TrimSpace(f.Name); name != "" {
		return name
	}
	return f.Remote()
}

// Validate rejects a forward the tunnel could not raise.
func (f PortForward) Validate() error {
	if err := validatePort("local port", f.LocalPort); err != nil {
		return err
	}
	if err := validatePort("remote port", f.RemotePort); err != nil {
		return err
	}
	host := strings.TrimSpace(f.RemoteHost)
	if host == "" {
		return errors.New("a forward needs the address to reach on the far side")
	}
	if len(host) > 253 {
		return errors.New("forward address is too long")
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("forward address %q contains invalid characters", host)
	}
	if len(f.Name) > 128 {
		return errors.New("forward name is too long (max 128 characters)")
	}
	return nil
}

func validatePort(label string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", label, port)
	}
	return nil
}

// ValidateForwards checks a whole forward list for a tunnel that is about to be
// raised, which is the only moment an empty list is wrong.
func ValidateForwards(forwards []PortForward) error {
	if len(forwards) == 0 {
		return errors.New("an SSH tunnel needs at least one port to forward")
	}
	return validateForwardShapes(forwards)
}

// validateForwardShapes checks the forwards that are there, including the one
// rule a single forward cannot see: two of them cannot claim the same local
// port, because the second listener would fail to bind and the tunnel would
// come up half working.
func validateForwardShapes(forwards []PortForward) error {
	if len(forwards) > 32 {
		return errors.New("too many forwards (max 32)")
	}
	seen := make(map[int]struct{}, len(forwards))
	for _, f := range forwards {
		if err := f.Validate(); err != nil {
			return err
		}
		if _, taken := seen[f.LocalPort]; taken {
			return fmt.Errorf("local port %d is forwarded twice", f.LocalPort)
		}
		seen[f.LocalPort] = struct{}{}
	}
	return nil
}

// SSHSettings is everything an SSH profile needs beyond the host and port it
// shares with every other profile.
type SSHSettings struct {
	User     string        `json:"user"`
	Forwards []PortForward `json:"forwards"`
}

// Validate checks the SSH-specific half of a profile that is ready to be used.
func (s SSHSettings) Validate() error {
	user := strings.TrimSpace(s.User)
	if user == "" {
		return errors.New("the login name on the SSH server is required")
	}
	if err := validateSSHUser(user); err != nil {
		return err
	}
	return ValidateForwards(s.Forwards)
}

// ValidateDraft checks only what has been filled in so far.
//
// The SSH half of a connection is set up on the connection's own page, after
// the connection exists, so a half-configured tunnel is a normal thing to have
// saved. Everything present still has to be well formed — a login name with a
// semicolon in it is wrong whether or not the ports are typed yet — but nothing
// is required until the tunnel is actually raised.
func (s SSHSettings) ValidateDraft() error {
	if user := strings.TrimSpace(s.User); user != "" {
		if err := validateSSHUser(user); err != nil {
			return err
		}
	}
	return validateForwardShapes(s.Forwards)
}

func validateSSHUser(user string) error {
	if len(user) > 64 {
		return errors.New("login name is too long (max 64 characters)")
	}
	if !sshUserPattern.MatchString(user) {
		return fmt.Errorf("login name %q contains invalid characters", user)
	}
	return nil
}

// TunnelInput carries the whole SSH half of a connection: the server it dials
// and the ports it lends.
//
// It is separate from ProfileInput because the two are edited in different
// places. What a connection is called and where it is filed belong to the
// connection itself and are edited in a dialog; the server, the login and the
// port map are the tunnel's own configuration and are edited on the
// connection's page, where the ports it currently has open are also shown.
type TunnelInput struct {
	Host string      `json:"host"`
	Port int         `json:"port"`
	SSH  SSHSettings `json:"ssh"`
}

// Normalise trims the input and restores the port the form is allowed to clear.
func (in TunnelInput) Normalise() TunnelInput {
	in.Host = strings.TrimSpace(in.Host)
	if in.Port == 0 {
		in.Port = DefaultSSHPort
	}
	in.SSH = in.SSH.Normalise()
	return in
}

// Validate accepts a tunnel that is not finished yet, and rejects one that is
// malformed.
func (in TunnelInput) Validate() error {
	in = in.Normalise()

	if in.Host != "" {
		if len(in.Host) > 253 {
			return errors.New("host is too long")
		}
		if !hostPattern.MatchString(in.Host) {
			return fmt.Errorf("host %q contains invalid characters", in.Host)
		}
	}
	if in.Port < 1 || in.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", in.Port)
	}
	return in.SSH.ValidateDraft()
}

// Normalise trims the settings and applies the defaults the form leaves out.
func (s SSHSettings) Normalise() SSHSettings {
	out := SSHSettings{
		User:     strings.TrimSpace(s.User),
		Forwards: make([]PortForward, 0, len(s.Forwards)),
	}
	for _, f := range s.Forwards {
		host := strings.TrimSpace(f.RemoteHost)
		if host == "" {
			// An ssh command line without a host in the -L spec is not valid,
			// but a half-filled form is the normal state of one being typed,
			// and the far side's own loopback is what the field is for.
			host = "localhost"
		}
		out.Forwards = append(out.Forwards, PortForward{
			Name:       strings.TrimSpace(f.Name),
			LocalPort:  f.LocalPort,
			RemoteHost: host,
			RemotePort: f.RemotePort,
		})
	}
	return out
}
