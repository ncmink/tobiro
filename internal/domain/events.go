package domain

// Event names emitted from Go to the Vue frontend over Wails runtime events.
//
// The interaction events (OTP, browser, certificate) exist so authentication
// can pause mid-flight and ask the user for something, without the backend ever
// persisting that answer.
const (
	EventSessionChanged = "session:changed"
	EventProfileChanged = "profile:changed"
	EventLogAppended    = "log:appended"

	// EventConnectRequested is raised when another biebie.net application asks
	// for a connection to be brought up, over local IPC or a deep link. The UI
	// opens that connection and offers its accounts; it does not connect on its
	// own, because only the person at the keyboard can answer a password.
	EventConnectRequested = "peer:connect_requested"

	EventAuthenticationRequired = "auth:required"
	EventOTPRequired            = "auth:otp_required"

	// EventHostKeyReview asks the user to vouch for an SSH server nobody on
	// this machine has met before. It is the one moment where accepting is a
	// security decision rather than a convenience, so it is never answered
	// automatically.
	EventHostKeyReview = "auth:host_key_review"

	EventBrowserAuthenticationNeeded = "auth:browser_required"
	EventCertificateRequired         = "auth:certificate_required"
	EventPasswordExpired             = "auth:password_expired"
	EventConnectionFailed            = "connection:failed"
)

// InteractionRequest asks the UI for a transient value during authentication.
//
// The response (an OTP code, for example) is held in memory for the duration of
// the connect call only. It is never written to disk, never logged, and never
// stored in the credential store.
type InteractionRequest struct {
	SessionID string `json:"sessionId"`
	ProfileID string `json:"profileId"`
	Kind      string `json:"kind"`
	Prompt    string `json:"prompt"`
	Message   string `json:"message"`
}

const (
	InteractionOTP         = "otp"
	InteractionBrowser     = "browser"
	InteractionCertificate = "certificate"
)

// HostKeyRequest asks the user whether an unrecognised SSH server is the one
// they meant to reach.
//
// It is only ever raised for a host that appears in no known_hosts file. A host
// whose key has *changed* is refused outright and never reaches the user as a
// question, because the one time that prompt matters is the one time nobody
// should be able to click through it.
type HostKeyRequest struct {
	ProfileID   string `json:"profileId"`
	Host        string `json:"host"`
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
	KnownHosts  string `json:"knownHosts"`
}

// ConnectionFailedEvent is emitted when an attempt ends in StateFailed.
type ConnectionFailedEvent struct {
	SessionID string `json:"sessionId"`
	ProfileID string `json:"profileId"`
	Reason    string `json:"reason"`
}
