package vpn

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/session"
	"tobiro/internal/vpn/tailscale"
	"tobiro/internal/vpn/tunnel"
)

// ProfileReader is the slice of the profile service the manager needs.
type ProfileReader interface {
	Get(id string) (domain.VPNProfile, error)
	List() []domain.VPNProfile
	RememberCredential(profileID, credentialID string) error
}

// CredentialResolver materialises a credential's secret for one connect call.
type CredentialResolver interface {
	Get(id string) (domain.Credential, error)
	Resolve(credentialID string) (domain.Credential, error)
}

// Manager orchestrates connections across providers.
//
// It is deliberately vendor-agnostic: it validates, resolves credentials,
// drives the state machine and handles fallbacks, while every vendor-specific
// command lives in an adapter.
type Manager struct {
	registry    *Registry
	profiles    ProfileReader
	credentials CredentialResolver
	sessions    *session.Manager
	log         *logging.Logger
	prompter    Prompter

	mu       sync.Mutex
	inflight map[string]context.CancelFunc
	downMiss map[string]int
	bursts   map[string]context.CancelFunc
	burstSeq uint64
	burstGen map[string]uint64

	capCache sync.Map // domain.VPNProvider -> *cachedCapabilities
}

type cachedCapabilities struct {
	caps   *domain.Capabilities
	client *domain.ClientInfo
	at     time.Time
}

const (
	capabilityTTL           = 30 * time.Second
	watchInterval           = 2 * time.Second
	burstInterval           = 400 * time.Millisecond
	burstDuration           = 45 * time.Second
	disconnectConfirmations = 3
)

// NewManager wires the manager.
func NewManager(
	registry *Registry,
	profiles ProfileReader,
	credentials CredentialResolver,
	sessions *session.Manager,
	log *logging.Logger,
) *Manager {
	return &Manager{
		registry:    registry,
		profiles:    profiles,
		credentials: credentials,
		sessions:    sessions,
		log:         log,
		inflight:    make(map[string]context.CancelFunc),
		downMiss:    make(map[string]int),
		bursts:      make(map[string]context.CancelFunc),
		burstGen:    make(map[string]uint64),
	}
}

// SetPrompter installs the UI bridge used for OTP and other interactions.
func (m *Manager) SetPrompter(p Prompter) { m.prompter = p }

// stateReporter adapts session transitions for adapters.
type stateReporter struct {
	manager   *Manager
	profileID string
}

func (r stateReporter) Report(state domain.ConnectionState, detail string) {
	if _, err := r.manager.sessions.Transition(r.profileID, state); err != nil {
		r.manager.log.Warn("Ignoring state report %s for profile=%s: %v", state, r.profileID, err)
		return
	}
	if detail != "" {
		r.manager.log.Info("profile=%s %s", r.profileID, detail)
	}
}

// Connect establishes a tunnel for a profile using one credential UUID.
//
// The password is read from the OS credential store inside this call, handed
// straight to the adapter, and released when the call returns. It is never
// returned to the frontend and is registered with the logger so it cannot
// appear in any log line.
func (m *Manager) Connect(ctx context.Context, profileID, credentialID string) (domain.VPNSession, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return domain.VPNSession{}, err
	}

	caps, _, err := m.capabilities(ctx, adapter)
	if err != nil {
		return domain.VPNSession{}, err
	}
	if !caps.Installed {
		return domain.VPNSession{}, fmt.Errorf("%s is not installed on this machine: %w",
			profile.Provider.DisplayName(), domain.ErrClientNotInstalled)
	}
	if !caps.CanConnect {
		// Refuse rather than guess at undocumented CLI arguments.
		return domain.VPNSession{}, &domain.UnsupportedOperationError{
			Provider:         profile.Provider,
			Operation:        "connect",
			Platform:         runtime.GOOS,
			Reason:           defaultString(caps.Notes, "the installed client offers no supported command-line connect"),
			NativeUIFallback: caps.CanOpenNativeClient,
		}
	}

	if credentialID != "" {
		credential, err := m.credentials.Resolve(credentialID)
		if err != nil {
			return domain.VPNSession{}, err
		}
		if credential.ProfileID != profileID {
			return domain.VPNSession{}, fmt.Errorf("credential %s does not belong to this connection", credentialID)
		}
		return m.connectWith(ctx, profile, adapter, caps, credential)
	}
	if caps.SupportsPassword {
		return domain.VPNSession{}, fmt.Errorf("choose an account to connect with")
	}
	return m.connectWith(ctx, profile, adapter, caps, domain.Credential{ProfileID: profileID})
}

