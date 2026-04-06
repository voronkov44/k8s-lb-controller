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

package status

import (
	"context"
	"net/netip"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testLoadBalancerIP = "203.0.113.10"

func TestDesiredLoadBalancerIngress(t *testing.T) {
	desired := DesiredLoadBalancerIngress(testLoadBalancerIP)

	if len(desired) != 1 {
		t.Fatalf("DesiredLoadBalancerIngress() len = %d, want 1", len(desired))
	}

	if desired[0].IP != testLoadBalancerIP {
		t.Fatalf("DesiredLoadBalancerIngress() IP = %q, want %q", desired[0].IP, testLoadBalancerIP)
	}
}

func TestLoadBalancerIngressEqual(t *testing.T) {
	current := []corev1.LoadBalancerIngress{{IP: testLoadBalancerIP}}
	same := []corev1.LoadBalancerIngress{{IP: testLoadBalancerIP}}
	other := []corev1.LoadBalancerIngress{{IP: "203.0.113.11"}}
	ipMode := corev1.LoadBalancerIPModeVIP
	withIPMode := []corev1.LoadBalancerIngress{{IP: testLoadBalancerIP, IPMode: &ipMode}}

	if !LoadBalancerIngressEqual(current, same) {
		t.Fatal("LoadBalancerIngressEqual() = false, want true")
	}

	if !LoadBalancerIngressEqual(withIPMode, same) {
		t.Fatal("LoadBalancerIngressEqual() with same IP and IPMode = false, want true")
	}

	if LoadBalancerIngressEqual(current, other) {
		t.Fatal("LoadBalancerIngressEqual() = true, want false")
	}
}

func TestUpdateServiceLoadBalancerStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	t.Run("updates status when needed", func(t *testing.T) {
		service := newStatusService("")
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress(testLoadBalancerIP),
		)
		if err != nil {
			t.Fatalf("UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if !updated {
			t.Fatal("UpdateServiceLoadBalancerStatus() updated = false, want true")
		}

		stored := &corev1.Service{}
		if err := k8sClient.Get(context.Background(), clientObjectKey(service), stored); err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if len(stored.Status.LoadBalancer.Ingress) != 1 || stored.Status.LoadBalancer.Ingress[0].IP != testLoadBalancerIP {
			t.Fatalf("stored status ingress = %+v, want IP %s", stored.Status.LoadBalancer.Ingress, testLoadBalancerIP)
		}
	})

	t.Run("skips update when status is already desired", func(t *testing.T) {
		service := newStatusService(testLoadBalancerIP)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress(testLoadBalancerIP),
		)
		if err != nil {
			t.Fatalf("UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if updated {
			t.Fatal("UpdateServiceLoadBalancerStatus() updated = true, want false")
		}
	})

	t.Run("skips update when status has same IP with IPMode", func(t *testing.T) {
		ipMode := corev1.LoadBalancerIPModeVIP
		service := newStatusServiceWithIngress(corev1.LoadBalancerIngress{
			IP:     testLoadBalancerIP,
			IPMode: &ipMode,
		})
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress(testLoadBalancerIP),
		)
		if err != nil {
			t.Fatalf("UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if updated {
			t.Fatal("UpdateServiceLoadBalancerStatus() updated = true, want false")
		}

		stored := &corev1.Service{}
		if err := k8sClient.Get(context.Background(), clientObjectKey(service), stored); err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if len(stored.Status.LoadBalancer.Ingress) != 1 || stored.Status.LoadBalancer.Ingress[0].IPMode == nil {
			t.Fatalf("stored status ingress = %+v, want preserved IPMode", stored.Status.LoadBalancer.Ingress)
		}
	})

	t.Run("rewrites status when stale ingress entries are present", func(t *testing.T) {
		service := newStatusServiceWithIngress(
			corev1.LoadBalancerIngress{IP: testLoadBalancerIP},
			corev1.LoadBalancerIngress{IP: "203.0.113.11"},
		)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress(testLoadBalancerIP),
		)
		if err != nil {
			t.Fatalf("UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if !updated {
			t.Fatal("UpdateServiceLoadBalancerStatus() updated = false, want true")
		}

		stored := &corev1.Service{}
		if err := k8sClient.Get(context.Background(), clientObjectKey(service), stored); err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if len(stored.Status.LoadBalancer.Ingress) != 1 || stored.Status.LoadBalancer.Ingress[0].IP != testLoadBalancerIP {
			t.Fatalf("stored status ingress = %+v, want only IP %s", stored.Status.LoadBalancer.Ingress, testLoadBalancerIP)
		}
	})

	t.Run("clears status when desired ingress is empty and remains stable", func(t *testing.T) {
		service := newStatusService(testLoadBalancerIP)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			nil,
		)
		if err != nil {
			t.Fatalf("first UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if !updated {
			t.Fatal("first UpdateServiceLoadBalancerStatus() updated = false, want true")
		}

		updated, err = UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			nil,
		)
		if err != nil {
			t.Fatalf("second UpdateServiceLoadBalancerStatus() error = %v", err)
		}

		if updated {
			t.Fatal("second UpdateServiceLoadBalancerStatus() updated = true, want false")
		}
	})
}

func TestHasLoadBalancerIngressIPInPool(t *testing.T) {
	service := newStatusService(testLoadBalancerIP)
	pool := []netip.Addr{
		netip.MustParseAddr(testLoadBalancerIP),
		netip.MustParseAddr("203.0.113.11"),
	}

	if !HasLoadBalancerIngressIPInPool(service, pool) {
		t.Fatal("HasLoadBalancerIngressIPInPool() = false, want true")
	}

	if HasLoadBalancerIngressIPInPool(newStatusService("198.51.100.10"), pool) {
		t.Fatal("HasLoadBalancerIngressIPInPool() = true, want false")
	}
}

func newStatusService(ingressIP string) *corev1.Service {
	service := newStatusServiceWithIngress(corev1.LoadBalancerIngress{})
	if ingressIP == "" {
		return service
	}

	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ingressIP}}
	return service
}

func newStatusServiceWithIngress(ingresses ...corev1.LoadBalancerIngress) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
	}

	nonEmptyIngresses := make([]corev1.LoadBalancerIngress, 0, len(ingresses))
	for _, ingress := range ingresses {
		if ingress.IP == "" && ingress.Hostname == "" && ingress.IPMode == nil && len(ingress.Ports) == 0 {
			continue
		}

		nonEmptyIngresses = append(nonEmptyIngresses, ingress)
	}

	if len(nonEmptyIngresses) > 0 {
		service.Status.LoadBalancer.Ingress = nonEmptyIngresses
	}

	return service
}

func clientObjectKey(service *corev1.Service) client.ObjectKey {
	return client.ObjectKey{
		Namespace: service.Namespace,
		Name:      service.Name,
	}
}
