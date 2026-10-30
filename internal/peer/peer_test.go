package peer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bctx "github.com/ncmink/biebie-protocol/context"
	"github.com/ncmink/biebie-protocol/ipc"

	"tobiro/internal/access"
	"tobiro/internal/credential"
	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/profile"
	"tobiro/internal/security"
	"tobiro/internal/session"
	"tobiro/internal/sshtunnel"
	"tobiro/internal/store"
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/mock"
)

// harness is the endpoint serving on a throwaway socket, with a client dialling
// it the same way Biebie Kube does.
type harness struct {
	server   *Server
	client   *ipc.Client
	profiles *profile.Service
	sessions *session.Manager

	// requested records what the endpoint asked the UI to open.
	requested chan string
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	dataStore, err := store.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	logger := logging.New()
	secrets := security.NewMemoryStore("test")
	profiles := profile.NewService(dataStore)
	credentials := credential.NewService(dataStore, secrets, logger)
	sessions := session.NewManager()

	registry := vpn.NewRegistry()
	registry.Register(mock.New())
	manager := vpn.NewManager(registry, profiles, credentials, sessions, logger)
	tunnels := sshtunnel.NewManager(profiles, credentials, sessions, logger)
	router := access.NewRouter(profiles, sessions, credentials, manager, tunnels)

	requested := make(chan string, 4)

	endpoint := shortSocket(t)
	server := NewFor(endpoint, Options{
		Profiles:           profiles,
		Sessions:           sessions,
		Access:             router,
		Log:                logger,
		OnConnectRequested: func(id string) { requested <- id },
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start endpoint: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	return &harness{
		server:    server,
		client:    ipc.NewClient(endpoint).WithTimeout(3 * time.Second),
		profiles:  profiles,
		sessions:  sessions,
		requested: requested,
	}
}

// shortSocket returns a socket path inside a directory of its own.
//
// t.TempDir() is not usable here: it embeds the test name, and a Unix socket
// path is capped at 103 bytes, which the longer test names in this file exceed.
func shortSocket(t *testing.T) ipc.Endpoint {
	t.Helper()
	dir, err := os.MkdirTemp("", "bp")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return ipc.Endpoint{Address: filepath.Join(dir, "s.sock"), Dir: dir}
}

func (h *harness) addProfile(t *testing.T, name string) domain.VPNProfile {
	t.Helper()
	p, err := h.profiles.Create(domain.ProfileInput{
		Name:     name,
		Provider: domain.ProviderMock,
		Host:     "vpn.example.test",
		Port:     443,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return p
}

// The endpoint has to answer a ping before anything else is worth testing:
// Biebie Kube uses it to decide between talking and launching.
func TestEndpointAnswersPing(t *testing.T) {
	h := newHarness(t)
	if !h.client.Available(context.Background()) {
		t.Fatal("a running Tobiro must answer a ping")
	}
}

func TestStatusOfAnIdleConnection(t *testing.T) {
	h := newHarness(t)
	p := h.addProfile(t, "Customer VPN")

	var status bctx.AccessStatus
	call(t, h, ipc.MethodAccessStatus, params{ProfileID: p.ID}, &status)

	if status.State != bctx.AccessDisconnected {
		t.Fatalf("state = %q, want disconnected", status.State)
	}
	if status.Connected {
		t.Fatal("an idle connection must not report itself connected")
	}
}

func TestStatusOfAConnectedConnection(t *testing.T) {
	h := newHarness(t)
	p := h.addProfile(t, "Customer VPN")

	if _, err := h.sessions.Start(p.ID, ""); err != nil {
		t.Fatalf("start session: %v", err)
	}
	if _, err := h.sessions.TransitionWith(p.ID, domain.StateConnected, func(s *domain.VPNSession) {
		s.AssignedIP = "10.42.0.7"
	}); err != nil {
		t.Fatalf("transition: %v", err)
	}

	var status bctx.AccessStatus
	call(t, h, ipc.MethodAccessStatus, params{ProfileID: p.ID}, &status)

	if !status.Connected || status.State != bctx.AccessConnected {
		t.Fatalf("status = %+v, want connected", status)
	}
	if status.AssignedIP != "10.42.0.7" {
		t.Fatalf("assigned ip = %q, want the tunnel address", status.AssignedIP)
	}
}

// An identifier Biebie Kube holds but this machine does not have is a state the
// engineer can act on, not a failed call. Returning an error would show them a
// broken integration when the real problem is a mismatched profile id.
func TestStatusOfAnUnknownProfileIsAState(t *testing.T) {
	h := newHarness(t)

	var status bctx.AccessStatus
	call(t, h, ipc.MethodAccessStatus, params{ProfileID: "not-on-this-machine"}, &status)

	if status.State != bctx.AccessUnknown {
		t.Fatalf("state = %q, want unknown", status.State)
	}
	if status.Detail == "" {
		t.Fatal("an unknown profile must explain itself")
	}
}

func TestConnectAsksTheUIAndDoesNotConnect(t *testing.T) {
	h := newHarness(t)
	p := h.addProfile(t, "Customer VPN")

	var result struct {
		Accepted bool `json:"accepted"`
	}
	call(t, h, ipc.MethodAccessConnect, params{ProfileID: p.ID}, &result)

	if !result.Accepted {
		t.Fatal("a connect request for a known profile must be accepted")
	}

	select {
	case got := <-h.requested:
		if got != p.ID {
			t.Fatalf("the UI was asked to open %q, want %q", got, p.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the UI was never asked to open the connection")
	}

	// The point of the whole design: accepting the request must not have raised
	// a tunnel. Only the person at the keyboard can choose an account.
	if _, live := h.sessions.Get(p.ID); live {
		t.Fatal("a peer request must not start a session on its own")
	}
}

// Here an unknown profile *is* a failure: the caller asked for something to
// happen and nothing can, so it must be told rather than left waiting.
func TestConnectToAnUnknownProfileFails(t *testing.T) {
	h := newHarness(t)

	err := h.client.Call(context.Background(), ipc.MethodAccessConnect,
		params{ProfileID: "not-on-this-machine"}, nil)
	if err == nil {
		t.Fatal("connecting to a profile that does not exist must fail")
	}
}

func TestHandoffIsSingleUse(t *testing.T) {
	h := newHarness(t)

	id, err := h.server.CreateHandoff(context.Background(), bctx.AppKube, bctx.BiebieContext{
		ContextID:       "ctx_test",
		CustomerID:      "smoi",
		CustomerName:    "SMOI",
		EnvironmentID:   "prod",
		EnvironmentName: "Production",
		EnvironmentKind: bctx.EnvironmentProduction,
		ClusterID:       "smoi-prod-eks",
		ClusterName:     "SMOI Production",
	})
	if err != nil {
		t.Fatalf("create handoff: %v", err)
	}

	type request struct {
		HandoffID string `json:"handoffId"`
		TargetApp string `json:"targetApp"`
	}
	first := request{HandoffID: id, TargetApp: string(bctx.AppKube)}

	// The reply is the context itself, which is the shape Biebie Kube's client
	// unmarshals into.
	var received bctx.BiebieContext
	if err := h.client.Call(context.Background(), ipc.MethodConsumeHandoff, first, &received); err != nil {
		t.Fatalf("consume handoff: %v", err)
	}
	if received.CustomerID != "smoi" {
		t.Fatalf("context = %+v, want the customer it was issued for", received)
	}
	if err := received.Validate(); err != nil {
		t.Fatalf("Biebie Kube would reject this context: %v", err)
	}

	if err := h.client.Call(context.Background(), ipc.MethodConsumeHandoff, first, nil); err == nil {
		t.Fatal("a handoff must not be redeemable twice")
	}
}

// A caller that dials an endpoint nobody is serving must be told the peer is
// unavailable, because that is what makes Biebie Kube fall back to launching
// this application instead of reporting an error.
func TestClosedEndpointReportsPeerUnavailable(t *testing.T) {
	h := newHarness(t)
	if err := h.server.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if h.client.Available(context.Background()) {
		t.Fatal("a closed endpoint must not answer")
	}
}

type params struct {
	ProfileID string `json:"profileId"`
}

func call(t *testing.T, h *harness, method ipc.Method, request any, result any) {
	t.Helper()
	if err := h.client.Call(context.Background(), method, request, result); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

// A cluster in Biebie Kube may be configured with a connection name rather than
// an identifier, because the name is the part an engineer can read off this
// window. Resolving it is this application's job, and reporting which
// identifier it landed on is how the caller stops using the name.
func TestConnectingByNameReportsTheIdentifierItResolvedTo(t *testing.T) {
	h := newHarness(t)
	name := "vpn-cat-" + strings.ReplaceAll(t.Name(), "/", "_")
	p := h.addProfile(t, name)

	var result bctx.AccessConnectResult
	call(t, h, ipc.MethodAccessConnect, params{ProfileID: strings.ToUpper(name)}, &result)

	if !result.Accepted {
		t.Fatal("a connect request naming a known connection must be accepted")
	}
	if result.ProfileID != p.ID {
		t.Fatalf("resolved to %q, want the identifier %q", result.ProfileID, p.ID)
	}

	select {
	case got := <-h.requested:
		if got != p.ID {
			t.Fatalf("the UI was asked to open %q, want the identifier %q", got, p.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the UI was never asked to open the connection")
	}
}

func TestStatusAnswersForAConnectionName(t *testing.T) {
	h := newHarness(t)
	name := "vpn-cat-" + strings.ReplaceAll(t.Name(), "/", "_")
	h.addProfile(t, name)

	var status bctx.AccessStatus
	call(t, h, ipc.MethodAccessStatus, params{ProfileID: name}, &status)

	// Resolving by name must not report the "unknown" state a genuinely absent
	// connection gets, or the card would stay grey while connecting worked.
	if status.State == bctx.AccessUnknown {
		t.Fatalf("status by name = %s, want a real state: %q", status.State, status.Detail)
	}
}
