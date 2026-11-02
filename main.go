package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ncmink/biebie-protocol/deeplink"

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

const mainWindow = "main"

const singleInstanceID = "net.tobiro.access"

func main() {
	appCore, err := buildApp()
	if err != nil {
		log.Fatalf("Tobiro failed to start: %v", err)
	}

	app := application.New(application.Options{
		Name:        "Tobiro",
		Description: "Desktop access manager for DevOps engineers.",
		Services: []application.Service{
			application.NewService(appCore),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				presentMainWindow()
				for _, arg := range data.Args {
					if deeplink.Scheme(arg) == deeplink.SchemeAccess {
						appCore.handleDeepLink(arg)
						return
					}
				}
				presentMainWindow()
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	configureUpdater(app)
	appCore.updater = app.Updater

	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		if link := event.Context().URL(); link != "" {
			appCore.handleDeepLink(link)
		}
	})

	app.OnShutdown(func() {
		appCore.shutdown(context.Background())
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             mainWindow,
		Title:            "Tobiro",
		Width:            1180,
		Height:           780,
		MinWidth:         960,
		MinHeight:        640,
		URL:              "/",
		BackgroundColour: application.NewRGB(20, 21, 26),
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 44,
		},
	})

	if link := deepLinkFromArgs(os.Args[1:]); link != "" {
		appCore.handleDeepLink(link)
	}

	appMenu := buildApplicationMenu(appCore)
	app.Menu.SetApplicationMenu(appMenu)
	appMenu.Update()

	if err := app.Run(); err != nil {
		log.Fatalf("Tobiro exited with an error: %v", err)
	}
}

func deepLinkFromArgs(args []string) string {
	for _, arg := range args {
		if strings.EqualFold(deeplink.Scheme(arg), deeplink.SchemeAccess) {
			return arg
		}
	}
	return ""
}

func presentMainWindow() {
	wails := application.Get()
	if wails == nil {
		return
	}
	window, ok := wails.Window.GetByName(mainWindow)
	if !ok {
		return
	}
	window.Show()
	window.UnMinimise()
	window.Focus()
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

	tunnels := sshtunnel.NewManager(profiles, credentials, sessions, logger)
	tunnels.SetPrompter(bridge)

	router := access.NewRouter(profiles, sessions, credentials, vpnManager, tunnels)

	return NewApp(profiles, credentials, sessions, router, vpnManager, tunnels, registry, secrets, dataStore, logger, bridge), nil
}

func resolveSecretStore(logger *logging.Logger) security.SecureCredentialStore {
	osStore := keychain.New()
	if available, _ := osStore.Available(); available {
		return osStore
	}
	_, reason := osStore.Available()
	logger.Warn("Falling back to in-memory credential storage: %s", reason)
	return security.NewMemoryStore(reason)
}
