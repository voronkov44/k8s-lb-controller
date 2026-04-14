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
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func TestDesiredExternalIPsDeduplicatesAndSorts(t *testing.T) {
	addresses, err := desiredExternalIPs([]provider.Service{
		newTestService("b", "203.0.113.11", nil),
		newTestService("a", "203.0.113.10", nil),
		newTestService("c", "203.0.113.11", nil),
	})
	if err != nil {
		t.Fatalf("desiredExternalIPs() error = %v", err)
	}

	want := []netip.Addr{
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
	}
	if !slices.Equal(addresses, want) {
		t.Fatalf("desiredExternalIPs() = %v, want %v", addresses, want)
	}
}

func TestEngineAttachesNewExternalIPBeforeApply(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyChanged: true, events: &events}
	ipm := newFakeExternalIPManager(&events)
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	changed, err := engine.Ensure(context.Background(), newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80},
	}))
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure() changed = false, want true")
	}

	wantEvents := []string{"attach 203.0.113.10", "apply"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestEngineDoesNotReattachExistingExternalIP(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyChanged: true, events: &events}
	ipm := newFakeExternalIPManager(&events, "203.0.113.10")
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	changed, err := engine.Ensure(context.Background(), newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80},
	}))
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure() changed = false, want true")
	}

	if strings.Contains(strings.Join(events, ","), "attach") {
		t.Fatalf("unexpected attach events: %v", events)
	}
}

func TestEngineDetachesExternalIPWhenLastServiceIsRemoved(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyChanged: true, events: &events}
	ipm := newFakeExternalIPManager(&events)
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	service := newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80},
	})
	if _, err := engine.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	applier.applyChanged = true
	resetTestEvents(&events)
	changed, err := engine.Delete(context.Background(), service.Ref())
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !changed {
		t.Fatal("Delete() changed = false, want true")
	}

	wantEvents := []string{"apply", "detach 203.0.113.10"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestEngineKeepsSharedExternalIPAttachedUntilLastServiceIsRemoved(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyChanged: true, events: &events}
	ipm := newFakeExternalIPManager(&events)
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	serviceA := newTestService("demo-a", "203.0.113.10", []provider.ServicePort{{Name: "http", Protocol: "TCP", Port: 80}})
	serviceB := newTestService("demo-b", "203.0.113.10", []provider.ServicePort{{Name: "https", Protocol: "TCP", Port: 443}})
	if _, err := engine.Ensure(context.Background(), serviceA); err != nil {
		t.Fatalf("Ensure(serviceA) error = %v", err)
	}
	if _, err := engine.Ensure(context.Background(), serviceB); err != nil {
		t.Fatalf("Ensure(serviceB) error = %v", err)
	}

	resetTestEvents(&events)
	applier.applyChanged = true
	changed, err := engine.Delete(context.Background(), serviceA.Ref())
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !changed {
		t.Fatal("Delete() changed = false, want true")
	}
	if strings.Contains(strings.Join(events, ","), "detach") {
		t.Fatalf("unexpected detach for shared IP: %v", events)
	}
}

func TestEngineRollsBackAttachedExternalIPWhenApplyFails(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyErr: errors.New("apply failed"), events: &events}
	ipm := newFakeExternalIPManager(&events)
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	_, err = engine.Ensure(context.Background(), newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80},
	}))
	if err == nil {
		t.Fatal("Ensure() error = nil, want non-nil")
	}
	if !slices.Equal(events, []string{"attach 203.0.113.10", "apply", "detach 203.0.113.10"}) {
		t.Fatalf("events = %v, want attach/apply/detach rollback", events)
	}
	if engine.ServiceCount() != 0 {
		t.Fatalf("ServiceCount() = %d, want 0 after rollback", engine.ServiceCount())
	}
}

func TestEngineCleansUpStaleExternalIPAfterSuccessfulApply(t *testing.T) {
	events := []string{}
	applier := &fakeConfigApplier{applyChanged: true, events: &events}
	ipm := newFakeExternalIPManager(&events)
	engine, err := NewEngine(EngineConfig{
		Applier:   applier,
		IPManager: ipm,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	service := newTestService("demo", "203.0.113.10", []provider.ServicePort{{Name: "http", Protocol: "TCP", Port: 80}})
	if _, err := engine.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	updated := service.DeepCopy()
	updated.ExternalIP = "203.0.113.11"
	resetTestEvents(&events)
	applier.applyChanged = true
	changed, err := engine.Ensure(context.Background(), updated)
	if err != nil {
		t.Fatalf("Ensure(updated) error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure(updated) changed = false, want true")
	}

	wantEvents := []string{"attach 203.0.113.11", "apply", "detach 203.0.113.10"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

type fakeConfigApplier struct {
	applyChanged bool
	applyErr     error
	events       *[]string
}

func (f *fakeConfigApplier) Apply(_ context.Context, _ []provider.Service) (bool, error) {
	if f.events != nil {
		*f.events = append(*f.events, "apply")
	}
	if f.applyErr != nil {
		return false, f.applyErr
	}

	return f.applyChanged, nil
}

func (f *fakeConfigApplier) Bootstrap(context.Context) error {
	return nil
}

type fakeExternalIPManager struct {
	current map[netip.Addr]struct{}
	events  *[]string
}

func newFakeExternalIPManager(events *[]string, initial ...string) *fakeExternalIPManager {
	current := make(map[netip.Addr]struct{}, len(initial))
	for _, addr := range initial {
		current[netip.MustParseAddr(addr)] = struct{}{}
	}

	return &fakeExternalIPManager{
		current: current,
		events:  events,
	}
}

func (m *fakeExternalIPManager) List(context.Context) ([]netip.Addr, error) {
	addresses := make([]netip.Addr, 0, len(m.current))
	for addr := range m.current {
		addresses = append(addresses, addr)
	}
	slices.SortFunc(addresses, func(left, right netip.Addr) int {
		return left.Compare(right)
	})

	return addresses, nil
}

func (m *fakeExternalIPManager) EnsurePresent(_ context.Context, addr netip.Addr) (bool, error) {
	if _, ok := m.current[addr]; ok {
		return false, nil
	}

	m.current[addr] = struct{}{}
	if m.events != nil {
		*m.events = append(*m.events, "attach "+addr.String())
	}
	return true, nil
}

func (m *fakeExternalIPManager) EnsureAbsent(_ context.Context, addr netip.Addr) (bool, error) {
	if _, ok := m.current[addr]; !ok {
		return false, nil
	}

	delete(m.current, addr)
	if m.events != nil {
		*m.events = append(*m.events, "detach "+addr.String())
	}
	return true, nil
}

func resetTestEvents(events *[]string) {
	if events != nil {
		*events = nil
	}
}
