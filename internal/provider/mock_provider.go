package provider

import (
	"context"
	"sort"
	"sync"
)

// MockProvider stores desired Service state in memory for tests and local development.
type MockProvider struct {
	mu       sync.RWMutex
	services map[ServiceRef]Service
}

// NewMockProvider creates an empty in-memory provider implementation.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		services: make(map[ServiceRef]Service),
	}
}

// Ensure upserts the desired Service state.
func (p *MockProvider) Ensure(_ context.Context, service Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.services[service.Ref()] = service.DeepCopy()
	return nil
}

// Delete removes Service state and succeeds even when the entry does not exist.
func (p *MockProvider) Delete(_ context.Context, ref ServiceRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.services, ref)
	return nil
}

// Get returns a copy of the stored Service state when present.
func (p *MockProvider) Get(ref ServiceRef) (Service, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	service, ok := p.services[ref]
	if !ok {
		return Service{}, false
	}

	return service.DeepCopy(), true
}

// List returns all stored Service states sorted by namespace and name.
func (p *MockProvider) List() []Service {
	p.mu.RLock()
	defer p.mu.RUnlock()

	services := make([]Service, 0, len(p.services))
	for _, service := range p.services {
		services = append(services, service.DeepCopy())
	}

	sort.Slice(services, func(i, j int) bool {
		if services[i].Namespace == services[j].Namespace {
			return services[i].Name < services[j].Name
		}

		return services[i].Namespace < services[j].Namespace
	})

	return services
}

// Snapshot returns a copy of the in-memory state keyed by Service reference.
func (p *MockProvider) Snapshot() map[ServiceRef]Service {
	p.mu.RLock()
	defer p.mu.RUnlock()

	snapshot := make(map[ServiceRef]Service, len(p.services))
	for ref, service := range p.services {
		snapshot[ref] = service.DeepCopy()
	}

	return snapshot
}

var _ Provider = (*MockProvider)(nil)