func (m *Manager) connectWith(ctx context.Context, profile domain.VPNProfile, adapter VPNProviderAdapter, caps *domain.Capabilities, credential domain.Credential) (domain.VPNSession, error) {
	profileID := profile.ID
	credentialID := credential.ID

	sess, err := m.sessions.Start(profileID, credentialID)
	if err != nil {
		return domain.VPNSession{}, err
	}

	// Identify the credential by UUID in logs, never by password.
	m.log.Info("Connecting profile=%s provider=%s endpoint=%s credential=%s",
		profile.Name, profile.Provider, profile.Endpoint(), defaultString(credential.ID, "none"))

	release := m.log.Protect(credential.Secret)

	connectCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.inflight[profileID] = cancel
	m.mu.Unlock()

	connectCtx = WithStateReporter(connectCtx, stateReporter{manager: m, profileID: profileID})
	if m.prompter != nil {
		connectCtx = WithPrompter(connectCtx, m.prompter)
	}

	defer func() {
		release()
		cancel()
		m.mu.Lock()
		delete(m.inflight, profileID)
		m.mu.Unlock()
	}()

	if err := adapter.Connect(connectCtx, profile, credential); err != nil {
		reason := m.log.Redact(err.Error())
		failed := m.sessions.Fail(profileID, reason)
		m.log.Error("Connection failed profile=%s: %s", profile.Name, reason)
		if m.prompter != nil {
			m.prompter.Notify(domain.EventConnectionFailed, domain.ConnectionFailedEvent{
				SessionID: failed.ID, ProfileID: profileID, Reason: reason,
			})
		}
		return failed, err
	}

	// Enrich the session with whatever the client can tell us.
	assignedIP, iface, identity := "", "", ""
	if caps.CanReadStatus {
		if status, statusErr := adapter.Status(ctx, profile); statusErr == nil && status != nil && status.Known {
			if status.State != domain.StateConnected {
				reason := strings.TrimSpace(status.Detail)
				if reason == "" {
					reason = "the tunnel is not connected for this connection"
				}
				failed := m.sessions.Fail(profileID, reason)
				m.log.Error("Connect finished but profile=%s is not connected: %s", profile.Name, reason)
				return failed, fmt.Errorf("%s", reason)
			}
			assignedIP, iface, identity = status.AssignedIP, status.Interface, status.Identity
		}
	}

	connected, err := m.sessions.TransitionWith(profileID, domain.StateConnected, func(s *domain.VPNSession) {
		if assignedIP != "" {
			s.AssignedIP = assignedIP
		}
		if iface != "" {
			s.Interface = iface
		}
		if identity != "" {
			s.Identity = identity
		}
	})
	if err != nil {
		// The adapter reported success but the machine disagrees; surface it
		// rather than showing a tunnel that is not there.
		m.log.Warn("Connected but could not record state for profile=%s: %v", profile.Name, err)
		connected, _ = m.sessions.Get(profileID)
	}

	if credentialID != "" {
		if err := m.profiles.RememberCredential(profileID, credentialID); err != nil {
			m.log.Warn("Could not remember last account for profile=%s: %v", profile.Name, err)
		}
	}

	m.log.Info("Connected profile=%s interface=%s ip=%s",
		profile.Name, defaultString(connected.Interface, "unknown"), defaultString(connected.AssignedIP, "unknown"))
	if profile.Provider == domain.ProviderTailscale {
		m.refreshTailscaleProfiles(ctx)
	}
	_ = sess
	return connected, nil
}

