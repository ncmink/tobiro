package main

import (
	"context"
	"fmt"
	"strings"

	"tobiro/internal/access"
	"tobiro/internal/credential"
	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/peer"
	"tobiro/internal/profile"
	"tobiro/internal/security"
	"tobiro/internal/session"
	"tobiro/internal/sshtunnel"
	"tobiro/internal/store"
	"tobiro/internal/vpn"
)

// appVersion is stamped into the Settings screen.
const appVersion = "0.0.1"

// App is the Wails binding surface: use-case entry points only.
//
// Every method here delegates to a service. Business rules live in
// internal/... so they stay testable and reusable as Tobiro grows from a
// VPN manager into a broader customer access workspace.
type App struct {
	ctx context.Context

	profiles    *profile.Service
	credentials *credential.Service
	sessions    *session.Manager

	// access routes connect/disconnect to the mechanism a profile uses. The
	// two managers below stay reachable for the operations only one of them
	// has: a vendor GUI to open, a tunnel to close on shutdown.
	access   *access.Router
	vpn      *vpn.Manager
	tunnels  *sshtunnel.Manager
	registry *vpn.Registry

	secrets security.SecureCredentialStore
	store   *store.Store
	log     *logging.Logger
	bridge  *uiBridge

	// peer is the endpoint the other biebie.net applications call. It is nil
	// when the endpoint could not be bound, which this application carries on
	// without — see service_peer.go.
	peer           *peer.Server
	notifier       *peer.Notifier
	pendingConnect pendingConnect
}

// NewApp builds the binding surface from already-wired services.
func NewApp(
	profiles *profile.Service,
	credentials *credential.Service,
	sessions *session.Manager,
	router *access.Router,
	vpnManager *vpn.Manager,
	tunnels *sshtunnel.Manager,
	registry *vpn.Registry,
	secrets security.SecureCredentialStore,
	dataStore *store.Store,
	log *logging.Logger,
	bridge *uiBridge,
) *App {
	return &App{
		profiles:    profiles,
		credentials: credentials,
		sessions:    sessions,
		access:      router,
		vpn:         vpnManager,
		tunnels:     tunnels,
		registry:    registry,
		secrets:     secrets,
		store:       dataStore,
		log:         log,
		bridge:      bridge,
	}
}

// startup captures the Wails context.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.bridge.attach(ctx)
	a.log.Info("Tobiro %s started", appVersion)

	available, detail := a.secrets.Available()
	if available {
		a.log.Info("Credential store: %s", a.secrets.Backend())
	} else {
		a.log.Warn("Credential store unavailable: %s", detail)
	}

	// Registered before the status poller starts, so the first sweep of the
	// vendor clients is announced rather than being the one change Biebie Kube
	// misses.
	a.startPeerNotifier()
	a.startPeerEndpoint()

	a.vpn.StartWatching(ctx)
}

// ---------------------------------------------------------------- bootstrap

// GetBootstrap returns the static catalogue and environment info for the UI.
func (a *App) GetBootstrap() BootstrapView {
	available, detail := a.secrets.Available()
	// Offer only adapters included in this build. In particular, the mock
	// adapter is registered only by wails dev, never by a packaged build.
	providers := make([]domain.ProviderInfo, 0)
	for _, provider := range domain.SupportedProviders() {
		if _, err := a.registry.Get(provider.ID); err == nil {
			providers = append(providers, provider)
		}
	}
	return BootstrapView{
		Providers:        providers,
		AuthMethods:      domain.SupportedAuthMethods(),
		SecretBackend:    a.secrets.Backend(),
		SecretAvailable:  available,
		SecretStatusText: detail,
		DataPath:         a.store.Path(),
		Version:          appVersion,
	}
}

// --------------------------------------------------------------- connections

// ListConnections returns every profile with its accounts, live session and
// provider capabilities.
func (a *App) ListConnections() []ConnectionView {
	a.access.RefreshAll(a.ctx)
	profiles := a.profiles.List()
	out := make([]ConnectionView, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, a.buildConnectionView(p))
	}
	return out
}

// GetConnection returns one connection view.
func (a *App) GetConnection(profileID string) (ConnectionView, error) {
	p, err := a.profiles.Get(profileID)
	if err != nil {
		return ConnectionView{}, err
	}
	a.access.RefreshStatus(a.ctx, p)
	return a.buildConnectionView(p), nil
}

// ReadSSHCommand turns an `ssh -N -L ...` command line into the fields the
// connection form needs, so a command someone already runs in a terminal can be
// saved by pasting it rather than retyped field by field.
func (a *App) ReadSSHCommand(command string) (domain.SSHCommand, error) {
	parsed, err := domain.ParseSSHCommand(command)
	if err != nil {
		return domain.SSHCommand{}, a.describe(err)
	}
	return parsed, nil
}

