package vpn_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tobiro/internal/credential"
	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/profile"
	"tobiro/internal/security"
	"tobiro/internal/session"
	"tobiro/internal/store"
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/mock"
)

type harness struct {
	dataPath    string
	profiles    *profile.Service
	credentials *credential.Service
	sessions    *session.Manager
	manager     *vpn.Manager
	log         *logging.Logger
	states      *stateRecorder
}

// stateRecorder captures every state the session passes through, so tests can
// assert on the whole journey rather than just the final state.
type stateRecorder struct {
	states []domain.ConnectionState
}

func (r *stateRecorder) observe(s domain.VPNSession) {
	if len(r.states) == 0 || r.states[len(r.states)-1] != s.State {
		r.states = append(r.states, s.State)
	}
}

func (r *stateRecorder) saw(state domain.ConnectionState) bool {
	for _, s := range r.states {
		if s == state {
			return true
		}
	}
	return false
}

// otpPrompter answers OTP requests with a fixed code.
type otpPrompter struct {
	code    string
	asked   bool
	lastReq domain.InteractionRequest
}

func (p *otpPrompter) RequestOTP(_ context.Context, req domain.InteractionRequest) (string, error) {
	p.asked = true
	p.lastReq = req
	return p.code, nil
}

func (p *otpPrompter) AwaitNativeFill(_ context.Context, _ domain.NativeFillRequest) error {
	return nil
}

func (p *otpPrompter) Notify(string, any) {}

func newHarness(t *testing.T) *harness {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "data.json")
	dataStore, err := store.Open(dataPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	logger := logging.New()
	secrets := security.NewMemoryStore("test")
	sessions := session.NewManager()
	recorder := &stateRecorder{}
	sessions.AddObserver(recorder.observe)

	profiles := profile.NewService(dataStore)
	credentials := credential.NewService(dataStore, secrets, logger)

	registry := vpn.NewRegistry()
	registry.Register(mock.New())

	return &harness{
		dataPath:    dataPath,
		profiles:    profiles,
		credentials: credentials,
		sessions:    sessions,
		manager:     vpn.NewManager(registry, profiles, credentials, sessions, logger),
		log:         logger,
		states:      recorder,
	}
}

