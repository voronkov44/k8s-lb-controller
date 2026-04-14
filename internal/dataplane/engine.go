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
	"errors"
	"fmt"
	"net/netip"
	"sync"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// ConfigApplier applies the rendered aggregate HAProxy configuration.
type ConfigApplier interface {
	Apply(ctx context.Context, services []provider.Service) (bool, error)
	Bootstrap(ctx context.Context) error
}

// ExternalIPManager reconciles the set of external IPs attached to the dataplane host.
type ExternalIPManager interface {
	List(ctx context.Context) ([]netip.Addr, error)
	EnsurePresent(ctx context.Context, addr netip.Addr) (bool, error)
	EnsureAbsent(ctx context.Context, addr netip.Addr) (bool, error)
}

// EngineConfig contains runtime settings for the in-memory dataplane engine.
type EngineConfig struct {
	ConfigPath      string
	ValidateCommand string
	ReloadCommand   string
	PIDFile         string
	Applier         ConfigApplier
	IPManager       ExternalIPManager
}

// Engine keeps full desired state in memory and materializes one aggregate HAProxy config.
type Engine struct {
	mu      sync.RWMutex
	store   *Store
	applier ConfigApplier
	ipm     ExternalIPManager
}

// NewEngine creates an Engine with an empty desired-state store.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	applier := cfg.Applier
	if applier == nil {
		createdApplier, err := NewApplier(ApplyConfig{
			ConfigPath:      cfg.ConfigPath,
			ValidateCommand: cfg.ValidateCommand,
			ReloadCommand:   cfg.ReloadCommand,
			PIDFile:         cfg.PIDFile,
		})
		if err != nil {
			return nil, err
		}
		applier = createdApplier
	}

	ipManager := cfg.IPManager
	if ipManager == nil {
		ipManager = noopExternalIPManager{}
	}

	return &Engine{
		store:   NewStore(),
		applier: applier,
		ipm:     ipManager,
	}, nil
}

// Bootstrap writes the minimal empty HAProxy config for the current empty in-memory state.
func (e *Engine) Bootstrap(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.applier.Bootstrap(ctx); err != nil {
		return err
	}

	return nil
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

	changed, err := e.reconcileLocked(ctx, nextStore)
	if err != nil {
		return false, err
	}

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

	changed, err := e.reconcileLocked(ctx, nextStore)
	if err != nil {
		return false, err
	}

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

func (e *Engine) reconcileLocked(ctx context.Context, nextStore *Store) (bool, error) {
	nextServices := nextStore.List()

	currentIPs, err := e.ipm.List(ctx)
	if err != nil {
		return false, fmt.Errorf("list attached external IPs: %w", err)
	}

	desiredIPs, err := desiredExternalIPs(nextServices)
	if err != nil {
		return false, err
	}

	newIPs := diffExternalIPs(desiredIPs, currentIPs)
	staleIPs := diffExternalIPs(currentIPs, desiredIPs)
	newlyAttached := make([]netip.Addr, 0, len(newIPs))

	for _, addr := range newIPs {
		changed, err := e.ipm.EnsurePresent(ctx, addr)
		if err != nil {
			return false, errors.Join(
				fmt.Errorf("attach external IP %s: %w", addr, err),
				rollbackAttachedExternalIPs(ctx, e.ipm, newlyAttached),
			)
		}
		if changed {
			newlyAttached = append(newlyAttached, addr)
		}
	}

	configChanged, err := e.applier.Apply(ctx, nextServices)
	if err != nil {
		return false, errors.Join(
			fmt.Errorf("apply aggregate HAProxy config: %w", err),
			rollbackAttachedExternalIPs(ctx, e.ipm, newlyAttached),
		)
	}

	e.store = nextStore
	changed := configChanged || len(newlyAttached) > 0

	for _, addr := range staleIPs {
		detached, err := e.ipm.EnsureAbsent(ctx, addr)
		if err != nil {
			return false, fmt.Errorf("detach stale external IP %s: %w", addr, err)
		}
		changed = changed || detached
	}

	return changed, nil
}

func rollbackAttachedExternalIPs(ctx context.Context, ipManager ExternalIPManager, attached []netip.Addr) error {
	if len(attached) == 0 {
		return nil
	}

	var rollbackErr error
	for index := len(attached) - 1; index >= 0; index-- {
		addr := attached[index]
		if _, err := ipManager.EnsureAbsent(ctx, addr); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback attached external IP %s: %w", addr, err))
		}
	}

	return rollbackErr
}

type noopExternalIPManager struct{}

func (noopExternalIPManager) List(context.Context) ([]netip.Addr, error) {
	return nil, nil
}

func (noopExternalIPManager) EnsurePresent(context.Context, netip.Addr) (bool, error) {
	return false, nil
}

func (noopExternalIPManager) EnsureAbsent(context.Context, netip.Addr) (bool, error) {
	return false, nil
}
