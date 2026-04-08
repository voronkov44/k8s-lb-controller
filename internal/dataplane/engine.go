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

package dataplane

import (
	"context"
	"sync"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// EngineConfig contains runtime settings for the in-memory dataplane engine.
type EngineConfig struct {
	ConfigPath      string
	ValidateCommand string
	ReloadCommand   string
}

// Engine keeps full desired state in memory and materializes one aggregate HAProxy config.
type Engine struct {
	mu      sync.RWMutex
	store   *Store
	applier *Applier
}

// NewEngine creates an Engine with an empty desired-state store.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	applier, err := NewApplier(ApplyConfig(cfg))
	if err != nil {
		return nil, err
	}

	return &Engine{
		store:   NewStore(),
		applier: applier,
	}, nil
}

// Ensure upserts one Service definition and reapplies the aggregate HAProxy config.
func (e *Engine) Ensure(ctx context.Context, service provider.Service) (bool, error) {
	normalized := normalizeService(service)
	if err := ValidateService(normalized); err != nil {
		return false, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	nextStore := e.store.Clone()
	nextStore.Upsert(normalized)

	changed, err := e.applier.Apply(ctx, nextStore.List())
	if err != nil {
		return false, err
	}

	e.store = nextStore
	return changed, nil
}

// Delete removes one Service definition when present and reapplies the aggregate HAProxy config.
func (e *Engine) Delete(ctx context.Context, ref provider.ServiceRef) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.store.Get(ref); !ok {
		return false, nil
	}

	nextStore := e.store.Clone()
	nextStore.Delete(ref)

	changed, err := e.applier.Apply(ctx, nextStore.List())
	if err != nil {
		return false, err
	}

	e.store = nextStore
	return changed, nil
}

// Snapshot returns a detached copy of the desired state currently stored by the engine.
func (e *Engine) Snapshot() map[provider.ServiceRef]provider.Service {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.store.Snapshot()
}

// ServiceCount returns the number of Services currently tracked by the engine.
func (e *Engine) ServiceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.store.Len()
}
