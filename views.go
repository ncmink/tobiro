package main

import (
	"tobiro/internal/domain"
	"tobiro/internal/logging"
)

// ConnectionView is everything the dashboard needs about one connection in a
// single round trip: the profile, its accounts (without secrets), the live
// session, and what the installed vendor client can actually do.
type ConnectionView struct {
	Profile      domain.VPNProfile       `json:"profile"`
	ProviderName string                  `json:"providerName"`
	Credentials  []domain.CredentialView `json:"credentials"`

	// Endpoint is the gateway address to display, or the provider's stand-in
	// label for clients that have no gateway of their own.
	Endpoint string `json:"endpoint"`

	// ManagesOwnAccount reports that the vendor client holds the identity, so
	// the UI offers no account list for this connection.
	ManagesOwnAccount bool `json:"managesOwnAccount"`

	State   domain.ConnectionState `json:"state"`
	Session *domain.SessionView    `json:"session"`

	Capabilities domain.Capabilities `json:"capabilities"`
	Client       domain.ClientInfo   `json:"client"`

	// Hidden reports that this connection's group is kept off the dashboard
	// until the user asks to see hidden groups.
	Hidden bool `json:"hidden"`

	// Requires describes the connection this one needs up first, if any. It is
	// resolved here rather than left as the identifier on the profile, because
	// every screen that mentions it wants the name and the live state.
	Requires *RequirementView `json:"requires"`

	// SuggestedCredentialID is what the connect dialog should preselect: the
	// last account used on this profile, else the default, else the only one.
	SuggestedCredentialID string `json:"suggestedCredentialId"`
	LastCredentialID      string `json:"lastCredentialId"`
}

// RequirementView names the connection that has to be up first and reports
// where it currently stands, so the UI can say "needs SMOI VPN, connected"
// without a second round trip.
type RequirementView struct {
	ProfileID string                 `json:"profileId"`
	Name      string                 `json:"name"`
	State     domain.ConnectionState `json:"state"`
}

// BootstrapView is the static catalogue the UI loads once at startup.
type BootstrapView struct {
	Providers   []domain.ProviderInfo   `json:"providers"`
	AuthMethods []domain.AuthMethodInfo `json:"authMethods"`

	SecretBackend    string `json:"secretBackend"`
	SecretAvailable  bool   `json:"secretAvailable"`
	SecretStatusText string `json:"secretStatusText"`

	DataPath string `json:"dataPath"`
	Version  string `json:"version"`
}

// ConnectRequestView is pushed to the UI when another biebie.net application
// asks for a connection to be brought up. It names the profile and nothing
// else: the account is still the user's to choose.
type ConnectRequestView struct {
	ProfileID string `json:"profileId"`
}

// OpenKubeInput names the cluster Tobiro should hand to Biebie Kube.
//
// Nothing here is a credential. Cluster identity must be supplied explicitly —
// Tobiro does not guess which Kubernetes cluster a forward belongs to.
type OpenKubeInput struct {
	ProfileID string `json:"profileId"`

	ClusterID   string `json:"clusterId"`
	ClusterName string `json:"clusterName"`

	CustomerID   string `json:"customerId"`
	CustomerName string `json:"customerName"`

	EnvironmentID   string `json:"environmentId,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	EnvironmentKind string `json:"environmentKind,omitempty"`

	// Server is the Kubernetes API endpoint when the engineer knows it, usually
	// from the forward that reaches port 6443.
	Server string `json:"server,omitempty"`
}

// OpenKubeResultView reports how the handoff was delivered.
type OpenKubeResultView struct {
	HandoffID string `json:"handoffId"`
	Link      string `json:"link,omitempty"`

	// Delivered is true when a running Biebie Kube consumed the handoff over IPC.
	Delivered bool `json:"delivered,omitempty"`

	// Launched is true when Biebie Kube was opened through the desktop handler.
	Launched bool `json:"launched,omitempty"`
}

// OTPRequestView is pushed to the UI when authentication pauses for a code.
type OTPRequestView struct {
	RequestID string `json:"requestId"`
	ProfileID string `json:"profileId"`
	Prompt    string `json:"prompt"`
	Message   string `json:"message"`
}

// HostKeyRequestView is pushed to the UI when an SSH server nobody on this
// machine has met answers a connection.
//
// The fingerprint is the whole point of the dialog: it is what the user checks
// against what the server's owner told them, and the only thing that
// distinguishes the right server from something answering in its place.
type HostKeyRequestView struct {
	RequestID   string `json:"requestId"`
	ProfileID   string `json:"profileId"`
	Host        string `json:"host"`
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
	KnownHosts  string `json:"knownHosts"`
}

// LogView wraps a redacted activity log entry.
type LogView = logging.Entry

// ExportResultView reports where the shareable connections file was written.
type ExportResultView struct {
	File        string `json:"file"`
	Connections int    `json:"connections"`
	Cancelled   bool   `json:"cancelled"`
}

// ImportResultView summarises what an imported file added. Skipped holds the
// names that already existed locally and were left untouched.
type ImportResultView struct {
	File        string   `json:"file"`
	Connections int      `json:"connections"`
	Accounts    int      `json:"accounts"`
	Skipped     []string `json:"skipped"`
	Cancelled   bool     `json:"cancelled"`
}

// SSHConfigHostView is one Host block from the user's OpenSSH config file.
type SSHConfigHostView struct {
	Alias        string `json:"alias"`
	HostName     string `json:"hostName"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	ForwardCount int    `json:"forwardCount"`
	Exists       bool   `json:"exists"`
}
