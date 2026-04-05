package haproxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

func TestRenderProducesDeterministicConfig(t *testing.T) {
	rendered, err := Render([]provider.Service{
		newTestService("default", "demo", "203.0.113.10", []provider.ServicePort{
			{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
				Backends: []provider.BackendEndpoint{
					{Address: "10.0.0.2", Port: 8080},
					{Address: "10.0.0.1", Port: 8080},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	const want = `# Managed by k8s-lb-controller. DO NOT EDIT.
global
    maxconn 2048

defaults
    mode tcp
    timeout connect 5s
    timeout client 30s
    timeout server 30s

frontend fe_default_demo_80_http_tcp
    bind 203.0.113.10:80
    mode tcp
    default_backend be_default_demo_80_http_tcp

backend be_default_demo_80_http_tcp
    mode tcp
    balance roundrobin
    server srv_0001_10_0_0_1_8080 10.0.0.1:8080
    server srv_0002_10_0_0_2_8080 10.0.0.2:8080
`

	if rendered != want {
		t.Fatalf("Render() =\n%s\nwant:\n%s", rendered, want)
	}
}

func TestRenderSanitizesIdentifiersAndSupportsEmptyBackends(t *testing.T) {
	rendered, err := Render([]provider.Service{
		newTestService("demo.ns", "api@demo", "203.0.113.11", []provider.ServicePort{
			{
				Name:     "http-admin",
				Protocol: "TCP",
				Port:     8080,
			},
		}),
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, "frontend fe_demo_ns_api_demo_8080_http_admin_tcp") {
		t.Fatalf("Render() missing sanitized frontend identifier:\n%s", rendered)
	}

	if !strings.Contains(rendered, "backend be_demo_ns_api_demo_8080_http_admin_tcp") {
		t.Fatalf("Render() missing sanitized backend identifier:\n%s", rendered)
	}

	if !strings.Contains(rendered, "server srv_unavailable 127.0.0.1:1 disabled") {
		t.Fatalf("Render() missing disabled placeholder backend:\n%s", rendered)
	}
}

func TestProviderEnsureWritesAggregateConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	serviceA := newTestService("default", "demo-a", "203.0.113.10", []provider.ServicePort{
		{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
			Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}},
		},
	})
	serviceB := newTestService("default", "demo-b", "203.0.113.11", []provider.ServicePort{
		{
			Name:     "metrics",
			Protocol: "TCP",
			Port:     9090,
			Backends: []provider.BackendEndpoint{{Address: "10.0.0.11", Port: 9091}},
		},
	})

	if err := haproxyProvider.Ensure(context.Background(), serviceA); err != nil {
		t.Fatalf("Ensure(serviceA) error = %v", err)
	}

	if err := haproxyProvider.Ensure(context.Background(), serviceB); err != nil {
		t.Fatalf("Ensure(serviceB) error = %v", err)
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "frontend fe_default_demo_a_80_http_tcp") {
		t.Fatalf("config missing frontend for demo-a:\n%s", rendered)
	}

	if !strings.Contains(rendered, "frontend fe_default_demo_b_9090_metrics_tcp") {
		t.Fatalf("config missing frontend for demo-b:\n%s", rendered)
	}

	if len(haproxyProvider.services) != 2 {
		t.Fatalf("provider services len = %d, want 2", len(haproxyProvider.services))
	}
}

func TestProviderDeleteRewritesConfigWithoutRemovedService(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	serviceA := newTestService("default", "demo-a", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})
	serviceB := newTestService("default", "demo-b", "203.0.113.11", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 81, Backends: []provider.BackendEndpoint{{Address: "10.0.0.11", Port: 8080}}},
	})

	if err := haproxyProvider.Ensure(context.Background(), serviceA); err != nil {
		t.Fatalf("Ensure(serviceA) error = %v", err)
	}
	if err := haproxyProvider.Ensure(context.Background(), serviceB); err != nil {
		t.Fatalf("Ensure(serviceB) error = %v", err)
	}

	if err := haproxyProvider.Delete(context.Background(), serviceA.Ref()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	rendered := readConfigFile(t, configPath)
	if strings.Contains(rendered, "frontend fe_default_demo_a_80_http_tcp") {
		t.Fatalf("config still contains removed service:\n%s", rendered)
	}

	if !strings.Contains(rendered, "frontend fe_default_demo_b_81_http_tcp") {
		t.Fatalf("config missing remaining service:\n%s", rendered)
	}
}

func TestProviderDeleteMissingEntryStillKeepsValidConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := haproxyProvider.Delete(context.Background(), provider.ServiceRef{Namespace: "default", Name: "missing"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "defaults") {
		t.Fatalf("config missing baseline defaults block:\n%s", rendered)
	}
}

func TestProviderEnsureValidateFailureReturnsErrorAndDoesNotApplyState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{
		ConfigPath:      configPath,
		ValidateCommand: "false",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	existingService := newTestService("default", "existing", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})
	haproxyProvider.services[existingService.Ref()] = existingService.DeepCopy()

	existingRendered, err := Render([]provider.Service{existingService})
	if err != nil {
		t.Fatalf("Render(existing) error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(existingRendered), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	newService := newTestService("default", "broken", "203.0.113.11", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 81, Backends: []provider.BackendEndpoint{{Address: "10.0.0.11", Port: 8080}}},
	})

	if err := haproxyProvider.Ensure(context.Background(), newService); err == nil {
		t.Fatal("Ensure() error = nil, want non-nil")
	}

	if len(haproxyProvider.services) != 1 {
		t.Fatalf("provider services len = %d, want 1", len(haproxyProvider.services))
	}

	if got := readConfigFile(t, configPath); got != existingRendered {
		t.Fatalf("config changed after validation failure =\n%s\nwant:\n%s", got, existingRendered)
	}
}

func TestProviderEnsureReloadFailureReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{
		ConfigPath:    configPath,
		ReloadCommand: "false",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	service := newTestService("default", "demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})

	if err := haproxyProvider.Ensure(context.Background(), service); err == nil {
		t.Fatal("Ensure() error = nil, want non-nil")
	}

	if len(haproxyProvider.services) != 0 {
		t.Fatalf("provider services len = %d, want 0", len(haproxyProvider.services))
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "frontend fe_default_demo_80_http_tcp") {
		t.Fatalf("config file was not updated before reload attempt:\n%s", rendered)
	}
}

func TestProviderEnsureWorksWithoutOptionalCommandsAndLeavesNoTempFiles(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "haproxy.cfg")
	haproxyProvider, err := NewProvider(Config{
		ConfigPath:      configPath,
		ValidateCommand: "grep -q frontend {{config}}",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	service := newTestService("default", "demo", "203.0.113.10", []provider.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
	})
	if err := haproxyProvider.Ensure(context.Background(), service); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temporary file %q still exists", entry.Name())
		}
	}
}

func newTestService(namespace, name, externalIP string, ports []provider.ServicePort) provider.Service {
	return provider.Service{
		Namespace:         namespace,
		Name:              name,
		LoadBalancerClass: "iedge.local/service-lb",
		ExternalIP:        externalIP,
		Ports:             ports,
	}
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	return string(data)
}
