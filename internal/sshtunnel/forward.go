package sshtunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
)

const probeTimeout = 2 * time.Second

// remoteDialer opens a connection to an address on the far side of the tunnel.
//
// It is a function rather than an *ssh.Client so a forwarder keeps working
// across a reconnect: the listener stays bound and the next accepted connection
// simply resolves the client that exists by then. Dropping the listeners on
// every network blip would take kubectl's endpoint away from under it.
type remoteDialer func(address string) (net.Conn, error)

// forwarder lends one local port to one address inside the customer network.
type forwarder struct {
	spec     domain.PortForward
	listener net.Listener
	dial     remoteDialer
	log      *logging.Logger

	closeOnce sync.Once

	active   atomic.Int64
	mu       sync.Mutex
	remoteOK *bool
	lastErr  string
}

// listen binds every forward, or none of them.
//
// A tunnel that came up with three of its four ports would be worse than one
// that refused: the missing port only shows up later as a connection refused
// from something that has no idea a tunnel is involved.
func listen(forwards []domain.PortForward, dial remoteDialer, log *logging.Logger) ([]*forwarder, error) {
	out := make([]*forwarder, 0, len(forwards))
	for _, spec := range forwards {
		listener, err := net.Listen("tcp", spec.Local())
		if err != nil {
			closeAll(out)
			return nil, describeBindError(spec, err)
		}
		f := &forwarder{spec: spec, listener: listener, dial: dial, log: log}
		go f.serve()
		out = append(out, f)
	}
	return out, nil
}

// describeBindError names the port, because "bind: address already in use" on
// its own sends the reader looking at the wrong end of the tunnel.
func describeBindError(spec domain.PortForward, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return failf(FailLocalPortInUse,
			"local port %d is already in use, so %s cannot be forwarded — close whatever is using it, or change the local port",
			spec.LocalPort, spec.Remote())
	}
	if errors.Is(err, syscall.EACCES) {
		return fmt.Errorf(
			"local port %d needs administrator rights to listen on — choose a port above 1023",
			spec.LocalPort)
	}
	return fmt.Errorf("listen on %s: %w", spec.Local(), err)
}

func (f *forwarder) serve() {
	for {
		local, err := f.listener.Accept()
		if err != nil {
			// A closed listener is how this loop is meant to end.
			return
		}
		go f.pipe(local)
	}
}

// pipe joins one accepted local connection to a fresh channel on the tunnel.
func (f *forwarder) pipe(local net.Conn) {
	defer local.Close()

	remote, err := f.dial(f.spec.Remote())
	if err != nil {
		f.noteDial(false, err)
		f.log.Warn("%s → %s: %v", f.spec.Local(), f.spec.Remote(), err)
		return
	}
	f.noteDial(true, nil)
	f.active.Add(1)
	defer f.active.Add(-1)
	defer remote.Close()

	// Both directions are copied, and the first to finish ends the pair: a
	// half-closed connection left waiting on the other side is how a forward
	// leaks goroutines over a long session.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	<-done
}

func (f *forwarder) noteDial(ok bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remoteOK = &ok
	if err != nil {
		f.lastErr = err.Error()
		return
	}
	f.lastErr = ""
}

func (f *forwarder) close() {
	f.closeOnce.Do(func() { _ = f.listener.Close() })
}

func closeAll(forwarders []*forwarder) {
	for _, f := range forwarders {
		f.close()
	}
}

// specs reports what these forwarders are lending, for the session record and
// for the peer endpoint to publish. Runtime status is filled in here so a
// refused remote target can be shown without taking the whole tunnel down.
func specs(forwarders []*forwarder) []domain.PortForward {
	out := make([]domain.PortForward, 0, len(forwarders))
	for _, f := range forwarders {
		out = append(out, f.view())
	}
	return out
}

func (f *forwarder) view() domain.PortForward {
	spec := f.spec
	active := int(f.active.Load())
	f.mu.Lock()
	spec.RemoteOK = f.remoteOK
	spec.LastError = f.lastErr
	f.mu.Unlock()

	switch {
	case active > 0:
		spec.State = domain.ForwardActive
		spec.Active = active
	case spec.LastError != "" && (spec.RemoteOK == nil || !*spec.RemoteOK):
		spec.State = domain.ForwardFailed
	default:
		spec.State = domain.ForwardListening
	}
	return spec
}

// probeRemotes asks each far-side address whether it answers, once.
//
// A refused target is recorded on that forward and left listening. The SSH
// session stays up: one dead service must not take the rest of the port map
// with it.
func probeRemotes(forwarders []*forwarder, dial remoteDialer) {
	var wg sync.WaitGroup
	for _, f := range forwarders {
		wg.Add(1)
		go func(f *forwarder) {
			defer wg.Done()
			err := probeOne(dial, f.spec.Remote())
			if err != nil {
				f.noteDial(false, err)
				f.log.Warn("%s is listening, but %s is not reachable: %v", f.spec.Local(), f.spec.Remote(), err)
				return
			}
			f.noteDial(true, nil)
		}(f)
	}
	wg.Wait()
}

func probeOne(dial remoteDialer, address string) error {
	type outcome struct {
		conn net.Conn
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		conn, err := dial(address)
		done <- outcome{conn, err}
	}()

	select {
	case result := <-done:
		if result.conn != nil {
			_ = result.conn.Close()
		}
		if result.err != nil {
			return fail(FailRemoteRefused, result.err)
		}
		return nil
	case <-time.After(probeTimeout):
		return failf(FailRemoteRefused, "remote target %s did not accept a connection", address)
	}
}
