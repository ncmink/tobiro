// Package ivanti adapts the Ivanti Secure Access Client (formerly Pulse
// Secure).
//
// Ivanti's command-line surface differs sharply per platform: Linux ships
// pulsesvc, Windows ships pulselauncher.exe, and macOS ships only the bundled
// jamCommand helper whose argument surface Ivanti does not document for
// third-party automation. This adapter detects what is present and reports it
// honestly rather than assuming one platform's commands work on another.
package ivanti

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/startup"
)

var (
	darwinAppPaths = []string{
		"/Applications/Ivanti Secure Access.app",
		"/Applications/Pulse Secure.app",
	}
	// jamCommand is the helper bundled with the macOS client. It is detected
	// and reported, but not driven: Ivanti does not publish its arguments as a
	// supported integration point.
	darwinHelperSuffix = "/Contents/Plugins/JamUI/jamCommand"

	linuxCLIPaths = []string{
		"/usr/local/pulse/pulsesvc",
		"/opt/pulsesecure/bin/pulsesvc",
	}
	windowsCLIPaths = []string{
		`C:\Program Files (x86)\Common Files\Pulse Secure\Integration\pulselauncher.exe`,
		`C:\Program Files\Common Files\Pulse Secure\Integration\pulselauncher.exe`,
		`C:\Program Files (x86)\Common Files\Ivanti\Integration\pulselauncher.exe`,
	}
	windowsAppPaths = []string{
		`C:\Program Files (x86)\Common Files\Pulse Secure\JamUI\Pulse.exe`,
		`C:\Program Files\Common Files\Pulse Secure\JamUI\Pulse.exe`,
	}
)

// Provider is the Ivanti adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderIvanti }

// Detect locates the client and any platform CLI helper.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderIvanti,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	switch runtime.GOOS {
	case "darwin":
		path := sysexec.FirstExisting(darwinAppPaths...)
		if path == "" {
			info.Detail = "Ivanti Secure Access was not found in /Applications."
			break
		}
		info.Installed = true
		info.Path = path
		info.Version = sysexec.BundleVersion(ctx, path)
		if helper := path + darwinHelperSuffix; sysexec.Exists(helper) {
			info.CLIPath = helper
			info.Detail = "The bundled jamCommand helper is present, but Ivanti does not document it as a supported automation interface, so Tobiro uses the GUI."
		} else {
			info.Detail = "Ivanti publishes no supported macOS command line; the GUI is used."
		}

	case "linux":
		cli := sysexec.FirstExisting(linuxCLIPaths...)
		if cli == "" {
			cli = sysexec.LookPath("pulsesvc")
		}
		if cli != "" {
			info.Installed = true
			info.CLIPath = cli
			info.Path = cli
			info.Detail = "The Linux pulsesvc launcher was found. Command support is disabled until verified against this installation."
		} else {
			info.Detail = "The Ivanti/Pulse Linux client (pulsesvc) was not found."
		}

	case "windows":
		cli := sysexec.FirstExisting(windowsCLIPaths...)
		app := sysexec.FirstExisting(windowsAppPaths...)
		if cli != "" || app != "" {
			info.Installed = true
			info.CLIPath = cli
			info.Path = app
			if info.Path == "" {
				info.Path = cli
			}
			info.Detail = "The Windows client was found. Command support is disabled until verified against this installation."
		} else {
			info.Detail = "Ivanti Secure Access was not found in Program Files."
		}

	default:
		info.Detail = fmt.Sprintf("Ivanti does not ship a Secure Access client for %s.", runtime.GOOS)
	}

	return info, nil
}

// Capabilities reports what is safely usable on this platform.
//
// Programmatic connect stays disabled until the adapter has been verified
// against a specific installed client version. The manager turns this into an
// "open the native client" action instead of a failed guess.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	caps := &domain.Capabilities{
		Installed:        info.Installed,
		CLIAvailable:     info.CLIPath != "",
		CanConnect:       false,
		CanDisconnect:    false,
		CanReadStatus:    info.Installed && statusSupported(),
		SupportsPassword: false,
		SupportsOTP:      false,
		RequiresNativeUI: true,
	}

	if info.Installed {
		caps.CanControlStartup = runtime.GOOS == "darwin"
	}

	switch {
	case !info.Installed:
		caps.Notes = info.Detail
	case runtime.GOOS == "darwin":
		caps.CanOpenNativeClient = true
		caps.Notes = "Ivanti documents no supported macOS command line, so connecting happens in the Ivanti client. Tobiro watches the tunnel and shows its status here."
	default:
		caps.CanOpenNativeClient = info.Path != ""
		caps.Notes = "A command-line launcher was detected but is not driven yet. Tobiro opens the Ivanti client instead."
	}
	return caps, nil
}

// Connect fails safely until the platform CLI has been verified.
func (p *Provider) Connect(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderIvanti,
		Operation:        "connect",
		Platform:         runtime.GOOS,
		Reason:           info.Detail,
		NativeUIFallback: info.Installed,
	}
}

// Disconnect fails safely for the same reason as Connect.
func (p *Provider) Disconnect(ctx context.Context, _ domain.VPNProfile) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderIvanti,
		Operation:        "disconnect",
		Platform:         runtime.GOOS,
		Reason:           info.Detail,
		NativeUIFallback: info.Installed,
	}
}

// Status reports the live tunnel state by reading what the vendor client
// publishes, so a connection made in the Ivanti window still shows up here.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	return readStatus(ctx, profile), nil
}

// OpenNativeClient launches the Ivanti GUI.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		info, err := p.Detect(ctx)
		if err != nil {
			return err
		}
		if !info.Installed || info.Path == "" {
			return fmt.Errorf("%s: %w", domain.ProviderIvanti.DisplayName(), domain.ErrClientNotInstalled)
		}
		path = info.Path
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}

func (p *Provider) startupSpec() startup.Spec {
	return startup.Spec{
		Labels:         []string{"net.pulsesecure.pulseagent"},
		LoginItemNames: []string{"Ivanti Secure Access", "Pulse Secure"},
	}
}

// StartupAtLogin reports whether Ivanti opens when the user logs in.
func (p *Provider) StartupAtLogin(ctx context.Context) (domain.StartupState, error) {
	return startup.Read(ctx, p.startupSpec())
}

// SetStartupAtLogin enables or disables Ivanti opening at login.
func (p *Provider) SetStartupAtLogin(ctx context.Context, enabled bool) error {
	return startup.Set(ctx, p.startupSpec(), enabled)
}
