// Package forcepoint adapts the Forcepoint NGFW VPN Client.
//
// Forcepoint does not publish a command-line interface for establishing or
// tearing down tunnels on any desktop platform. This adapter therefore detects
// the client and reports RequiresNativeUI, so Tobiro launches the vendor
// GUI instead of guessing at undocumented arguments.
package forcepoint

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/startup"
	"tobiro/internal/vpn/tunnel"
)

// macOS and Windows install locations, newest naming first.
var (
	darwinAppPaths = []string{
		"/Applications/Forcepoint NGFW VPN Client.app",
		"/Applications/Forcepoint VPN Client.app",
		"/Applications/Stonesoft VPN Client.app",
	}
	windowsExePaths = []string{
		`C:\Program Files\Forcepoint\VPN Client\Sgvpn.exe`,
		`C:\Program Files (x86)\Forcepoint\VPN Client\Sgvpn.exe`,
		`C:\Program Files\Stonesoft\VPN Client\Sgvpn.exe`,
		`C:\Program Files (x86)\Stonesoft\VPN Client\Sgvpn.exe`,
	}
)

// Provider is the Forcepoint adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderForcepoint }

// Detect locates the installed client and reads its version.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderForcepoint,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	switch runtime.GOOS {
	case "darwin":
		if path := sysexec.FirstExisting(darwinAppPaths...); path != "" {
			info.Installed = true
			info.Path = path
			info.Version = sysexec.BundleVersion(ctx, path)
			info.Detail = "Forcepoint publishes no macOS command line for connecting; the GUI is used."
		} else {
			info.Detail = "Forcepoint NGFW VPN Client was not found in /Applications."
		}
	case "windows":
		if path := sysexec.FirstExisting(windowsExePaths...); path != "" {
			info.Installed = true
			info.Path = path
			info.Detail = "Forcepoint publishes no supported command line for connecting; the GUI is used."
		} else {
			info.Detail = "Forcepoint NGFW VPN Client was not found in Program Files."
		}
	default:
		info.Detail = fmt.Sprintf("Forcepoint does not ship an NGFW VPN Client for %s.", runtime.GOOS)
	}

	return info, nil
}

// Capabilities reports GUI-only control.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	caps := &domain.Capabilities{
		Installed:        info.Installed,
		CLIAvailable:     false,
		CanConnect:       false,
		CanDisconnect:    false,
		CanReadStatus:    info.Installed && tunnel.Supported(),
		SupportsPassword: info.Installed,
		SupportsOTP:      false,
		RequiresNativeUI: true,
	}
	if info.Installed {
		caps.CanOpenNativeClient = true
		caps.CanControlStartup = runtime.GOOS == "darwin"
		if runtime.GOOS == "darwin" {
			caps.CanFillNativeLogin = true
			caps.Notes = "Open the Forcepoint login window, then press your fill shortcut (default ⌘I). Tobiro fills username and password; you click Authenticate. Needs Accessibility permission for Tobiro."
		} else {
			caps.Notes = "Forcepoint documents no CLI for connecting, so logging in happens in the Forcepoint client. Tobiro watches the tunnel and shows its status here."
		}
	} else {
		caps.Notes = info.Detail
	}
	return caps, nil
}

// Connect is not available: Forcepoint documents no CLI for it. Rather than
// guessing, the adapter fails safely and points at the GUI fallback.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, _ domain.Credential) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderForcepoint,
		Operation:        "connect",
		Platform:         runtime.GOOS,
		Reason:           "the Forcepoint NGFW VPN Client exposes no documented command-line login",
		NativeUIFallback: info.Installed,
	}
}

// Disconnect is not available for the same reason as Connect.
func (p *Provider) Disconnect(ctx context.Context, _ domain.VPNProfile) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderForcepoint,
		Operation:        "disconnect",
		Platform:         runtime.GOOS,
		Reason:           "the Forcepoint NGFW VPN Client exposes no documented command-line control",
		NativeUIFallback: info.Installed,
	}
}

// Status reports the live tunnel state.
//
// Forcepoint exposes nothing to third-party tools, so this reads the machine's
// own network state instead: the client's control connection to the gateway
// identifies which profile is up, and the tunnel interface supplies the
// assigned address. A connection made in the Forcepoint window therefore still
// shows up here.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	if !tunnel.Supported() {
		return &domain.VPNStatus{
			Known:  false,
			Detail: "Tunnel status is not read on this platform; check the Forcepoint client window.",
		}, nil
	}

	hints := []string{"Forcepoint", "Stonesoft", "sgvpn"}
	reading := tunnel.ProbeWith(ctx, tunnel.Options{
		Host:         profile.Host,
		Port:         profile.Port,
		ProcessHints: hints,
	})
	if reading.Connected {
		return tunnel.Status(reading), nil
	}

	// The host on the profile is frequently the thing the VPN exists to
	// reach — an SSH bastion, an API server — not Forcepoint's own
	// concentrator. The client still holds one control socket to that
	// concentrator; if the socket is ours, the tunnel is up.
	if vendor := tunnel.ProbeVendor(ctx, hints); vendor.Connected {
		return tunnel.Status(vendor), nil
	}
	return tunnel.Status(reading), nil
}

// OpenNativeClient launches the Forcepoint GUI.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		info, err := p.Detect(ctx)
		if err != nil {
			return err
		}
		if !info.Installed {
			return fmt.Errorf("%s: %w", domain.ProviderForcepoint.DisplayName(), domain.ErrClientNotInstalled)
		}
		path = info.Path
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}

func (p *Provider) startupSpec(ctx context.Context) startup.Spec {
	spec := startup.Spec{
		LoginItemNames: []string{
			"Forcepoint NGFW VPN Client",
			"Forcepoint VPN Client",
			"Stonesoft VPN Client",
		},
	}
	if info, err := p.Detect(ctx); err == nil && info != nil {
		spec.AppPath = info.Path
	}
	return spec
}

// StartupAtLogin reports whether Forcepoint opens when the user logs in.
func (p *Provider) StartupAtLogin(ctx context.Context) (domain.StartupState, error) {
	return startup.Read(ctx, p.startupSpec(ctx))
}

// SetStartupAtLogin enables or disables Forcepoint opening at login.
func (p *Provider) SetStartupAtLogin(ctx context.Context, enabled bool) error {
	return startup.Set(ctx, p.startupSpec(ctx), enabled)
}
