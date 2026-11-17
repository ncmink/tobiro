// Package openvpn adapts the official OpenVPN Connect client.
//
// OpenVPN Connect 3.3+ documents --list-profiles and related management flags,
// but not connect or disconnect. Tobiro opens the GUI and reads tunnel state
// from the client's own log and network state.
package openvpn

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
)

// Provider is the OpenVPN Connect adapter.
type Provider struct {
	appPath string
	cliPath string
}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderOpenVPN }

func (p *Provider) detectPaths(ctx context.Context) {
	if p.cliPath != "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		for _, app := range platformAppPaths() {
			if !sysexec.Exists(app) {
				continue
			}
			cli := cliFromApp(app)
			if sysexec.Exists(cli) {
				p.appPath = app
				p.cliPath = cli
				return
			}
		}
	case "windows":
		if exe := sysexec.FindInProgramFiles([]string{"OpenVPN Connect"}, "OpenVPN Connect.exe"); exe != "" {
			p.appPath = exe
			p.cliPath = exe
		}
	}
	_ = ctx
}

// Detect locates OpenVPN Connect and its documented CLI entry point.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	p.detectPaths(ctx)
	info := &domain.ClientInfo{
		Provider:   domain.ProviderOpenVPN,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}
	if p.cliPath == "" {
		info.Detail = "OpenVPN Connect was not found on this machine."
		return info, nil
	}
	info.Installed = true
	info.Path = p.appPath
	info.CLIPath = p.cliPath
	if runtime.GOOS == "darwin" {
		info.Version = sysexec.BundleVersion(ctx, p.appPath)
	}
	res, err := sysexec.Run(ctx, p.cliPath, "--version")
	if err == nil && res.Stdout != "" {
		info.Detail = "OpenVPN Connect was found. Connect and disconnect use the OpenVPN Connect window; Tobiro lists profiles and reports status."
	} else {
		info.Detail = "OpenVPN Connect was found. Tobiro opens it and reads tunnel status."
	}
	return info, nil
}

// Capabilities reports GUI-first control with readable status on macOS and Windows.
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
	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}
	caps.CanOpenNativeClient = info.Path != ""
	caps.Notes = "OpenVPN Connect does not document a connect command line, so you connect in its window. " +
		"Pick the profile on this card; Tobiro reports when that profile is up."
	return caps, nil
}

// Connect fails safely until OpenVPN documents a supported connect command.
func (p *Provider) Connect(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	info, err := p.Detect(ctx)
	if err != nil {
		return err
	}
	return &domain.UnsupportedOperationError{
		Provider:         domain.ProviderOpenVPN,
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
		Provider:         domain.ProviderOpenVPN,
		Operation:        "disconnect",
		Platform:         runtime.GOOS,
		Reason:           info.Detail,
		NativeUIFallback: info.Installed,
	}
}

// Status reports whether the profile bound to this card is connected.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	return readStatus(ctx, p, profile), nil
}

// ListVendorProfiles offers profiles imported into OpenVPN Connect.
func (p *Provider) ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error) {
	p.detectPaths(ctx)
	if p.cliPath == "" {
		return nil, fmt.Errorf("%s: %w", domain.ProviderOpenVPN.DisplayName(), domain.ErrClientNotInstalled)
	}
	return vendorOptions(ctx, p.cliPath, platformLogPath())
}

// OpenNativeClient launches OpenVPN Connect.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	rememberOpened(profile)
	p.detectPaths(ctx)
	path := profile.ClientPath
	if path == "" {
		path = p.appPath
	}
	if path == "" {
		info, err := p.Detect(ctx)
		if err != nil {
			return err
		}
		if !info.Installed || info.Path == "" {
			return fmt.Errorf("%s: %w", domain.ProviderOpenVPN.DisplayName(), domain.ErrClientNotInstalled)
		}
		path = info.Path
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}