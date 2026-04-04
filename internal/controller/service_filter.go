package controller

import corev1 "k8s.io/api/core/v1"

func isManagedLoadBalancerService(service *corev1.Service, expectedLoadBalancerClass string) bool {
	if service == nil {
		return false
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}

	if service.Spec.LoadBalancerClass == nil {
		return false
	}

	return *service.Spec.LoadBalancerClass == expectedLoadBalancerClass
}

func serviceLoadBalancerClass(service *corev1.Service) string {
	if service == nil || service.Spec.LoadBalancerClass == nil {
		return ""
	}

	return *service.Spec.LoadBalancerClass
}
