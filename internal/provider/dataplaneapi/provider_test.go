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

package dataplaneapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func TestProviderEnsureSuccess(t *testing.T) {
	service := newTestService()

	var gotMethod string
	var gotPath string
	var gotContentType string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"changed":true}`))
	}))
	defer server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Ensure(context.Background(), service)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Fatal("Ensure() changed = false, want true")
	}

	if gotMethod != http.MethodPut {
		t.Fatalf("HTTP method = %q, want %q", gotMethod, http.MethodPut)
	}

	if gotPath != "/services/default/demo" {
		t.Fatalf("URL path = %q, want %q", gotPath, "/services/default/demo")
	}

	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", gotContentType, "application/json")
	}

	var gotPayload servicePayload
	if err := json.Unmarshal(gotBody, &gotPayload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	wantPayload := servicePayloadFromService(service)
	if !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("request payload = %+v, want %+v", gotPayload, wantPayload)
	}
}

func TestProviderDeleteSuccess(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"changed":false}`))
	}))
	defer server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Delete(context.Background(), provider.ServiceRef{
		Namespace: "default",
		Name:      "demo",
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if changed {
		t.Fatal("Delete() changed = true, want false")
	}

	if gotMethod != http.MethodDelete {
		t.Fatalf("HTTP method = %q, want %q", gotMethod, http.MethodDelete)
	}

	if gotPath != "/services/default/demo" {
		t.Fatalf("URL path = %q, want %q", gotPath, "/services/default/demo")
	}
}

func TestProviderEnsureRejectsNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Ensure(context.Background(), newTestService())
	if err == nil {
		t.Fatal("Ensure() error = nil, want non-nil")
	}
	if changed {
		t.Fatal("Ensure() changed = true, want false")
	}

	if !strings.Contains(err.Error(), "unexpected status 502 Bad Gateway") {
		t.Fatalf("Ensure() error = %v, want unexpected status", err)
	}
}

func TestProviderDeleteRejectsMalformedJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"changed":"yes"}`))
	}))
	defer server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Delete(context.Background(), provider.ServiceRef{
		Namespace: "default",
		Name:      "demo",
	})
	if err == nil {
		t.Fatal("Delete() error = nil, want non-nil")
	}
	if changed {
		t.Fatal("Delete() changed = true, want false")
	}

	if !strings.Contains(err.Error(), "decode changed response") {
		t.Fatalf("Delete() error = %v, want decode changed response", err)
	}
}

func TestProviderEnsureTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"changed":true}`))
	}))
	defer server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: server.URL, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Ensure(context.Background(), newTestService())
	if err == nil {
		t.Fatal("Ensure() error = nil, want non-nil")
	}
	if changed {
		t.Fatal("Ensure() changed = true, want false")
	}

	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("Ensure() error = %v, want timeout-related error", err)
	}
}

func TestProviderDeleteNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"changed":true}`))
	}))
	serverURL := server.URL
	server.Close()

	dataplaneProvider, err := NewProvider(Config{BaseURL: serverURL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	changed, err := dataplaneProvider.Delete(context.Background(), provider.ServiceRef{
		Namespace: "default",
		Name:      "demo",
	})
	if err == nil {
		t.Fatal("Delete() error = nil, want non-nil")
	}
	if changed {
		t.Fatal("Delete() changed = true, want false")
	}

	if !strings.Contains(err.Error(), "via dataplane API") {
		t.Fatalf("Delete() error = %v, want dataplane API context", err)
	}
}

func newTestService() provider.Service {
	return provider.Service{
		Namespace:         "default",
		Name:              "demo",
		LoadBalancerClass: "iedge.local/service-lb",
		ExternalIP:        "203.0.113.10",
		Ports: []provider.ServicePort{
			{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: "8080",
				Backends: []provider.BackendEndpoint{
					{Address: "10.0.0.10", Port: 8080},
					{Address: "10.0.0.11", Port: 8080},
				},
			},
		},
	}
}
