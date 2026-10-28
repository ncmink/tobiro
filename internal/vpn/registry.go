package vpn

import (
	"fmt"
	"sort"
	"sync"

	"tobiro/internal/domain"
)

// Registry maps provider keys to adapters.
//
// New vendors are added by registering an adapter — the manager, services and
// UI need no modification:
//
//	registry.Register(forcepoint.New())
//	registry.Register(ivanti.New())
//	registry.Register(forticlient.New())
//	registry.Register(tailscale.New())
type Registry struct {
	mu        sync.RWMutex
	providers map[domain.VPNProvider]VPNProviderAdapter
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[domain.VPNProvider]VPNProviderAdapter)}
}

// Register adds an adapter, replacing any previous one with the same ID.
func (r *Registry) Register(adapter VPNProviderAdapter) {
	r.mu.Lock()
	r.providers[adapter.ID()] = adapter
	r.mu.Unlock()
}

// Get returns the adapter for a provider key.
func (r *Registry) Get(id domain.VPNProvider) (VPNProviderAdapter, error) {
	r.mu.RLock()
	adapter, ok := r.providers[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no adapter registered for provider %q: %w", id, domain.ErrNotFound)
	}
	return adapter, nil
}

// IDs lists registered provider keys in stable order.
func (r *Registry) IDs() []domain.VPNProvider {
	r.mu.RLock()
	ids := make([]domain.VPNProvider, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// All returns every registered adapter in stable order.
func (r *Registry) All() []VPNProviderAdapter {
	ids := r.IDs()
	out := make([]VPNProviderAdapter, 0, len(ids))
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range ids {
		out = append(out, r.providers[id])
	}
	return out
}
