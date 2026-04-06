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
	"fmt"
	"slices"
)

// Provider manages external load balancer state for a single Service.
type Provider interface {
	// Ensure makes the desired Service state present and reports whether the provider state changed.
	Ensure(ctx context.Context, service Service) (bool, error)
	// Delete removes the desired Service state and reports whether the provider state changed.
	Delete(ctx context.Context, ref ServiceRef) (bool, error)
}

// ServiceRef identifies a Service by namespace and name.
type ServiceRef struct {
	Namespace string
	Name      string
}

// String returns a stable human-readable representation of the Service reference.
func (r ServiceRef) String() string {
	return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
}

// ServicePort describes a Service port relevant to the provider.
type ServicePort struct {
	Name       string
	Protocol   string
	Port       int32
	TargetPort string
	Backends   []BackendEndpoint
}

// BackendEndpoint describes a discovered backend endpoint for one Service port.
type BackendEndpoint struct {
	Address string
	Port    int32
}

// Service contains the desired provider state for one managed Service.
type Service struct {
	Namespace         string
	Name              string
	LoadBalancerClass string
	ExternalIP        string
	Ports             []ServicePort
}

// Ref returns the identifying reference for the Service.
func (s Service) Ref() ServiceRef {
	return ServiceRef{
		Namespace: s.Namespace,
		Name:      s.Name,
	}
}

// Equal reports whether the Service describes the same provider state.
func (s Service) Equal(other Service) bool {
	if s.Namespace != other.Namespace ||
		s.Name != other.Name ||
		s.LoadBalancerClass != other.LoadBalancerClass ||
		s.ExternalIP != other.ExternalIP ||
		len(s.Ports) != len(other.Ports) {
		return false
	}

	return slices.EqualFunc(s.Ports, other.Ports, func(left, right ServicePort) bool {
		return left.Equal(right)
	})
}

// DeepCopy returns a detached copy of the Service model.
func (s Service) DeepCopy() Service {
	copied := s
	copied.Ports = make([]ServicePort, 0, len(s.Ports))
	for _, port := range s.Ports {
		copied.Ports = append(copied.Ports, port.DeepCopy())
	}
	return copied
}

// DeepCopy returns a detached copy of the Service port model.
func (p ServicePort) DeepCopy() ServicePort {
	copied := p
	copied.Backends = slices.Clone(p.Backends)
	return copied
}

// Equal reports whether the ServicePort describes the same provider state.
func (p ServicePort) Equal(other ServicePort) bool {
	return p.Name == other.Name &&
		p.Protocol == other.Protocol &&
		p.Port == other.Port &&
		p.TargetPort == other.TargetPort &&
		slices.Equal(p.Backends, other.Backends)
}