// CreateConnection adds a VPN connection profile.
func (a *App) CreateConnection(input domain.ProfileInput) (ConnectionView, error) {
	p, err := a.profiles.Create(input)
	if err != nil {
		return ConnectionView{}, err
	}
	a.log.Info("Added connection %q provider=%s endpoint=%s", p.Name, p.Provider, p.Endpoint())
	a.bridge.Notify(domain.EventProfileChanged, p.ID)
	return a.buildConnectionView(p), nil
}

// UpdateConnection edits a connection profile.
//
// A live SSH tunnel may be edited. It keeps serving the ports it actually bound
// at connect time, so the saved profile and the running tunnel can differ until
// the next connect — which is far kinder than refusing to let someone add a
// forward they have just discovered they need, and losing their session for it.
//
// A VPN connection still has to be down. What is being edited there is the
// address and vendor profile an installed client is at that moment driving, and
// this application is not the thing holding the connection open.
func (a *App) UpdateConnection(profileID string, input domain.ProfileInput) (ConnectionView, error) {
	if a.sessions.State(profileID).IsActive() && !a.isSSH(profileID) {
		return ConnectionView{}, fmt.Errorf("disconnect before editing this connection")
	}
	p, err := a.profiles.Update(profileID, input)
	if err != nil {
		return ConnectionView{}, err
	}
	a.log.Info("Updated connection %q", p.Name)
	a.bridge.Notify(domain.EventProfileChanged, p.ID)
	return a.buildConnectionView(p), nil
}

// SaveTunnel stores the SSH half of a connection: the server, the login name
// and the ports it lends.
//
// It may be saved while the tunnel is up. The running tunnel keeps the ports it
// bound at connect time, so the saved map and the live one can differ until the
// next connect — which is far kinder than making someone drop their session to
// add a forward they have just discovered they need.
func (a *App) SaveTunnel(profileID string, input domain.TunnelInput) (ConnectionView, error) {
	p, err := a.profiles.SetTunnel(profileID, input)
	if err != nil {
		return ConnectionView{}, a.describe(err)
	}
	a.log.Info("Updated the SSH tunnel for %q endpoint=%s forwards=%d",
		p.Name, p.EndpointDisplay(), len(p.SSH.Forwards))
	a.bridge.Notify(domain.EventProfileChanged, p.ID)
	return a.buildConnectionView(p), nil
}

// DeleteConnection removes a profile along with its accounts and their secrets.
func (a *App) DeleteConnection(profileID string) error {
	if a.sessions.State(profileID).IsActive() {
		return fmt.Errorf("disconnect before deleting this connection")
	}
	if err := a.credentials.DeleteForProfile(profileID); err != nil {
		return err
	}
	if err := a.profiles.Delete(profileID); err != nil {
		return err
	}
	a.sessions.Remove(profileID)
	a.log.Info("Deleted connection %s", profileID)
	a.bridge.Notify(domain.EventProfileChanged, profileID)
	return nil
}

// MoveConnection files a connection under another group. Pass an empty group to
// take it out of every group, or the archive to put it away.
//
// Unlike editing a connection this is allowed while the tunnel is up: a group
// says where a connection is listed, not how it connects, so there is nothing
// to disconnect first.
func (a *App) MoveConnection(profileID string, group string) (ConnectionView, error) {
	p, err := a.profiles.SetGroup(profileID, group)
	if err != nil {
		return ConnectionView{}, a.describe(err)
	}
	a.log.Info("Moved connection %q to %s", p.Name, domain.GroupLabel(p.Group))
	a.bridge.Notify(domain.EventProfileChanged, p.ID)
	return a.buildConnectionView(p), nil
}

// -------------------------------------------------------------------- groups

// ListGroups returns the connection groups in use, with their counts and
// whether each one is hidden from the dashboard by default.
func (a *App) ListGroups() []domain.GroupView {
	return a.profiles.Groups()
}

// SetGroupHidden hides or reveals a whole group of connections and returns the
// refreshed group list. Hiding is presentation only: nothing is deleted, and a
// hidden connection can still be opened from the dashboard's hidden view.
func (a *App) SetGroupHidden(group string, hidden bool) ([]domain.GroupView, error) {
	if err := a.profiles.SetGroupHidden(group, hidden); err != nil {
		return nil, a.describe(err)
	}
	if hidden {
		a.log.Info("Hiding group %q from the connection list", domain.GroupLabel(group))
	} else {
		a.log.Info("Showing group %q on the connection list", domain.GroupLabel(group))
	}
	a.bridge.Notify(domain.EventProfileChanged, "")
	return a.profiles.Groups(), nil
}

// --------------------------------------------------------------- credentials

// AddCredential adds an account to a connection. The password goes straight to
// the OS credential store and is never persisted in application data.
func (a *App) AddCredential(profileID string, input domain.CredentialInput) (domain.CredentialView, error) {
	return a.credentials.Create(profileID, input)
}

