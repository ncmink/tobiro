package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/session"
)

// ProfileReader is the slice of the profile service this manager needs.
type ProfileReader interface {
	Get(id string) (domain.VPNProfile, error)
	List() []domain.VPNProfile
	RememberCredential(profileID, credentialID string) error
}

// CredentialResolver materialises a credential's secret for one connect call.
type CredentialResolver interface {
	Resolve(credentialID string) (domain.Credential, error)
}

// Prompter lets the manager ask the user something mid-connect and tell the UI
// what happened. It is the same bridge the VPN side prompts for OTP through.
type Prompter interface {
	// ConfirmHostKey asks whether an unrecognised SSH server should be trusted.
	ConfirmHostKey(ctx context.Context, req domain.HostKeyRequest) (bool, error)
	Notify(event string, payload any)
}

// Manager owns every live SSH tunnel in this process.
//
// It mirrors vpn.Manager's surface so the two can sit behind one router, but
// the resemblance stops at the edges: there is no client to detect, no
// capabilities to probe, and no status to read back from anything. A tunnel
// exists exactly as long as this process holds it open, which is why Connect
// and Disconnect here are the whole truth rather than a request to software
// that will decide for itself.
type Manager struct {
	profiles    ProfileReader
	credentials CredentialResolver
	sessions    *session.Manager
	log         *logging.Logger
	prompter    Prompter

	// knownHosts overrides the file host keys are checked against. Empty means
	// the user's own ~/.ssh/known_hosts, which is the point: a server already
	// accepted in a terminal is not asked about again here.
	knownHosts string

	mu      sync.Mutex
	tunnels map[string]*tunnel
}

// NewManager wires the manager.
func NewManager(profiles ProfileReader, credentials CredentialResolver, sessions *session.Manager, log *logging.Logger) *Manager {
	return &Manager{
		profiles:    profiles,
		credentials: credentials,
		sessions:    sessions,
		log:         log,
		tunnels:     make(map[string]*tunnel),
	}
}

// SetPrompter installs the UI bridge used to confirm a new host key.
func (m *Manager) SetPrompter(p Prompter) { m.prompter = p }

// SetKnownHosts points host key verification at a file other than the user's.
func (m *Manager) SetKnownHosts(path string) { m.knownHosts = path }

// Connect raises the tunnel for a profile using one credential UUID.
func (m *Manager) Connect(ctx context.Context, profileID, credentialID string) (domain.VPNSession, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	if !profile.IsSSH() {
		return domain.VPNSession{}, fmt.Errorf("connection %q is not an SSH tunnel", profile.Name)
	}
	// A tunnel is created before it is configured, so the first thing to
	// establish is whether there is anything here to dial.
	if err := profile.ReadyToConnect(); err != nil {
		return domain.VPNSession{}, err
	}

	credential := domain.Credential{ProfileID: profileID, AuthMethod: domain.AuthSSHAgent}
	if credentialID != "" {
		credential, err = m.credentials.Resolve(credentialID)
		if err != nil {
			return domain.VPNSession{}, err
		}
		if credential.ProfileID != profileID {
			return domain.VPNSession{}, fmt.Errorf("credential %s does not belong to this connection", credentialID)
		}
	}

	sess, err := m.sessions.Start(profileID, credential.ID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	_ = sess

	// The passphrase or password is registered with the logger for the length
	// of this call, so no error text built from it can reach the activity log.
	release := m.log.Protect(credential.Secret)
	defer release()

	m.log.Info("Opening SSH tunnel profile=%s endpoint=%s credential=%s",
		profile.Name, profile.EndpointDisplay(), defaultString(credential.ID, "ssh-agent"))

	config, err := clientConfig(profile, credential,
		newVerifier(ctx, profileID, m.knownHosts, m.prompter, m.log).callback)
	if err != nil {
		return m.fail(profile, err), err
	}

	t, err := open(ctx, profile, config, m.log, m.sink(profileID))
	if err != nil {
		err = advise(profile, err)
		return m.fail(profile, err), err
	}

	m.mu.Lock()
	if existing, ok := m.tunnels[profileID]; ok {
		// Nothing should reach here — the session manager refuses a second
		// connect while one is active — but a stray tunnel would hold ports
		// nothing could ever release.
		m.mu.Unlock()
		existing.close()
		m.mu.Lock()
	}
	m.tunnels[profileID] = t
	m.mu.Unlock()

	forwards := t.forwards()
	connected, err := m.sessions.TransitionWith(profileID, domain.StateConnected, func(s *domain.VPNSession) {
		s.AssignedIP = domain.LoopbackAddress
		s.Identity = profile.SSH.User + "@" + profile.Host
		s.Interface = "ssh"
		s.Forwards = forwards
	})
	if err != nil {
		m.log.Warn("Tunnel to %s is up but its state could not be recorded: %v", profile.Endpoint(), err)
		connected, _ = m.sessions.Get(profileID)
	}

	if credential.ID != "" {
		if err := m.profiles.RememberCredential(profileID, credential.ID); err != nil {
			m.log.Warn("Could not remember last account for profile=%s: %v", profile.Name, err)
		}
	}

	for _, f := range forwards {
		m.log.Info("Forwarding %s → %s (%s)", f.Local(), f.Remote(), f.Label())
	}
	return connected, nil
}

// Disconnect closes the tunnel and releases its local ports.
func (m *Manager) Disconnect(ctx context.Context, profileID string) (domain.VPNSession, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}

	m.mu.Lock()
	t, ok := m.tunnels[profileID]
	delete(m.tunnels, profileID)
	m.mu.Unlock()

	if _, err := m.sessions.Transition(profileID, domain.StateDisconnecting); err != nil {
		m.log.Warn("Disconnect requested for profile=%s in an unexpected state: %v", profile.Name, err)
	}
	if ok {
		t.close()
	}

	final, err := m.sessions.TransitionWith(profileID, domain.StateDisconnected, func(s *domain.VPNSession) {
		s.AssignedIP = ""
		s.Interface = ""
		s.Identity = ""
		s.Forwards = nil
	})
	if err != nil {
		final = domain.VPNSession{ProfileID: profileID, State: domain.StateDisconnected}
	}
	m.log.Info("Closed SSH tunnel profile=%s", profile.Name)
	return final, nil
}

