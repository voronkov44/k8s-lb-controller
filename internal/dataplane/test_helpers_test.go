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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioDiscard{}, nil))
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func newTestService(name, externalIP string, ports []provider.ServicePort) provider.Service {
	return provider.Service{
		Namespace:         "default",
		Name:              name,
		LoadBalancerClass: "iedge.local/service-lb",
		ExternalIP:        externalIP,
		Ports:             ports,
	}
}

func testConfigPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "haproxy.cfg")
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	return string(data)
}

func countMarkerLines(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	return len(strings.FieldsFunc(string(data), func(r rune) bool {
		return r == '\n'
	}))
}
