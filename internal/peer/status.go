package peer

import (
	bctx "github.com/ncmink/biebie-protocol/context"

	"tobiro/internal/domain"
)

// accessState maps the Tobiro connection lifecycle onto the coarser set
// the protocol defines.
//
// The two are deliberately different sizes. Tobiro separates
// authenticating from waiting for a one-time code because its own screens act
// on that difference; a peer only needs to know whether customer access is
// usable yet, so those collapse into one state here.
func accessState(state domain.ConnectionState) bctx.AccessState {
	switch state {
	case domain.StateConnected:
		return bctx.AccessConnected

	case domain.StateConnecting, domain.StateAuthenticating, domain.StateWaitingForOTP:
		return bctx.AccessConnecting

	case domain.StateFailed:
		return bctx.AccessFailed

	case domain.StateDisconnected, domain.StateDisconnecting:
		// Disconnecting is reported as down. The question a peer is really
		// asking is whether it can reach the customer network right now, and a
		// tunnel being torn down is not something to start a Kubernetes session
		// on — answering "connected" would only move the failure later, into a
		// timeout with no explanation.
		return bctx.AccessDisconnected

	default:
		return bctx.AccessUnknown
	}
}

// statusFor describes a profile's connectivity for another application.
//
// redact is applied to anything derived from an adapter error, because a
// failure message is the one field here that did not start life as an
// identifier.
func statusFor(profileID string, profile domain.VPNProfile, session domain.VPNSession, live bool, redact func(string) string) bctx.AccessStatus {
	if !live {
		return bctx.AccessStatus{
			ProfileID: profileID,
			State:     bctx.AccessDisconnected,
			Detail:    "This connection is not up in Tobiro.",
		}
	}

	state := accessState(session.State)
	status := bctx.AccessStatus{
		ProfileID: profileID,
		State:     state,
		Connected: state == bctx.AccessConnected,
	}

	// The tunnel address is only meaningful while the tunnel is up. Reporting a
	// stale one would have a peer diagnose against an address that no longer
	// routes anywhere.
	//
	// The forwards are withheld for a stronger reason than staleness: a peer
	// that believed in a closed forward would send a cluster's traffic to a
	// loopback port that either refuses it or, worse, now belongs to something
	// else entirely.
	if status.Connected {
		status.AssignedIP = session.AssignedIP
		status.ConnectedAt = session.ConnectedAt
		status.Gateway = profile.Host
		status.Forwards = forwardsOf(session)
	}

	switch state {
	case bctx.AccessFailed:
		status.Detail = redact(defaultDetail(session.Error, "The last attempt to connect failed."))
	case bctx.AccessConnecting:
		status.Detail = "Tobiro is still bringing this connection up."
	case bctx.AccessDisconnected:
		status.Detail = "This connection is not up in Tobiro."
	}

	return status
}

// forwardsOf translates the session's live forwards into the wire shape.
//
// The translation is deliberate rather than a shared struct: this application's
// own model is free to grow fields that have no business crossing to a peer.
func forwardsOf(session domain.VPNSession) []bctx.Forward {
	if len(session.Forwards) == 0 {
		return nil
	}
	out := make([]bctx.Forward, 0, len(session.Forwards))
	for _, f := range session.Forwards {
		out = append(out, bctx.Forward{
			Name:       f.Name,
			LocalPort:  f.LocalPort,
			RemoteHost: f.RemoteHost,
			RemotePort: f.RemotePort,
		})
	}
	return out
}

func defaultDetail(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
