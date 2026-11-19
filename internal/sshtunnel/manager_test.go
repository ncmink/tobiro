package sshtunnel

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/session"
	"tobiro/internal/sshtunnel/sshtest"
)

// profiles is the slice of the profile service the manager reads.
type profiles struct {
	profile    domain.VPNProfile
	remembered string
}

func (p *profiles) Get(id string) (domain.VPNProfile, error) {
	if id != p.profile.ID {
		return domain.VPNProfile{}, os.ErrNotExist
	}
	return p.profile, nil
}

func (p *profiles) List() []domain.VPNProfile { return []domain.VPNProfile{p.profile} }

func (p *profiles) RememberCredential(_, credentialID string) error {
	p.remembered = credentialID
	return nil
}

type credentials struct{ credential domain.Credential }

func (c credentials) Resolve(id string) (domain.Credential, error) {
	if id != c.credential.ID {
		return domain.Credential{}, os.ErrNotExist
	}
	return c.credential, nil
}

// prompter records what the UI was asked and answers with a fixed decision.
type prompter struct {
	accept       bool
	confirmDelay time.Duration
	asked        chan domain.HostKeyRequest
	events       chan string
	failure      chan domain.ConnectionFailedEvent
}

func newPrompter(accept bool) *prompter {
	return &prompter{
		accept:  accept,
		asked:   make(chan domain.HostKeyRequest, 4),
		events:  make(chan string, 8),
		failure: make(chan domain.ConnectionFailedEvent, 4),
	}
}

func (p *prompter) ConfirmHostKey(_ context.Context, req domain.HostKeyRequest) (bool, error) {
	if p.confirmDelay > 0 {
		time.Sleep(p.confirmDelay)
	}
	p.asked <- req
	return p.accept, nil
}

func (p *prompter) Notify(event string, payload any) {
	p.events <- event
	if failed, ok := payload.(domain.ConnectionFailedEvent); ok {
		p.failure <- failed
	}
}

// rig is a manager wired to a loopback SSH server, with HOME pointed at a
// throwaway directory so nothing here touches the developer's known_hosts.
type rig struct {
	manager  *Manager
	server   *sshtest.Server
	sessions *session.Manager
	profiles *profiles
	prompter *prompter

	// echo is the address on the far side that the forward reaches.
	echo string

	home string
}

func newRig(t *testing.T, options ...sshtest.Option) *rig {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	// A stray agent socket in the environment would be tried before anything
	// this test set up.
	t.Setenv("SSH_AUTH_SOCK", "")

	server, err := sshtest.Start(options...)
	if err != nil {
		t.Fatalf("start SSH server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	backend := echo(t)
	remotePort := backend.Addr().(*net.TCPAddr).Port

	profile := domain.VPNProfile{
		ID:       "p1",
		Name:     "SMOI Bastion",
		Kind:     domain.KindSSH,
		Provider: domain.ProviderSSH,
		Host:     server.Host(),
		Port:     server.Port(),
		SSH: domain.SSHSettings{
			User: "server1",
			Forwards: []domain.PortForward{
				{Name: "echo", LocalPort: freePort(t), RemoteHost: "127.0.0.1", RemotePort: remotePort},
			},
		},
	}

	store := &profiles{profile: profile}
	creds := credentials{credential: domain.Credential{
		ID: "c1", ProfileID: "p1", Username: "server1",
		AuthMethod: domain.AuthPassword, Secret: "hunter2",
	}}

	sessions := session.NewManager()
	manager := NewManager(store, creds, sessions, testLogger())
	prompt := newPrompter(true)
	manager.SetPrompter(prompt)

	// Without this, a tunnel left open by a finished test keeps redialling a
	// server that has gone, for as long as the whole run lasts.
	t.Cleanup(manager.Shutdown)

	return &rig{
		manager:  manager,
		server:   server,
		sessions: sessions,
		profiles: store,
		prompter: prompt,
		echo:     backend.Addr().String(),
		home:     home,
	}
}

func (r *rig) trustTheServer(t *testing.T) {
	t.Helper()
	dir := filepath.Join(r.home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make .ssh: %v", err)
	}
	path := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(path, []byte(r.server.KnownHostsLine()), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
}

func (r *rig) localPort() int { return r.profiles.profile.SSH.Forwards[0].LocalPort }

// speak sends a line through the forward and reads the echo back, which is only
// possible if the listener, the SSH channel and the far-side dial all worked.
func speak(t *testing.T, address, line string) string {
	t.Helper()

	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", address, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(line)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(line))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	return string(buf)
}

// unreachableBastion points the profile at a port nothing is listening on,
// which is what a bastion inside a customer network looks like from outside it.
func (r *rig) unreachableBastion(t *testing.T) {
	t.Helper()
	r.profiles.profile.Port = freePort(t)
}

func TestABastionThatDoesNotAnswerSuggestsTheVPN(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)
	r.unreachableBastion(t)

	_, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("expected the connect to fail")
	}
	// The dial failure alone says only that nobody answered, which is the least
	// useful half of the answer when the customer's VPN is down.
	if !strings.Contains(err.Error(), "needs another connection first") {
		t.Fatalf("expected the error to suggest recording the VPN, got %v", err)
	}
}

func TestATunnelThatAlreadyKnowsItsVPNIsNotGivenTheHint(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)
	r.unreachableBastion(t)
	r.profiles.profile.Requires = "the-customer-vpn"

	_, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("expected the connect to fail")
	}
	// The router brought the VPN up before this call, so the address really is
	// unreachable and advice about the VPN would send the engineer the wrong way.
	if strings.Contains(err.Error(), "needs another connection first") {
		t.Fatalf("expected no advice about the VPN, got %v", err)
	}
}

