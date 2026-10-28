package domain

import (
	"fmt"
	"time"
)

// ConnectionState is the lifecycle of one connection attempt.
//
// A boolean is not enough: the UI has to distinguish "dialling" from "waiting
// for the user's OTP" from "tearing down", and each state allows a different
// set of actions.
type ConnectionState string

const (
	StateDisconnected   ConnectionState = "disconnected"
	StateConnecting     ConnectionState = "connecting"
	StateAuthenticating ConnectionState = "authenticating"
	StateWaitingForOTP  ConnectionState = "waiting_for_otp"
	StateConnected      ConnectionState = "connected"
	StateDisconnecting  ConnectionState = "disconnecting"
	StateFailed         ConnectionState = "failed"
)

// allowedTransitions defines the state machine explicitly. Anything not listed
// is a bug in an adapter and is rejected rather than silently applied.
//
// Disconnected/failed → connected is allowed because Tobiro observes
// tunnels raised in the vendor GUI; it is not always the process that dials.
//
// Connected → connecting is the laptop-lid case: a connection this process
// holds can die on its own and be repaired underneath the same profile. Without
// it, a tunnel that dropped would go on claiming to be up while it reconnected,
// and anything asking whether the customer network is reachable — including
// another application — would be told yes.
var allowedTransitions = map[ConnectionState][]ConnectionState{
	StateDisconnected:   {StateConnecting, StateConnected},
	StateConnecting:     {StateAuthenticating, StateConnected, StateFailed, StateDisconnecting},
	StateAuthenticating: {StateWaitingForOTP, StateConnected, StateFailed, StateDisconnecting},
	StateWaitingForOTP:  {StateAuthenticating, StateConnected, StateFailed, StateDisconnecting},
	StateConnected:      {StateConnecting, StateDisconnecting, StateFailed, StateDisconnected},
	StateDisconnecting:  {StateDisconnected, StateFailed},
	StateFailed:         {StateConnecting, StateDisconnected, StateConnected},
}

// CanTransition reports whether from -> to is a legal move.
func CanTransition(from, to ConnectionState) bool {
	if from == to {
		return true
	}
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether the state represents a finished attempt.
func (s ConnectionState) IsTerminal() bool {
	return s == StateDisconnected || s == StateFailed
}

// IsActive reports whether a session is occupying the profile (so the UI shows
// Disconnect rather than Connect).
func (s ConnectionState) IsActive() bool {
	switch s {
	case StateConnecting, StateAuthenticating, StateWaitingForOTP, StateConnected, StateDisconnecting:
		return true
	default:
		return false
	}
}

// VPNSession is a single connection attempt/lifetime for one profile.
//
// It records which credential UUID is in use, so the app can say "SMOI is
// connected using Ming" without ever treating the username as the identity.
type VPNSession struct {
	ID           string `json:"id"`
	ProfileID    string `json:"profileId"`
	CredentialID string `json:"credentialId"`

	State ConnectionState `json:"state"`

	StartedAt   time.Time  `json:"startedAt"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`

	AssignedIP string `json:"assignedIp"`
	Interface  string `json:"interface"`
	Identity   string `json:"identity"`

	// Forwards are the local ports an SSH tunnel is currently lending to the
	// customer network. It is empty for a VPN session, which hands the machine
	// a route instead of a set of ports.
	Forwards []PortForward `json:"forwards,omitempty"`

	Error string `json:"error"`
}

// DurationSeconds reports how long the tunnel has been up, or 0 if it never
// reached connected.
func (s VPNSession) DurationSeconds() int {
	if s.ConnectedAt == nil {
		return 0
	}
	end := time.Now()
	if s.EndedAt != nil {
		end = *s.EndedAt
	}
	d := int(end.Sub(*s.ConnectedAt).Seconds())
	if d < 0 {
		return 0
	}
	return d
}

// SessionView is the frontend-facing session shape, with the computed duration
// resolved server-side.
//
// Fields are listed explicitly rather than embedding VPNSession, because the
// Wails TypeScript model generator does not flatten embedded structs.
type SessionView struct {
	ID           string `json:"id"`
	ProfileID    string `json:"profileId"`
	CredentialID string `json:"credentialId"`

	State ConnectionState `json:"state"`

	StartedAt   time.Time  `json:"startedAt"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`

	AssignedIP string        `json:"assignedIp"`
	Interface  string        `json:"interface"`
	Identity   string        `json:"identity"`
	Forwards   []PortForward `json:"forwards"`
	Error      string        `json:"error"`

	DurationSeconds int `json:"durationSeconds"`
}

// View converts to the frontend representation.
func (s VPNSession) View() SessionView {
	return SessionView{
		ID:              s.ID,
		ProfileID:       s.ProfileID,
		CredentialID:    s.CredentialID,
		State:           s.State,
		StartedAt:       s.StartedAt,
		ConnectedAt:     s.ConnectedAt,
		EndedAt:         s.EndedAt,
		AssignedIP:      s.AssignedIP,
		Interface:       s.Interface,
		Identity:        s.Identity,
		Forwards:        s.Forwards,
		Error:           s.Error,
		DurationSeconds: s.DurationSeconds(),
	}
}

// InvalidTransitionError reports a rejected state machine move.
type InvalidTransitionError struct {
	From ConnectionState
	To   ConnectionState
}

func (e InvalidTransitionError) Error() string {
	return fmt.Sprintf("invalid connection state transition %s -> %s", e.From, e.To)
}
