package ipam

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAllocatorAllocate(t *testing.T) {
	pool := []netip.Addr{
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
		netip.MustParseAddr("203.0.113.12"),
	}
	class := "iedge.local/service-lb"
	otherClass := "diploma.local/other"
	now := metav1.NewTime(time.Now())

	managedMatcher := func(service *corev1.Service) bool {
		if service == nil {
			return false
		}

		if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
			return false
		}

		if service.Spec.LoadBalancerClass == nil {
			return false
		}

		return *service.Spec.LoadBalancerClass == class
	}

	tests := []struct {
		name       string
		current    *corev1.Service
		services   []corev1.Service
		wantIP     netip.Addr
		wantReused bool
		wantErr    error
	}{
		{
			name:    "keeps already assigned valid IP",
			current: newIPAMService("demo", "default", class, "203.0.113.11"),
			services: []corev1.Service{
				*newIPAMService("demo", "default", class, "203.0.113.11"),
				*newIPAMService("other", "default", class, "203.0.113.10"),
			},
			wantIP:     netip.MustParseAddr("203.0.113.11"),
			wantReused: true,
		},
		{
			name:    "allocates first free IP",
			current: newIPAMService("demo", "default", class, ""),
			services: []corev1.Service{
				*newIPAMService("taken", "default", class, "203.0.113.10"),
			},
			wantIP: netip.MustParseAddr("203.0.113.11"),
		},
		{
			name:    "returns pool exhaustion",
			current: newIPAMService("demo", "default", class, ""),
			services: []corev1.Service{
				*newIPAMService("svc-1", "default", class, "203.0.113.10"),
				*newIPAMService("svc-2", "default", class, "203.0.113.11"),
				*newIPAMService("svc-3", "default", class, "203.0.113.12"),
			},
			wantErr: ErrNoFreeIP,
		},
		{
			name:    "ignores non managed services as owners",
			current: newIPAMService("demo", "default", class, ""),
			services: []corev1.Service{
				*newIPAMService("other-class", "default", otherClass, "203.0.113.10"),
				*newIPAMClusterIPService("cluster-ip", "default", class, "203.0.113.11"),
			},
			wantIP: netip.MustParseAddr("203.0.113.10"),
		},
		{
			name:    "ignores deleting services as owners",
			current: newIPAMService("demo", "default", class, ""),
			services: []corev1.Service{
				*newDeletingIPAMService("deleting", "default", class, "203.0.113.10", &now),
			},
			wantIP: netip.MustParseAddr("203.0.113.10"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocator := NewAllocator(pool, managedMatcher)

			allocation, err := allocator.Allocate(tt.current, tt.services)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Allocate() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}

			if allocation.IP != tt.wantIP {
				t.Fatalf("Allocate() IP = %s, want %s", allocation.IP, tt.wantIP)
			}

			if allocation.Reused != tt.wantReused {
				t.Fatalf("Allocate() Reused = %t, want %t", allocation.Reused, tt.wantReused)
			}
		})
	}
}

func newIPAMService(name, namespace, class, ingressIP string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:              corev1.ServiceTypeLoadBalancer,
			LoadBalancerClass: &class,
		},
	}

	if ingressIP != "" {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ingressIP}}
	}

	return service
}

func newDeletingIPAMService(name, namespace, class, ingressIP string, deletedAt *metav1.Time) *corev1.Service {
	service := newIPAMService(name, namespace, class, ingressIP)
	service.DeletionTimestamp = deletedAt
	return service
}

func newIPAMClusterIPService(name, namespace, class, ingressIP string) *corev1.Service {
	service := newIPAMService(name, namespace, class, ingressIP)
	service.Spec.Type = corev1.ServiceTypeClusterIP
	return service
}