func (h *harness) newProfile(t *testing.T, name string) domain.VPNProfile {
	t.Helper()
	p, err := h.profiles.Create(domain.ProfileInput{
		Name: name, Provider: domain.ProviderMock, Host: "vpn.example", Port: 443,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return p
}

func TestConnectWalksTheStateMachine(t *testing.T) {
	h := newHarness(t)
	p := h.newProfile(t, "SMOI")

	cred, err := h.credentials.Create(p.ID, domain.CredentialInput{
		Label: "Ming", Username: "ming", AuthMethod: domain.AuthPassword, Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	sess, err := h.manager.Connect(context.Background(), p.ID, cred.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if sess.State != domain.StateConnected {
		t.Fatalf("expected connected, got %s", sess.State)
	}
	if sess.CredentialID != cred.ID {
		t.Fatalf("session should record the credential UUID, got %q", sess.CredentialID)
	}
	if sess.AssignedIP == "" {
		t.Fatal("expected an assigned IP from the mock client")
	}
	for _, want := range []domain.ConnectionState{domain.StateConnecting, domain.StateAuthenticating, domain.StateConnected} {
		if !h.states.saw(want) {
			t.Fatalf("expected the session to pass through %s, saw %v", want, h.states.states)
		}
	}

	// The last-used account is remembered by UUID, not username.
	if prefs := h.profiles.Preferences(p.ID); prefs.LastCredentialID != cred.ID {
		t.Fatalf("expected last credential %s, got %q", cred.ID, prefs.LastCredentialID)
	}

	if _, err := h.manager.Disconnect(context.Background(), p.ID); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if got := h.sessions.State(p.ID); got != domain.StateDisconnected {
		t.Fatalf("expected disconnected, got %s", got)
	}
}

func TestConnectRequestsOTPAndDiscardsIt(t *testing.T) {
	h := newHarness(t)
	prompter := &otpPrompter{code: "123456"}
	h.manager.SetPrompter(prompter)

	p := h.newProfile(t, "K-CARE")
	cred, err := h.credentials.Create(p.ID, domain.CredentialInput{
		Label: "Ming", Username: "ming", AuthMethod: domain.AuthPasswordOTP, Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	if _, err := h.manager.Connect(context.Background(), p.ID, cred.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if !prompter.asked {
		t.Fatal("expected the adapter to request a one-time code")
	}
	if !h.states.saw(domain.StateWaitingForOTP) {
		t.Fatalf("expected a waiting_for_otp state, saw %v", h.states.states)
	}

	// Neither the password nor the OTP may reach the log or disk.
	assertNothingLeaked(t, h, "hunter2", "123456")
}

func TestFailedConnectionEndsInFailedState(t *testing.T) {
	h := newHarness(t)
	p := h.newProfile(t, "Broken")

	// The mock rejects the username "fail", exercising the failure path.
	cred, err := h.credentials.Create(p.ID, domain.CredentialInput{
		Label: "Bad account", Username: "fail", AuthMethod: domain.AuthPassword, Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	sess, err := h.manager.Connect(context.Background(), p.ID, cred.ID)
	if err == nil {
		t.Fatal("expected the connection to fail")
	}
	if sess.State != domain.StateFailed {
		t.Fatalf("expected failed, got %s", sess.State)
	}
	if sess.Error == "" {
		t.Fatal("expected a failure reason on the session")
	}
}

func TestUnsupportedProviderFailsSafely(t *testing.T) {
	h := newHarness(t)

	// No adapter is registered for Forcepoint in this harness, so the manager
	// must refuse rather than improvise.
	p, err := h.profiles.Create(domain.ProfileInput{
		Name: "Unregistered", Provider: domain.ProviderForcepoint, Host: "vpn.example", Port: 443,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	if _, err := h.manager.Connect(context.Background(), p.ID, "whatever"); err == nil {
		t.Fatal("expected connect to fail for an unregistered provider")
	}
}

func TestDuplicateUsernamesAreDistinctIdentities(t *testing.T) {
	h := newHarness(t)
	smoi := h.newProfile(t, "SMOI")
	other := h.newProfile(t, "Customer ABC")

	// The same username on the same profile, plus again on another profile.
	first, err := h.credentials.Create(smoi.ID, domain.CredentialInput{
		Label: "DevOps Shared", Username: "devops", Password: "one",
	})
	if err != nil {
		t.Fatalf("create first credential: %v", err)
	}
	second, err := h.credentials.Create(smoi.ID, domain.CredentialInput{
		Label: "DevOps Backup", Username: "devops", Password: "two",
	})
	if err != nil {
		t.Fatalf("create second credential: %v", err)
	}
	third, err := h.credentials.Create(other.ID, domain.CredentialInput{
		Label: "DevOps Shared", Username: "devops", Password: "three",
	})
	if err != nil {
		t.Fatalf("create third credential: %v", err)
	}

	ids := map[string]bool{first.ID: true, second.ID: true, third.ID: true}
	if len(ids) != 3 {
		t.Fatalf("expected three distinct credential UUIDs, got %v", ids)
	}

	// Each identity keeps its own secret despite sharing a username.
	for id, want := range map[string]string{first.ID: "one", second.ID: "two", third.ID: "three"} {
		resolved, err := h.credentials.Resolve(id)
		if err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
		if resolved.Secret != want {
			t.Fatalf("credential %s resolved the wrong secret", id)
		}
	}

	// Deleting one must not disturb the other with the same username.
	if err := h.credentials.Delete(second.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := h.credentials.Resolve(first.ID); err != nil {
		t.Fatalf("the surviving credential should still resolve: %v", err)
	}
}

func TestSecretsNeverReachDiskOrViews(t *testing.T) {
	h := newHarness(t)
	p := h.newProfile(t, "SMOI")

	view, err := h.credentials.Create(p.ID, domain.CredentialInput{
		Label: "Ming", Username: "ming", Password: "SuperSecret123",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if !view.HasPassword {
		t.Fatal("the view should report that a password is stored")
	}

	assertNothingLeaked(t, h, "SuperSecret123")

	// The reference persisted on disk is derived from UUIDs only.
	cred, err := h.credentials.Get(view.ID)
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if want := "tobiro/" + p.ID + "/" + view.ID; cred.SecretRef != want {
		t.Fatalf("secret ref = %q, want %q", cred.SecretRef, want)
	}
}

func TestLoggerRedactsProtectedSecrets(t *testing.T) {
	logger := logging.New()
	release := logger.Protect("SuperSecret123")
	defer release()

	logger.Info("connecting with password=SuperSecret123 for user ming")

	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if strings.Contains(entries[0].Message, "SuperSecret123") {
		t.Fatalf("the log leaked the secret: %s", entries[0].Message)
	}
}

// assertNothingLeaked checks the on-disk data file and the activity log for any
// of the given secrets.
func assertNothingLeaked(t *testing.T, h *harness, secrets ...string) {
	t.Helper()

	raw, err := os.ReadFile(h.dataPath)
	if err != nil {
		t.Fatalf("read data file: %v", err)
	}
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret %q was written to %s", secret, h.dataPath)
		}
		for _, entry := range h.log.Entries() {
			if strings.Contains(entry.Message, secret) {
				t.Fatalf("secret %q appeared in the activity log: %s", secret, entry.Message)
			}
		}
	}
}

func TestSessionDurationTracksConnectedTime(t *testing.T) {
	connectedAt := time.Now().Add(-90 * time.Second)
	s := domain.VPNSession{State: domain.StateConnected, ConnectedAt: &connectedAt}

	if got := s.DurationSeconds(); got < 89 || got > 92 {
		t.Fatalf("expected roughly 90 seconds, got %d", got)
	}
}
