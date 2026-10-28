// Package tailscale adapts the official Tailscale client.
//
// Tailscale publishes a command line (tailscale up / down / status) on every
// desktop platform. This adapter drives that CLI and never reimplements the
// protocol. A Tobiro "connection" here is the machine's Tailscale
// daemon: on, off, and the assigned 100.x address.
package tailscale

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/startup"
)

var (
	darwinAppPaths = []string{
		"/Applications/Tailscale.app",
	}
	darwinCLIPaths = []string{
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		"/usr/local/bin/tailscale",
		"/opt/homebrew/bin/tailscale",
	}
	linuxCLIPaths = []string{
		"/usr/bin/tailscale",
		"/usr/local/bin/tailscale",
	}
	windowsCLIPaths = []string{
		`C:\Program Files\Tailscale\tailscale.exe`,
		`C:\Program Files (x86)\Tailscale\tailscale.exe`,
	}
	windowsAppPaths = []string{
		`C:\Program Files\Tailscale\tailscale-ipn.exe`,
		`C:\Program Files\Tailscale\Tailscale.exe`,
	}
)

// Provider is the Tailscale adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderTailscale }

// Detect locates the Tailscale app and CLI.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	info := &domain.ClientInfo{
		Provider:   domain.ProviderTailscale,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
	}

	cli := p.findCLI()
	app := p.findApp()

	if cli == "" && app == "" {
		info.Detail = "Tailscale was not found. Install Tailscale, then Tobiro can turn it on and off."
		return info, nil
	}

	info.Installed = true
	info.CLIPath = cli
	info.Path = app
	if info.Path == "" {
		info.Path = cli
	}
	if cli != "" {
		info.Version = cliVersion(ctx, cli)
		info.Detail = "Tailscale CLI is available for connect, disconnect and status."
	} else {
		info.Detail = "Tailscale is installed, but the CLI was not found on PATH."
	}
	return info, nil
}

func (p *Provider) findCLI() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = darwinCLIPaths
	case "linux":
		candidates = linuxCLIPaths
	case "windows":
		candidates = windowsCLIPaths
	}
	if path := sysexec.FirstExisting(candidates...); path != "" {
		return path
	}
	if path := sysexec.LookPath("tailscale"); path != "" && domain.ValidateExecutablePath(path) == nil {
		return path
	}
	return ""
}

func (p *Provider) findApp() string {
	switch runtime.GOOS {
	case "darwin":
		return sysexec.FirstExisting(darwinAppPaths...)
	case "windows":
		return sysexec.FirstExisting(windowsAppPaths...)
	default:
		return ""
	}
}

func cliVersion(ctx context.Context, cli string) string {
	res, err := sysexec.Run(ctx, cli, "version")
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(res.Stdout), "\n")
	return strings.TrimSpace(line)
}

// Capabilities reports documented CLI control.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}

	hasCLI := info.Installed && info.CLIPath != ""
	hasApp := p.findApp() != ""
	caps := &domain.Capabilities{
		Installed:           info.Installed,
		CLIAvailable:        hasCLI,
		CanConnect:          hasCLI,
		CanDisconnect:       hasCLI,
		CanReadStatus:       hasCLI,
		SupportsPassword:    false,
		SupportsOTP:         false,
		RequiresNativeUI:    false,
		CanOpenNativeClient: hasApp,
		CanControlStartup:   runtime.GOOS == "darwin" && info.Installed,
	}
	if !info.Installed {
		caps.Notes = info.Detail
		return caps, nil
	}
	if !hasCLI {
		caps.Notes = "Tailscale is installed, but the CLI is missing so Tobiro can only open the Tailscale app."
		return caps, nil
	}
	caps.Notes = "Connect and Disconnect drive the Tailscale CLI. Each connection can target a different account (tailscale switch). Status shows whether that account is active and its Tailscale IP."
	return caps, nil
}

func (p *Provider) requireCLI(ctx context.Context) (string, *domain.ClientInfo, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return "", nil, err
	}
	if !info.Installed {
		return "", info, fmt.Errorf("%s: %w", domain.ProviderTailscale.DisplayName(), domain.ErrClientNotInstalled)
	}
	if info.CLIPath == "" {
		return "", info, &domain.UnsupportedOperationError{
			Provider:         domain.ProviderTailscale,
			Operation:        "cli",
			Platform:         runtime.GOOS,
			Reason:           "the Tailscale CLI was not found",
			NativeUIFallback: info.Path != "",
		}
	}
	if err := domain.ValidateExecutablePath(info.CLIPath); err != nil {
		return "", info, err
	}
	return info.CLIPath, info, nil
}

