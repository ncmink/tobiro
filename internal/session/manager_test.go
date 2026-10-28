package session

import (
	"testing"
	"time"

	"tobiro/internal/domain"
)

// A session change has two audiences now: this application's window, and Biebie
// Kube waiting on the same tunnel. Neither may silence the other, which is what
// a single-observer slot did the moment a second one was registered.
func TestEveryObserverSeesEveryChange(t *testing.T) {
	m := NewManager()

	var window, peer []domain.ConnectionState
	m.AddObserver(func(s domain.VPNSession) { window = append(window, s.State) })
	m.AddObserver(func(s domain.VPNSession) { peer = append(peer, s.State) })

	if _, err := m.Start("p1", ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Transition("p1", domain.StateConnected); err != nil {
		t.Fatalf("transition: %v", err)
	}

	want := []domain.ConnectionState{domain.StateConnecting, domain.StateConnected}
	for name, got := range map[string][]domain.ConnectionState{"window": window, "peer": peer} {
		if len(got) != len(want) {
			t.Fatalf("%s saw %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s saw %v, want %v", name, got, want)
			}
		}
	}
}

func TestAddObserverIgnoresNil(t *testing.T) {
	m := NewManager()
	m.AddObserver(nil)

	// Registering nothing must not leave a nil in the list for the next change
	// to call.
	if _, err := m.Start("p1", ""); err != nil {
		t.Fatalf("start: %v", err)
	}
}

func TestSyncFromStatusAdoptsUnknownTunnel(t *testing.T) {
	m := NewManager()
	m.SyncFromStatus("p1", domain.VPNStatus{
		Known:      true,
		State:      domain.StateConnected,
		AssignedIP: "172.30.0.138",
		Interface:  "utun7",
	})

	got, ok := m.Get("p1")
	if !ok {
		t.Fatal("expected an adopted session")
	}
	if got.State != domain.StateConnected {
		t.Fatalf("state = %s, want connected", got.State)
	}
	if got.AssignedIP != "172.30.0.138" || got.Interface != "utun7" {
		t.Fatalf("tunnel facts = %s %s", got.AssignedIP, got.Interface)
	}
}

func TestSyncFromStatusCopiesIdentity(t *testing.T) {
	m := NewManager()
	m.SyncFromStatus("p1", domain.VPNStatus{
		Known:    true,
		State:    domain.StateConnected,
		Identity: "ming@biebie.net",
	})
	got, ok := m.Get("p1")
	if !ok || got.Identity != "ming@biebie.net" {
		t.Fatalf("identity = %q, want ming@biebie.net", got.Identity)
	}
}

func TestSyncFromStatusReAdoptsAfterDisconnect(t *testing.T) {
	m := NewManager()
	now := time.Now()
	ended := now
	m.sessions["p1"] = domain.VPNSession{
		ID:        "sess-1",
		ProfileID: "p1",
		State:     domain.StateDisconnected,
		StartedAt: now.Add(-time.Minute),
		EndedAt:   &ended,
	}

	m.SyncFromStatus("p1", domain.VPNStatus{
		Known:      true,
		State:      domain.StateConnected,
		AssignedIP: "172.30.0.138",
		Interface:  "utun7",
	})

	got, ok := m.Get("p1")
	if !ok {
		t.Fatal("expected the session to still exist")
	}
	if got.State != domain.StateConnected {
		t.Fatalf("state = %s, want connected — a live vendor tunnel must be re-adopted after a probe miss", got.State)
	}
	if got.EndedAt != nil {
		t.Fatal("EndedAt should be cleared when the tunnel is observed up again")
	}
	if got.AssignedIP != "172.30.0.138" || got.Interface != "utun7" {
		t.Fatalf("tunnel facts = %s %s", got.AssignedIP, got.Interface)
	}
}

func TestSyncFromStatusReAdoptsAfterFailed(t *testing.T) {
	m := NewManager()
	m.sessions["p1"] = domain.VPNSession{
		ID:        "sess-1",
		ProfileID: "p1",
		State:     domain.StateFailed,
		Error:     "probe miss",
	}

	m.SyncFromStatus("p1", domain.VPNStatus{
		Known: true,
		State: domain.StateConnected,
	})

	got, ok := m.Get("p1")
	if !ok || got.State != domain.StateConnected {
		t.Fatalf("state = %v, want connected", got.State)
	}
	if got.Error != "" {
		t.Fatalf("error should be cleared, got %q", got.Error)
	}
}

func TestSyncFromStatusIgnoresUnknownReadings(t *testing.T) {
	m := NewManager()
	m.sessions["p1"] = domain.VPNSession{ID: "sess-1", ProfileID: "p1", State: domain.StateConnected}

	m.SyncFromStatus("p1", domain.VPNStatus{Known: false, State: domain.StateDisconnected})

	got, _ := m.Get("p1")
	if got.State != domain.StateConnected {
		t.Fatalf("unknown reading must not clobber connected, got %s", got.State)
	}
}
