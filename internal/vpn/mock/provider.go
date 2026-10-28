// Package mock provides a fully simulated VPN client.
//
// It exists so the connection state machine, OTP interaction and failure
// handling can be exercised end to end on any machine, with no vendor software
// installed. It is also the reference for what a real adapter must do.
package mock

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/vpn"
)

// Provider simulates a vendor client.
type Provider struct {
	mu    sync.Mutex
	state map[string]*tunnel // keyed by profile ID
}

type tunnel struct {
	connected  bool
	assignedIP string
	iface      string
	since      time.Time
}

// New creates the mock provider.
func New() *Provider {
	return &Provider{state: make(map[string]*tunnel)}
}

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderMock }

// Detect always reports the simulated client as present.
func (p *Provider) Detect(_ context.Context) (*domain.ClientInfo, error) {
	return &domain.ClientInfo{
		Provider:   domain.ProviderMock,
		Installed:  true,
		Version:    "1.0.0-mock",
		Path:       "(built in)",
		Platform:   "all",
		DetectedAt: time.Now().Format(time.RFC3339),
		Detail:     "Simulated client for development. No network traffic is tunnelled.",
	}, nil
}

// Capabilities reports full programmatic control.
func (p *Provider) Capabilities(_ context.Context) (*domain.Capabilities, error) {
	return &domain.Capabilities{
		Installed:           true,
		CLIAvailable:        true,
		CanConnect:          true,
		CanDisconnect:       true,
		CanReadStatus:       true,
		SupportsPassword:    true,
		SupportsOTP:         true,
		RequiresNativeUI:    false,
		CanOpenNativeClient: false,
		Notes:               "Simulated client: connects instantly and tunnels nothing.",
	}, nil
}

// Connect walks the full state machine, including an OTP round trip when the
// credential's auth method asks for one.
func (p *Provider) Connect(ctx context.Context, profile domain.VPNProfile, credential domain.Credential) error {
	vpn.ReportState(ctx, domain.StateConnecting, fmt.Sprintf("Dialling %s", profile.Endpoint()))
	if err := sleep(ctx, 700*time.Millisecond); err != nil {
		return err
	}

	vpn.ReportState(ctx, domain.StateAuthenticating, "Authentication requested")
	if err := sleep(ctx, 500*time.Millisecond); err != nil {
		return err
	}

	// Deterministic failure hook so the failure path is demonstrable.
	if strings.EqualFold(credential.Username, "fail") {
		return fmt.Errorf("authentication rejected by the gateway")
	}
	if credential.RequiresSecret() && credential.Secret == "" {
		return fmt.Errorf("no password was supplied for this account")
	}

	if credential.AuthMethod == domain.AuthPasswordOTP {
		if err := p.requestOTP(ctx, profile); err != nil {
			return err
		}
	}

	if err := sleep(ctx, 400*time.Millisecond); err != nil {
		return err
	}

	t := &tunnel{
		connected:  true,
		assignedIP: simulatedIP(profile.ID),
		iface:      "utun-mock",
		since:      time.Now(),
	}
	p.mu.Lock()
	p.state[profile.ID] = t
	p.mu.Unlock()
	return nil
}

func (p *Provider) requestOTP(ctx context.Context, profile domain.VPNProfile) error {
	prompter, ok := vpn.PrompterFrom(ctx)
	if !ok {
		return fmt.Errorf("this account requires a one-time code, but no prompt is available")
	}

	vpn.ReportState(ctx, domain.StateWaitingForOTP, "Waiting for one-time code")
	code, err := prompter.RequestOTP(ctx, domain.InteractionRequest{
		ProfileID: profile.ID,
		Kind:      domain.InteractionOTP,
		Prompt:    "One-time code",
		Message:   fmt.Sprintf("Enter the 6-digit code for %s.", profile.Name),
	})
	if err != nil {
		return err
	}

	// The code is validated and immediately discarded — never stored, never
	// logged, never echoed back to the UI.
	if len(strings.TrimSpace(code)) < 6 {
		return fmt.Errorf("the one-time code was rejected")
	}
	if strings.TrimSpace(code) == "000000" {
		return fmt.Errorf("the one-time code was rejected")
	}

	vpn.ReportState(ctx, domain.StateAuthenticating, "One-time code accepted")
	return nil
}

// Disconnect tears down the simulated tunnel.
func (p *Provider) Disconnect(ctx context.Context, profile domain.VPNProfile) error {
	if err := sleep(ctx, 400*time.Millisecond); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.state, profile.ID)
	p.mu.Unlock()
	return nil
}

// Status reports the simulated tunnel state.
func (p *Provider) Status(_ context.Context, profile domain.VPNProfile) (*domain.VPNStatus, error) {
	p.mu.Lock()
	t, ok := p.state[profile.ID]
	p.mu.Unlock()

	if !ok || !t.connected {
		return &domain.VPNStatus{State: domain.StateDisconnected, Known: true, Detail: "Simulated tunnel is down"}, nil
	}
	return &domain.VPNStatus{
		State:      domain.StateConnected,
		AssignedIP: t.assignedIP,
		Interface:  t.iface,
		Known:      true,
		Detail:     "Simulated tunnel is up",
	}, nil
}

// OpenNativeClient has nothing to open.
func (p *Provider) OpenNativeClient(_ context.Context, _ domain.VPNProfile) error {
	return &domain.UnsupportedOperationError{
		Provider:  domain.ProviderMock,
		Operation: "open native client",
		Platform:  "all",
		Reason:    "the simulated client has no GUI",
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// simulatedIP derives a stable address per profile so the UI shows something
// consistent across reconnects.
func simulatedIP(profileID string) string {
	sum := sha256.Sum256([]byte(profileID))
	return fmt.Sprintf("172.30.%d.%d", int(sum[0])%254+1, int(sum[1])%254+1)
}
