// Command endpoint-smoke runs the real Tobiro endpoint against a
// throwaway data directory, so the other half of the integration can be checked
// without launching the desktop application.
//
// It is a development tool. Nothing in the application imports it.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"tobiro/internal/access"
	"tobiro/internal/credential"
	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/peer"
	"tobiro/internal/profile"
	"tobiro/internal/security"
	"tobiro/internal/session"
	"tobiro/internal/sshtunnel"
	"tobiro/internal/sshtunnel/sshtest"
	"tobiro/internal/store"
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/mock"
)

func main() {
	dir, err := os.MkdirTemp("", "access-smoke")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	dataStore, err := store.Open(filepath.Join(dir, "data.json"))
	if err != nil {
		log.Fatal(err)
	}

	logger := logging.New()
	profiles := profile.NewService(dataStore)
	credentials := credential.NewService(dataStore, security.NewMemoryStore("smoke"), logger)
	sessions := session.NewManager()

	registry := vpn.NewRegistry()
	registry.Register(mock.New())
	manager := vpn.NewManager(registry, profiles, credentials, sessions, logger)
	tunnels := sshtunnel.NewManager(profiles, credentials, sessions, logger)

	// The bastion below is invented, lives for one run, and would otherwise
	// leave a line about a long-gone port in the developer's own known_hosts.
	tunnels.SetKnownHosts(filepath.Join(dir, "known_hosts"))
	router := access.NewRouter(profiles, sessions, credentials, manager, tunnels)

	created, err := profiles.Create(domain.ProfileInput{
		Name:     "Project01 Production VPN",
		Provider: domain.ProviderMock,
		Group:    "Project01",
		Host:     "vpn.example",
		Port:     443,
	})
	if err != nil {
		log.Fatal(err)
	}

	// The VPN gets an account of its own because nobody chooses one for a
	// connection they did not click on: the router brings it up with whatever
	// was last used, or its default.
	if _, err := credentials.Create(created.ID, domain.CredentialInput{
		Label:      "Ming",
		Username:   "ming",
		AuthMethod: domain.AuthPassword,
		Password:   "not-a-real-password",
		IsDefault:  true,
	}); err != nil {
		log.Fatal(err)
	}

	// A real SSH server on loopback, standing in for the customer's bastion,
	// and a web server behind it standing in for something only reachable from
	// inside. The tunnel raised against them is genuine: a real handshake, real
	// channels, and the same port map Biebie Kube would read from a status.
	bastion, err := sshtest.Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = bastion.Close() }()

	behind := insideTheCustomerNetwork()
	defer func() { _ = behind.Close() }()

	tunnel, err := profiles.Create(domain.ProfileInput{
		Name:  "SMOI Bastion",
		Kind:  domain.KindSSH,
		Group: "SMOI",
		Host:  bastion.Host(),
		Port:  bastion.Port(),
		// The customer's bastion is inside the network the VPN opens, which is
		// the arrangement this tool exists to exercise: one connect below, two
		// connections up, in order.
		Requires: created.ID,
		SSH: domain.SSHSettings{
			User: "server1",
			Forwards: []domain.PortForward{
				{Name: "Kubernetes API", LocalPort: localPort(16443), RemoteHost: "127.0.0.1", RemotePort: portOf(behind)},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	account, err := credentials.Create(tunnel.ID, domain.CredentialInput{
		Label:      "server1",
		Username:   "server1",
		AuthMethod: domain.AuthPassword,
		Password:   "port-22-only",
		IsDefault:  true,
	})
	if err != nil {
		log.Fatal(err)
	}

	server, err := peer.New(peer.Options{
		Profiles: profiles,
		Sessions: sessions,
		Access:   router,
		Log:      logger,
		OnConnectRequested: func(profileID string) {
			fmt.Printf("CONNECT REQUESTED profile=%s\n", profileID)
		},
		OnError: func(err error) { fmt.Printf("ENDPOINT ERROR %v\n", err) },
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	// Announced on stdout so the checking side knows what to ask for.
	fmt.Printf("LISTENING %s\n", server.Address())
	fmt.Printf("PROFILE %s\n", created.ID)
	fmt.Printf("TUNNEL %s\n", tunnel.ID)

	// Only the tunnel is asked for. The VPN it sits behind is the router's
	// business, and it comes up first or nothing does.
	tunnels.SetPrompter(acceptAnyHost{})
	live, err := router.Connect(context.Background(), tunnel.ID, account.ID)
	if err != nil {
		log.Fatal(err)
	}
	defer tunnels.Shutdown()

	fmt.Printf("VPN %s\n", sessions.State(created.ID))
	for _, f := range live.Forwards {
		fmt.Printf("FORWARD %s -> %s\n", f.Local(), f.Remote())
	}
	fmt.Println("READY")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

// insideTheCustomerNetwork serves something only the tunnel can reach, so the
// forward can be proved end to end with a single curl at the local port.
func insideTheCustomerNetwork() net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "reached %s inside the customer network\n", r.Host)
	})}
	go func() { _ = server.Serve(listener) }()
	return listener
}

func portOf(listener net.Listener) int {
	addr, _ := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// localPort prefers the port a person would type, and gives way to whatever is
// already using it — a developer running this is quite likely to have a real
// tunnel on 16443 already, and the forwards are announced on stdout anyway.
func localPort(preferred int) int {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
	if err == nil {
		_ = listener.Close()
		return preferred
	}

	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = free.Close() }()
	return portOf(free)
}

// acceptAnyHost stands in for the confirmation dialog. The server it accepts
// was started by this process a moment ago.
type acceptAnyHost struct{}

func (acceptAnyHost) ConfirmHostKey(_ context.Context, req domain.HostKeyRequest) (bool, error) {
	fmt.Printf("HOST KEY %s %s\n", req.Host, req.Fingerprint)
	return true, nil
}

func (acceptAnyHost) Notify(event string, _ any) {
	fmt.Printf("EVENT %s\n", event)
}