func TestARejectedPasswordIsNotBlamedOnTheVPN(t *testing.T) {
	r := newRig(t, sshtest.RequirePassword("something-else"))
	r.trustTheServer(t)

	_, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("expected the connect to fail")
	}
	if strings.Contains(err.Error(), "needs another connection first") {
		t.Fatalf("a failure after the connection opened must not suggest the VPN, got %v", err)
	}
}

// The whole feature, end to end: a real handshake against a real SSH server,
// a real forwarded channel, and a session record a peer could publish.
func TestConnectOpensATunnelThatCarriesTraffic(t *testing.T) {
	r := newRig(t, sshtest.RequirePassword("hunter2"))
	r.trustTheServer(t)

	live, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if live.State != domain.StateConnected {
		t.Fatalf("state = %s, want connected", live.State)
	}
	if live.Identity != "server1@"+r.server.Host() {
		t.Errorf("identity = %q", live.Identity)
	}
	// The session's address is what the UI shows and what a peer publishes, and
	// for a tunnel the honest answer is that everything answers on this machine.
	if live.AssignedIP != domain.LoopbackAddress {
		t.Errorf("assigned ip = %q, want loopback", live.AssignedIP)
	}
	if len(live.Forwards) != 1 || live.Forwards[0].LocalPort != r.localPort() {
		t.Fatalf("forwards = %+v", live.Forwards)
	}

	if got := speak(t, live.Forwards[0].Local(), "ping\n"); got != "ping\n" {
		t.Fatalf("read %q through the tunnel", got)
	}

	if r.profiles.remembered != "c1" {
		t.Errorf("last account = %q, want the one just used", r.profiles.remembered)
	}
}

// Disconnect has to give the ports back. A listener outliving its tunnel would
// accept a connection and then have nowhere to send it.
func TestDisconnectReleasesTheLocalPorts(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	local := net.JoinHostPort("127.0.0.1", strconv.Itoa(r.localPort()))

	final, err := r.manager.Disconnect(context.Background(), "p1")
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if final.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected", final.State)
	}
	if len(final.Forwards) != 0 {
		t.Errorf("a closed tunnel still reports %+v", final.Forwards)
	}

	listener, err := net.Listen("tcp", local)
	if err != nil {
		t.Fatalf("port %d was still bound after disconnect: %v", r.localPort(), err)
	}
	_ = listener.Close()
}

// An unknown server is a question, not a silent yes. This is the security
// boundary of the whole feature: the tunnel carries Kubernetes credentials.
func TestAnUnknownHostIsPutToTheUser(t *testing.T) {
	r := newRig(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	select {
	case asked := <-r.prompter.asked:
		if asked.Fingerprint != r.server.Fingerprint() {
			t.Errorf("prompt showed %q, want the server's own fingerprint", asked.Fingerprint)
		}
		if asked.ProfileID != "p1" {
			t.Errorf("prompt was attributed to %q", asked.ProfileID)
		}
	default:
		t.Fatal("an unrecognised server was accepted without asking")
	}

	// Accepting it once is accepting it for good, the way the ssh command line
	// behaves — otherwise the dialog becomes something to click through.
	recorded, err := os.ReadFile(filepath.Join(r.home, ".ssh", "known_hosts"))
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if !strings.Contains(string(recorded), r.server.Host()) {
		t.Errorf("known_hosts does not mention the server: %q", recorded)
	}
}

func TestRefusingTheHostKeyRefusesTheTunnel(t *testing.T) {
	r := newRig(t)
	r.prompter.accept = false

	_, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("the tunnel came up despite the host key being refused")
	}
	if !strings.Contains(err.Error(), "not accepted") {
		t.Errorf("error %q does not say the key was refused", err)
	}
	if KindOf(err) != FailHostKeyUnknown {
		t.Errorf("kind = %q, want %s", KindOf(err), FailHostKeyUnknown)
	}

	if _, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(r.localPort())), 200*time.Millisecond); err == nil {
		t.Error("a refused tunnel left its local port listening")
	}
	if current, ok := r.sessions.Get("p1"); !ok || current.State != domain.StateFailed {
		t.Errorf("session = %+v, want a failed state the UI can explain", current)
	}
}

