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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerPutServiceSuccess(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPut, "/services/default/demo", bytes.NewBufferString(`{
		"externalIP":"203.0.113.10",
		"loadBalancerClass":"iedge.local/service-lb",
		"ports":[{"name":"http","protocol":"TCP","port":80,"backends":[{"address":"10.0.0.10","port":8080}]}]
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var payload changedResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.Changed {
		t.Fatal("changed = false, want true")
	}
}

func TestHandlerDeleteServiceSuccess(t *testing.T) {
	handler := newTestHandler(t)

	putRequest := httptest.NewRequest(http.MethodPut, "/services/default/demo", bytes.NewBufferString(`{
		"externalIP":"203.0.113.10",
		"loadBalancerClass":"iedge.local/service-lb",
		"ports":[{"name":"http","protocol":"TCP","port":80,"backends":[{"address":"10.0.0.10","port":8080}]}]
	}`))
	putResponse := httptest.NewRecorder()
	handler.ServeHTTP(putResponse, putRequest)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/services/default/demo", nil)
	deleteResponse := httptest.NewRecorder()
	handler.ServeHTTP(deleteResponse, deleteRequest)

	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", deleteResponse.Code, http.StatusOK)
	}

	var payload changedResponse
	if err := json.Unmarshal(deleteResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.Changed {
		t.Fatal("changed = false, want true")
	}
}

func TestHandlerDeleteMissingServiceReturnsChangedFalse(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodDelete, "/services/default/missing", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var payload changedResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Changed {
		t.Fatal("changed = true, want false")
	}
}

func TestHandlerRejectsMalformedJSON(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPut, "/services/default/demo", bytes.NewBufferString(`{"externalIP":`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestHandlerRejectsInvalidPath(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPut, "/services/default", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestHandlerRejectsMethodNotAllowed(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/services/default/demo", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerHealthzAndReadyz(t *testing.T) {
	handler := newTestHandler(t)

	healthzRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthzResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthzResponse, healthzRequest)

	if healthzResponse.Code != http.StatusOK {
		t.Fatalf("healthz status code = %d, want %d", healthzResponse.Code, http.StatusOK)
	}

	readyzRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyzResponse := httptest.NewRecorder()
	handler.ServeHTTP(readyzResponse, readyzRequest)

	if readyzResponse.Code != http.StatusOK {
		t.Fatalf("readyz status code = %d, want %d", readyzResponse.Code, http.StatusOK)
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	engine, err := NewEngine(EngineConfig{ConfigPath: testConfigPath(t)})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	return NewHandler(engine, newDiscardLogger(), HandlerConfig{})
}
