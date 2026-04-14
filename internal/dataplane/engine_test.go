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
	"strings"
	"testing"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func TestEngineEnsureNewServiceReturnsChanged(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	changed, err := engine.Ensure(context.Background(), newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	}))
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure() changed = false, want true")
	}

	if engine.ServiceCount() != 1 {
		t.Fatalf("ServiceCount() = %d, want 1", engine.ServiceCount())
	}
}

func TestEngineEnsureIdenticalServiceTwiceReturnsFalseOnSecondCall(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	service := newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
			Backends: []provider.BackendEndpoint{
				{Address: "10.0.0.11", Port: 8080},
				{Address: "10.0.0.10", Port: 8080},
			},
		},
	})

	changed, err := engine.Ensure(context.Background(), service)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure() changed = false, want true on first call")
	}

	service.Ports[0].Backends[0], service.Ports[0].Backends[1] = service.Ports[0].Backends[1], service.Ports[0].Backends[0]
	changed, err = engine.Ensure(context.Background(), service)
	if err != nil {
		t.Fatalf("Ensure() second error = %v", err)
	}
	if changed {
		t.Fatal("Ensure() changed = true, want false for identical effective state")
	}
}

func TestEngineDeleteExistingServiceReturnsChanged(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	service := newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})
	if _, err := engine.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	changed, err := engine.Delete(context.Background(), service.Ref())
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !changed {
		t.Fatal("Delete() changed = false, want true")
	}

	if engine.ServiceCount() != 0 {
		t.Fatalf("ServiceCount() = %d, want 0", engine.ServiceCount())
	}
}

func TestEngineDeleteMissingServiceReturnsFalse(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	changed, err := engine.Delete(context.Background(), provider.ServiceRef{Namespace: "default", Name: "missing"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if changed {
		t.Fatal("Delete() changed = true, want false")
	}
}

func TestEngineWritesDeterministicConfigOrdering(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	serviceB := newTestService("demo-b", "203.0.113.11", []provider.ServicePort{
		{Name: "metrics", Protocol: "TCP", Port: 9090, Backends: []provider.BackendEndpoint{{Address: "10.0.0.20", Port: 9091}}},
	})
	serviceA := newTestService("demo-a", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})

	if _, err := engine.Ensure(context.Background(), serviceB); err != nil {
		t.Fatalf("Ensure(serviceB) error = %v", err)
	}
	if _, err := engine.Ensure(context.Background(), serviceA); err != nil {
		t.Fatalf("Ensure(serviceA) error = %v", err)
	}

	rendered := readConfigFile(t, configPath)
	indexA := strings.Index(rendered, "frontend fe_default_demo_a_80_http_tcp")
	indexB := strings.Index(rendered, "frontend fe_default_demo_b_9090_metrics_tcp")
	if indexA == -1 || indexB == -1 {
		t.Fatalf("rendered config missing expected frontends:\n%s", rendered)
	}
	if indexA > indexB {
		t.Fatalf("service ordering is not deterministic:\n%s", rendered)
	}
}

func TestEngineConfigOutputChangesWhenBackendsChange(t *testing.T) {
	configPath := testConfigPath(t)
	engine, err := NewEngine(EngineConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	service := newTestService("demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})
	if _, err := engine.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	before := readConfigFile(t, configPath)

	service.Ports[0].Backends = []provider.BackendEndpoint{
		{Address: "10.0.0.10", Port: 8080},
		{Address: "10.0.0.11", Port: 8080},
	}
	if _, err := engine.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() second error = %v", err)
	}
	after := readConfigFile(t, configPath)

	if before == after {
		t.Fatal("config did not change after backend update")
	}

	if !strings.Contains(after, "server srv_0002_10_0_0_11_8080 10.0.0.11:8080") {
		t.Fatalf("updated config missing second backend:\n%s", after)
	}
}
