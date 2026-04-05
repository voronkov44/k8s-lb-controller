package status

import (
	"context"
	"fmt"
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
		_, currentHasIP := firstIngressIP(current)
		return !currentHasIP
	}

	return containsIngressIP(current, desiredIP)
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

func firstIngressIP(ingresses []corev1.LoadBalancerIngress) (string, bool) {
	for _, ingress := range ingresses {
		ip := strings.TrimSpace(ingress.IP)
		if ip != "" {
			return ip, true
		}
	}

	return "", false
}

func containsIngressIP(ingresses []corev1.LoadBalancerIngress, expectedIP string) bool {
	for _, ingress := range ingresses {
		if strings.TrimSpace(ingress.IP) == expectedIP {
			return true
		}
	}

	return false
}