// UpdateCredential edits an account. An empty password keeps the stored one.
func (a *App) UpdateCredential(credentialID string, input domain.CredentialInput) (domain.CredentialView, error) {
	return a.credentials.Update(credentialID, input)
}

// DeleteCredential removes an account and its stored password.
func (a *App) DeleteCredential(credentialID string) error {
	return a.credentials.Delete(credentialID)
}

// SetDefaultCredential marks an account as the profile default.
func (a *App) SetDefaultCredential(credentialID string) error {
	return a.credentials.SetDefault(credentialID)
}

// ---------------------------------------------------------------- connecting

// Connect establishes a tunnel using a specific credential UUID.
func (a *App) Connect(profileID string, credentialID string) (domain.SessionView, error) {
	sess, err := a.access.Connect(a.ctx, profileID, credentialID)
	if err != nil {
		return sess.View(), a.describe(err)
	}
	return sess.View(), nil
}

// Disconnect tears down a tunnel.
func (a *App) Disconnect(profileID string) (domain.SessionView, error) {
	sess, err := a.access.Disconnect(a.ctx, profileID)
	if err != nil {
		return sess.View(), a.describe(err)
	}
	return sess.View(), nil
}

// OpenNativeClient launches the vendor GUI, the fallback when the installed
// client cannot be driven programmatically. Pass a credential UUID to fill
// the vendor login fields (username + password) without clicking Connect.
func (a *App) OpenNativeClient(profileID string, credentialID string) error {
	if err := a.requireVendorClient(profileID, "open"); err != nil {
		return err
	}
	return a.describe(a.vpn.OpenNativeClient(a.ctx, profileID, credentialID))
}

// GetClientStartup reports whether the vendor GUI opens when the user logs in.
func (a *App) GetClientStartup(profileID string) (domain.StartupState, error) {
	if err := a.requireVendorClient(profileID, "start at login"); err != nil {
		return domain.StartupState{}, err
	}
	state, err := a.vpn.ClientStartup(a.ctx, profileID)
	return state, a.describe(err)
}

// SetClientStartup enables or disables the vendor GUI at login.
func (a *App) SetClientStartup(profileID string, enabled bool) (domain.StartupState, error) {
	if err := a.requireVendorClient(profileID, "start at login"); err != nil {
		return domain.StartupState{}, err
	}
	state, err := a.vpn.SetClientStartup(a.ctx, profileID, enabled)
	return state, a.describe(err)
}

// GetNetworkDiagnosis reports whether a live tunnel captures all traffic.
//
// An SSH tunnel has nothing to diagnose here: it installs no routes and
// captures nothing, which is exactly why it can be used where a full-tunnel VPN
// would take the whole machine with it.
func (a *App) GetNetworkDiagnosis(profileID string) (domain.NetworkDiagnosis, error) {
	p, err := a.profiles.Get(profileID)
	if err != nil {
		return domain.NetworkDiagnosis{}, a.describe(err)
	}
	if p.IsSSH() {
		return domain.NetworkDiagnosis{
			Known:   true,
			Summary: "This tunnel only lends local ports.",
			Detail: "An SSH tunnel installs no routes, so nothing else on this machine " +
				"is affected. Only programs that use the forwarded ports go through it.",
		}, nil
	}
	diag, err := a.vpn.DiagnoseNetwork(a.ctx, profileID)
	return diag, a.describe(err)
}

// RefreshStatus re-polls every profile whose client can report status.
func (a *App) RefreshStatus() []domain.SessionView {
	a.access.RefreshAll(a.ctx)
	sessions := a.sessions.All()
	out := make([]domain.SessionView, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.View())
	}
	return out
}

// SubmitOTP delivers a one-time code to the waiting connection attempt. The
// code is consumed in memory and never stored.
func (a *App) SubmitOTP(requestID string, code string) error {
	return a.bridge.resolveOTP(requestID, code)
}

// CancelInteraction abandons a pending prompt.
func (a *App) CancelInteraction(requestID string) error {
	return a.bridge.cancelInteraction(requestID)
}

// ----------------------------------------------------------------- providers

// ListProviderStatus reports detection and capabilities for every adapter.
func (a *App) ListProviderStatus() []domain.ProviderStatusView {
	return a.vpn.AllProviderStatus(a.ctx)
}

// RefreshProviderDetection re-probes the machine for installed clients.
func (a *App) RefreshProviderDetection() []domain.ProviderStatusView {
	a.vpn.InvalidateDetection()
	a.log.Info("Re-detecting installed VPN clients")
	return a.vpn.AllProviderStatus(a.ctx)
}

