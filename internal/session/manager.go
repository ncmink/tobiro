// Package session tracks live VPN connection attempts and enforces the
// connection state machine.
package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"tobiro/internal/domain"
)

// Observer is notified whenever a session changes, so the UI can update.
type Observer func(domain.VPNSession)

// Manager owns the in-memory session table.
//
// Sessions are runtime state and deliberately not persisted: on restart the
// app re-detects reality from the vendor clients rather than trusting a stale
// file that claims a tunnel is up.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]domain.VPNSession // keyed by profile ID

	// observers is a list because a session change now has two audiences: this
	// application's own window, and any other biebie.net application waiting on
	// the same tunnel. Neither may be able to displace the other.
	observers []Observer
}

// NewManager creates a session manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]domain.VPNSession)}
}

// AddObserver registers a change callback.
//
// The list is replaced rather than appended to in place, so a snapshot taken
// under the lock stays safe to read after it is released — which is how every
// notification here avoids calling out while holding the lock.
func (m *Manager) AddObserver(fn Observer) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	m.observers = append(append(make([]Observer, 0, len(m.observers)+1), m.observers...), fn)
	m.mu.Unlock()
}

// Start opens a session for a profile, refusing to double-connect.
func (m *Manager) Start(profileID, credentialID string) (domain.VPNSession, error) {
	m.mu.Lock()
	if existing, ok := m.sessions[profileID]; ok && existing.State.IsActive() {
		m.mu.Unlock()
		return domain.VPNSession{}, fmt.Errorf("this connection is already %s", existing.State)
	}

	s := domain.VPNSession{
		ID:           uuid.NewString(),
		ProfileID:    profileID,
		CredentialID: credentialID,
		State:        domain.StateConnecting,
		StartedAt:    time.Now(),
	}
	m.sessions[profileID] = s
	observers := m.observers
	m.mu.Unlock()

	notify(observers, s)
	return s, nil
}

// Transition moves a session to a new state, rejecting illegal moves.
func (m *Manager) Transition(profileID string, to domain.ConnectionState) (domain.VPNSession, error) {
	return m.apply(profileID, to, nil)
}

// TransitionWith moves state and mutates session fields in the same lock.
func (m *Manager) TransitionWith(profileID string, to domain.ConnectionState, mutate func(*domain.VPNSession)) (domain.VPNSession, error) {
	return m.apply(profileID, to, mutate)
}

func (m *Manager) apply(profileID string, to domain.ConnectionState, mutate func(*domain.VPNSession)) (domain.VPNSession, error) {
	m.mu.Lock()
	current, ok := m.sessions[profileID]
	if !ok {
		m.mu.Unlock()
		return domain.VPNSession{}, fmt.Errorf("no active session for profile %s: %w", profileID, domain.ErrNotFound)
	}
	if !domain.CanTransition(current.State, to) {
		m.mu.Unlock()
		return domain.VPNSession{}, domain.InvalidTransitionError{From: current.State, To: to}
	}

	now := time.Now()
	current.State = to
	switch to {
	case domain.StateConnected:
		if current.ConnectedAt == nil {
			current.ConnectedAt = &now
		}
		current.EndedAt = nil
		current.Error = ""
	case domain.StateDisconnected, domain.StateFailed:
		current.EndedAt = &now
	}
	if mutate != nil {
		mutate(&current)
	}

	m.sessions[profileID] = current
	observers := m.observers
	m.mu.Unlock()

	notify(observers, current)
	return current, nil
}

// Fail moves a session to StateFailed with a reason.
func (m *Manager) Fail(profileID string, reason string) domain.VPNSession {
	s, err := m.TransitionWith(profileID, domain.StateFailed, func(s *domain.VPNSession) {
		s.Error = reason
	})
	if err != nil {
		// The session vanished or the move was illegal; synthesise a failed
		// record so the UI still reflects the outcome.
		return domain.VPNSession{ProfileID: profileID, State: domain.StateFailed, Error: reason}
	}
	return s
}

// Get returns the session for a profile.
func (m *Manager) Get(profileID string) (domain.VPNSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[profileID]
	return s, ok
}

// State reports the current state, defaulting to disconnected.
func (m *Manager) State(profileID string) domain.ConnectionState {
	if s, ok := m.Get(profileID); ok {
		return s.State
	}
	return domain.StateDisconnected
}

// All returns every tracked session.
func (m *Manager) All() []domain.VPNSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.VPNSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// Remove drops a session record entirely.
func (m *Manager) Remove(profileID string) {
	m.mu.Lock()
	delete(m.sessions, profileID)
	m.mu.Unlock()
}

// SyncFromStatus reconciles tracked state with a reading from the vendor
// client. Readings the adapter marked unknown are ignored so a client that
// cannot report status never clobbers what we know.
func (m *Manager) SyncFromStatus(profileID string, status domain.VPNStatus) {
	if !status.Known {
		return
	}

	m.mu.Lock()
	current, ok := m.sessions[profileID]
	if !ok {
		// The tunnel was raised outside Tobiro; adopt it only if the
		// client reports it as up.
		if status.State == domain.StateConnected {
			now := time.Now()
			adopted := domain.VPNSession{
				ID:          uuid.NewString(),
				ProfileID:   profileID,
				State:       domain.StateConnected,
				StartedAt:   now,
				ConnectedAt: &now,
				AssignedIP:  status.AssignedIP,
				Interface:   status.Interface,
				Identity:    status.Identity,
			}
			m.sessions[profileID] = adopted
			observers := m.observers
			m.mu.Unlock()
			notify(observers, adopted)
			return
		}
		m.mu.Unlock()
		return
	}

	// A session that already ended (disconnected/failed) is the same as "no
	// session" for observation: the vendor GUI coming back up must be able to
	// re-adopt, not sit forever in disconnected because user-initiated
	// Connect() is the only path to connecting.
	if (current.State == domain.StateDisconnected || current.State == domain.StateFailed) &&
		status.State == domain.StateConnected {
		now := time.Now()
		current.State = domain.StateConnected
		current.StartedAt = now
		current.ConnectedAt = &now
		current.EndedAt = nil
		current.Error = ""
		applyStatusFacts(&current, status)
		m.sessions[profileID] = current
		observers := m.observers
		m.mu.Unlock()
		notify(observers, current)
		return
	}

	// Mid-authentication states are ours to own; the client cannot see them.
	if current.State == domain.StateWaitingForOTP || current.State == domain.StateAuthenticating {
		m.mu.Unlock()
		return
	}
	if current.State == status.State {
		changed := applyStatusFacts(&current, status)
		m.sessions[profileID] = current
		observers := m.observers
		m.mu.Unlock()
		if changed {
			notify(observers, current)
		}
		return
	}
	if !domain.CanTransition(current.State, status.State) {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	_, _ = m.TransitionWith(profileID, status.State, func(s *domain.VPNSession) {
		applyStatusFacts(s, status)
	})
}

func applyStatusFacts(s *domain.VPNSession, status domain.VPNStatus) bool {
	changed := false
	if status.AssignedIP != "" && status.AssignedIP != s.AssignedIP {
		s.AssignedIP = status.AssignedIP
		changed = true
	}
	if status.Interface != "" && status.Interface != s.Interface {
		s.Interface = status.Interface
		changed = true
	}
	if status.Identity != "" && status.Identity != s.Identity {
		s.Identity = status.Identity
		changed = true
	}
	return changed
}

func notify(observers []Observer, s domain.VPNSession) {
	for _, observe := range observers {
		observe(s)
	}
}