// A wrong password must fail as a wrong password, and it must never appear in
// the activity log — including inside the error text the server produced.
func TestABadPasswordFailsWithoutLeakingIt(t *testing.T) {
	r := newRig(t, sshtest.RequirePassword("the-real-one"))
	r.trustTheServer(t)

	_, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("the tunnel came up with the wrong password")
	}
	if KindOf(err) != FailAuth {
		t.Errorf("kind = %q, want %s", KindOf(err), FailAuth)
	}

	select {
	case failed := <-r.prompter.failure:
		if strings.Contains(failed.Reason, "hunter2") {
			t.Errorf("the failure notice carries the password: %q", failed.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("the UI was never told the connection failed")
	}
}

// After a crash the session file can claim a tunnel this process does not hold.
// Left alone, a peer would route a cluster into a port that is not listening.
func TestRefreshCorrectsASessionWithNoTunnelBehindIt(t *testing.T) {
	r := newRig(t)

	if _, err := r.sessions.Start("p1", ""); err != nil {
		t.Fatalf("start session: %v", err)
	}
	if _, err := r.sessions.TransitionWith("p1", domain.StateConnected, func(s *domain.VPNSession) {
		s.Forwards = r.profiles.profile.SSH.Forwards
	}); err != nil {
		t.Fatalf("transition: %v", err)
	}

	r.manager.RefreshAll(context.Background())

	current, _ := r.sessions.Get("p1")
	if current.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected", current.State)
	}
}

// A live tunnel must survive a refresh, or every status question a peer asks
// would tear down the connection it was asking about.
func TestRefreshLeavesALiveTunnelAlone(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	r.manager.RefreshAll(context.Background())

	current, _ := r.sessions.Get("p1")
	if current.State != domain.StateConnected {
		t.Fatalf("a refresh took down a live tunnel: state = %s", current.State)
	}
	if got := speak(t, current.Forwards[0].Local(), "still-here\n"); got != "still-here\n" {
		t.Fatalf("read %q after a refresh", got)
	}
}

// A tunnel that is still in the map but whose local port is no longer served is
// not a tunnel. Treating the map entry as proof is how a dead tunnel stayed
// "connected" forever.
func TestRefreshReleasesATunnelWhoseListenerIsGone(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	r.manager.mu.Lock()
	held := r.manager.tunnels["p1"]
	r.manager.mu.Unlock()
	held.mu.Lock()
	for _, f := range held.forwarders {
		f.close()
	}
	held.mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	for !held.forwarders[0].stopped.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	r.manager.RefreshAll(context.Background())

	current, _ := r.sessions.Get("p1")
	if current.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected", current.State)
	}
	r.manager.mu.Lock()
	_, stillHeld := r.manager.tunnels["p1"]
	r.manager.mu.Unlock()
	if stillHeld {
		t.Fatal("the dead tunnel is still held")
	}
}

// The supervisor ending on its own means nothing will ever redial the tunnel.
func TestRefreshReleasesATunnelWhoseSupervisorHasStopped(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	r.manager.mu.Lock()
	held := r.manager.tunnels["p1"]
	r.manager.mu.Unlock()
	held.cancel()
	<-held.done

	r.manager.RefreshAll(context.Background())

	current, _ := r.sessions.Get("p1")
	if current.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected", current.State)
	}
}

func TestShutdownClosesEveryTunnel(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	r.manager.Shutdown()

	local := net.JoinHostPort("127.0.0.1", strconv.Itoa(r.localPort()))
	listener, err := net.Listen("tcp", local)
	if err != nil {
		t.Fatalf("shutdown left port %d bound: %v", r.localPort(), err)
	}
	_ = listener.Close()
}