// ListVendorProfiles reports what the given provider already has configured on
// this machine, so the connection form can offer those instead of asking the
// user to type a name.
//
// Providers that cannot enumerate anything return an empty list rather than an
// error, which the form treats as "ask for a name".
func (a *App) ListVendorProfiles(provider string) ([]domain.VendorProfileOption, error) {
	options, err := a.vpn.ListVendorProfiles(a.ctx, domain.VPNProvider(strings.TrimSpace(provider)))
	if err != nil {
		return nil, a.describe(err)
	}
	return options, nil
}

// ---------------------------------------------------------------------- logs

// GetLogs returns the redacted activity log.
func (a *App) GetLogs() []logging.Entry { return a.log.Entries() }

// ClearLogs empties the activity log.
func (a *App) ClearLogs() { a.log.Clear() }

// ------------------------------------------------------------------ internal

func (a *App) buildConnectionView(p domain.VPNProfile) ConnectionView {
	prefs := a.profiles.Preferences(p.ID)
	view := ConnectionView{
		Profile:           p,
		ProviderName:      p.Provider.DisplayName(),
		Endpoint:          p.EndpointDisplay(),
		ManagesOwnAccount: p.Provider.ManagesOwnAccount(),
		Credentials:       a.credentials.ListForProfile(p.ID),
		State:             domain.StateDisconnected,
		LastCredentialID:  prefs.LastCredentialID,
		Hidden:            a.profiles.GroupHidden(p.Group),
	}
	view.SuggestedCredentialID = a.credentials.SelectDefault(p.ID, prefs.LastCredentialID)
	view.Requires = a.requirementOf(p)

	if s, ok := a.sessions.Get(p.ID); ok {
		sv := s.View()
		view.Session = &sv
		view.State = s.State
	}

	if p.IsSSH() {
		view.Capabilities = sshCapabilities(p)
		return view
	}

	status := a.vpn.ProviderStatus(a.ctx, p.Provider)
	view.Capabilities = status.Capabilities
	view.Client = status.Client
	return view
}

// requirementOf resolves the connection a profile needs up first.
//
// A reference that no longer resolves is reported as nothing rather than as an
// error: the profile service clears these on delete, so the only way to see one
// is a hand-edited file, and a missing name is not worth breaking the dashboard
// over. The Connect attempt still says what is wrong.
func (a *App) requirementOf(p domain.VPNProfile) *RequirementView {
	if p.Requires == "" {
		return nil
	}
	needed, err := a.profiles.Get(p.Requires)
	if err != nil {
		return nil
	}
	return &RequirementView{
		ProfileID: needed.ID,
		Name:      needed.Name,
		State:     a.sessions.State(needed.ID),
	}
}

// sshCapabilities describes what an SSH tunnel can do.
//
// Everything a VPN adapter has to discover about an installed client is already
// known here: the protocol is implemented in this process, so connecting and
// disconnecting always work and there is no vendor window to fall back to.
//
// What can differ is whether the tunnel has been set up yet. One that is still
// missing its server or its ports reports that it cannot connect, and says
// which part is missing, so the dashboard offers the reason in place of a
// button that could only fail.
func sshCapabilities(profile domain.VPNProfile) domain.Capabilities {
	capabilities := domain.Capabilities{
		Installed:        true,
		CanConnect:       true,
		CanDisconnect:    true,
		CanReadStatus:    true,
		SupportsPassword: true,
	}
	if err := profile.ReadyToConnect(); err != nil {
		capabilities.CanConnect = false
		capabilities.Notes = err.Error()
	}
	return capabilities
}

// requireVendorClient refuses an operation that only means something for
// software someone else installed.
func (a *App) requireVendorClient(profileID, operation string) error {
	p, err := a.profiles.Get(profileID)
	if err != nil {
		return a.describe(err)
	}
	if p.IsSSH() {
		return fmt.Errorf("%q is an SSH tunnel — there is no vendor client to %s", p.Name, operation)
	}
	return nil
}

// isSSH reports whether a profile is a tunnel this application holds itself. A
// profile that has gone missing answers false, so the caller's guard stays the
// stricter of the two paths.
func (a *App) isSSH(profileID string) bool {
	p, err := a.profiles.Get(profileID)
	return err == nil && p.IsSSH()
}

// describe turns an internal error into a message the UI can show, expanding
// unsupported-operation errors into actionable guidance.
func (a *App) describe(err error) error {
	if err == nil {
		return nil
	}
	if unsupported, ok := domain.AsUnsupported(err); ok {
		msg := fmt.Sprintf("%s cannot %s from Tobiro on %s. %s",
			unsupported.Provider.DisplayName(), unsupported.Operation,
			unsupported.Platform, unsupported.Reason)
		if unsupported.NativeUIFallback {
			msg += " Use \"Open native client\" to finish in the vendor app."
		}
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("%s", a.log.Redact(err.Error()))
}
