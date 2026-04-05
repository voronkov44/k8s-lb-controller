package provider

import (
	"context"
	"fmt"
	"slices"
)

// Provider manages external load balancer state for a single Service.
type Provider interface {
	Ensure(ctx context.Context, service Service) error
	Delete(ctx context.Context, ref ServiceRef) error
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
