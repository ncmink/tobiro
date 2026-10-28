package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
)

// The timings the supervisor runs on. They are variables so a test can watch a
// tunnel die and repair itself in a moment rather than in half a minute.
var (
	// keepaliveInterval is how often the tunnel asks the server whether it is
	// still there. A laptop that slept, a VPN that dropped underneath, or a
	// server that rebooted all look identical from here: the connection stops
	// answering and nothing tells us.
	keepaliveInterval = 15 * time.Second

	// keepaliveTimeout bounds one such question, so a wedged connection is
	// noticed rather than waited on.
	keepaliveTimeout = 10 * time.Second

	// redialMin/redialMax bound the backoff between attempts to repair a
	// tunnel that went down on its own.
	redialMin = 2 * time.Second
	redialMax = 30 * time.Second
)

// stateSink receives the transitions the supervisor drives, so the manager can
// apply them to the session table without this file knowing about sessions.
type stateSink func(state domain.ConnectionState, detail string)

// tunnel is one live SSH connection and the listeners it feeds.
//
// The listeners outlive any single connection. When the server goes away, the
// ports stay bound and the supervisor redials underneath them, so a client
// holding the endpoint — kubectl, Biebie Kube, a browser — sees a connection
// refused for a few seconds instead of losing the address entirely.
type tunnel struct {
	profile domain.VPNProfile
	config  *ssh.ClientConfig
	log     *logging.Logger
	onState stateSink

	mu         sync.Mutex
	client     *ssh.Client
	forwarders []*forwarder
	closed     bool

	cancel context.CancelFunc
	done   chan struct{}
}

// open dials the server, binds every forward, and starts supervising.
//
// The first dial is synchronous on purpose: a bad password or an unaccepted
// host key is the user's own next action, and reporting it as a failed connect
// is far more useful than a connection that claims to be coming up forever.
func open(ctx context.Context, profile domain.VPNProfile, config *ssh.ClientConfig, log *logging.Logger, onState stateSink) (*tunnel, error) {
	t := &tunnel{
		profile: profile,
		config:  config,
		log:     log,
		onState: onState,
		done:    make(chan struct{}),
	}

	client, err := t.dial(ctx)
	if err != nil {
		return nil, err
	}
	t.client = client

	forwarders, err := listen(profile.SSH.Forwards, t.dialRemote, log)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	t.forwarders = forwarders
	// A refused remote target is recorded on that forward. The listeners stay
	// up so the service can appear later without tearing the SSH session down.
	probeRemotes(forwarders, t.dialRemote)

	// The supervisor gets its own context: it has to outlive the connect call,
	// which returns as soon as the tunnel is up.
	superviseCtx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	go t.supervise(superviseCtx)

	return t, nil
}

func (t *tunnel) dial(ctx context.Context) (*ssh.Client, error) {
	address := t.profile.Endpoint()

	dialer := net.Dialer{Timeout: t.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, failf(FailUnreachable, "reach %s: %w", address, err)
	}

	// The handshake gets the same deadline as the dial. Without it, a host that
	// accepts TCP and then says nothing holds the attempt open indefinitely.
	if t.config.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(t.config.Timeout))
	}
	sshConn, channels, requests, err := ssh.NewClientConn(conn, address, t.config)
	if err != nil {
		_ = conn.Close()
		if KindOf(err) != "" {
			return nil, err
		}
		return nil, fail(FailAuth, fmt.Errorf("authenticate to %s: %w", address, err))
	}
	_ = conn.SetDeadline(time.Time{})

	return ssh.NewClient(sshConn, channels, requests), nil
}

// dialRemote opens a channel to an address inside the customer network, using
// whichever connection is current.
func (t *tunnel) dialRemote(address string) (net.Conn, error) {
	t.mu.Lock()
	client := t.client
	t.mu.Unlock()

	if client == nil {
		return nil, errors.New("the tunnel is down; it is being reconnected")
	}
	return client.Dial("tcp", address)
}

// supervise keeps the tunnel alive until it is closed.
func (t *tunnel) supervise(ctx context.Context) {
	defer close(t.done)

	for {
		if !t.watch(ctx) {
			return
		}
		if !t.repair(ctx) {
			return
		}
	}
}

// watch polls the current connection until it stops answering. It reports
// whether the caller should try to repair the tunnel, which is false when the
// tunnel was closed deliberately.
func (t *tunnel) watch(ctx context.Context) bool {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if err := t.keepalive(); err != nil {
				if t.isClosed() {
					return false
				}
				t.log.Warn("Tunnel to %s stopped answering: %v", t.profile.Endpoint(), err)
				t.onState(domain.StateConnecting, "reconnecting")
				return true
			}
		}
	}
}

// keepalive asks the server a question it is required to answer. The reply is
// meaningless; only the fact that one arrived matters.
func (t *tunnel) keepalive() error {
	t.mu.Lock()
	client := t.client
	t.mu.Unlock()
	if client == nil {
		return errors.New("no connection")
	}

	result := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		result <- err
	}()

	select {
	case err := <-result:
		return err
	case <-time.After(keepaliveTimeout):
		return errors.New("the server did not answer a keepalive")
	}
}

// repair redials with backoff until the tunnel is up again or it is closed.
func (t *tunnel) repair(ctx context.Context) bool {
	t.mu.Lock()
	if t.client != nil {
		_ = t.client.Close()
		t.client = nil
	}
	t.mu.Unlock()

	delay := redialMin
	for {
		client, err := t.dial(ctx)
		if err == nil {
			t.mu.Lock()
			if t.closed {
				t.mu.Unlock()
				_ = client.Close()
				return false
			}
			t.client = client
			t.mu.Unlock()

			t.log.Info("Tunnel to %s is back up", t.profile.Endpoint())
			t.onState(domain.StateConnected, "reconnected")
			return true
		}

		if t.isClosed() {
			return false
		}
		t.log.Warn("Could not reconnect to %s, retrying in %s: %v", t.profile.Endpoint(), delay, err)

		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		if delay *= 2; delay > redialMax {
			delay = redialMax
		}
	}
}

func (t *tunnel) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// forwards reports what this tunnel is lending.
func (t *tunnel) forwards() []domain.PortForward {
	t.mu.Lock()
	defer t.mu.Unlock()
	return specs(t.forwarders)
}

// close releases the local ports and the connection. It is safe to call twice.
func (t *tunnel) close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	client := t.client
	t.client = nil
	forwarders := t.forwarders
	t.forwarders = nil
	cancel := t.cancel
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	closeAll(forwarders)
	if client != nil {
		_ = client.Close()
	}
	<-t.done
}