// Connect runs `tailscale switch` when needed, then `tailscale up`. If the
// machine is not logged in, it points at the Tailscale app instead of waiting
// on an interactive browser prompt.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, _ domain.Credential) error {
	cli, _, err := p.requireCLI(ctx)
	if err != nil {
		return err
	}

	status := p.readStatus(ctx, cli)
	if status.NeedsLogin {
		return &domain.UnsupportedOperationError{
			Provider:         domain.ProviderTailscale,
			Operation:        "connect",
			Platform:         runtime.GOOS,
			Reason:           "Tailscale is not signed in. Open the Tailscale app and log in, then connect again.",
			NativeUIFallback: true,
		}
	}

	target := switchTargetForProfile(profile.VendorProfile)
	accounts, listErr := p.listSwitchAccounts(ctx, cli)
	if listErr != nil {
		return listErr
	}

	if target == "" {
		if len(accounts) > 1 {
			return fmt.Errorf(
				"“%s” has no Tailscale account set and this machine has %d profiles. Edit the connection and pick one (%s)",
				profile.Name, len(accounts), profileChoices(accounts),
			)
		}
	} else {
		resolved := resolveProfile(accounts, target)
		if err := resolveConnectError(profile.VendorProfile, resolved); err != nil {
			return err
		}
		if !resolved.Account.Selected {
			if err := p.runSwitch(ctx, cli, resolved.Account.ID); err != nil {
				return err
			}
		}
	}

	res, err := sysexec.Run(ctx, cli, "up")
	if err != nil {
		return fmt.Errorf("tailscale up: %s", defaultString(strings.TrimSpace(res.Combined()), err.Error()))
	}

	if target != "" {
		accounts, listErr = p.listSwitchAccounts(ctx, cli)
		if listErr != nil {
			return listErr
		}
		resolved := resolveProfile(accounts, target)
		if resolved.Result != resolveFound || !resolved.Account.Selected {
			label := optionNameForAccount(resolved.Account, accounts)
			if label == "" {
				label = target
			}
			return fmt.Errorf(
				"Tailscale did not switch to %q (%s). Open the Tailscale app or run tailscale switch %q, then connect again.",
				target, label, resolved.Account.ID,
			)
		}
	}
	return nil
}

// Disconnect runs `tailscale down` when this connection's account is active.
func (p *Provider) Disconnect(ctx context.Context, profile domain.VPNProfile) error {
	cli, _, err := p.requireCLI(ctx)
	if err != nil {
		return err
	}

	reading := p.readStatus(ctx, cli)
	target := switchTargetForProfile(profile.VendorProfile)
	if target != "" && reading.BackendState == "Running" {
		accounts, selErr := p.listSwitchAccounts(ctx, cli)
		if selErr != nil {
			return selErr
		}
		resolved := resolveProfile(accounts, target)
		if resolved.Result != resolveFound || !resolved.Account.Selected {
			return nil
		}
	}

	res, err := sysexec.Run(ctx, cli, "down")
	if err != nil {
		return fmt.Errorf("tailscale down: %s", defaultString(strings.TrimSpace(res.Combined()), err.Error()))
	}
	return nil
}

// Status reports whether Tailscale is up for this connection's account.
func (p *Provider) Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	cli, _, err := p.requireCLI(ctx)
	if err != nil {
		return &domain.VPNStatus{Known: false, Detail: err.Error()}, nil
	}
	reading := p.readStatus(ctx, cli)
	sw, err := p.readSwitchList(ctx, cli)
	if err != nil {
		return &domain.VPNStatus{Known: false, Detail: err.Error()}, nil
	}
	return reading.vpnStatusForProfile(profile.VendorProfile, sw), nil
}

