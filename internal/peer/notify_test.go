package peer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	bctx "github.com/ncmink/biebie-protocol/context"
	"github.com/ncmink/biebie-protocol/ipc"

	"tobiro/internal/domain"
)

// fakeKube stands in for Biebie Kube's endpoint, which already handles this
// method — the gap this closes was only ever on the sending side.
func fakeKube(t *testing.T) (*Notifier, <-chan bctx.AccessSessionChanged) {
	t.Helper()

	endpoint := shortSocket(t)
	server := ipc.NewServer(endpoint)

	received := make(chan bctx.AccessSessionChanged, 8)
	server.Handle(ipc.MethodAccessSessionChanged, func(_ context.Context, params json.RawMessage) (any, error) {
		var event bctx.AccessSessionChanged
		if err := json.Unmarshal(params, &event); err != nil {
			return nil, err
		}
		received <- event
		return map[string]bool{"ok": true}, nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start fake Biebie Kube: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	notifier := NewNotifierFor(endpoint)
	t.Cleanup(notifier.Close)

	return notifier, received
}

func TestConnectingAndConnectedAreAnnounced(t *testing.T) {
	notifier, received := fakeKube(t)

	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnecting})
	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnected})

	for _, want := range []bctx.AccessState{bctx.AccessConnecting, bctx.AccessConnected} {
		select {
		case got := <-received:
			if got.ProfileID != "smoi-vpn" || got.State != want {
				t.Fatalf("event = %+v, want %s", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Biebie Kube was never told about %s", want)
		}
	}
}

// Vendor clients are polled, so the same state arrives repeatedly. Only a real
// change is worth a round trip.
func TestUnchangedStateIsNotAnnouncedAgain(t *testing.T) {
	notifier, received := fakeKube(t)

	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnected})
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("the first change was never announced")
	}

	for range 5 {
		notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnected})
	}

	select {
	case got := <-received:
		t.Fatalf("a repeated state was announced again: %+v", got)
	case <-time.After(500 * time.Millisecond):
	}
}

// The authentication states Tobiro distinguishes for its own screens
// collapse into one outward state, so pausing for a one-time code does not look
// to a peer like the connection moved.
func TestAuthenticationStepsLookLikeOneConnectingState(t *testing.T) {
	notifier, received := fakeKube(t)

	for _, state := range []domain.ConnectionState{
		domain.StateConnecting,
		domain.StateAuthenticating,
		domain.StateWaitingForOTP,
	} {
		notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: state})
	}

	select {
	case got := <-received:
		if got.State != bctx.AccessConnecting {
			t.Fatalf("state = %q, want connecting", got.State)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nothing was announced")
	}

	select {
	case got := <-received:
		t.Fatalf("the authentication steps were announced separately: %+v", got)
	case <-time.After(500 * time.Millisecond):
	}
}

// Biebie Kube not running is the ordinary case. It must cost nothing and raise
// nothing: this runs on the session manager's own notification path.
func TestNotifyingAClosedPeerIsHarmless(t *testing.T) {
	notifier := NewNotifierFor(shortSocket(t))
	t.Cleanup(notifier.Close)

	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnected})
	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateDisconnected})
}

func TestObservingAfterCloseDoesNotPanic(t *testing.T) {
	notifier := NewNotifierFor(shortSocket(t))
	notifier.Close()
	notifier.Close() // idempotent, because shutdown ordering is not guaranteed

	// A status poll can still be in flight when the window closes.
	notifier.Observe(domain.VPNSession{ProfileID: "smoi-vpn", State: domain.StateConnected})
}
