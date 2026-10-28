package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tobiro/internal/domain"
)

// rig stands in for the whole app: a handful of profiles, the state each one is
// in, and a record of every connect and disconnect in the order it happened.
//
// Order is the point of these tests. A tunnel dialled before its VPN is up is
// exactly the failure this code exists to prevent, and the only way to see it
// is to watch the sequence.
type rig struct {
	profiles map[string]domain.VPNProfile
	states   map[string]domain.ConnectionState
	accounts map[string]string
	refuse   map[string]error
	log      []string
	router   *Router
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		profiles: map[string]domain.VPNProfile{},
		states:   map[string]domain.ConnectionState{},
		accounts: map[string]string{},
		refuse:   map[string]error{},
	}
	r.router = NewRouter(r, r, r, mechanism{rig: r, name: "vpn"}, mechanism{rig: r, name: "ssh"})
	return r
}

// vpn adds a VPN profile. requires names the connection it sits behind, by the
// same identifier the store would hold.
func (r *rig) vpn(id, requires string) domain.VPNProfile {
	p := domain.VPNProfile{ID: id, Name: id, Provider: domain.ProviderMock, Requires: requires}
	r.profiles[id] = p
	r.states[id] = domain.StateDisconnected
	return p
}

func (r *rig) tunnel(id, requires string) domain.VPNProfile {
	p := domain.VPNProfile{ID: id, Name: id, Kind: domain.KindSSH, Provider: domain.ProviderSSH, Requires: requires}
	r.profiles[id] = p
	r.states[id] = domain.StateDisconnected
	return p
}

func (r *rig) Get(id string) (domain.VPNProfile, error) {
	p, ok := r.profiles[id]
	if !ok {
		return domain.VPNProfile{}, fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	}
	return p, nil
}

func (r *rig) Dependents(id string) []domain.VPNProfile {
	var out []domain.VPNProfile
	for _, p := range r.profiles {
		if p.Requires == id {
			out = append(out, p)
		}
	}
	return out
}

func (r *rig) Preferences(profileID string) domain.ProfilePreferences {
	return domain.ProfilePreferences{ProfileID: profileID, LastCredentialID: r.accounts[profileID]}
}

func (r *rig) State(profileID string) domain.ConnectionState {
	if state, ok := r.states[profileID]; ok {
		return state
	}
	return domain.StateDisconnected
}

func (r *rig) SelectDefault(profileID, lastUsedID string) string {
	if lastUsedID != "" {
		return lastUsedID
	}
	return "default-account"
}

func (r *rig) actions() string { return strings.Join(r.log, ", ") }

// mechanism is one of the two connectors, labelled so the log says which one
// was asked — a tunnel must never be handed to the vendor manager.
type mechanism struct {
	rig  *rig
	name string
}

func (m mechanism) Connect(_ context.Context, profileID, credentialID string) (domain.VPNSession, error) {
	r := m.rig
	if err := r.refuse[profileID]; err != nil {
		r.log = append(r.log, fmt.Sprintf("%s refused %s", m.name, profileID))
		r.states[profileID] = domain.StateFailed
		return domain.VPNSession{}, err
	}
	r.log = append(r.log, fmt.Sprintf("%s connected %s as %s", m.name, profileID, credentialID))
	r.states[profileID] = domain.StateConnected
	return domain.VPNSession{ProfileID: profileID, CredentialID: credentialID, State: domain.StateConnected}, nil
}

func (m mechanism) Disconnect(_ context.Context, profileID string) (domain.VPNSession, error) {
	r := m.rig
	r.log = append(r.log, fmt.Sprintf("%s disconnected %s", m.name, profileID))
	r.states[profileID] = domain.StateDisconnected
	return domain.VPNSession{ProfileID: profileID, State: domain.StateDisconnected}, nil
}

func (m mechanism) RefreshStatus(context.Context, domain.VPNProfile) {}
func (m mechanism) RefreshAll(context.Context)                       {}