func (m *Manager) refreshTailscaleProfiles(ctx context.Context) {
	var profiles []domain.VPNProfile
	for _, p := range m.profiles.List() {
		if p.Provider == domain.ProviderTailscale {
			profiles = append(profiles, p)
		}
	}
	m.refreshTailscaleStatuses(ctx, profiles)
}

// Disconnect tears down a tunnel.
func (m *Manager) Disconnect(ctx context.Context, profileID string) (domain.VPNSession, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return domain.VPNSession{}, err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return domain.VPNSession{}, err
	}

	// Cancel an in-flight connect (e.g. the user gave up at the OTP prompt).
	m.mu.Lock()
	if cancel, ok := m.inflight[profileID]; ok {
		cancel()
	}
	m.mu.Unlock()

	caps, _, err := m.capabilities(ctx, adapter)
	if err != nil {
		return domain.VPNSession{}, err
	}
	if !caps.CanDisconnect {
		return domain.VPNSession{}, &domain.UnsupportedOperationError{
			Provider:         profile.Provider,
			Operation:        "disconnect",
			Platform:         runtime.GOOS,
			Reason:           defaultString(caps.Notes, "the installed client offers no supported command-line disconnect"),
			NativeUIFallback: caps.CanOpenNativeClient,
		}
	}

	if _, err := m.sessions.Transition(profileID, domain.StateDisconnecting); err != nil {
		m.log.Warn("Disconnect requested for profile=%s in an unexpected state: %v", profile.Name, err)
	}
	m.log.Info("Disconnecting profile=%s", profile.Name)

	if err := adapter.Disconnect(ctx, profile); err != nil {
		reason := m.log.Redact(err.Error())
		m.log.Error("Disconnect failed profile=%s: %s", profile.Name, reason)
		return m.sessions.Fail(profileID, reason), err
	}

	final, err := m.sessions.TransitionWith(profileID, domain.StateDisconnected, func(s *domain.VPNSession) {
		s.AssignedIP = ""
		s.Interface = ""
		s.Identity = ""
	})
	if err != nil {
		final = domain.VPNSession{ProfileID: profileID, State: domain.StateDisconnected}
	}
	m.log.Info("Disconnected profile=%s", profile.Name)
	if profile.Provider == domain.ProviderTailscale {
		m.refreshTailscaleProfiles(ctx)
	}
	return final, nil
}

// RefreshStatus polls the vendor client and reconciles the session table.
func (m *Manager) RefreshStatus(ctx context.Context, profile domain.VPNProfile) {
	if profile.Provider == domain.ProviderTailscale {
		return
	}
	m.syncProfileStatus(ctx, profile)
}

func (m *Manager) syncProfileStatus(ctx context.Context, profile domain.VPNProfile) {
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return
	}
	status, err := adapter.Status(ctx, profile)
	if err != nil || status == nil || !status.Known {
		return
	}
	m.applyVPNStatus(profile, *status)
}

func (m *Manager) applyVPNStatus(profile domain.VPNProfile, status domain.VPNStatus) {
	previous, hadSession := m.sessions.Get(profile.ID)
	previousState := domain.StateDisconnected
	if hadSession {
		previousState = previous.State
	}
	if !m.confirmStatus(profile.ID, status) {
		return
	}
	m.sessions.SyncFromStatus(profile.ID, status)
	if next, ok := m.sessions.Get(profile.ID); ok && next.State != previousState {
		m.log.Info("Status profile=%s provider=%s %s → %s ip=%s iface=%s",
			profile.Name, profile.Provider, previousState, next.State,
			defaultString(next.AssignedIP, "none"), defaultString(next.Interface, "none"))
	}
}

