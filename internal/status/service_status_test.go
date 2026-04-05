package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDesiredLoadBalancerIngress(t *testing.T) {
	desired := DesiredLoadBalancerIngress("203.0.113.10")

	if len(desired) != 1 {
		t.Fatalf("DesiredLoadBalancerIngress() len = %d, want 1", len(desired))
	}

	if desired[0].IP != "203.0.113.10" {
		t.Fatalf("DesiredLoadBalancerIngress() IP = %q, want %q", desired[0].IP, "203.0.113.10")
	}
}

func TestLoadBalancerIngressEqual(t *testing.T) {
	current := []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	same := []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	other := []corev1.LoadBalancerIngress{{IP: "203.0.113.11"}}
	ipMode := corev1.LoadBalancerIPModeVIP
	withIPMode := []corev1.LoadBalancerIngress{{IP: "203.0.113.10", IPMode: &ipMode}}

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
		service := newStatusService("demo", "default", "")
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress("203.0.113.10"),
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

		if len(stored.Status.LoadBalancer.Ingress) != 1 || stored.Status.LoadBalancer.Ingress[0].IP != "203.0.113.10" {
			t.Fatalf("stored status ingress = %+v, want IP 203.0.113.10", stored.Status.LoadBalancer.Ingress)
		}
	})

	t.Run("skips update when status is already desired", func(t *testing.T) {
		service := newStatusService("demo", "default", "203.0.113.10")
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Service{}).
			WithObjects(service).
			Build()

		updated, err := UpdateServiceLoadBalancerStatus(
			context.Background(),
			k8sClient,
			service,
			DesiredLoadBalancerIngress("203.0.113.10"),
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
		service := newStatusServiceWithIngress("demo", "default", corev1.LoadBalancerIngress{
			IP:     "203.0.113.10",
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
			DesiredLoadBalancerIngress("203.0.113.10"),
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
}

func newStatusService(name, namespace, ingressIP string) *corev1.Service {
	service := newStatusServiceWithIngress(name, namespace, corev1.LoadBalancerIngress{})
	if ingressIP == "" {
		return service
	}

	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ingressIP}}
	return service
}

func newStatusServiceWithIngress(name, namespace string, ingress corev1.LoadBalancerIngress) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if ingress.IP != "" || ingress.Hostname != "" || ingress.IPMode != nil || len(ingress.Ports) > 0 {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{ingress}
	}

	return service
}

func clientObjectKey(service *corev1.Service) client.ObjectKey {
	return client.ObjectKey{
		Namespace: service.Namespace,
		Name:      service.Name,
	}
}
