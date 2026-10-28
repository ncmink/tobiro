// Package macosvpn adapts the VPN services configured in macOS System Settings:
// L2TP over IPsec, IKEv2 and Cisco IPsec.
//
// macOS publishes `scutil --nc list|status|start|stop`, which is more than any
// vendor client here documents. Three of those four are used. Connect is not,
// because the shared secret it would need is held by macOS in the System
// keychain and is not readable by an app — and this adapter does not ask the
// user to hand that secret over so it can pass it back. See Connect.
//
// So the division is by what needs a secret: connecting stays in System
// Settings, where the secret already is, while reporting the tunnel and
// stopping it — neither of which needs one — are driven from here.
//
// A profile names an existing service. Tobiro never creates or edits one:
// that means writing privileged system network preferences, which is exactly the
// kind of guessing this codebase refuses to do.
package macosvpn

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

// settingsPane is the System Settings page where these services are managed.
const settingsPane = "x-apple.systempreferences:com.apple.Network-Settings.extension"

// Provider is the macOS VPN adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderMacOSVPN }

// Detect reports how many VPN services macOS is managing.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderMacOSVPN,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	if !nc.Supported() {
		info.Detail = fmt.Sprintf("macOS VPN services are not available on %s.", runtime.GOOS)
		return info, nil
	}

	info.Installed = true
	info.Path = "/usr/sbin/scutil"
	info.CLIPath = "/usr/sbin/scutil"
	info.Version = productVersion(ctx)

	services, err := nc.List(ctx)
	if err != nil {
		info.Detail = err.Error()
		return info, nil
	}
	names := nc.SystemManagedNames(services)
	switch len(names) {
	case 0:
		info.Detail = "No VPN services are configured yet. Add one in System Settings › Network › VPN."
	case 1:
		info.Detail = "macOS is managing one VPN service: " + names[0] + "."
	default:
		info.Detail = fmt.Sprintf("macOS is managing %d VPN services: %s.", len(names), strings.Join(names, ", "))
	}
	return info, nil
}

// ListVendorProfiles offers the services macOS already has configured, so the
// connection form can present them rather than ask for a name the user would
// have to read out of System Settings first.
//
// Services owned by third-party apps are left out: macOS cannot start those, and
// Surfshark and Tailscale have their own adapters here.
func (p *Provider) ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error) {
	services, err := nc.List(ctx)
	if err != nil {
		return nil, err
	}

	var out []domain.VendorProfileOption
	for _, service := range services {
		if !service.SystemManaged() {
			continue
		}
		out = append(out, domain.VendorProfileOption{
			Name:      service.Name,
			Detail:    describeKind(service.Kind),
			State:     strings.ToLower(service.State),
			Connected: service.Connected,
		})
	}
	return out, nil
}

// describeKind names the protocol behind a scutil service type.
//
// "IPSec" covers both IKEv2 and Cisco IPsec and macOS does not distinguish them
// here, so both are named rather than one being guessed at.
func describeKind(kind string) string {
	switch kind {
	case "PPP:L2TP":
		return "L2TP over IPsec"
	case "PPP:PPTP":
		return "PPTP"
	case "IPSec":
		return "IKEv2 or Cisco IPsec"
	default:
		return kind
	}
}

func productVersion(ctx context.Context) string {
	res, err := sysexec.Run(ctx, "/usr/bin/sw_vers", "-productVersion")
	if err != nil {
		return ""
	}
	if v := strings.TrimSpace(res.Stdout); v != "" {
		return "macOS " + v
	}
	return ""
}

// Capabilities reports full control, which no vendor client here can offer.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	caps := &domain.Capabilities{
		Installed:    info.Installed,
		CLIAvailable: info.CLIPath != "",
		// The password lives in the Keychain that macOS reads on connect, so
		// there is nothing for Tobiro to store or type.
		SupportsPassword: false,
		SupportsOTP:      false,
		RequiresNativeUI: false,
	}
	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}

	// Disconnect and status need no secret, so those are driven directly. Only
	// connecting is blocked, and it is blocked for everyone: see Connect.
	caps.CanConnect = false
	caps.CanDisconnect = true
	caps.CanReadStatus = true
	caps.CanOpenNativeClient = true
	caps.NativeClientLabel = "Open Network settings"
	caps.RequiresNativeUI = true
	caps.Notes = "Connect the VPN once in System Settings — macOS keeps the IPSec shared secret in the System keychain and will not hand it to an app. Tobiro then reports the tunnel, and can disconnect it."
	return caps, nil
}

