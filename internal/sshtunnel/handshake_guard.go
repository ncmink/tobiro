package sshtunnel

import (
	"net"
	"sync"
	"time"
)

// handshakeTimeout bounds how long a handshake may sit without progress,
// excluding time spent waiting on the user at a host-key prompt.
var handshakeTimeout = dialTimeout

// handshakeGuard closes the TCP connection when the handshake takes too long,
// but can be paused while the user reads a host-key prompt.
type handshakeGuard struct {
	mu   sync.Mutex
	conn net.Conn
	timer *time.Timer
}

func (g *handshakeGuard) bind(conn net.Conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conn = conn
	g.armLocked()
}

func (g *handshakeGuard) arm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armLocked()
}

func (g *handshakeGuard) armLocked() {
	if g.conn == nil || handshakeTimeout <= 0 {
		return
	}
	if g.timer != nil {
		g.timer.Stop()
	}
	conn := g.conn
	g.timer = time.AfterFunc(handshakeTimeout, func() { _ = conn.Close() })
}

func (g *handshakeGuard) pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
}

func (g *handshakeGuard) stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	g.conn = nil
}