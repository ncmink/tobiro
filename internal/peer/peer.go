// Package peer is Tobiro's local endpoint for the other biebie.net
// applications.
//
// It answers two questions — "is this customer's network up?" and "what context
// does this handoff carry?" — and accepts one request: bring a connection up.
// That request only ever raises this application's own window. Tobiro
// never signs in on a caller's behalf, because a peer asking for a tunnel is not
// the same as a person choosing an account, and only the person can answer a
// password or a one-time code.
//
// Nothing here reads a credential. The endpoint deals in profile identifiers
// and connection state, and the protocol's own validation refuses a message
// that tries to carry more.
package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bctx "github.com/ncmink/biebie-protocol/context"
	"github.com/ncmink/biebie-protocol/handoff"
	"github.com/ncmink/biebie-protocol/ipc"
	"github.com/ncmink/biebie-protocol/version"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/profile"
	"tobiro/internal/session"
)

// StatusRefresher re-reads reality for a profile before this endpoint answers
// a question about it.
//
// It is an interface rather than a manager because the two mechanisms behind it
// refresh very differently — one re-reads a vendor client, the other checks
// whether this process still holds a tunnel — and the endpoint has no reason to
// know which one it just asked.
type StatusRefresher interface {
	RefreshStatus(ctx context.Context, profile domain.VPNProfile)
	RefreshAll(ctx context.Context)
}

// Server serves the Biebie Context Protocol on this user's Tobiro
// endpoint.
type Server struct {
	rpc *ipc.Server

	profiles *profile.Service
	sessions *session.Manager
	access   StatusRefresher
	log      *logging.Logger

	// tickets holds handoffs this application has issued. They are in-memory
	// and short-lived by design: a ticket that outlived the process that made
	// it has no reason to be honoured.
	tickets *handoff.Store

	onConnectRequested func(profileID string)
}

// Options wires the endpoint to the services it reports on.
type Options struct {
	Profiles *profile.Service
	Sessions *session.Manager
	Access   StatusRefresher
	Log      *logging.Logger

	// OnConnectRequested is called when another application asks for a profile
	// to be brought up. The implementation is expected to surface this
	// application's window and let the user choose an account.
	OnConnectRequested func(profileID string)

	// OnError observes serving failures. It is never given a payload.
	OnError func(error)
}

// New prepares the endpoint for this user. Nothing is bound until Start.
func New(opts Options) (*Server, error) {
	endpoint, err := ipc.AccessEndpoint()
	if err != nil {
		return nil, err
	}
	return NewFor(endpoint, opts), nil
}

// NewFor prepares the endpoint at a specific address, so tests can serve on a
// throwaway socket instead of the one the running application owns.
func NewFor(endpoint ipc.Endpoint, opts Options) *Server {
	rpc := ipc.NewServer(endpoint)
	rpc.OnError = opts.OnError

	s := &Server{
		rpc:                rpc,
		profiles:           opts.Profiles,
		sessions:           opts.Sessions,
		access:             opts.Access,
		log:                opts.Log,
		tickets:            handoff.NewStore(),
		onConnectRequested: opts.OnConnectRequested,
	}

	rpc.Handle(ipc.MethodAccessStatus, s.handleStatus)
	rpc.Handle(ipc.MethodAccessConnect, s.handleConnect)
	rpc.Handle(ipc.MethodAccessProfiles, s.handleProfiles)
	rpc.Handle(ipc.MethodConsumeHandoff, s.handleConsumeHandoff)

	return s
}

// Start binds the endpoint and serves in the background.
//
// Failing to bind is not fatal to the application: it normally means another
// Tobiro is already running and already answering, and this process has
// nothing useful to add by refusing to start.
func (s *Server) Start() error { return s.rpc.Start() }

// Close releases the endpoint and removes the socket.
func (s *Server) Close() error { return s.rpc.Close() }

// Address reports where the endpoint listens, for the Settings screen and for
// diagnosing a peer that cannot find it.
func (s *Server) Address() string { return s.rpc.Address() }

