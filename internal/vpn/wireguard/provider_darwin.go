//go:build darwin

package wireguard

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/nc"
)

const (
	connectPollInterval = 500 * time.Millisecond
	connectTimeout      = 30 * time.Second
)

var (
	wgCLIPaths = []string{
		"/opt/homebrew/bin/wg",
		"/opt/homebrew/bin/wg-quick",
		"/usr/local/bin/wg",
		"/usr/local/bin/wg-quick",
	}
)

// Detect locates the WireGuard app and any tunnels macOS knows about.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderWireGuard,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	if !sysexec.Exists(wireGuardAppPath) {
		info.Detail = "WireGuard was not found in /Applications."
		return info, nil
	}
	info.Installed = true
	info.Path = wireGuardAppPath
	info.Version = sysexec.BundleVersion(ctx, wireGuardAppPath)

	if cli := sysexec.FirstExisting(wgCLIPaths...); cli != "" {
		if info.CLIPath == "" {
			info.CLIPath = cli
		}
		info.Detail = tunnelDetail(ctx) + " The wg/wg-quick CLI was also found; it needs root and is not driven from Tobiro."
	} else {
		info.Detail = tunnelDetail(ctx)
	}

	if nc.Supported() {
		services, err := nc.List(ctx)
		if err == nil {
			tunnels := ownedByWireGuard(services)
			switch len(tunnels) {
			case 0:
				info.Detail += " No WireGuard tunnels are registered with macOS yet."
			case 1:
				info.Detail += " macOS sees one tunnel: " + tunnels[0].Name + "."
			default:
				info.Detail += fmt.Sprintf(" macOS sees %d tunnels: %s.", len(tunnels), strings.Join(tunnelNames(tunnels), ", "))
			}
		}
	}
	return info, nil
}

// Capabilities reports connect, disconnect and status through scutil --nc.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	caps := &domain.Capabilities{
		Installed:        info.Installed,
		CLIAvailable:     info.CLIPath != "",
		SupportsPassword: false,
		SupportsOTP:      false,
		RequiresNativeUI: false,
	}
	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}

	ncOK := nc.Supported()
	caps.CanReadStatus = ncOK
	caps.CanConnect = ncOK
	caps.CanDisconnect = ncOK
	caps.CanOpenNativeClient = true
	caps.NativeClientLabel = "Open WireGuard"
	caps.Notes = "Tobiro starts and stops tunnels the WireGuard app registered with macOS. Keys and peer settings stay in WireGuard."
	if info.CLIPath != "" {
		caps.Notes += " The wg-quick CLI is detected for diagnostics only — it needs root."
	}
	return caps, nil
}

// ListVendorProfiles returns tunnels registered by the WireGuard app.
func (p *Provider) ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error) {
	if !nc.Supported() {
		return nil, fmt.Errorf("macOS network connections are not available")
	}
	services, err := nc.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.VendorProfileOption
	for _, service := range ownedByWireGuard(services) {
		out = append(out, domain.VendorProfileOption{
			Name:      service.Name,
			Detail:    "WireGuard tunnel",
			State:     strings.ToLower(service.State),
			Connected: service.Connected,
		})
	}
	return out, nil
}

func (p *Provider) resolve(ctx context.Context, profile domain.VPNProfile) (nc.Service, error) {
	if !nc.Supported() {
		return nc.Service{}, &domain.UnsupportedOperationError{
			Provider:  domain.ProviderWireGuard,
			Operation: "resolve",
			Platform:  runtime.GOOS,
			Reason:    "WireGuard tunnels are only driven through macOS network connections",
		}
	}

	wanted := strings.TrimSpace(profile.VendorProfile)
	if wanted == "" {
		return nc.Service{}, fmt.Errorf("set %s on this connection to a tunnel name from the WireGuard app",
			domain.ProviderWireGuard.VendorProfileField().Label)
	}

	services, err := nc.List(ctx)
	if err != nil {
		return nc.Service{}, err
	}
	tunnels := ownedByWireGuard(services)

	service, found := nc.FindByName(tunnels, wanted)
	if !found {
		return nc.Service{}, fmt.Errorf("macOS has no WireGuard tunnel named %q. %s", wanted, offerTunnels(tunnels))
	}
	return service, nil
}

