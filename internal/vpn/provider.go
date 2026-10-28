// Package vpn defines the provider abstraction and orchestrates native VPN
// clients. It contains no vendor-specific commands — those live entirely inside
// each adapter package.
package vpn

import (
	"context"

	"tobiro/internal/domain"
)

// VPNProviderAdapter is implemented once per vendor client.
//
// An adapter orchestrates an officially installed client. It must never
// reimplement a proprietary VPN protocol, and must never invent undocumented
// CLI arguments: when an operation is not officially supported on the detected
// platform/version it returns a *domain.UnsupportedOperationError so the app
// can fall back to launching the native GUI.
type VPNProviderAdapter interface {
	// ID returns the provider key used by the registry and profiles.
	ID() domain.VPNProvider

	// Detect probes the machine for the installed client.
	Detect(ctx context.Context) (*domain.ClientInfo, error)

	// Capabilities reports what this adapter can do with the installed client
	// on this platform and version.
	Capabilities(ctx context.Context) (*domain.Capabilities, error)

	// Connect establishes the tunnel. The credential carries a transient
	// Secret resolved from the OS credential store; adapters must not log,
	// persist or forward it anywhere except the vendor client.
	Connect(ctx context.Context, profile domain.VPNProfile, credential domain.Credential) error

	// Disconnect tears the tunnel down.
	Disconnect(ctx context.Context, profile domain.VPNProfile) error

	// Status reads the current tunnel state from the client. Adapters that
	// cannot read status return a status with Known=false rather than guessing.
	Status(ctx context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error)

	// OpenNativeClient launches the vendor GUI, the fallback when programmatic
	// control is unavailable.
	OpenNativeClient(ctx context.Context, profile domain.VPNProfile) error
}

// LoginFiller is implemented by adapters that can type a stored account into
// the vendor GUI. Implementations must never click Connect — the user submits
// in the vendor window.
type LoginFiller interface {
	FillNativeLogin(ctx context.Context, profile domain.VPNProfile, credential domain.Credential) error
}

// StartupController is implemented by adapters that can stop (or restore) the
// vendor GUI launching when the user logs in. Implementations must not disable
// privileged tunnel helpers — only the login/tray agent.
type StartupController interface {
	StartupAtLogin(ctx context.Context) (domain.StartupState, error)
	SetStartupAtLogin(ctx context.Context, enabled bool) error
}

// VendorProfileLister is implemented by adapters that can enumerate the
// vendor-side selectors already configured on this machine, so the form offers
// them instead of asking for a name the user cannot see from that window.
//
// Implementations only read: discovering a profile must never create, edit or
// connect one.
type VendorProfileLister interface {
	ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error)
}

// NetworkDoctor is implemented by adapters that can explain how a live tunnel
// affects other programs (full tunnel vs split tunnel). Implementations only
// read; they must never install or delete routes.
type NetworkDoctor interface {
	DiagnoseNetwork(ctx context.Context, profile domain.VPNProfile) (domain.NetworkDiagnosis, error)
}

// Prompter lets an adapter ask the user for a transient value mid-flight, such
// as an OTP code. Implementations bridge to the UI over Wails events.
//
// Values returned here are used once and discarded: they are never persisted
// and never logged.
type Prompter interface {
	RequestOTP(ctx context.Context, req domain.InteractionRequest) (string, error)
	Notify(event string, payload any)
}

// StateReporter lets an adapter drive the connection state machine while a
// connect call is in flight (connecting -> authenticating -> waiting_for_otp).
type StateReporter interface {
	Report(state domain.ConnectionState, detail string)
}

type ctxKey int

const (
	ctxKeyPrompter ctxKey = iota
	ctxKeyReporter
)

// WithPrompter attaches a Prompter to the context handed to Connect.
func WithPrompter(ctx context.Context, p Prompter) context.Context {
	return context.WithValue(ctx, ctxKeyPrompter, p)
}

// PrompterFrom extracts the Prompter, if one was attached.
func PrompterFrom(ctx context.Context) (Prompter, bool) {
	p, ok := ctx.Value(ctxKeyPrompter).(Prompter)
	return p, ok
}

// WithStateReporter attaches a StateReporter to the context.
func WithStateReporter(ctx context.Context, r StateReporter) context.Context {
	return context.WithValue(ctx, ctxKeyReporter, r)
}

// StateReporterFrom extracts the StateReporter, if one was attached.
func StateReporterFrom(ctx context.Context) (StateReporter, bool) {
	r, ok := ctx.Value(ctxKeyReporter).(StateReporter)
	return r, ok
}

// ReportState is a nil-safe helper for adapters.
func ReportState(ctx context.Context, state domain.ConnectionState, detail string) {
	if r, ok := StateReporterFrom(ctx); ok {
		r.Report(state, detail)
	}
}
