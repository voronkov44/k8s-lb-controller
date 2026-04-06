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
	"sync"
	"testing"
)

func TestMockProviderEnsureCreatesState(t *testing.T) {
	mock := NewMockProvider()
	service := Service{
		Namespace:         "default",
		Name:              "demo",
		LoadBalancerClass: "iedge.local/service-lb",
		ExternalIP:        "203.0.113.10",
		Ports: []ServicePort{
			{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: "80",
				Backends: []BackendEndpoint{
					{Address: "10.0.0.10", Port: 8080},
				},
			},
		},
	}

	if _, err := mock.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	stored, ok := mock.Get(service.Ref())
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}

	if stored.ExternalIP != "203.0.113.10" {
		t.Fatalf("stored ExternalIP = %q, want %q", stored.ExternalIP, "203.0.113.10")
	}

	if len(stored.Ports) != 1 || len(stored.Ports[0].Backends) != 1 {
		t.Fatalf("stored Ports = %+v, want one backend", stored.Ports)
	}
}

func TestMockProviderEnsureUpdatesExistingState(t *testing.T) {
	mock := NewMockProvider()
	service := Service{
		Namespace:         "default",
		Name:              "demo",
		LoadBalancerClass: "iedge.local/service-lb",
		ExternalIP:        "203.0.113.10",
	}

	if _, err := mock.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	service.ExternalIP = "203.0.113.11"
	service.Ports = []ServicePort{{Name: "http", Protocol: "TCP", Port: 80, TargetPort: "80"}}
	if _, err := mock.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() second error = %v", err)
	}

	stored, ok := mock.Get(service.Ref())
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}

	if stored.ExternalIP != "203.0.113.11" {
		t.Fatalf("stored ExternalIP = %q, want %q", stored.ExternalIP, "203.0.113.11")
	}

	if len(stored.Ports) != 1 || stored.Ports[0].Port != 80 {
		t.Fatalf("stored Ports = %+v, want port 80", stored.Ports)
	}
}

func TestMockProviderDeleteRemovesState(t *testing.T) {
	mock := NewMockProvider()
	service := Service{
		Namespace:  "default",
		Name:       "demo",
		ExternalIP: "203.0.113.10",
	}

	if _, err := mock.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	if _, err := mock.Delete(context.Background(), service.Ref()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, ok := mock.Get(service.Ref()); ok {
		t.Fatal("Get() ok = true, want false")
	}
}

func TestMockProviderDeleteMissingEntryIsNotAnError(t *testing.T) {
	mock := NewMockProvider()

	if _, err := mock.Delete(context.Background(), ServiceRef{Namespace: "default", Name: "missing"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestMockProviderSnapshotAndList(t *testing.T) {
	mock := NewMockProvider()
	serviceA := Service{Namespace: "b", Name: "demo-b", ExternalIP: "203.0.113.11"}
	serviceB := Service{Namespace: "a", Name: "demo-a", ExternalIP: "203.0.113.10"}

	if _, err := mock.Ensure(context.Background(), serviceA); err != nil {
		t.Fatalf("Ensure(serviceA) error = %v", err)
	}
	if _, err := mock.Ensure(context.Background(), serviceB); err != nil {
		t.Fatalf("Ensure(serviceB) error = %v", err)
	}

	snapshot := mock.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("Snapshot() len = %d, want 2", len(snapshot))
	}

	list := mock.List()
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}

	if list[0].Namespace != "a" || list[0].Name != "demo-a" {
		t.Fatalf("List()[0] = %+v, want namespace a name demo-a", list[0])
	}
}

func TestMockProviderConcurrentEnsure(t *testing.T) {
	mock := NewMockProvider()
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			service := Service{
				Namespace:  "default",
				Name:       "demo",
				ExternalIP: "203.0.113.10",
				Ports: []ServicePort{
					{Name: "http", Protocol: "TCP", Port: int32(80 + index), TargetPort: "80"},
				},
			}

			_, _ = mock.Ensure(context.Background(), service)
		}(i)
	}

	wg.Wait()

	stored, ok := mock.Get(ServiceRef{Namespace: "default", Name: "demo"})
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}

	if stored.ExternalIP != "203.0.113.10" {
		t.Fatalf("stored ExternalIP = %q, want %q", stored.ExternalIP, "203.0.113.10")
	}
}
