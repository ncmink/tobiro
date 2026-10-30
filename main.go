package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"tobiro/internal/access"
	"tobiro/internal/credential"
	"tobiro/internal/logging"
	"tobiro/internal/profile"
	"tobiro/internal/security"
	"tobiro/internal/security/keychain"
	"tobiro/internal/session"
	"tobiro/internal/sshtunnel"
	"tobiro/internal/store"
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/forcepoint"
	"tobiro/internal/vpn/forticlient"
	"tobiro/internal/vpn/ivanti"
	"tobiro/internal/vpn/macosvpn"
	"tobiro/internal/vpn/surfshark"
	"tobiro/internal/vpn/tailscale"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app, err := buildApp()
	if err != nil {
		log.Fatalf("Tobiro failed to start: %v", err)
	}

	err = wails.Run(&options.App{
		Title:            "Tobiro",
		Width:            1180,
		Height:           780,
		MinWidth:         960,
		MinHeight:        640,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 20, G: 21, B: 26, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Tobiro",
				Icon:    appIcon,
				Message: "Desktop access manager for DevOps engineers.\nPart of the biebie.net product family.",
			},
			// macOS hands a tobiro:// URL to the instance that is already
			// running, so this is how a click in Biebie Kube arrives.
			OnUrlOpen: app.handleDeepLink,
		},
		// Elsewhere a deep link is delivered by launching the application again
		// with the URL in its arguments. One instance must win, both so the URL
		// reaches the window the user is looking at and so two processes do not
		// contend for the same local endpoint.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "net.tobiro.access",
			OnSecondInstanceLaunch: app.handleSecondInstance,
		},
		Menu: applicationMenu(app),
		Bind: []interface{}{app},
	})
	if err != nil {
		log.Fatalf("Tobiro exited with an error: %v", err)
	}
}

// buildApp is the composition root: it wires storage, security, services and
// provider adapters, then hands the assembled use-case surface to Wails.
func buildApp() (*App, error) {
	logger := logging.New()

	dataPath, err := store.DefaultPath()
	if err != nil {
		return nil, err
	}
	dataStore, err := store.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("open application data: %w", err)
	}

	secrets := resolveSecretStore(logger)

	bridge := newUIBridge()
	sessions := session.NewManager()
	bridge.wire(logger, sessions)

	profiles := profile.NewService(dataStore)
	credentials := credential.NewService(dataStore, secrets, logger)

	// Adapters register themselves here. Adding a vendor (Cisco Secure Client,
	// GlobalProtect, OpenVPN, WireGuard, AWS Client VPN) means writing an
	// adapter and adding one line — the manager stays vendor-agnostic.
	registry := vpn.NewRegistry()
	registry.Register(forcepoint.New())
	registry.Register(ivanti.New())
	registry.Register(forticlient.New())
	registry.Register(tailscale.New())
	registry.Register(surfshark.New())
	registry.Register(macosvpn.New())
	registerDevelopmentProviders(registry)

	vpnManager := vpn.NewManager(registry, profiles, credentials, sessions, logger)
	vpnManager.SetPrompter(bridge)

	// SSH tunnels are a peer of the vendor clients, not one of them: there is
	// no adapter to register because there is no installed client to drive.
	tunnels := sshtunnel.NewManager(profiles, credentials, sessions, logger)
	tunnels.SetPrompter(bridge)

	router := access.NewRouter(profiles, sessions, credentials, vpnManager, tunnels)

	return NewApp(profiles, credentials, sessions, router, vpnManager, tunnels, registry, secrets, dataStore, logger, bridge), nil
}

// resolveSecretStore prefers the OS credential store and degrades to an
// in-memory store when it is unavailable (headless Linux, CI). Either way, a
// password is never written to application data.
func resolveSecretStore(logger *logging.Logger) security.SecureCredentialStore {
	osStore := keychain.New()
	if available, _ := osStore.Available(); available {
		return osStore
	}
	_, reason := osStore.Available()
	logger.Warn("Falling back to in-memory credential storage: %s", reason)
	return security.NewMemoryStore(reason)
}