func (m *Manager) refreshTailscaleStatuses(ctx context.Context, profiles []domain.VPNProfile) {
	if len(profiles) == 0 {
		return
	}
	adapter, err := m.registry.Get(domain.ProviderTailscale)
	if err != nil {
		return
	}
	ts, ok := adapter.(*tailscale.Provider)
	if !ok {
		for _, p := range profiles {
			m.syncProfileStatus(ctx, p)
		}
		return
	}
	tailscale.ReconcileProfiles(ctx, ts, profiles, m.sessions, m.applyVPNStatus)
}

// RefreshAll probes every stored profile. The Go-side watcher calls this so
// status does not depend on the webview's timer staying awake.
func (m *Manager) RefreshAll(ctx context.Context) {
	all := m.profiles.List()
	var tailscaleProfiles []domain.VPNProfile
	for _, profile := range all {
		if profile.Provider == domain.ProviderTailscale {
			tailscaleProfiles = append(tailscaleProfiles, profile)
			continue
		}
		m.syncProfileStatus(ctx, profile)
	}
	m.refreshTailscaleStatuses(ctx, tailscaleProfiles)
}

// StartWatching polls tunnel status until ctx is cancelled.
func (m *Manager) StartWatching(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(watchInterval)
		defer ticker.Stop()
		m.RefreshAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.RefreshAll(ctx)
			}
		}
	}()
}

// confirmStatus applies hysteresis so a single missed netstat sample cannot
// bounce a live tunnel back to Disconnected. Going *up* is applied immediately.
func (m *Manager) confirmStatus(profileID string, status domain.VPNStatus) bool {
	if !status.Known {
		return false
	}

	if status.State == domain.StateConnected {
		m.mu.Lock()
		m.downMiss[profileID] = 0
		m.mu.Unlock()
		return true
	}

	current, active := m.sessions.Get(profileID)
	if !active || current.State != domain.StateConnected {
		m.mu.Lock()
		m.downMiss[profileID] = 0
		m.mu.Unlock()
		return true
	}

	m.mu.Lock()
	m.downMiss[profileID]++
	streak := m.downMiss[profileID]
	if streak < disconnectConfirmations {
		m.mu.Unlock()
		return false
	}
	m.downMiss[profileID] = 0
	m.mu.Unlock()
	return true
}

// OpenNativeClient launches the vendor GUI only. Use FillNativeLogin when the
// user has the login form on screen and presses Fill in Tobiro.
func (m *Manager) OpenNativeClient(ctx context.Context, profileID, credentialID string) error {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return err
	}
	if credentialID != "" {
		if _, err := m.resolveForFill(profileID, credentialID); err != nil {
			return err
		}
	}

	m.log.Info("Opening native client for profile=%s provider=%s", profile.Name, profile.Provider)
	if err := adapter.OpenNativeClient(ctx, profile); err != nil {
		return err
	}

	m.WatchAfterOpen(ctx, profile)
	return nil
}

// FillNativeLogin keystrokes username and password into the vendor field that
// already has keyboard focus when the user presses ⌘I. See nativefill on macOS.
func (m *Manager) FillNativeLogin(ctx context.Context, profileID, credentialID string) error {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return err
	}
	filler, ok := adapter.(LoginFiller)
	if !ok || runtime.GOOS != "darwin" {
		return fmt.Errorf("%s cannot fill login fields from Tobiro on this platform", profile.Provider.DisplayName())
	}

	credential, err := m.resolveForFill(profileID, credentialID)
	if err != nil {
		return err
	}
	if !canFillLogin(credential) {
		return fmt.Errorf("choose an account with a username or saved password")
	}

	release := m.log.Protect(credential.Secret)
	defer release()

	err = filler.FillNativeLogin(ctx, profile, credential)
	if err != nil {
		return err
	}
	m.log.Info("Filled login fields in the native client for credential=%s", credential.ID)
	_ = m.profiles.RememberCredential(profileID, credential.ID)
	return nil
}

