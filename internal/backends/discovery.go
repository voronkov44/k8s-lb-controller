package backends

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

// ServicePortBackends contains the discovered backends for a single Service port.
type ServicePortBackends struct {
	Name       string
	Protocol   corev1.Protocol
	Port       int32
	TargetPort intstr.IntOrString
	Backends   []provider.BackendEndpoint
}

// Discover builds backend endpoints for each Service port from EndpointSlice resources.
func Discover(service *corev1.Service, endpointSlices []discoveryv1.EndpointSlice) []ServicePortBackends {
	if service == nil {
		return nil
	}

	discovered := make([]ServicePortBackends, 0, len(service.Spec.Ports))
	for _, servicePort := range service.Spec.Ports {
		portBackends := ServicePortBackends{
			Name:       servicePort.Name,
			Protocol:   normalizeServiceProtocol(servicePort.Protocol),
			Port:       servicePort.Port,
			TargetPort: servicePort.TargetPort,
		}

		seen := make(map[string]struct{})
		for _, endpointSlice := range endpointSlices {
			if !endpointSliceBelongsToService(service, endpointSlice) {
				continue
			}

			for _, endpointSlicePort := range endpointSlice.Ports {
				if !endpointSlicePortMatchesServicePort(servicePort, endpointSlicePort) {
					continue
				}

				for _, endpoint := range endpointSlice.Endpoints {
					if !endpointReady(endpoint) {
						continue
					}

					for _, address := range endpoint.Addresses {
						normalizedAddress, ok := normalizeIPv4Address(address)
						if !ok {
							continue
						}

						backend := provider.BackendEndpoint{
							Address: normalizedAddress,
							Port:    *endpointSlicePort.Port,
						}

						key := backend.Address + ":" + int32ToString(backend.Port)
						if _, ok := seen[key]; ok {
							continue
						}

						seen[key] = struct{}{}
						portBackends.Backends = append(portBackends.Backends, backend)
					}
				}
			}
		}

		sort.Slice(portBackends.Backends, func(i, j int) bool {
			if portBackends.Backends[i].Address == portBackends.Backends[j].Address {
				return portBackends.Backends[i].Port < portBackends.Backends[j].Port
			}

			return portBackends.Backends[i].Address < portBackends.Backends[j].Address
		})

		discovered = append(discovered, portBackends)
	}

	return discovered
}

func endpointSliceBelongsToService(service *corev1.Service, endpointSlice discoveryv1.EndpointSlice) bool {
	if service == nil {
		return false
	}

	if endpointSlice.Namespace != service.Namespace {
		return false
	}

	return endpointSlice.Labels[discoveryv1.LabelServiceName] == service.Name
}

func endpointSlicePortMatchesServicePort(
	servicePort corev1.ServicePort,
	endpointSlicePort discoveryv1.EndpointPort,
) bool {
	if endpointSlicePort.Port == nil || *endpointSlicePort.Port <= 0 {
		return false
	}

	if normalizeServiceProtocol(servicePort.Protocol) != normalizeEndpointSliceProtocol(endpointSlicePort.Protocol) {
		return false
	}

	endpointPortName := strings.TrimSpace(valueOrEmpty(endpointSlicePort.Name))
	if servicePort.Name != "" && endpointPortName == servicePort.Name {
		return true
	}

	targetPortName, targetPortNumber := servicePortTargetPort(servicePort)
	if targetPortName != "" {
		return endpointPortName == targetPortName
	}

	return *endpointSlicePort.Port == targetPortNumber
}

func normalizeServiceProtocol(protocol corev1.Protocol) corev1.Protocol {
	if protocol == "" {
		return corev1.ProtocolTCP
	}

	return protocol
}

func normalizeEndpointSliceProtocol(protocol *corev1.Protocol) corev1.Protocol {
	if protocol == nil || *protocol == "" {
		return corev1.ProtocolTCP
	}

	return *protocol
}

func servicePortTargetPort(servicePort corev1.ServicePort) (string, int32) {
	switch servicePort.TargetPort.Type {
	case intstr.String:
		if strings.TrimSpace(servicePort.TargetPort.StrVal) != "" {
			return strings.TrimSpace(servicePort.TargetPort.StrVal), 0
		}
	case intstr.Int:
		if servicePort.TargetPort.IntVal > 0 {
			return "", servicePort.TargetPort.IntVal
		}
	}

	return "", servicePort.Port
}

func endpointReady(endpoint discoveryv1.Endpoint) bool {
	return endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
}

func normalizeIPv4Address(address string) (string, bool) {
	parsedAddress, err := netip.ParseAddr(strings.TrimSpace(address))
	if err != nil || !parsedAddress.Is4() {
		return "", false
	}

	return parsedAddress.String(), true
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func int32ToString(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}