func TestATunnelBringsUpTheVPNItSitsBehindFirst(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.accounts["smoi-vpn"] = "ming"

	if _, err := r.router.Connect(context.Background(), "smoi-bastion", "server1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	want := "vpn connected smoi-vpn as ming, ssh connected smoi-bastion as server1"
	if r.actions() != want {
		t.Fatalf("expected %q, got %q", want, r.actions())
	}
}

func TestAChainComesUpFromTheOutsideIn(t *testing.T) {
	r := newRig(t)
	r.vpn("edge-vpn", "")
	r.tunnel("inner-bastion", "edge-vpn")
	r.tunnel("deep-bastion", "inner-bastion")

	if _, err := r.router.Connect(context.Background(), "deep-bastion", "me"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	want := "vpn connected edge-vpn as default-account, " +
		"ssh connected inner-bastion as default-account, " +
		"ssh connected deep-bastion as me"
	if r.actions() != want {
		t.Fatalf("expected %q, got %q", want, r.actions())
	}
}

func TestAVPNAlreadyCarryingWorkIsLeftAlone(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.states["smoi-vpn"] = domain.StateConnected

	if _, err := r.router.Connect(context.Background(), "smoi-bastion", "server1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Reconnecting it would drop whatever else is running over it.
	want := "ssh connected smoi-bastion as server1"
	if r.actions() != want {
		t.Fatalf("expected %q, got %q", want, r.actions())
	}
}

func TestATunnelIsNotDialledWhenItsVPNWillNotCome(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.refuse["smoi-vpn"] = errors.New("the vendor client is not installed")

	_, err := r.router.Connect(context.Background(), "smoi-bastion", "server1")
	if err == nil {
		t.Fatal("expected the connection to fail with its prerequisite")
	}
	// The message has to name both, or the engineer is left reading about a
	// VPN they did not click on.
	for _, want := range []string{"smoi-bastion", "smoi-vpn", "not installed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected the error to mention %q, got %v", want, err)
		}
	}
	if r.actions() != "vpn refused smoi-vpn" {
		t.Fatalf("expected the tunnel never to be dialled, got %q", r.actions())
	}
}

func TestAVPNMidwayUpIsNotJoined(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.states["smoi-vpn"] = domain.StateWaitingForOTP

	_, err := r.router.Connect(context.Background(), "smoi-bastion", "server1")
	if err == nil {
		t.Fatal("expected the attempt to be declined while the VPN is still coming up")
	}
	// A second attempt on the same profile would mean two sessions for one
	// connection, so this waits for the person answering the code prompt.
	if len(r.log) != 0 {
		t.Fatalf("expected nothing to be attempted, got %q", r.actions())
	}
}

func TestDisconnectingAVPNClosesTheTunnelsBehindIt(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.tunnel("smoi-jump", "smoi-vpn")
	r.tunnel("elsewhere", "")
	r.states["smoi-vpn"] = domain.StateConnected
	r.states["smoi-bastion"] = domain.StateConnected
	r.states["smoi-jump"] = domain.StateConnected
	r.states["elsewhere"] = domain.StateConnected

	if _, err := r.router.Disconnect(context.Background(), "smoi-vpn"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if !strings.Contains(r.actions(), "ssh disconnected smoi-bastion") ||
		!strings.Contains(r.actions(), "ssh disconnected smoi-jump") {
		t.Fatalf("expected both tunnels to be closed, got %q", r.actions())
	}
	if strings.Contains(r.actions(), "elsewhere") {
		t.Fatalf("expected an unrelated connection to be left alone, got %q", r.actions())
	}
	// The tunnels go first: a tunnel closed deliberately beats one left to
	// discover its route has gone.
	if !strings.HasSuffix(r.actions(), "vpn disconnected smoi-vpn") {
		t.Fatalf("expected the VPN to come down last, got %q", r.actions())
	}
}

func TestATunnelThatIsAlreadyDownIsNotDisconnectedAgain(t *testing.T) {
	r := newRig(t)
	r.vpn("smoi-vpn", "")
	r.tunnel("smoi-bastion", "smoi-vpn")
	r.states["smoi-vpn"] = domain.StateConnected

	if _, err := r.router.Disconnect(context.Background(), "smoi-vpn"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if r.actions() != "vpn disconnected smoi-vpn" {
		t.Fatalf("expected only the VPN to be touched, got %q", r.actions())
	}
}

func TestARequirementThatWentMissingIsReportedPlainly(t *testing.T) {
	r := newRig(t)
	r.tunnel("smoi-bastion", "a-vpn-that-was-deleted")

	_, err := r.router.Connect(context.Background(), "smoi-bastion", "server1")
	if err == nil {
		t.Fatal("expected a dangling requirement to fail the connection")
	}
	if !strings.Contains(err.Error(), "no longer here") {
		t.Fatalf("expected the error to say what to do about it, got %v", err)
	}
}

func TestAHandEditedLoopIsStoppedRatherThanFollowed(t *testing.T) {
	r := newRig(t)
	// The profile service refuses to store this; only an edited data file can
	// produce it, and it must not take the app down.
	r.tunnel("one", "two")
	r.tunnel("two", "one")

	_, err := r.router.Connect(context.Background(), "one", "me")
	if err == nil {
		t.Fatal("expected the chain to be given up on")
	}
	if !strings.Contains(err.Error(), "too many connections") {
		t.Fatalf("expected the depth limit to be reported, got %v", err)
	}
}
