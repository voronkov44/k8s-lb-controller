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

package haproxy

import (
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

// Render produces a deterministic HAProxy configuration for the provided Services.
func Render(services []provider.Service) (string, error) {
	sortedServices := make([]provider.Service, 0, len(services))
	for _, service := range services {
		sortedServices = append(sortedServices, service.DeepCopy())
	}

	sort.Slice(sortedServices, func(i, j int) bool {
		if sortedServices[i].Namespace == sortedServices[j].Namespace {
			return sortedServices[i].Name < sortedServices[j].Name
		}

		return sortedServices[i].Namespace < sortedServices[j].Namespace
	})

	var builder strings.Builder
	builder.WriteString(configHeader)

	for _, service := range sortedServices {
		if err := validateService(service); err != nil {
			return "", fmt.Errorf("render %s: %w", service.Ref().String(), err)
		}

		ports := make([]provider.ServicePort, 0, len(service.Ports))
		for _, port := range service.Ports {
			ports = append(ports, port.DeepCopy())
		}

		sort.Slice(ports, func(i, j int) bool {
			if ports[i].Port != ports[j].Port {
				return ports[i].Port < ports[j].Port
			}
			if ports[i].Protocol != ports[j].Protocol {
				return ports[i].Protocol < ports[j].Protocol
			}
			if ports[i].Name != ports[j].Name {
				return ports[i].Name < ports[j].Name
			}

			return ports[i].TargetPort < ports[j].TargetPort
		})

		for _, port := range ports {
			backendName := backendIdentifier(service, port)
			frontendName := frontendIdentifier(service, port)

			builder.WriteString("\nfrontend ")
			builder.WriteString(frontendName)
			builder.WriteString("\n")
			builder.WriteString("    bind ")
			builder.WriteString(service.ExternalIP)
			builder.WriteString(":")
			builder.WriteString(int32ToString(port.Port))
			builder.WriteString("\n")
			builder.WriteString("    mode tcp\n")
			builder.WriteString("    default_backend ")
			builder.WriteString(backendName)
			builder.WriteString("\n\n")

			builder.WriteString("backend ")
			builder.WriteString(backendName)
			builder.WriteString("\n")
			builder.WriteString("    mode tcp\n")
			builder.WriteString("    balance roundrobin\n")

			backends := slices.Clone(port.Backends)
			sort.Slice(backends, func(i, j int) bool {
				if backends[i].Address == backends[j].Address {
					return backends[i].Port < backends[j].Port
				}

				return backends[i].Address < backends[j].Address
			})

			if len(backends) == 0 {
				builder.WriteString("    server srv_unavailable 127.0.0.1:1 disabled\n")
				continue
			}

			for index, backend := range backends {
				builder.WriteString("    server ")
				builder.WriteString(serverIdentifier(index, backend))
				builder.WriteString(" ")
				builder.WriteString(backend.Address)
				builder.WriteString(":")
				builder.WriteString(int32ToString(backend.Port))
				builder.WriteString("\n")
			}
		}
	}

	return builder.String(), nil
}

func validateService(service provider.Service) error {
	if strings.TrimSpace(service.Namespace) == "" {
		return fmt.Errorf("namespace must not be empty")
	}

	if strings.TrimSpace(service.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}

	externalIP, err := netip.ParseAddr(service.ExternalIP)
	if err != nil || !externalIP.Is4() {
		return fmt.Errorf("external IP %q must be a valid IPv4 address", service.ExternalIP)
	}

	for _, port := range service.Ports {
		if !strings.EqualFold(strings.TrimSpace(port.Protocol), "TCP") && strings.TrimSpace(port.Protocol) != "" {
			return fmt.Errorf("service port %d uses unsupported protocol %q", port.Port, port.Protocol)
		}

		if port.Port <= 0 {
			return fmt.Errorf("service port must be greater than zero")
		}

		for _, backend := range port.Backends {
			backendAddress, err := netip.ParseAddr(backend.Address)
			if err != nil || !backendAddress.Is4() {
				return fmt.Errorf("backend address %q must be a valid IPv4 address", backend.Address)
			}

			if backend.Port <= 0 {
				return fmt.Errorf("backend port must be greater than zero")
			}
		}
	}

	return nil
}

func frontendIdentifier(service provider.Service, port provider.ServicePort) string {
	return "fe_" + sanitizeIdentifier(identifierSuffix(service, port))
}

func backendIdentifier(service provider.Service, port provider.ServicePort) string {
	return "be_" + sanitizeIdentifier(identifierSuffix(service, port))
}

func serverIdentifier(index int, backend provider.BackendEndpoint) string {
	return sanitizeIdentifier(fmt.Sprintf("srv_%04d_%s_%d", index+1, backend.Address, backend.Port))
}

func identifierSuffix(service provider.Service, port provider.ServicePort) string {
	return strings.ToLower(fmt.Sprintf(
		"%s_%s_%d_%s_%s",
		service.Namespace,
		service.Name,
		port.Port,
		port.Name,
		port.Protocol,
	))
}

func sanitizeIdentifier(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "unnamed"
	}

	var builder strings.Builder
	builder.Grow(len(trimmed))
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	sanitized := builder.String()
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return "unnamed"
	}

	return sanitized
}

func int32ToString(value int32) string {
	return fmt.Sprintf("%d", value)
}