// RefreshStatus reconciles the session table with the tunnels this process
// actually holds.
//
// Unlike a vendor client there is nothing external to read: an SSH tunnel that
// is not in this map does not exist anywhere. So the only correction to make is
// the one that matters after a crash or a stale record — a session claiming to
// be up with no tunnel behind it.
func (m *Manager) RefreshStatus(_ context.Context, profile domain.VPNProfile) {
	if !profile.IsSSH() {
		return
	}

	m.mu.Lock()
	_, held := m.tunnels[profile.ID]
	m.mu.Unlock()
	if held {
		return
	}

	if current, ok := m.sessions.Get(profile.ID); ok && current.State == domain.StateConnected {
		m.sessions.SyncFromStatus(profile.ID, domain.VPNStatus{
			Known: true,
			State: domain.StateDisconnected,
		})
	}
}

// RefreshAll reconciles every SSH profile.
func (m *Manager) RefreshAll(ctx context.Context) {
	for _, profile := range m.profiles.List() {
		m.RefreshStatus(ctx, profile)
	}
}

// Shutdown closes every tunnel, so the application does not leave listeners
// bound behind a window the user just closed.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	tunnels := m.tunnels
	m.tunnels = make(map[string]*tunnel)
	m.mu.Unlock()

	for profileID, t := range tunnels {
		t.close()
		m.log.Info("Closed SSH tunnel for profile=%s on shutdown", profileID)
	}
}

// sink applies the transitions a supervising tunnel drives.
func (m *Manager) sink(profileID string) stateSink {
	return func(state domain.ConnectionState, detail string) {
		_, err := m.sessions.TransitionWith(profileID, state, func(s *domain.VPNSession) {
			if state == domain.StateConnecting {
				s.Error = ""
			}
		})
		if err != nil {
			m.log.Warn("Ignoring tunnel state %s for profile=%s: %v", state, profileID, err)
			return
		}
		if detail != "" {
			m.log.Info("profile=%s tunnel %s", profileID, detail)
		}
	}
}

func (m *Manager) fail(profile domain.VPNProfile, cause error) domain.VPNSession {
	reason := m.log.Redact(cause.Error())
	failed := m.sessions.Fail(profile.ID, reason)
	m.log.Error("SSH tunnel failed profile=%s: %s", profile.Name, reason)
	if m.prompter != nil {
		m.prompter.Notify(domain.EventConnectionFailed, domain.ConnectionFailedEvent{
			SessionID: failed.ID, ProfileID: profile.ID, Reason: reason,
		})
	}
	return failed
}

// advise adds the piece of advice a failed dial usually needs.
//
// A bastion inside a customer's network answers nothing at all until their VPN
// is carrying the route, and the failure on its own — a TCP timeout against a
// private address — says only that nobody answered. A tunnel that already
// records what it sits behind is left alone: the router brought that up first,
// so the address really is unreachable and a hint would only mislead.
func advise(profile domain.VPNProfile, cause error) error {
	if profile.Requires != "" || !dialFailed(cause) {
		return cause
	}
	return fmt.Errorf(`%w — if %s is only reachable from inside the customer's network, connect their `+
		`VPN first and record it under "needs another connection first" on this tunnel`, cause, profile.Host)
}

// dialFailed separates "the address did not answer" from everything that
// happens after the connection is open, such as a rejected password.
func dialFailed(cause error) bool {
	var opErr *net.OpError
	return errors.As(cause, &opErr) && opErr.Op == "dial"
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