// ReadMachineStatus reads Tailscale once for reconciling several connection cards.
func (p *Provider) ReadMachineStatus(ctx context.Context) (MachineReading, ActiveSwitch, error) {
	cli, _, err := p.requireCLI(ctx)
	if err != nil {
		return MachineReading{}, ActiveSwitch{}, err
	}
	reading := p.readStatus(ctx, cli)
	sw, err := p.readSwitchList(ctx, cli)
	if err != nil {
		return reading.machine(), ActiveSwitch{}, err
	}
	return reading.machine(), sw, nil
}

// StatusFromReading maps one shared daemon reading to one profile's card.
func StatusFromReading(machine MachineReading, vendorProfile string, sw ActiveSwitch, allowEmptyVendorMatch bool) *domain.VPNStatus {
	return machineReading(machine).vpnStatusForProfileOptions(vendorProfile, sw, allowEmptyVendorMatch)
}

func (p *Provider) readStatus(ctx context.Context, cli string) statusReading {
	res, err := sysexec.Run(ctx, cli, "status", "--json")
	if err != nil {
		combined := res.Combined()
		if looksLikeDaemonDown(combined) {
			return statusReading{BackendState: "Stopped", Detail: "Tailscale is not running."}
		}
		return statusReading{Detail: strings.TrimSpace(combined)}
	}
	return parseStatusJSON(res.Stdout)
}

func (p *Provider) listSwitchAccounts(ctx context.Context, cli string) ([]switchAccount, error) {
	res, err := sysexec.Run(ctx, cli, "switch", "--list", "--json")
	if err != nil {
		combined := strings.TrimSpace(res.Combined())
		if combined == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%s", combined)
	}
	return parseSwitchListJSON(res.Stdout)
}

func (p *Provider) readSwitchList(ctx context.Context, cli string) (ActiveSwitch, error) {
	accounts, err := p.listSwitchAccounts(ctx, cli)
	if err != nil {
		return ActiveSwitch{}, err
	}
	return ActiveSwitch{Accounts: accounts}, nil
}

func (p *Provider) runSwitch(ctx context.Context, cli, target string) error {
	res, err := sysexec.Run(ctx, cli, "switch", target)
	if err != nil {
		combined := strings.TrimSpace(res.Combined())
		msg := defaultString(combined, err.Error())
		if looksLikeUnknownAccount(msg) {
			return &domain.UnsupportedOperationError{
				Provider:         domain.ProviderTailscale,
				Operation:        "connect",
				Platform:         runtime.GOOS,
				Reason:           fmt.Sprintf("Tailscale account %q is not on this machine yet. Open the Tailscale app, sign in or join that tailnet, then connect again.", target),
				NativeUIFallback: true,
			}
		}
		return fmt.Errorf("tailscale switch: %s", msg)
	}
	return nil
}

func looksLikeUnknownAccount(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such") ||
		strings.Contains(msg, "unknown") ||
		strings.Contains(msg, "not logged in") ||
		strings.Contains(msg, "not on this device")
}

// ListVendorProfiles offers accounts already logged in on this machine.
func (p *Provider) ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error) {
	cli, _, err := p.requireCLI(ctx)
	if err != nil {
		return nil, err
	}
	return ListAccounts(ctx, cli)
}

// OpenNativeClient launches the Tailscale GUI when one is installed.
func (p *Provider) OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error {
	path := profile.ClientPath
	if path == "" {
		path = p.findApp()
	}
	if path == "" {
		return fmt.Errorf("Tailscale app was not found")
	}
	if err := domain.ValidateExecutablePath(path); err != nil {
		return err
	}
	return sysexec.OpenApplication(ctx, path)
}

func (p *Provider) startupSpec() startup.Spec {
	return startup.Spec{
		Labels: []string{
			"io.tailscale.ipn.macsys.login-item-helper",
			"io.tailscale.ipn.macos.login-item-helper",
		},
		PrefDomain: "io.tailscale.ipn.macsys",
		PrefKey:    "TailscaleStartOnLogin",
	}
}

// StartupAtLogin reports whether the Tailscale app opens when the user logs in.
func (p *Provider) StartupAtLogin(ctx context.Context) (domain.StartupState, error) {
	return startup.Read(ctx, p.startupSpec())
}

// SetStartupAtLogin enables or disables Tailscale opening at login.
func (p *Provider) SetStartupAtLogin(ctx context.Context, enabled bool) error {
	return startup.Set(ctx, p.startupSpec(), enabled)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
