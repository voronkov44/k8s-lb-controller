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

package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/voronkov44/k8s-lb-controller/internal/config"
	dataplaneapiprovider "github.com/voronkov44/k8s-lb-controller/internal/provider/dataplaneapi"
	haproxyprovider "github.com/voronkov44/k8s-lb-controller/internal/provider/haproxy"
)

func TestBuildProviderLocalHAProxyMode(t *testing.T) {
	serviceProvider, err := buildProvider(config.Config{
		ProviderMode:           config.ProviderModeLocalHAProxy,
		HAProxyConfigPath:      filepath.Join(t.TempDir(), "haproxy.cfg"),
		HAProxyValidateCommand: "haproxy -c -f {{config}}",
		HAProxyReloadCommand:   "service haproxy reload",
	})
	if err != nil {
		t.Fatalf("buildProvider() error = %v", err)
	}

	if _, ok := serviceProvider.(*haproxyprovider.Provider); !ok {
		t.Fatalf("buildProvider() type = %T, want *haproxy.Provider", serviceProvider)
	}
}

func TestBuildProviderDataplaneAPIMode(t *testing.T) {
	serviceProvider, err := buildProvider(config.Config{
		ProviderMode:        config.ProviderModeDataplaneAPI,
		DataplaneAPIURL:     "http://127.0.0.1:18080",
		DataplaneAPITimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("buildProvider() error = %v", err)
	}

	if _, ok := serviceProvider.(*dataplaneapiprovider.Provider); !ok {
		t.Fatalf("buildProvider() type = %T, want *dataplaneapi.Provider", serviceProvider)
	}
}

func TestBuildProviderRejectsInvalidMode(t *testing.T) {
	if _, err := buildProvider(config.Config{
		ProviderMode: config.ProviderMode("invalid-mode"),
	}); err == nil {
		t.Fatal("buildProvider() error = nil, want non-nil")
	}
}
