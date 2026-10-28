// Package surfshark adapts the official Surfshark client.
//
// Surfshark is a consumer VPN: the account is a subscription rather than a
// corporate gateway, and the server is chosen from Surfshark's own location
// list inside its app. A Tobiro "connection" here is therefore the
// machine's Surfshark tunnel — up or down — not a route to a named host.
//
// Surfshark publishes no automation interface on macOS or Windows: the desktop
// builds drive their tunnel through a NetworkExtension system extension (macOS)
// or a service (Windows), neither of which is documented for third-party use.
// This adapter consequently detects the client and opens its window rather than
// inventing arguments for it. Status is still read on macOS, because a
// NetworkExtension tunnel makes macOS itself the owner of the session state.
package surfshark

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
		"/Applications/Surfshark.app",
	}
	// Surfshark's Linux build ships a command line. It is detected for
	// diagnostics and left undriven until it has been verified, the same
	// treatment the FortiClient adapter gives fortivpn.
	linuxCLIPaths = []string{
		"/usr/bin/surfshark-vpn",
		"/usr/local/bin/surfshark-vpn",
	}
	windowsExePaths = []string{
		`C:\Program Files\Surfshark\Surfshark.exe`,
		`C:\Program Files (x86)\Surfshark\Surfshark.exe`,
	}
)

// Provider is the Surfshark adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderSurfshark }

// Detect locates the Surfshark client.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderSurfshark,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	switch runtime.GOOS {
	case "darwin":
		path := sysexec.FirstExisting(darwinAppPaths...)
		if path == "" {
			info.Detail = "Surfshark was not found in /Applications."
			break
		}
		info.Installed = true
		info.Path = path
		info.Version = sysexec.BundleVersion(ctx, path)
		info.Detail = "Surfshark was found. " + tunnelProviderDetail(ctx)

	case "linux":
		cli := sysexec.FirstExisting(linuxCLIPaths...)
		if cli == "" {
			cli = sysexec.LookPath("surfshark-vpn")
		}
		if cli == "" {
			info.Detail = "Surfshark for Linux (surfshark-vpn) was not found."
			break
		}
		info.Installed = true
		info.CLIPath = cli
		info.Path = cli
		info.Detail = "The Linux surfshark-vpn command line was found. Command support is disabled until verified against this installation."

	case "windows":
		path := sysexec.FirstExisting(windowsExePaths...)
		if path == "" {
			info.Detail = "Surfshark was not found in Program Files."
			break
		}
		info.Installed = true
		info.Path = path
		info.Detail = "Surfshark was found. Surfshark documents no connect command line for Windows, so its window is used."

	default:
		info.Detail = fmt.Sprintf("Surfshark is not available for %s.", runtime.GOOS)
	}

	return info, nil
}

// Capabilities reports GUI-only control with a readable tunnel state.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	caps := &domain.Capabilities{
		Installed:     info.Installed,
		CLIAvailable:  info.CLIPath != "",
		CanConnect:    false,
		CanDisconnect: false,
		CanReadStatus: info.Installed && statusSupported(),
		// The subscription lives in the Surfshark app, which stays signed in.
		// Tobiro has nothing to type and no code to relay.
		SupportsPassword: false,
		SupportsOTP:      false,
		RequiresNativeUI: true,
	}

	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}

	caps.CanOpenNativeClient = info.Path != ""
	caps.CanControlStartup = runtime.GOOS == "darwin"

	switch {
	case runtime.GOOS == "linux":
		caps.Notes = "Tobiro opens Surfshark so you can pick a location there. The surfshark-vpn command line is detected but not driven until it has been verified."
	case statusSupported():
		caps.Notes = "Tobiro opens Surfshark so you can pick a location there, then reports whether macOS has Surfshark's tunnel connected."
	default:
		caps.Notes = "Tobiro opens Surfshark so you can pick a location there. Surfshark documents no connect command line on this platform."
	}
	return caps, nil
}

// Connect is not available: Surfshark exposes no supported way to start its
// tunnel from another program, so the window is offered instead.
func (p *Provider) Connect(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return p.unsupported(ctx, "connect")
}

// Disconnect is unavailable for the same reason as Connect.
func (p *Provider) Disconnect(ctx context.Context, _ domain.VPNProfile) error {
	return p.unsupported(ctx, "disconnect")
}

func (p *Provider) unsupported(ctx context.Context, operation string) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	reason := info.Detail
	if info.Installed && runtime.GOOS != "linux" {
		reason = "Surfshark publishes no command line for its desktop client, so the tunnel is started and stopped in the Surfshark window."
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderSurfshark,
		Operation:        operation,
		Platform:         runtime.GOOS,
		Reason:           reason,
		NativeUIFallback: info.Installed,
	}
}

// Status reports whether Surfshark's tunnel is currently carrying traffic.
//
// The profile is ignored: a Surfshark connection has no gateway of its own, so
// there is nothing on the profile to match against.
func (p *Provider) Status(ctx context.Context, _ domain.VPNProfile) (*domain.VPNStatus, error) {
	if !statusSupported() {
		return &domain.VPNStatus{
			Known:  false,
			Detail: "Tunnel status is not read on this platform; check the Surfshark window.",
		}, nil
	}

	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}
	if !info.Installed {
		return &domain.VPNStatus{Known: false, Detail: info.Detail}, nil
	}
	return readStatus(ctx), nil
}

// OpenNativeClient launches the Surfshark window.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		info, err := p.Detect(ctx)
		if err != nil {
			return err
		}
		if !info.Installed || info.Path == "" {
			return fmt.Errorf("%s: %w", domain.ProviderSurfshark.DisplayName(), domain.ErrClientNotInstalled)
		}
		path = info.Path
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}

// startupSpec covers both the direct-download and App Store bundle identifiers,
// since only one of them is present on any given machine.
func (p *Provider) startupSpec() startup.Spec {
	return startup.Spec{
		Labels: []string{
			"com.surfshark.vpnclient.macos.direct.launchAgent",
			"com.surfshark.vpnclient.macos.launchAgent",
		},
		LoginItemNames: []string{"Surfshark"},
		AppPath:        sysexec.FirstExisting(darwinAppPaths...),
	}
}

// StartupAtLogin reports whether Surfshark opens when the user logs in.
func (p *Provider) StartupAtLogin(ctx context.Context) (domain.StartupState, error) {
	return startup.Read(ctx, p.startupSpec())
}

// SetStartupAtLogin enables or disables Surfshark opening at login.
//
// This only touches the login agent. Surfshark's tunnel system extension is
// left alone, so the tunnel still comes up when the user asks for it.
func (p *Provider) SetStartupAtLogin(ctx context.Context, enabled bool) error {
	return startup.Set(ctx, p.startupSpec(), enabled)
}