// handleStatus answers whether a customer connection is currently up.
func (s *Server) handleStatus(ctx context.Context, params json.RawMessage) (any, error) {
	var req struct {
		ProfileID string `json:"profileId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.New("malformed status request")
	}
	if req.ProfileID == "" {
		return nil, errors.New("a profile id is required")
	}

	stored, err := s.profiles.Resolve(req.ProfileID)
	if err != nil {
		// An unknown profile is a state, not a failed call. The caller is
		// configured with a reference this machine does not have, and the
		// engineer can only fix that if they are told so plainly.
		return bctx.Unknown(req.ProfileID,
			"No connection with this id or name exists in Tobiro."), nil
	}

	// A tunnel raised in the vendor's own GUI is still customer access, so the
	// vendor client is re-read before answering rather than trusting only what
	// this application started itself.
	s.access.RefreshStatus(ctx, stored)

	live, ok := s.sessions.Get(stored.ID)
	status := statusFor(req.ProfileID, stored, live, ok, s.redact)
	if wire, ok := version.NegotiatedFrom(ctx); ok {
		status = status.ForWireVersion(wire)
	}
	return status, nil
}

// handleConnect asks this application to bring a connection up.
//
// It reports only that the request was accepted. Connecting is deliberately not
// awaited: it can need a password, a one-time code or a vendor window, none of
// which the caller should be held open for.
func (s *Server) handleConnect(_ context.Context, params json.RawMessage) (any, error) {
	var req struct {
		ProfileID string `json:"profileId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.New("malformed connect request")
	}
	if req.ProfileID == "" {
		return nil, errors.New("a profile id is required")
	}

	stored, err := s.profiles.Resolve(req.ProfileID)
	if err != nil {
		// Here an unknown profile *is* a failure: the caller asked for
		// something to happen and nothing can.
		return nil, fmt.Errorf("no connection with id or name %s exists in Tobiro", req.ProfileID)
	}

	s.log.Info("Biebie Kube asked to bring up connection %q", stored.Name)

	if s.onConnectRequested != nil {
		go s.onConnectRequested(stored.ID)
	}
	// The identifier is reported back because the request may have named the
	// connection instead, and only this application knows which identifier that
	// was. Everything it says afterwards about this tunnel uses the identifier,
	// so a caller holding the name needs to learn it once.
	return bctx.AccessConnectResult{Accepted: true, ProfileID: stored.ID}, nil
}

// handleProfiles lists the connections held here.
//
// This exists so Biebie Kube can offer a picker instead of a text field. A
// mistyped identifier is otherwise invisible: everything looks configured, and
// the failure only shows up as a connect button that never connects.
//
// Names and identifiers only. Nothing returned here is enough to connect with.
func (s *Server) handleProfiles(ctx context.Context, _ json.RawMessage) (any, error) {
	stored := s.profiles.List()

	// One refresh across every profile, so the states in the list are the
	// vendor clients' rather than only what this application started itself.
	s.access.RefreshAll(ctx)

	out := make([]bctx.AccessProfile, 0, len(stored))
	for _, p := range stored {
		live, ok := s.sessions.Get(p.ID)
		state := bctx.AccessDisconnected
		if ok {
			state = accessState(live.State)
		}
		out = append(out, bctx.AccessProfile{
			ID:       p.ID,
			Name:     p.Name,
			Group:    p.Group,
			Provider: p.Provider.DisplayName(),
			Kind:     string(p.AccessKind()),
			State:    state,
		})
	}
	return out, nil
}

// handleConsumeHandoff redeems a ticket this application issued.
func (s *Server) handleConsumeHandoff(_ context.Context, params json.RawMessage) (any, error) {
	var req struct {
		HandoffID string `json:"handoffId"`
		TargetApp string `json:"targetApp"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errors.New("malformed handoff request")
	}

	received, err := s.tickets.ConsumeFor(req.HandoffID, bctx.App(req.TargetApp))
	if err != nil {
		// The ticket id is safe to log: it is random, single-use, and by now
		// spent or expired.
		s.log.Warn("Refused handoff %s: %v", req.HandoffID, err)
		return nil, err
	}
	return received, nil
}

// CreateHandoff issues a ticket for another application to redeem, which is how
// Tobiro opens a cluster in Biebie Kube without either side sharing a
// store. The context is validated by the protocol before the ticket exists.
func (s *Server) CreateHandoff(ctx context.Context, target bctx.App, c bctx.BiebieContext) (string, error) {
	return s.tickets.CreateHandoff(ctx, handoff.ContextHandoff{
		SourceApp: bctx.AppAccess,
		TargetApp: target,
		Context:   c,
	})
}

// redact keeps adapter failure text from leaving this process with a secret in
// it. The logger owns the list of things that must never appear.
func (s *Server) redact(message string) string {
	if s.log == nil {
		return message
	}
	return s.log.Redact(message)
}
