package ipam

import (
	"errors"
	"fmt"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ErrNoFreeIP is returned when no address is available in the configured pool.
var ErrNoFreeIP = errors.New("no free IPs in pool")

// ServiceMatcher decides whether a Service should participate in IP allocation.
type ServiceMatcher func(*corev1.Service) bool

// Allocation describes the desired IP allocation result for a Service.
type Allocation struct {
	IP     netip.Addr
	Reused bool
}

// Allocator chooses external IPv4 addresses for managed Services.
type Allocator struct {
	pool    []netip.Addr
	matches ServiceMatcher
}

// NewAllocator creates a new allocator for the provided pool and Service matcher.
func NewAllocator(pool []netip.Addr, matches ServiceMatcher) *Allocator {
	copiedPool := append([]netip.Addr(nil), pool...)

	return &Allocator{
		pool:    copiedPool,
		matches: matches,
	}
}

// Allocate returns the desired external IP for the current Service.
func (a *Allocator) Allocate(current *corev1.Service, services []corev1.Service) (Allocation, error) {
	if current == nil {
		return Allocation{}, fmt.Errorf("service is nil")
	}

	if len(a.pool) == 0 {
		return Allocation{}, fmt.Errorf("IP pool is empty")
	}

	usedIPs := a.collectUsedIPs(current, services)
	if currentIP, ok := assignedIPFromStatus(current); ok && Contains(a.pool, currentIP) {
		if _, used := usedIPs[currentIP]; !used {
			return Allocation{IP: currentIP, Reused: true}, nil
		}
	}

	for _, candidate := range a.pool {
		if _, used := usedIPs[candidate]; used {
			continue
		}

		return Allocation{IP: candidate}, nil
	}

	return Allocation{}, ErrNoFreeIP
}

func (a *Allocator) collectUsedIPs(current *corev1.Service, services []corev1.Service) map[netip.Addr]types.NamespacedName {
	usedIPs := make(map[netip.Addr]types.NamespacedName, len(services))
	currentKey := serviceKey(current)

	for i := range services {
		service := &services[i]

		if service.DeletionTimestamp != nil {
			continue
		}

		if serviceKey(service) == currentKey {
			continue
		}

		if a.matches != nil && !a.matches(service) {
			continue
		}

		for _, ingress := range service.Status.LoadBalancer.Ingress {
			if ingress.IP == "" {
				continue
			}

			addr, err := netip.ParseAddr(ingress.IP)
			if err != nil || !addr.Is4() {
				continue
			}

			if !Contains(a.pool, addr) {
				continue
			}

			usedIPs[addr] = serviceKey(service)
		}
	}

	return usedIPs
}

func assignedIPFromStatus(service *corev1.Service) (netip.Addr, bool) {
	if service == nil {
		return netip.Addr{}, false
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP == "" {
			continue
		}

		addr, err := netip.ParseAddr(ingress.IP)
		if err != nil || !addr.Is4() {
			continue
		}

		return addr, true
	}

	return netip.Addr{}, false
}

func serviceKey(service *corev1.Service) types.NamespacedName {
	if service == nil {
		return types.NamespacedName{}
	}

	return types.NamespacedName{
		Namespace: service.Namespace,
		Name:      service.Name,
	}
}
