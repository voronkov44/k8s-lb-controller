package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsManagedLoadBalancerService(t *testing.T) {
	class := "k8s-lb-controller"
	otherClass := "diploma.local/other"

	tests := []struct {
		name    string
		service *corev1.Service
		want    bool
	}{
		{
			name:    "matches configured class",
			service: newTestService(corev1.ServiceTypeLoadBalancer, &class),
			want:    true,
		},
		{
			name:    "ignores non load balancer service",
			service: newTestService(corev1.ServiceTypeClusterIP, &class),
			want:    false,
		},
		{
			name:    "ignores missing load balancer class",
			service: newTestService(corev1.ServiceTypeLoadBalancer, nil),
			want:    false,
		},
		{
			name:    "ignores different load balancer class",
			service: newTestService(corev1.ServiceTypeLoadBalancer, &otherClass),
			want:    false,
		},
		{
			name:    "ignores nil service",
			service: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isManagedLoadBalancerService(tt.service, class)
			if got != tt.want {
				t.Fatalf("isManagedLoadBalancerService() = %t, want %t", got, tt.want)
			}
		})
	}
}

func newTestService(serviceType corev1.ServiceType, loadBalancerClass *string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:              serviceType,
			LoadBalancerClass: loadBalancerClass,
		},
	}
}
