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

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const managedServiceClass = "iedge.local/service-lb"

func TestIsManagedLoadBalancerService(t *testing.T) {
	class := managedServiceClass
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
