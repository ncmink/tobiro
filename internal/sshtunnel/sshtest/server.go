// Package sshtest runs an SSH server on loopback that speaks just enough of the
// protocol to carry a local port forward.
//
// It exists because the interesting half of internal/sshtunnel — the handshake,
// the direct-tcpip channels, the keepalives — cannot be exercised against a
// fake. Everything here is deliberately permissive about who may log in and
// deliberately strict about nothing else: it is a stand-in for the far side of
// a tunnel, and it is imported only by tests and by cmd/endpoint-smoke.
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Server is a listening SSH server. Close it to release the port.
type Server struct {
	config   *ssh.ServerConfig
	listener net.Listener
	hostKey  ssh.PublicKey

	// Password, when set, is the only one accepted. Empty accepts any login,
	// which is what a test wanting to reach the forwarding code wants.
	password string

	mu   sync.Mutex
	live map[net.Conn]struct{}

	closeOnce sync.Once
	done      chan struct{}
}

// Option configures a server before it starts.
type Option func(*Server)

// RequirePassword makes the server reject any other password, so a test can
// prove the credential actually reached the handshake.
func RequirePassword(password string) Option {
	return func(s *Server) { s.password = password }
}

// Start brings up a server on a loopback port of the kernel's choosing.
func Start(options ...Option) (*Server, error) {
	server := &Server{live: make(map[net.Conn]struct{}), done: make(chan struct{})}
	for _, option := range options {
		option(server)
	}

	signer, err := hostKey()
	if err != nil {
		return nil, err
	}
	server.hostKey = signer.PublicKey()

	server.config = &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if server.password != "" && string(password) != server.password {
				return nil, fmt.Errorf("wrong password")
			}
			return nil, nil
		},
		PublicKeyCallback: func(_ ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
			if server.password != "" {
				return nil, fmt.Errorf("this server wants a password")
			}
			return nil, nil
		},
	}
	server.config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server.listener = listener

	go server.accept()
	return server, nil
}

// Addr is the host:port the server is listening on.
func (s *Server) Addr() string { return s.listener.Addr().String() }

// Host and Port split Addr for the fields a profile keeps them in.
func (s *Server) Host() string {
	host, _, _ := net.SplitHostPort(s.Addr())
	return host
}

func (s *Server) Port() int {
	addr, _ := s.listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// KnownHostsLine is the entry that makes this server already-trusted, for a
// test that wants the tunnel rather than the host key prompt.
func (s *Server) KnownHostsLine() string {
	return knownhosts.Line([]string{knownhosts.Normalize(s.Addr())}, s.hostKey) + "\n"
}

// Fingerprint is what a host key prompt would show.
func (s *Server) Fingerprint() string { return ssh.FingerprintSHA256(s.hostKey) }

// Drop severs every connection the server is holding while staying open for
// new ones. That is what a laptop waking from sleep finds: the old connection
// is gone, and the server it was to is perfectly fine.
func (s *Server) Drop() {
	s.mu.Lock()
	held := s.live
	s.live = make(map[net.Conn]struct{})
	s.mu.Unlock()

	for conn := range held {
		_ = conn.Close()
	}
}

// Close stops the server and drops what it is holding.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.listener.Close()
		s.Drop()
	})
	return nil
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	s.mu.Lock()
	s.live[conn] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.live, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	_, channels, requests, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}

	// keepalive@openssh.com arrives here. Replying at all is the point: the
	// client is measuring whether this connection still answers.
	go ssh.DiscardRequests(requests)

	for channel := range channels {
		if channel.ChannelType() != "direct-tcpip" {
			_ = channel.Reject(ssh.UnknownChannelType, "only port forwarding is served here")
			continue
		}
		go s.forward(channel)
	}
}

// directTCPIP is the channel payload of an `ssh -L` connection, from RFC 4254.
type directTCPIP struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

func (s *Server) forward(request ssh.NewChannel) {
	var payload directTCPIP
	if err := ssh.Unmarshal(request.ExtraData(), &payload); err != nil {
		_ = request.Reject(ssh.ConnectionFailed, "unreadable forwarding request")
		return
	}

	target := net.JoinHostPort(payload.DestAddr, fmt.Sprint(payload.DestPort))
	remote, err := net.Dial("tcp", target)
	if err != nil {
		_ = request.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer remote.Close()

	channel, requests, err := request.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	go ssh.DiscardRequests(requests)

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, channel); done <- struct{}{} }()
	go func() { _, _ = io.Copy(channel, remote); done <- struct{}{} }()
	<-done
}

// hostKey generates a throwaway key. It lives as long as the server does, so a
// restart looks like a rebuilt host — which is itself worth being able to test.
func hostKey() (ssh.Signer, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(pem.EncodeToMemory(block))
}