// The laptop-lid case the whole supervisor exists for: the connection dies
// under a tunnel nobody touched, and the tunnel puts itself back together
// without the engineer noticing, on the same local ports as before.
func TestATunnelRepairsItselfAfterTheConnectionDies(t *testing.T) {
	quicken(t)

	r := newRig(t)
	r.trustTheServer(t)

	live, err := r.manager.Connect(context.Background(), "p1", "c1")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	local := live.Forwards[0].Local()

	// Every state is recorded rather than polled for: the repair can be quicker
	// than any poll, and a test that missed it would report the outage never
	// happened.
	states := make(chan domain.ConnectionState, 16)
	r.sessions.AddObserver(func(s domain.VPNSession) { states <- s.State })

	r.server.Drop()

	// The outage has to be visible as an outage. A tunnel that quietly stopped
	// carrying traffic while still claiming to be connected is the state this
	// supervisor exists to prevent.
	awaitState(t, states, domain.StateConnecting)
	awaitState(t, states, domain.StateConnected)

	// The same local port, still bound. It stays that way across the outage on
	// purpose: a client holding the endpoint sees a refused connection for a
	// moment rather than losing the address entirely.
	if got := speak(t, local, "after-the-nap\n"); got != "after-the-nap\n" {
		t.Fatalf("read %q through the repaired tunnel", got)
	}
}

func awaitState(t *testing.T, states <-chan domain.ConnectionState, want domain.ConnectionState) {
	t.Helper()

	deadline := time.After(15 * time.Second)
	for {
		select {
		case state := <-states:
			if state == want {
				return
			}
		case <-deadline:
			t.Fatalf("the tunnel never reached %s", want)
		}
	}
}

// quicken shortens the supervisor's timings for the length of one test, so a
// reconnection can be watched in a moment rather than in half a minute.
func quicken(t *testing.T) {
	t.Helper()

	interval, timeout, min, max := keepaliveInterval, keepaliveTimeout, redialMin, redialMax
	keepaliveInterval = 50 * time.Millisecond
	keepaliveTimeout = 2 * time.Second
	redialMin = 20 * time.Millisecond
	redialMax = 100 * time.Millisecond

	t.Cleanup(func() {
		keepaliveInterval, keepaliveTimeout, redialMin, redialMax = interval, timeout, min, max
	})
}

// A VPN profile reaching this manager is a routing bug, and it must not be
// answered with a half-attempted SSH connection.
// A server that accepts TCP and then says nothing mimics a wedged handshake.
// Disconnect must cancel the attempt without leaving listeners or a connected session.
// The handshake timer must not fire while the user is reading a host-key prompt.
func TestAHostKeyPromptLongerThanTheHandshakeBudgetStillSucceeds(t *testing.T) {
	prev := handshakeTimeout
	handshakeTimeout = 80 * time.Millisecond
	t.Cleanup(func() { handshakeTimeout = prev })

	r := newRig(t)
	r.prompter.accept = true
	r.prompter.confirmDelay = 200 * time.Millisecond

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
}

func TestASilentServerEventuallyTimesOutTheHandshake(t *testing.T) {
	prev := handshakeTimeout
	handshakeTimeout = 80 * time.Millisecond
	t.Cleanup(func() { handshakeTimeout = prev })

	r := newRig(t)
	r.trustTheServer(t)

	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = hold.Close() })
	go func() {
		for {
			conn, err := hold.Accept()
			if err != nil {
				return
			}
			_ = conn // never speaks SSH
		}
	}()
	addr := hold.Addr().(*net.TCPAddr)
	r.profiles.profile.Host = addr.IP.String()
	r.profiles.profile.Port = addr.Port

	_, err = r.manager.Connect(context.Background(), "p1", "c1")
	if err == nil {
		t.Fatal("expected handshake timeout")
	}
}

func TestDisconnectDuringConnectCancelsTheAttempt(t *testing.T) {
	r := newRig(t)
	r.trustTheServer(t)

	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = hold.Close() })
	go func() {
		for {
			conn, err := hold.Accept()
			if err != nil {
				return
			}
			// Hold the connection open without speaking SSH.
			go func(c net.Conn) {
				<-t.Context().Done()
				_ = c.Close()
			}(conn)
		}
	}()
	addr := hold.Addr().(*net.TCPAddr)
	r.profiles.profile.Host = addr.IP.String()
	r.profiles.profile.Port = addr.Port

	done := make(chan struct{})
	var connectErr error
	go func() {
		_, connectErr = r.manager.Connect(context.Background(), "p1", "c1")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if _, err := r.manager.Disconnect(context.Background(), "p1"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connect did not return after disconnect")
	}
	if connectErr == nil {
		t.Fatal("expected connect to be cancelled")
	}

	current, ok := r.sessions.Get("p1")
	if !ok || current.State != domain.StateDisconnected {
		t.Fatalf("session = %+v, want disconnected", current)
	}

	local := net.JoinHostPort("127.0.0.1", strconv.Itoa(r.localPort()))
	if _, err := net.DialTimeout("tcp", local, 200*time.Millisecond); err == nil {
		t.Error("a cancelled connect left the forward port listening")
	}
	r.manager.mu.Lock()
	_, held := r.manager.tunnels["p1"]
	r.manager.mu.Unlock()
	if held {
		t.Fatal("a cancelled connect left a tunnel in the map")
	}
}