func (m *Manager) startupController(profileID string) (StartupController, domain.VPNProfile, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return nil, domain.VPNProfile{}, err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return nil, profile, err
	}
	controller, ok := adapter.(StartupController)
	if !ok {
		return nil, profile, fmt.Errorf("%s cannot change start-at-login from Tobiro", profile.Provider.DisplayName())
	}
	return controller, profile, nil
}

// ClientStartup reports whether the vendor GUI opens when the user logs in.
func (m *Manager) ClientStartup(ctx context.Context, profileID string) (domain.StartupState, error) {
	controller, _, err := m.startupController(profileID)
	if err != nil {
		return domain.StartupState{}, err
	}
	return controller.StartupAtLogin(ctx)
}

// SetClientStartup enables or disables the vendor GUI at login.
func (m *Manager) SetClientStartup(ctx context.Context, profileID string, enabled bool) (domain.StartupState, error) {
	controller, profile, err := m.startupController(profileID)
	if err != nil {
		return domain.StartupState{}, err
	}
	if err := controller.SetStartupAtLogin(ctx, enabled); err != nil {
		return domain.StartupState{}, err
	}
	state, err := controller.StartupAtLogin(ctx)
	if err != nil {
		return domain.StartupState{}, err
	}
	if enabled {
		m.log.Info("Enabled start at login for profile=%s provider=%s", profile.Name, profile.Provider)
	} else {
		m.log.Info("Disabled start at login for profile=%s provider=%s", profile.Name, profile.Provider)
	}
	return state, nil
}

// DiagnoseNetwork reports how a live tunnel affects other programs.
func (m *Manager) DiagnoseNetwork(ctx context.Context, profileID string) (domain.NetworkDiagnosis, error) {
	profile, err := m.profiles.Get(profileID)
	if err != nil {
		return domain.NetworkDiagnosis{}, err
	}
	adapter, err := m.registry.Get(profile.Provider)
	if err != nil {
		return domain.NetworkDiagnosis{}, err
	}
	doctor, ok := adapter.(NetworkDoctor)
	if !ok {
		return domain.NetworkDiagnosis{Known: false}, nil
	}
	return doctor.DiagnoseNetwork(ctx, profile)
}

// ProfileForNativeProcess returns the profile whose vendor app owns the named
// process, preferring preferredID when it matches. Empty means no match.
func (m *Manager) ProfileForNativeProcess(processName, preferredID string) string {
	if processName == "" {
		return ""
	}
	owns := func(p domain.VPNProfile) bool {
		adapter, err := m.registry.Get(p.Provider)
		if err != nil {
			return false
		}
		owner, ok := adapter.(NativeProcessOwner)
		return ok && owner.OwnsNativeProcess(processName)
	}
	if preferredID != "" {
		if p, err := m.profiles.Get(preferredID); err == nil && owns(p) {
			return p.ID
		}
	}
	for _, p := range m.profiles.List() {
		if owns(p) {
			return p.ID
		}
	}
	return ""
}

func (m *Manager) resolveForFill(profileID, credentialID string) (domain.Credential, error) {
	if credentialID == "" {
		return domain.Credential{}, nil
	}
	cred, err := m.credentials.Get(credentialID)
	if err != nil {
		return domain.Credential{}, err
	}
	if cred.ProfileID != profileID {
		return domain.Credential{}, fmt.Errorf("credential %s does not belong to this connection", credentialID)
	}
	resolved, err := m.credentials.Resolve(credentialID)
	if err != nil {
		if cred.RequiresSecret() {
			return domain.Credential{}, err
		}
		m.log.Info("No stored password for credential=%s; filling username only", credentialID)
		return cred, nil
	}
	return resolved, nil
}

func canFillLogin(credential domain.Credential) bool {
	return strings.TrimSpace(credential.Username) != "" || credential.Secret != ""
}

