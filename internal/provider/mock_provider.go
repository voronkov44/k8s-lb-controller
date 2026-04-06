/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
func (p *MockProvider) Ensure(_ context.Context, service Service) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if current, ok := p.services[service.Ref()]; ok && current.Equal(service) {
		return false, nil
	}

	p.services[service.Ref()] = service.DeepCopy()
	return true, nil
}

// Delete removes Service state and succeeds even when the entry does not exist.
func (p *MockProvider) Delete(_ context.Context, ref ServiceRef) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.services[ref]; !ok {
		return false, nil
	}

	delete(p.services, ref)
	return true, nil
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
