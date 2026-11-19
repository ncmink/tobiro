// Package access routes a connection request to the mechanism that can serve
// it.
//
// Tobiro reaches a customer network in more than one way now, and the
// layers above — the Wails bindings, the endpoint the other biebie.net
// applications call — have no business knowing which. They ask for a connection
// by identifier; this package knows that a VPN profile is the vendor manager's
// and an SSH profile is the tunnel manager's.
//
// It also owns the order things have to come up in. A connection can record
// another it needs first — typically an SSH bastion that only answers once the
// customer's VPN is carrying the route — and one Connect here brings up the
// whole path.
//
// Only the four operations both mechanisms genuinely share live here. Detecting
// an installed client, opening a vendor GUI or diagnosing a route are VPN
// concepts, and pretending an SSH tunnel has an opinion about them would put a
// disabled button on every screen.
package access

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tobiro/internal/domain"
)

// maxChain bounds how far a prerequisite chain is followed.
//
// The profile service refuses to store a loop, so this only ever matters for a
// hand-edited data file — and a bound that is never reached in practice is
// better than a stack that grows until the app dies.
const maxChain = 8

// Connector is what both mechanisms can do.
type Connector interface {
	Connect(ctx context.Context, profileID, credentialID string) (domain.VPNSession, error)
	Disconnect(ctx context.Context, profileID string) (domain.VPNSession, error)
	RefreshStatus(ctx context.Context, profile domain.VPNProfile)
	RefreshAll(ctx context.Context)
}

// ProfileReader is the lookup the router needs to know which one to ask, and to
// follow the chain a connection sits on.
type ProfileReader interface {
	Get(id string) (domain.VPNProfile, error)
	Dependents(id string) []domain.VPNProfile
	Preferences(profileID string) domain.ProfilePreferences
}

// StateReader answers whether a connection is already up, so a prerequisite
// that is serving other work is left exactly as it is.
type StateReader interface {
	State(profileID string) domain.ConnectionState
}

// CredentialChooser picks the account to bring a prerequisite up with: the last
// one used on it, else its default. Nobody is asked to choose an account for a
// connection they did not click on.
type CredentialChooser interface {
	SelectDefault(profileID, lastUsedID string) string
}

// DisconnectPolicy reports whether a profile can be torn down programmatically.
type DisconnectPolicy interface {
	CanDisconnect(ctx context.Context, profile domain.VPNProfile) bool
}

// Router dispatches on a profile's access kind and honours the order a
// connection has to be reached in.
type Router struct {
	profiles    ProfileReader
	states      StateReader
	credentials CredentialChooser
	vpn         Connector
	ssh         Connector

	mu      sync.Mutex
	flights map[string]*connectFlight
}

type connectFlight struct {
	done chan struct{}
	err  error
}

// NewRouter wires the router to both mechanisms.
func NewRouter(profiles ProfileReader, states StateReader, credentials CredentialChooser, vpn, ssh Connector) *Router {
	return &Router{
		profiles:    profiles,
		states:      states,
		credentials: credentials,
		vpn:         vpn,
		ssh:         ssh,
		flights:     make(map[string]*connectFlight),
	}
}

// Connect brings a connection up through whichever mechanism owns it, after
// whatever it needs first is up.
func (r *Router) Connect(ctx context.Context, profileID, credentialID string) (domain.VPNSession, error) {
	profile, err := r.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	if err := r.raise(ctx, profile, 0); err != nil {
		return domain.VPNSession{}, err
	}
	return r.connectorFor(profile).Connect(ctx, profileID, credentialID)
}