type routeCreds map[string]domain.Credential

func (r routeCreds) Resolve(id string) (domain.Credential, error) {
	c, ok := r[id]
	if !ok {
		return domain.Credential{}, os.ErrNotExist
	}
	return c, nil
}

type mapProfiles struct {
	all map[string]domain.VPNProfile
}

func (m *mapProfiles) Get(id string) (domain.VPNProfile, error) {
	p, ok := m.all[id]
	if !ok {
		return domain.VPNProfile{}, os.ErrNotExist
	}
	return p, nil
}

func (m *mapProfiles) List() []domain.VPNProfile {
	out := make([]domain.VPNProfile, 0, len(m.all))
	for _, p := range m.all {
		out = append(out, p)
	}
	return out
}

func (m *mapProfiles) RememberCredential(_, _ string) error { return nil }

func TestConnectThroughProxyJump(t *testing.T) {
	jumpSrv, err := sshtest.Start()
	if err != nil {
		t.Fatalf("jump server: %v", err)
	}
	t.Cleanup(func() { _ = jumpSrv.Close() })
	targetSrv, err := sshtest.Start()
	if err != nil {
		t.Fatalf("target server: %v", err)
	}
	t.Cleanup(func() { _ = targetSrv.Close() })

	backend := echo(t)
	remotePort := backend.Addr().(*net.TCPAddr).Port
	localPort := freePort(t)

	jumpProfile := domain.VPNProfile{
		ID: "jump", Name: "jump", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
		Host: jumpSrv.Host(), Port: jumpSrv.Port(),
		SSH: domain.SSHSettings{
			User: "jumpuser",
			Forwards: []domain.PortForward{
				{Name: "placeholder", LocalPort: freePort(t), RemoteHost: "127.0.0.1", RemotePort: 22},
			},
		},
	}
	targetProfile := domain.VPNProfile{
		ID: "target", Name: "target", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
		JumpHost: "jump",
		Host:     targetSrv.Host(),
		Port:     targetSrv.Port(),
		SSH: domain.SSHSettings{
			User: "targetuser",
			Forwards: []domain.PortForward{
				{Name: "echo", LocalPort: localPort, RemoteHost: "127.0.0.1", RemotePort: remotePort},
			},
		},
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	known := jumpSrv.KnownHostsLine() + targetSrv.KnownHostsLine()
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(known), 0o600); err != nil {
		t.Fatalf("known_hosts: %v", err)
	}

	store := &mapProfiles{all: map[string]domain.VPNProfile{"jump": jumpProfile, "target": targetProfile}}
	creds := routeCreds{
		"cj": {ID: "cj", ProfileID: "jump", Username: "jumpuser", AuthMethod: domain.AuthPassword, Secret: "hunter2"},
		"ct": {ID: "ct", ProfileID: "target", Username: "targetuser", AuthMethod: domain.AuthPassword, Secret: "hunter2"},
	}
	sessions := session.NewManager()
	manager := NewManager(store, creds, sessions, testLogger())
	manager.SetPrompter(newPrompter(true))
	t.Cleanup(manager.Shutdown)

	before := jumpSrv.ForwardCount()
	if _, err := manager.Connect(context.Background(), "jump", "cj"); err != nil {
		t.Fatalf("connect jump: %v", err)
	}
	if _, err := manager.Connect(context.Background(), "target", "ct"); err != nil {
		t.Fatalf("connect target: %v", err)
	}
	if jumpSrv.ForwardCount() <= before {
		t.Fatalf("expected the jump host to open a forwarded channel, before=%d after=%d", before, jumpSrv.ForwardCount())
	}
	if got := speak(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)), "hop\n"); got != "hop\n" {
		t.Fatalf("read %q through proxied tunnel", got)
	}
}

func TestAVPNProfileIsRefused(t *testing.T) {
	r := newRig(t)
	r.profiles.profile.Kind = domain.KindVPN

	if _, err := r.manager.Connect(context.Background(), "p1", "c1"); err == nil {
		t.Fatal("a VPN profile was accepted as an SSH tunnel")
	}
}
