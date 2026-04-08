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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// Config contains runtime settings for the remote dataplane API provider.
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Provider applies desired Service state by calling a remote dataplane API.
type Provider struct {
	baseURL *url.URL
	client  *http.Client
}

type changedResponse struct {
	Changed *bool `json:"changed"`
}

type servicePayload struct {
	Namespace         string               `json:"namespace"`
	Name              string               `json:"name"`
	LoadBalancerClass string               `json:"loadBalancerClass"`
	ExternalIP        string               `json:"externalIP"`
	Ports             []servicePortPayload `json:"ports"`
}

type servicePortPayload struct {
	Name       string                   `json:"name"`
	Protocol   string                   `json:"protocol"`
	Port       int32                    `json:"port"`
	TargetPort string                   `json:"targetPort"`
	Backends   []backendEndpointPayload `json:"backends"`
}

type backendEndpointPayload struct {
	Address string `json:"address"`
	Port    int32  `json:"port"`
}

// NewProvider creates a dataplane API backed provider.
func NewProvider(cfg Config) (*Provider, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base URL must not be empty")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		return nil, fmt.Errorf("base URL must be an absolute URL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https")
	}

	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than zero")
	}

	return &Provider{
		baseURL: parsedURL,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

// Ensure upserts a Service by sending the desired state to the dataplane API.
func (p *Provider) Ensure(ctx context.Context, service provider.Service) (bool, error) {
	payloadBytes, err := json.Marshal(servicePayloadFromService(service))
	if err != nil {
		return false, fmt.Errorf("marshal service %s: %w", service.Ref(), err)
	}

	return p.doRequest(
		ctx,
		http.MethodPut,
		service.Ref(),
		bytes.NewReader(payloadBytes),
		"application/json",
	)
}

// Delete removes a Service by calling the dataplane API.
func (p *Provider) Delete(ctx context.Context, ref provider.ServiceRef) (bool, error) {
	return p.doRequest(ctx, http.MethodDelete, ref, nil, "")
}

func (p *Provider) doRequest(
	ctx context.Context,
	method string,
	ref provider.ServiceRef,
	body io.Reader,
	contentType string,
) (bool, error) {
	endpoint := p.serviceURL(ref)

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return false, fmt.Errorf("%s %s: create request: %w", method, ref, err)
	}

	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return false, fmt.Errorf("%s %s via dataplane API %s: %w", method, ref, endpoint, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return false, fmt.Errorf("%s %s via dataplane API %s: read response body: %w", method, ref, endpoint, err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(bodyBytes))
		if message == "" {
			return false, fmt.Errorf(
				"%s %s via dataplane API %s: unexpected status %s",
				method,
				ref,
				endpoint,
				response.Status,
			)
		}

		return false, fmt.Errorf(
			"%s %s via dataplane API %s: unexpected status %s: %s",
			method,
			ref,
			endpoint,
			response.Status,
			message,
		)
	}

	var decoded changedResponse
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		return false, fmt.Errorf(
			"%s %s via dataplane API %s: decode changed response: %w",
			method,
			ref,
			endpoint,
			err,
		)
	}

	if decoded.Changed == nil {
		return false, fmt.Errorf("%s %s via dataplane API %s: response missing changed field", method, ref, endpoint)
	}

	return *decoded.Changed, nil
}

func (p *Provider) serviceURL(ref provider.ServiceRef) string {
	endpoint := *p.baseURL
	endpoint.Path = path.Join("/", p.baseURL.Path, "services", ref.Namespace, ref.Name)
	return endpoint.String()
}

func servicePayloadFromService(service provider.Service) servicePayload {
	payload := servicePayload{
		Namespace:         service.Namespace,
		Name:              service.Name,
		LoadBalancerClass: service.LoadBalancerClass,
		ExternalIP:        service.ExternalIP,
		Ports:             make([]servicePortPayload, 0, len(service.Ports)),
	}

	for _, port := range service.Ports {
		payloadPort := servicePortPayload{
			Name:       port.Name,
			Protocol:   port.Protocol,
			Port:       port.Port,
			TargetPort: port.TargetPort,
			Backends:   make([]backendEndpointPayload, 0, len(port.Backends)),
		}

		for _, backend := range port.Backends {
			payloadPort.Backends = append(payloadPort.Backends, backendEndpointPayload{
				Address: backend.Address,
				Port:    backend.Port,
			})
		}

		payload.Ports = append(payload.Ports, payloadPort)
	}

	return payload
}

var _ provider.Provider = (*Provider)(nil)
