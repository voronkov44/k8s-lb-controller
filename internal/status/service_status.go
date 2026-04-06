package status

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DesiredLoadBalancerIngress builds the desired LoadBalancer ingress slice for an assigned IP.
func DesiredLoadBalancerIngress(assignedIP string) []corev1.LoadBalancerIngress {
	return []corev1.LoadBalancerIngress{
		{IP: assignedIP},
	}
}

// LoadBalancerIngressEqual reports whether two ingress slices are semantically equivalent for Phase 2.
func LoadBalancerIngressEqual(current, desired []corev1.LoadBalancerIngress) bool {
	desiredIP, desiredHasIP := firstIngressIP(desired)
	if !desiredHasIP {
		return len(current) == 0
	}

	if len(current) != 1 {
		return false
	}

	return strings.TrimSpace(current[0].IP) == desiredIP
}

// NeedsLoadBalancerIngressUpdate reports whether the Service status differs from the desired ingress.
func NeedsLoadBalancerIngressUpdate(service *corev1.Service, desired []corev1.LoadBalancerIngress) bool {
	if service == nil {
		return true
	}

	return !LoadBalancerIngressEqual(service.Status.LoadBalancer.Ingress, desired)
}

// UpdateServiceLoadBalancerStatus updates Service status only when the desired ingress differs.
func UpdateServiceLoadBalancerStatus(
	ctx context.Context,
	k8sClient client.Client,
	service *corev1.Service,
	desired []corev1.LoadBalancerIngress,
) (bool, error) {
	if service == nil {
		return false, fmt.Errorf("service is nil")
	}

	if !NeedsLoadBalancerIngressUpdate(service, desired) {
		return false, nil
	}

	updated := service.DeepCopy()
	updated.Status.LoadBalancer.Ingress = desired

	if err := k8sClient.Status().Update(ctx, updated); err != nil {
		return false, fmt.Errorf("update service status %s/%s: %w", service.Namespace, service.Name, err)
	}

	service.Status = updated.Status

	return true, nil
}

// HasLoadBalancerIngressIPInPool reports whether the Service currently publishes an IPv4 ingress
// address from the controller-managed pool.
func HasLoadBalancerIngressIPInPool(service *corev1.Service, pool []netip.Addr) bool {
	if service == nil {
		return false
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		ip := strings.TrimSpace(ingress.IP)
		if ip == "" {
			continue
		}

		addr, err := netip.ParseAddr(ip)
		if err != nil || !addr.Is4() {
			continue
		}

		if slices.Contains(pool, addr) {
			return true
		}
	}

	return false
}

func firstIngressIP(ingresses []corev1.LoadBalancerIngress) (string, bool) {
	for _, ingress := range ingresses {
		ip := strings.TrimSpace(ingress.IP)
		if ip != "" {
			return ip, true
		}
	}

	return "", false
}
