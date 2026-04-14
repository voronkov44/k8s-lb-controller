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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func TestApplierWritesConfigFile(t *testing.T) {
	configPath := testConfigPath(t)
	applier, err := NewApplier(ApplyConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	changed, err := applier.Apply(context.Background(), []provider.Service{
		newTestService("demo", "203.0.113.10", []provider.ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
		}),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !changed {
		t.Fatal("Apply() changed = false, want true")
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "frontend fe_default_demo_80_http_tcp") {
		t.Fatalf("rendered config missing expected frontend:\n%s", rendered)
	}
}

func TestApplierSkipsRewriteAndReloadWhenConfigIsUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "haproxy.cfg")
	reloadScriptPath := filepath.Join(tempDir, "reload.sh")
	reloadMarkerPath := filepath.Join(tempDir, "reload-count")

	reloadScript := "#!/bin/sh\n" +
		"echo reload >> " + reloadMarkerPath + "\n"
	if err := os.WriteFile(reloadScriptPath, []byte(reloadScript), 0o700); err != nil {
		t.Fatalf("WriteFile(reloadScript) error = %v", err)
	}

	applier, err := NewApplier(ApplyConfig{
		ConfigPath:    configPath,
		ReloadCommand: reloadScriptPath,
	})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	services := []provider.Service{
		newTestService("demo", "203.0.113.10", []provider.ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
		}),
	}

	changed, err := applier.Apply(context.Background(), services)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !changed {
		t.Fatal("Apply() changed = false, want true on first apply")
	}

	if got := countMarkerLines(t, reloadMarkerPath); got != 1 {
		t.Fatalf("reload count after first apply = %d, want 1", got)
	}

	changed, err = applier.Apply(context.Background(), services)
	if err != nil {
		t.Fatalf("Apply() second error = %v", err)
	}
	if changed {
		t.Fatal("Apply() changed = true, want false when config is unchanged")
	}

	if got := countMarkerLines(t, reloadMarkerPath); got != 1 {
		t.Fatalf("reload count after no-op apply = %d, want 1", got)
	}
}

func TestApplierReturnsValidationFailureAndCleansUpCandidateFiles(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "haproxy.cfg")
	applier, err := NewApplier(ApplyConfig{
		ConfigPath:      configPath,
		ValidateCommand: "false",
	})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	_, err = applier.Apply(context.Background(), []provider.Service{
		newTestService("demo", "203.0.113.10", []provider.ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
		}),
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want non-nil")
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temporary file %q still exists after validation failure", entry.Name())
		}
	}
}

func TestApplierReturnsReloadFailureAfterWritingConfig(t *testing.T) {
	configPath := testConfigPath(t)
	applier, err := NewApplier(ApplyConfig{
		ConfigPath:    configPath,
		ReloadCommand: "false",
	})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	_, err = applier.Apply(context.Background(), []provider.Service{
		newTestService("demo", "203.0.113.10", []provider.ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
		}),
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want non-nil")
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "frontend fe_default_demo_80_http_tcp") {
		t.Fatalf("config file was not updated before reload failure:\n%s", rendered)
	}
}

func TestApplierBootstrapWritesMinimalConfig(t *testing.T) {
	configPath := testConfigPath(t)
	applier, err := NewApplier(ApplyConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	if err := applier.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	rendered := readConfigFile(t, configPath)
	if !strings.Contains(rendered, "defaults") {
		t.Fatalf("bootstrap config missing defaults section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "frontend fe_bootstrap_loopback") {
		t.Fatalf("bootstrap config missing bootstrap frontend:\n%s", rendered)
	}
	if !strings.Contains(rendered, "bind 127.0.0.2:65535") {
		t.Fatalf("bootstrap config missing loopback bind:\n%s", rendered)
	}
	if strings.Contains(rendered, "frontend fe_default_") {
		t.Fatalf("bootstrap config unexpectedly contains service frontends:\n%s", rendered)
	}
}

func TestApplierBootstrapConfigIsReplacedByRenderedServiceState(t *testing.T) {
	configPath := testConfigPath(t)
	applier, err := NewApplier(ApplyConfig{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	if err := applier.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	changed, err := applier.Apply(context.Background(), []provider.Service{
		newTestService("demo", "203.0.113.10", []provider.ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, Backends: []provider.BackendEndpoint{{Address: "10.0.0.10", Port: 8080}}},
		}),
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !changed {
		t.Fatal("Apply() changed = false, want true")
	}

	rendered := readConfigFile(t, configPath)
	if strings.Contains(rendered, "frontend fe_bootstrap_loopback") {
		t.Fatalf("rendered service config still contains bootstrap frontend:\n%s", rendered)
	}
	if !strings.Contains(rendered, "frontend fe_default_demo_80_http_tcp") {
		t.Fatalf("rendered service config missing expected frontend:\n%s", rendered)
	}
}

func TestApplierBootstrapSkipsReloadWhenPIDFileIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "haproxy.cfg")
	reloadScriptPath := filepath.Join(tempDir, "reload.sh")
	reloadMarkerPath := filepath.Join(tempDir, "reload-count")

	reloadScript := "#!/bin/sh\n" +
		"echo reload >> " + reloadMarkerPath + "\n"
	if err := os.WriteFile(reloadScriptPath, []byte(reloadScript), 0o700); err != nil {
		t.Fatalf("WriteFile(reloadScript) error = %v", err)
	}

	applier, err := NewApplier(ApplyConfig{
		ConfigPath:    configPath,
		ReloadCommand: reloadScriptPath,
		PIDFile:       filepath.Join(tempDir, "missing.pid"),
	})
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}

	if err := applier.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if _, err := os.Stat(reloadMarkerPath); !os.IsNotExist(err) {
		t.Fatalf("reload marker exists, want bootstrap without reload: %v", err)
	}
}
