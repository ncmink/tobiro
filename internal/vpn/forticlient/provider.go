// Package forticlient adapts FortiClient.
//
// Fortinet documents a VPN command line only for the Linux build
// (/opt/forticlient/fortivpn). The macOS and Windows builds ship internal
// helper binaries that Fortinet does not publish as an automation interface, so
// this adapter detects them for diagnostics but does not drive them.
//
// Where a CLI is used at all, FortiClient connects by tunnel name — the tunnel
// must already exist in FortiClient. That is expressed as a capability rather
// than worked around.
//
// On macOS the password field rejects paste. FillNativeLogin types a stored
// account into the FortiClient window instead, and never clicks Connect.
package forticlient

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/tunnel"
)

var (
	darwinAppPaths = []string{
		"/Applications/FortiClient.app",
	}
	// The macOS build keeps its helpers here. They are undocumented for
	// third-party use and are reported for diagnostics only.
	darwinSupportBin = "/Library/Application Support/Fortinet/FortiClient/bin"

	linuxCLIPaths = []string{
		"/opt/forticlient/fortivpn",
		"/usr/bin/fortivpn",
	}
	windowsExePaths = []string{
		`C:\Program Files\Fortinet\FortiClient\FortiClient.exe`,
		`C:\Program Files (x86)\Fortinet\FortiClient\FortiClient.exe`,
	}
	windowsCLIPaths = []string{
		`C:\Program Files\Fortinet\FortiClient\FortiClient.exe`,
		`C:\Program Files (x86)\Fortinet\FortiClient\FortiClient.exe`,
	}
)

// Provider is the FortiClient adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderFortiClient }

// Detect locates FortiClient and any CLI.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderFortiClient,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	switch runtime.GOOS {
	case "darwin":
		path := sysexec.FirstExisting(darwinAppPaths...)
		if path == "" {
			info.Detail = "FortiClient was not found in /Applications."
			break
		}
		info.Installed = true
		info.Path = path
		info.Version = sysexec.BundleVersion(ctx, path)
		if sysexec.Exists(darwinSupportBin) {
			info.Detail = "FortiClient's macOS helper binaries are present but undocumented for automation; Tobiro uses the GUI."
		} else {
			info.Detail = "Fortinet publishes no macOS VPN command line; the GUI is used."
		}

	case "linux":
		cli := sysexec.FirstExisting(linuxCLIPaths...)
		if cli == "" {
			cli = sysexec.LookPath("fortivpn")
		}
		if cli != "" {
			info.Installed = true
			info.CLIPath = cli
			info.Path = cli
			info.Detail = "The Linux fortivpn command line was found. Command support is disabled until verified against this installation."
		} else {
			info.Detail = "FortiClient for Linux (fortivpn) was not found."
		}

	case "windows":
		app := sysexec.FirstExisting(windowsExePaths...)
		if app != "" {
			info.Installed = true
			info.Path = app
			info.CLIPath = sysexec.FirstExisting(windowsCLIPaths...)
			info.Detail = "FortiClient was found. Fortinet does not document a supported connect command line for Windows; the GUI is used."
		} else {
			info.Detail = "FortiClient was not found in Program Files."
		}

	default:
		info.Detail = fmt.Sprintf("FortiClient is not available for %s.", runtime.GOOS)
	}

	return info, nil
}

// Capabilities reports what is safely usable on this platform.
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
		CanReadStatus:    info.Installed && tunnel.Supported(),
		SupportsPassword: runtime.GOOS == "darwin",
		SupportsOTP:      false,
		RequiresNativeUI: true,
	}

	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}

	caps.CanOpenNativeClient = info.Path != ""
	if caps.CanOpenNativeClient {
		caps.NativeClientLabel = "Open FortiClient"
	}
	if runtime.GOOS == "darwin" && caps.CanOpenNativeClient {
		caps.CanFillNativeLogin = true
	}
	switch runtime.GOOS {
	case "linux":
		caps.Notes = "The fortivpn command line connects by tunnel name, so the tunnel must already exist in FortiClient. Command support is disabled until verified."
	case "darwin":
		caps.Notes = "Open the FortiClient login window, then press ⌘I. Tobiro fills username and password; you click Connect in FortiClient. Needs Accessibility permission for Tobiro."
	default:
		caps.Notes = "Tobiro opens FortiClient so you can complete the login there. Fortinet documents no supported connect command line on this platform."
	}
	return caps, nil
}

// Connect fails safely until the platform CLI has been verified.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, _ domain.Credential) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	reason := info.Detail
	if runtime.GOOS == "linux" && profile.VendorProfile == "" {
		reason = "fortivpn connects by tunnel name — set Vendor Profile to the tunnel configured in FortiClient."
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderFortiClient,
		Operation:        "connect",
		Platform:         runtime.GOOS,
		Reason:           reason,
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
		Provider:         domain.ProviderFortiClient,
		Operation:        "disconnect",
		Platform:         runtime.GOOS,
		Reason:           info.Detail,
		NativeUIFallback: info.Installed,
	}
}

// Status reports the live tunnel state from the machine's own network state,
// so a tunnel raised in the FortiClient window still shows up here.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	if !tunnel.Supported() {
		return &domain.VPNStatus{
			Known:  false,
			Detail: "Tunnel status is not read on this platform; check the FortiClient window.",
		}, nil
	}

	reading := tunnel.ProbeWith(ctx, tunnel.Options{
		Host:         profile.Host,
		Port:         profile.Port,
		ProcessHints: []string{"FortiClient", "fortivpn", "fct_"},
	})
	return tunnel.Status(reading), nil
}

// OpenNativeClient launches the FortiClient GUI.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		info, err := p.Detect(ctx)
		if err != nil {
			return err
		}
		if !info.Installed || info.Path == "" {
			return fmt.Errorf("%s: %w", domain.ProviderFortiClient.DisplayName(), domain.ErrClientNotInstalled)
		}
		path = info.Path
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}
