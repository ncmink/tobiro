package sshtunnel

import (
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
)

func testLogger() *logging.Logger { return logging.New() }

// echo stands in for whatever answers on the far side of the tunnel.
func echo(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener
}

// freePort takes a port and gives it straight back, so the number is one the
// kernel was willing to hand out a moment ago.
func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

func directDialer(t *testing.T) remoteDialer {
	t.Helper()
	return func(address string) (net.Conn, error) {
		return net.DialTimeout("tcp", address, 2*time.Second)
	}
}

func TestForwardCarriesTrafficToTheRemoteAddress(t *testing.T) {
	remote := echo(t)
	_, remotePort, _ := net.SplitHostPort(remote.Addr().String())
	port, _ := strconv.Atoi(remotePort)

	forwarders, err := listen([]domain.PortForward{
		{Name: "echo", LocalPort: freePort(t), RemoteHost: "127.0.0.1", RemotePort: port},
	}, directDialer(t), testLogger())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(forwarders)

	conn, err := net.DialTimeout("tcp", forwarders[0].spec.Local(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial the forward: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf) != "ping\n" {
		t.Fatalf("read %q through the forward", buf)
	}
}

// A forward only ever answers on loopback. Binding 0.0.0.0 would lend a
// customer's internal service to every machine sharing the café wifi.
func TestForwardsBindLoopbackOnly(t *testing.T) {
	forwarders, err := listen([]domain.PortForward{
		{LocalPort: freePort(t), RemoteHost: "localhost", RemotePort: 6443},
	}, directDialer(t), testLogger())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(forwarders)

	host, _, err := net.SplitHostPort(forwarders[0].listener.Addr().String())
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("listening on %q, want loopback only", host)
	}
}

// A tunnel that came up with one of its two ports missing is worse than one
// that refused: the gap only shows up later, as a connection refused from
// something with no idea a tunnel is involved.
func TestABusyPortFailsTheWholeTunnelAndReleasesTheRest(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer busy.Close()

	_, busyPort, _ := net.SplitHostPort(busy.Addr().String())
	taken, _ := strconv.Atoi(busyPort)
	first := freePort(t)

	forwarders, err := listen([]domain.PortForward{
		{Name: "Kubernetes API", LocalPort: first, RemoteHost: "localhost", RemotePort: 6443},
		{LocalPort: taken, RemoteHost: "localhost", RemotePort: 31539},
	}, directDialer(t), testLogger())
	if err == nil {
		closeAll(forwarders)
		t.Fatal("expected a busy local port to fail the tunnel")
	}

	// The port number is the actionable part; without it the reader goes
	// looking at the far end of the tunnel.
	if !strings.Contains(err.Error(), strconv.Itoa(taken)) {
		t.Errorf("error %q does not name the busy port", err)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error %q does not say what went wrong", err)
	}

	// The port that did bind has to be given back, or a retry after freeing
	// the conflict would now fail on a port this attempt is still holding.
	retry, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Fatalf("the first forward's port was left bound after the failure: %v", err)
	}
	_ = retry.Close()
}

func TestSpecsReportWhatIsLent(t *testing.T) {
	want := []domain.PortForward{
		{Name: "Kubernetes API", LocalPort: freePort(t), RemoteHost: "localhost", RemotePort: 6443},
		{LocalPort: freePort(t), RemoteHost: "localhost", RemotePort: 31539},
	}

	forwarders, err := listen(want, directDialer(t), testLogger())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(forwarders)

	got := specs(forwarders)
	if len(got) != len(want) {
		t.Fatalf("specs returned %d forwards, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].LocalPort != want[i].LocalPort ||
			got[i].RemoteHost != want[i].RemoteHost || got[i].RemotePort != want[i].RemotePort {
			t.Errorf("forward %d = %+v, want %+v", i, got[i], want[i])
		}
		if got[i].State != domain.ForwardListening {
			t.Errorf("forward %d state = %q, want listening", i, got[i].State)
		}
	}
}

func TestABusyPortIsALocalPortInUseFailure(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer busy.Close()
	_, busyPort, _ := net.SplitHostPort(busy.Addr().String())
	taken, _ := strconv.Atoi(busyPort)

	_, err = listen([]domain.PortForward{
		{LocalPort: taken, RemoteHost: "localhost", RemotePort: 6443},
	}, directDialer(t), testLogger())
	if KindOf(err) != FailLocalPortInUse {
		t.Fatalf("kind = %q, want %s (%v)", KindOf(err), FailLocalPortInUse, err)
	}
}

// A refused remote target is that forward's problem. The listener stays, and
// a sibling forward that can answer is left alone.
func TestADeadRemoteDoesNotTakeTheOtherForwardDown(t *testing.T) {
	alive := echo(t)
	_, remotePort, _ := net.SplitHostPort(alive.Addr().String())
	port, _ := strconv.Atoi(remotePort)
	dead := freePort(t)

	forwarders, err := listen([]domain.PortForward{
		{Name: "Kubernetes API", LocalPort: freePort(t), RemoteHost: "127.0.0.1", RemotePort: port},
		{Name: "missing", LocalPort: freePort(t), RemoteHost: "127.0.0.1", RemotePort: dead},
	}, directDialer(t), testLogger())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(forwarders)

	probeRemotes(forwarders, directDialer(t))
	got := specs(forwarders)
	if len(got) != 2 {
		t.Fatalf("got %d forwards", len(got))
	}
	if got[0].RemoteOK == nil || !*got[0].RemoteOK || got[0].State != domain.ForwardListening {
		t.Fatalf("live target should stay listening: %+v", got[0])
	}
	if got[1].RemoteOK == nil || *got[1].RemoteOK || got[1].State != domain.ForwardFailed {
		t.Fatalf("dead target should be recorded as failed, not tear the tunnel down: %+v", got[1])
	}

	// The dead forward is still bound, so a later service can appear.
	conn, err := net.DialTimeout("tcp", forwarders[1].spec.Local(), 2*time.Second)
	if err != nil {
		t.Fatalf("dead remote's local port should still accept: %v", err)
	}
	_ = conn.Close()
}