// Connect asks macOS to bring the named WireGuard tunnel up.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, _ domain.Credential) error {
	service, err := p.resolve(ctx, profile)
	if err != nil {
		return err
	}

	reading, err := nc.Status(ctx, service.ID)
	if err == nil && reading.Connected {
		return nil
	}

	if err := nc.Start(ctx, service.ID); err != nil {
		return fmt.Errorf("start WireGuard tunnel %q: %w", service.Name, err)
	}

	deadline := time.Now().Add(connectTimeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reading, err := nc.Status(ctx, service.ID)
		if err == nil && reading.Connected {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("WireGuard tunnel %q did not connect within %s", service.Name, connectTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(connectPollInterval):
		}
	}
}

// Disconnect asks macOS to tear the tunnel down.
//
// With On-Demand on, macOS restarts the tunnel on the next matching packet, so
// a stop would bounce straight back. The WireGuard app turns On-Demand off when
// it deactivates a tunnel; scutil cannot, so Tobiro refuses and points there.
func (p *Provider) Disconnect(ctx context.Context, profile domain.VPNProfile) error {
	service, err := p.resolve(ctx, profile)
	if err != nil {
		return err
	}
	if onDemand, err := nc.OnDemandEnabled(ctx, service.ID); err == nil && onDemand {
		return &domain.UnsupportedOperationError{
			Provider:  domain.ProviderWireGuard,
			Operation: "disconnect",
			Platform:  runtime.GOOS,
			Reason: "On-Demand is enabled for " + service.Name + ", so macOS reconnects it as soon as traffic flows. " +
				"Deactivate it in the WireGuard app (which also turns On-Demand off), or disable On-Demand there first",
			NativeUIFallback: true,
		}
	}
	return nc.Stop(ctx, service.ID)
}

// Status reads the tunnel's live state from macOS.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	service, err := p.resolve(ctx, profile)
	if err != nil {
		return &domain.VPNStatus{Known: false, Detail: err.Error()}, nil
	}

	reading, err := nc.Status(ctx, service.ID)
	if err != nil {
		return &domain.VPNStatus{Known: false, Detail: err.Error()}, nil
	}
	if !reading.Connected {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: service.Name + " is " + strings.ToLower(reading.State) + ".",
		}, nil
	}

	status := &domain.VPNStatus{
		State:      domain.StateConnected,
		AssignedIP: reading.AssignedIP,
		Interface:  reading.Interface,
		Known:      true,
		Detail:     service.Name + " is connected.",
	}
	if reading.Interface != "" && reading.AssignedIP != "" {
		status.Detail = "Tunnel up on " + reading.Interface + " with address " + reading.AssignedIP + "."
	}
	return status, nil
}

// OpenNativeClient launches the WireGuard app.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		path = wireGuardAppPath
	}
	if !sysexec.Exists(path) {
		return fmt.Errorf("%s: %w", domain.ProviderWireGuard.DisplayName(), domain.ErrClientNotInstalled)
	}
	return sysexec.OpenApplication(ctx, path)
}

func tunnelDetail(ctx context.Context) string {
	const fallback = "Tunnels are imported and edited in the WireGuard app."

	if !sysexec.Exists("/usr/bin/systemextensionsctl") {
		return fallback
	}
	res, err := sysexec.Run(ctx, "/usr/bin/systemextensionsctl", "list")
	if err != nil && res.Stdout == "" {
		return fallback
	}

	found := false
	for _, line := range strings.Split(res.Stdout, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "wireguard") {
			continue
		}
		found = true
		if strings.Contains(lower, "[activated enabled]") {
			return "The WireGuard tunnel extension is approved. Tunnels are imported in the WireGuard app."
		}
	}
	if found {
		return "The WireGuard tunnel extension is not approved yet — allow it under System Settings › General › Login Items & Extensions › Network Extensions."
	}
	return fallback
}