// WatchAfterOpen invalidates the probe cache and polls this profile rapidly
// until the vendor client comes up, or until burstDuration elapses.
func (m *Manager) WatchAfterOpen(ctx context.Context, profile domain.VPNProfile) {
	tunnel.Invalidate()
	m.RefreshStatus(ctx, profile)

	m.mu.Lock()
	if cancel, ok := m.bursts[profile.ID]; ok {
		cancel()
	}
	m.burstSeq++
	seq := m.burstSeq
	m.burstGen[profile.ID] = seq
	burstCtx, cancel := context.WithTimeout(ctx, burstDuration)
	m.bursts[profile.ID] = cancel
	m.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			m.mu.Lock()
			if m.burstGen[profile.ID] == seq {
				delete(m.bursts, profile.ID)
				delete(m.burstGen, profile.ID)
			}
			m.mu.Unlock()
		}()

		ticker := time.NewTicker(burstInterval)
		defer ticker.Stop()
		for {
			select {
			case <-burstCtx.Done():
				return
			case <-ticker.C:
				tunnel.Invalidate()
				m.RefreshStatus(burstCtx, profile)
				if session, ok := m.sessions.Get(profile.ID); ok && session.State == domain.StateConnected {
					return
				}
			}
		}
	}()
}

// ProviderStatus reports detection + capabilities for one provider.
func (m *Manager) ProviderStatus(ctx context.Context, id domain.VPNProvider) domain.ProviderStatusView {
	view := domain.ProviderStatusView{
		Provider:    id,
		DisplayName: id.DisplayName(),
	}
	adapter, err := m.registry.Get(id)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	caps, info, err := m.capabilities(ctx, adapter)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	if info != nil {
		view.Client = *info
	}
	if caps != nil {
		view.Capabilities = *caps
	}
	return view
}

// AllProviderStatus reports detection for every registered adapter.
func (m *Manager) AllProviderStatus(ctx context.Context) []domain.ProviderStatusView {
	ids := m.registry.IDs()
	out := make([]domain.ProviderStatusView, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.ProviderStatus(ctx, id))
	}
	return out
}

// ListVendorProfiles asks an adapter what is already configured on this machine,
// for the connection form to offer as choices.
//
// Most adapters cannot enumerate anything — no vendor here documents a way to
// read its tunnel list — so an empty result is the normal answer and not an
// error. The form falls back to asking for a name.
func (m *Manager) ListVendorProfiles(ctx context.Context, id domain.VPNProvider) ([]domain.VendorProfileOption, error) {
	adapter, err := m.registry.Get(id)
	if err != nil {
		return nil, err
	}

	lister, ok := adapter.(VendorProfileLister)
	if !ok {
		return nil, nil
	}
	return lister.ListVendorProfiles(ctx)
}

// InvalidateDetection clears the capability cache so Settings can re-probe
// after the user installs a client.
func (m *Manager) InvalidateDetection() {
	m.capCache.Range(func(key, _ any) bool {
		m.capCache.Delete(key)
		return true
	})
}

// capabilities probes an adapter, caching briefly so a dashboard render does
// not shell out repeatedly.
func (m *Manager) capabilities(ctx context.Context, adapter VPNProviderAdapter) (*domain.Capabilities, *domain.ClientInfo, error) {
	if cached, ok := m.capCache.Load(adapter.ID()); ok {
		entry := cached.(*cachedCapabilities)
		if time.Since(entry.at) < capabilityTTL {
			return entry.caps, entry.client, nil
		}
	}

	info, err := adapter.Detect(ctx)
	if err != nil && !errors.Is(err, domain.ErrClientNotInstalled) {
		return nil, nil, fmt.Errorf("detect %s: %w", adapter.ID().DisplayName(), err)
	}
	caps, err := adapter.Capabilities(ctx)
	if err != nil {
		return nil, info, fmt.Errorf("read capabilities for %s: %w", adapter.ID().DisplayName(), err)
	}

	m.capCache.Store(adapter.ID(), &cachedCapabilities{caps: caps, client: info, at: time.Now()})
	return caps, info, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