// raise brings up everything profile needs before it can be reached.
//
// The chain is walked from the top down, so a bastion behind a VPN behind
// another VPN comes up in the order the packets need. A prerequisite that is
// already connected is left alone: it may well be carrying somebody else's
// work.
func (r *Router) raise(ctx context.Context, profile domain.VPNProfile, depth int) error {
	if depth >= maxChain {
		return fmt.Errorf("%q sits behind too many connections to bring up (more than %d)", profile.Name, maxChain)
	}

	for _, ref := range profile.Prerequisites() {
		needed, err := r.profiles.Get(ref)
		if err != nil {
			return fmt.Errorf("%q needs a connection that is no longer here; edit it and choose another", profile.Name)
		}

		switch state := r.states.State(needed.ID); {
		case state == domain.StateConnected:
			continue
		case state.IsActive():
			if r.waitFlight(ctx, needed.ID) {
				if r.states.State(needed.ID) == domain.StateConnected {
					continue
				}
			}
			return fmt.Errorf("%q is still coming up, and %q needs it first — try again once it is connected",
				needed.Name, profile.Name)
		}

		if err := r.raise(ctx, needed, depth+1); err != nil {
			return err
		}
		if err := r.ensureConnected(ctx, needed, profile.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Router) ensureConnected(ctx context.Context, needed domain.VPNProfile, childName string) error {
	if r.states.State(needed.ID) == domain.StateConnected {
		return nil
	}

	r.mu.Lock()
	if existing, ok := r.flights[needed.ID]; ok {
		r.mu.Unlock()
		select {
		case <-existing.done:
			return existing.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	flight := &connectFlight{done: make(chan struct{})}
	r.flights[needed.ID] = flight
	r.mu.Unlock()

	credentialID := r.credentials.SelectDefault(needed.ID, r.profiles.Preferences(needed.ID).LastCredentialID)
	var err error
	if _, connectErr := r.connectorFor(needed).Connect(ctx, needed.ID, credentialID); connectErr != nil {
		err = fmt.Errorf("%q needs %q first, and that could not be brought up: %w", childName, needed.Name, connectErr)
	} else {
		err = nil
	}

	flight.err = err
	close(flight.done)

	r.mu.Lock()
	delete(r.flights, needed.ID)
	r.mu.Unlock()
	return err
}

func (r *Router) waitFlight(ctx context.Context, profileID string) bool {
	r.mu.Lock()
	flight, ok := r.flights[profileID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case <-flight.done:
		return true
	case <-ctx.Done():
		return false
	}
}

// Disconnect tears one down, and with it anything that was only reachable
// through it.
//
// Dependents go first so a tunnel is closed deliberately rather than left to
// discover its route has gone and retry against a network that is no longer
// there. This only follows a disconnect the user asked for; a VPN that drops on
// its own still leaves the tunnel to repair itself when the route returns.
func (r *Router) Disconnect(ctx context.Context, profileID string) (domain.VPNSession, error) {
	profile, err := r.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	connector := r.connectorFor(profile)
	if policy, ok := connector.(DisconnectPolicy); ok && !policy.CanDisconnect(ctx, profile) {
		return domain.VPNSession{}, &domain.UnsupportedOperationError{
			Provider:  profile.Provider,
			Operation: "disconnect",
			Reason:    "this connection cannot be disconnected from here",
		}
	}
	r.drop(ctx, profile, 0, map[string]bool{})
	return connector.Disconnect(ctx, profileID)
}

func (r *Router) drop(ctx context.Context, profile domain.VPNProfile, depth int, visited map[string]bool) {
	if depth >= maxChain {
		return
	}
	if visited[profile.ID] {
		return
	}
	visited[profile.ID] = true
	for _, dependent := range r.profiles.Dependents(profile.ID) {
		r.drop(ctx, dependent, depth+1, visited)
		if r.states.State(dependent.ID).IsActive() {
			// A dependent that will not close is reported by its own manager and
			// logged there. The connection the user actually asked to drop still
			// comes down.
			_, _ = r.connectorFor(dependent).Disconnect(ctx, dependent.ID)
		}
	}
}

// RefreshStatus re-reads reality for one profile.
func (r *Router) RefreshStatus(ctx context.Context, profile domain.VPNProfile) {
	r.connectorFor(profile).RefreshStatus(ctx, profile)
}

// RefreshAll re-reads reality for every profile.
//
// Both mechanisms are asked rather than the profiles being partitioned here:
// each one already walks the profile list and skips what is not its own, and
// the VPN side has to sweep the vendor clients whether or not a profile is
// currently listed against them.
func (r *Router) RefreshAll(ctx context.Context) {
	r.vpn.RefreshAll(ctx)
	r.ssh.RefreshAll(ctx)
}

// StartWatching re-reads reality for every profile, VPN and SSH alike, every
// interval until ctx is cancelled.
//
// Status must not depend on the window asking: an SSH tunnel has no vendor
// client whose state could be read later, so without this sweep a tunnel that
// died stays "connected" until something happens to list the connections.
func (r *Router) StartWatching(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		r.RefreshAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.RefreshAll(ctx)
			}
		}
	}()
}

// For resolves the mechanism that owns a profile.
func (r *Router) For(profileID string) (Connector, error) {
	profile, err := r.profiles.Get(profileID)
	if err != nil {
		return nil, err
	}
	return r.connectorFor(profile), nil
}

func (r *Router) connectorFor(profile domain.VPNProfile) Connector {
	if profile.IsSSH() {
		return r.ssh
	}
	return r.vpn
}
