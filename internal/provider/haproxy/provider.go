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

package haproxy

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/voronkov44/k8s-lb-controller/internal/dataplane"
	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// Provider stores desired Service state in memory and materializes it as an HAProxy config file.
type Provider struct {
	mu       sync.Mutex
	applier  *dataplane.Applier
	services map[provider.ServiceRef]provider.Service
}

// NewProvider creates a file-based HAProxy provider.
func NewProvider(cfg Config) (*Provider, error) {
	applier, err := dataplane.NewApplier(dataplane.ApplyConfig{
		ConfigPath:      cfg.ConfigPath,
		ValidateCommand: cfg.ValidateCommand,
		ReloadCommand:   cfg.ReloadCommand,
	})
	if err != nil {
		return nil, err
	}

	return &Provider{
		applier:  applier,
		services: make(map[provider.ServiceRef]provider.Service),
	}, nil
}

// Ensure upserts a Service entry and applies the aggregate config only when the rendered output changes.
func (p *Provider) Ensure(ctx context.Context, service provider.Service) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextState := cloneServices(p.services)
	nextState[service.Ref()] = service.DeepCopy()

	changed, err := p.apply(ctx, nextState)
	if err != nil {
		return false, err
	}

	p.services = nextState
	return changed, nil
}

// Delete removes a Service entry and applies the aggregate config only when the rendered output changes.
func (p *Provider) Delete(ctx context.Context, ref provider.ServiceRef) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextState := cloneServices(p.services)
	delete(nextState, ref)

	changed, err := p.apply(ctx, nextState)
	if err != nil {
		return false, err
	}

	p.services = nextState
	return changed, nil
}

func (p *Provider) apply(ctx context.Context, services map[provider.ServiceRef]provider.Service) (bool, error) {
	return p.applier.Apply(ctx, servicesToList(services))
}

func cloneServices(services map[provider.ServiceRef]provider.Service) map[provider.ServiceRef]provider.Service {
	cloned := make(map[provider.ServiceRef]provider.Service, len(services))
	for ref, service := range services {
		cloned[ref] = service.DeepCopy()
	}

	return cloned
}

func servicesToList(services map[provider.ServiceRef]provider.Service) []provider.Service {
	list := make([]provider.Service, 0, len(services))
	for _, service := range services {
		list = append(list, service.DeepCopy())
	}

	slices.SortFunc(list, func(a, b provider.Service) int {
		if a.Namespace != b.Namespace {
			return strings.Compare(a.Namespace, b.Namespace)
		}

		return strings.Compare(a.Name, b.Name)
	})

	return list
}

var _ provider.Provider = (*Provider)(nil)