// resolve finds the macOS service a profile names.
//
// The error lists what the machine actually offers, because the alternative is
// the user guessing at a name they cannot see from this window.
func (p *Provider) resolve(ctx context.Context, profile domain.VPNProfile) (nc.Service, error) {
	if !nc.Supported() {
		return nc.Service{}, &domain.UnsupportedOperationError{
			Provider:  domain.ProviderMacOSVPN,
			Operation: "resolve",
			Platform:  runtime.GOOS,
			Reason:    "macOS VPN services are only available on macOS",
		}
	}

	wanted := strings.TrimSpace(profile.VendorProfile)
	if wanted == "" {
		return nc.Service{}, fmt.Errorf("set %s on this connection to the service you configured in System Settings",
			domain.ProviderMacOSVPN.VendorProfileField().Label)
	}

	services, err := nc.List(ctx)
	if err != nil {
		return nc.Service{}, err
	}

	service, found := nc.FindByName(services, wanted)
	if !found {
		return nc.Service{}, fmt.Errorf("macOS has no VPN service named %q. %s", wanted, offer(services))
	}
	if !service.SystemManaged() {
		return nc.Service{}, fmt.Errorf(
			"%q is managed by another app (%s), so macOS cannot start it — use that app's own connection instead",
			service.Name, service.ProviderID())
	}
	return service, nil
}

func offer(services []nc.Service) string {
	names := nc.SystemManagedNames(services)
	if len(names) == 0 {
		return "No VPN services are configured in System Settings › Network › VPN."
	}
	return "Configured services: " + strings.Join(names, ", ") + "."
}

// Connect is refused, and the reason is a deliberate design decision rather
// than a missing feature.
//
// `scutil --nc start` does accept --user, --password and --secret, and starting
// a service that needs no secret from us works. But L2TP over IPsec keeps its
// shared secret in the *System* keychain, which macOS will not read for a
// user-level process: racoon answers "The IPSec Shared Secret is missing"
// however healthy the configuration is. System Settings succeeds because it runs
// somewhere that may read it; this app does not.
//
// Passing --secret would work, but only by holding the shared secret here — and
// this app does not take custody of a secret macOS is already keeping. So
// connecting stays where the secret already lives, and Tobiro does the
// parts that need no secret at all: reporting the tunnel, and stopping it.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, _ domain.Credential) error {
	// Resolved first so a mistyped service name is reported as that, rather
	// than as this.
	service, err := p.resolve(ctx, profile)
	if err != nil {
		return err
	}

	return &domain.UnsupportedOperationError{
		Provider:  domain.ProviderMacOSVPN,
		Operation: "connect",
		Platform:  runtime.GOOS,
		Reason: "macOS keeps the IPSec shared secret for " + service.Name +
			" in the System keychain and will not read it for an app. Connect it in System Settings › Network › VPN — Tobiro picks the tunnel up from there, and can disconnect it",
		NativeUIFallback: true,
	}
}

// Disconnect asks macOS to tear the service down.
func (p *Provider) Disconnect(ctx context.Context, profile domain.VPNProfile) error {
	service, err := p.resolve(ctx, profile)
	if err != nil {
		return err
	}
	return nc.Stop(ctx, service.ID)
}

// Status reads the service's live state straight from macOS.
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

// OpenNativeClient opens the System Settings page these services live on, the
// only place they can be created or edited.
func (p *Provider) OpenNativeClient(ctx context.Context, _ domain.VPNProfile) error {
	if !nc.Supported() {
		return fmt.Errorf("%s: %w", domain.ProviderMacOSVPN.DisplayName(), domain.ErrClientNotInstalled)
	}
	if _, err := sysexec.Run(ctx, "/usr/bin/open", settingsPane); err != nil {
		return fmt.Errorf("open Network settings: %w", err)
	}
	return nil
}
