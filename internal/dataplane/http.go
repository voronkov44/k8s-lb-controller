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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// DefaultMaxRequestBodyBytes limits the size of one dataplane API request body.
const DefaultMaxRequestBodyBytes int64 = 1 << 20

// HandlerConfig contains HTTP API settings for the dataplane server.
type HandlerConfig struct {
	MaxRequestBodyBytes int64
}

type changedResponse struct {
	Changed bool `json:"changed"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Handler serves the dataplane HTTP API.
type Handler struct {
	engine              *Engine
	logger              *slog.Logger
	maxRequestBodyBytes int64
	ready               atomic.Bool
}

// NewHandler creates a dataplane HTTP handler.
func NewHandler(engine *Engine, logger *slog.Logger, cfg HandlerConfig) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	maxRequestBodyBytes := cfg.MaxRequestBodyBytes
	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}

	handler := &Handler{
		engine:              engine,
		logger:              logger,
		maxRequestBodyBytes: maxRequestBodyBytes,
	}
	handler.ready.Store(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.handleHealthz)
	mux.HandleFunc("/readyz", handler.handleReadyz)
	mux.HandleFunc("/services", handler.handleServices)
	mux.HandleFunc("/services/", handler.handleServices)
	return mux
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	if !h.ready.Load() {
		writeJSONError(w, http.StatusServiceUnavailable, "server not ready")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleServices(w http.ResponseWriter, r *http.Request) {
	ref, err := serviceRefFromPath(r.URL.Path)
	if err != nil {
		h.logger.Warn("invalid dataplane request path", "method", r.Method, "path", r.URL.Path, "error", err.Error())
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.handleEnsureService(w, r, ref)
	case http.MethodDelete:
		h.handleDeleteService(w, r, ref)
	default:
		writeMethodNotAllowed(w, http.MethodPut, http.MethodDelete)
	}
}

func (h *Handler) handleEnsureService(w http.ResponseWriter, r *http.Request, ref provider.ServiceRef) {
	service, err := decodeServiceRequest(w, r, h.maxRequestBodyBytes)
	if err != nil {
		h.logger.Warn("invalid dataplane service payload", "method", r.Method, "path", r.URL.Path, "error", err.Error())
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if service.Namespace != "" && service.Namespace != ref.Namespace {
		writeJSONError(w, http.StatusBadRequest, "request body namespace does not match request path")
		return
	}
	if service.Name != "" && service.Name != ref.Name {
		writeJSONError(w, http.StatusBadRequest, "request body name does not match request path")
		return
	}

	service.Namespace = ref.Namespace
	service.Name = ref.Name

	if err := ValidateService(normalizeService(service)); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	changed, err := h.engine.Ensure(r.Context(), service)
	if err != nil {
		h.logger.Error("failed to ensure dataplane service", "serviceRef", ref.String(), "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to apply desired state")
		return
	}

	h.logger.Info("dataplane service ensured", "serviceRef", ref.String(), "changed", changed)
	writeJSON(w, http.StatusOK, changedResponse{Changed: changed})
}

func (h *Handler) handleDeleteService(w http.ResponseWriter, r *http.Request, ref provider.ServiceRef) {
	changed, err := h.engine.Delete(r.Context(), ref)
	if err != nil {
		h.logger.Error("failed to delete dataplane service", "serviceRef", ref.String(), "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to apply desired state")
		return
	}

	h.logger.Info("dataplane service deleted", "serviceRef", ref.String(), "changed", changed)
	writeJSON(w, http.StatusOK, changedResponse{Changed: changed})
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, maxRequestBodyBytes int64) (provider.Service, error) {
	defer func() {
		_ = r.Body.Close()
	}()

	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	var service provider.Service
	if err := decoder.Decode(&service); err != nil {
		var maxBytesError *http.MaxBytesError
		switch {
		case errors.As(err, &maxBytesError):
			return provider.Service{}, fmt.Errorf("request body exceeds %d bytes", maxRequestBodyBytes)
		default:
			return provider.Service{}, fmt.Errorf("decode request body: %w", err)
		}
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return provider.Service{}, fmt.Errorf("request body must contain exactly one JSON object")
	}

	return service, nil
}

func serviceRefFromPath(requestPath string) (provider.ServiceRef, error) {
	trimmedPath := strings.Trim(requestPath, "/")
	segments := strings.Split(trimmedPath, "/")
	if len(segments) != 3 || segments[0] != "services" {
		return provider.ServiceRef{}, fmt.Errorf("path must match /services/{namespace}/{name}")
	}

	namespace, err := url.PathUnescape(segments[1])
	if err != nil {
		return provider.ServiceRef{}, fmt.Errorf("decode namespace: %w", err)
	}

	name, err := url.PathUnescape(segments[2])
	if err != nil {
		return provider.ServiceRef{}, fmt.Errorf("decode name: %w", err)
	}

	if namespace == "" || name == "" {
		return provider.ServiceRef{}, fmt.Errorf("path must include namespace and name")
	}

	return provider.ServiceRef{
		Namespace: namespace,
		Name:      name,
	}, nil
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
