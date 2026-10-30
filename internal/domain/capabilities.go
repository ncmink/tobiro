package domain

// ClientInfo is the result of probing the machine for an installed vendor
// client. It is intentionally descriptive rather than prescriptive: Biebie
// Access reports what it found and lets the user decide.
type ClientInfo struct {
	Provider VPNProvider `json:"provider"`

	Installed  bool   `json:"installed"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	CLIPath    string `json:"cliPath"`
	Platform   string `json:"platform"`
	DetectedAt string `json:"detectedAt"`

	// Detail explains what detection concluded, including why an operation may
	// be unavailable on this platform/version.
	Detail string `json:"detail"`
}

// Capabilities declares what an adapter can actually do with the installed
// client on THIS platform and version.
//
// Vendors differ wildly: a documented CLI on Linux frequently has no macOS or
// Windows equivalent. Rather than guessing undocumented arguments, an adapter
// reports the truth here and the UI degrades to launching the native client.
type Capabilities struct {
	Installed        bool `json:"installed"`
	CLIAvailable     bool `json:"cliAvailable"`
	CanConnect       bool `json:"canConnect"`
	CanDisconnect    bool `json:"canDisconnect"`
	CanReadStatus    bool `json:"canReadStatus"`
	SupportsPassword bool `json:"supportsPassword"`
	SupportsOTP      bool `json:"supportsOtp"`
	RequiresNativeUI bool `json:"requiresNativeUi"`

	// AccountOptional reports that a connection can be made with no account
	// saved here at all.
	//
	// An SSH tunnel can: a key already loaded in the agent, or one of the
	// defaults in ~/.ssh, is everything it needs. Without this the UI would
	// insist on an account that holds nothing and withhold Connect from a
	// tunnel that is perfectly able to dial.
	AccountOptional bool `json:"accountOptional"`

	// CanOpenNativeClient reports whether the vendor GUI can be launched as a
	// fallback path for the user to finish the connection manually.
	CanOpenNativeClient bool `json:"canOpenNativeClient"`

	// CanFillNativeLogin reports whether Tobiro can type a saved account into
	// the vendor login form when the user asks (macOS Accessibility).
	CanFillNativeLogin bool `json:"canFillNativeLogin"`

	// NativeClientLabel names that action when "native client" would be wrong —
	// the macOS VPN adapter opens System Settings, not a vendor app. Empty
	// leaves the UI's own wording in place.
	NativeClientLabel string `json:"nativeClientLabel"`

	// CanControlStartup reports whether this adapter can stop the vendor
	// client from launching when the user logs in.
	CanControlStartup bool `json:"canControlStartup"`

	// Notes is shown in the UI to explain a limitation in plain language.
	Notes string `json:"notes"`
}

// VPNStatus is a point-in-time reading from the vendor client.
type VPNStatus struct {
	State      ConnectionState `json:"state"`
	AssignedIP string          `json:"assignedIp"`
	Interface  string          `json:"interface"`
	// Identity is who the vendor client is signed in as (e.g. a Tailscale
	// LoginName). Empty when the adapter cannot tell.
	Identity string `json:"identity"`
	Detail   string `json:"detail"`

	// Known is false when the adapter cannot read status on this platform, so
	// the caller keeps its own tracked state instead of overwriting it.
	Known bool `json:"known"`
}

// StartupState is whether the vendor client launches when the user logs in.
//
// This is the GUI/tray at login, not the privileged helper the tunnel needs
// once the user connects from Tobiro.
type StartupState struct {
	Known   bool   `json:"known"`
	Enabled bool   `json:"enabled"`
	Detail  string `json:"detail"`
}

// NetworkDiagnosis is a read-only view of how a live tunnel affects the rest
// of the machine. It never changes routes — it only reports what the vendor
// client already installed.
type NetworkDiagnosis struct {
	Known bool `json:"known"`

	// CapturesAllTraffic is true when a full-tunnel route (default, 0/0, or
	// the 0/1+128.0/1 pair) sends every packet through this VPN.
	CapturesAllTraffic bool `json:"capturesAllTraffic"`

	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// ProviderStatusView bundles detection and capabilities for the Settings screen.
type ProviderStatusView struct {
	Provider     VPNProvider  `json:"provider"`
	DisplayName  string       `json:"displayName"`
	Client       ClientInfo   `json:"client"`
	Capabilities Capabilities `json:"capabilities"`
	Error        string       `json:"error"`
}
