package backends

import (
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

func TestDiscoverFiltersAndSortsIPv4ReadyBackends(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}

	ready := true
	notReady := false
	httpName := "http"
	otherService := "other"

	slicesToDiscover := []discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-a",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.0.0.2", "2001:db8::1"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
				{
					Addresses: []string{"10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &notReady,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-b",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"invalid-address", "10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
				{
					Addresses: []string{},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: otherService,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.0.0.9"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
			},
		},
	}

	discovered := Discover(service, slicesToDiscover)
	if len(discovered) != 1 {
		t.Fatalf("Discover() len = %d, want 1", len(discovered))
	}

	wantBackends := []provider.BackendEndpoint{
		{Address: "10.0.0.1", Port: 8080},
		{Address: "10.0.0.2", Port: 8080},
	}
	if !slices.Equal(discovered[0].Backends, wantBackends) {
		t.Fatalf("Discover() backends = %+v, want %+v", discovered[0].Backends, wantBackends)
	}
}

func TestDiscoverMatchesPortsByNameAndTargetPort(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromString("web"),
				},
				{
					Name:       "metrics",
					Protocol:   corev1.ProtocolTCP,
					Port:       9090,
					TargetPort: intstr.FromInt32(9091),
				},
			},
		},
	}

	webName := "web"
	metricsName := "metrics"

	discovered := Discover(service, []discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &webName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
				{Name: &metricsName, Port: ptr.To[int32](9091), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.0.0.3"}},
			},
		},
	})

	if len(discovered) != 2 {
		t.Fatalf("Discover() len = %d, want 2", len(discovered))
	}

	if !slices.Equal(discovered[0].Backends, []provider.BackendEndpoint{{Address: "10.0.0.3", Port: 8080}}) {
		t.Fatalf("http backends = %+v, want 10.0.0.3:8080", discovered[0].Backends)
	}

	if !slices.Equal(discovered[1].Backends, []provider.BackendEndpoint{{Address: "10.0.0.3", Port: 9091}}) {
		t.Fatalf("metrics backends = %+v, want 10.0.0.3:9091", discovered[1].Backends)
	}
}

func TestDiscoverIgnoresOtherNamespacesAndDeduplicatesBackends(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}

	httpName := "http"

	discovered := Discover(service, []discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-default",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.0.0.2"}},
				{Addresses: []string{"10.0.0.2"}},
				{Addresses: []string{"10.0.0.1"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-other-namespace",
				Namespace: "other",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.0.0.9"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-empty",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			Ports: []discoveryv1.EndpointPort{
				{Name: &httpName, Port: ptr.To[int32](8080), Protocol: ptr.To(corev1.ProtocolTCP)},
			},
		},
	})

	if len(discovered) != 1 {
		t.Fatalf("Discover() len = %d, want 1", len(discovered))
	}

	wantBackends := []provider.BackendEndpoint{
		{Address: "10.0.0.1", Port: 8080},
		{Address: "10.0.0.2", Port: 8080},
	}
	if !slices.Equal(discovered[0].Backends, wantBackends) {
		t.Fatalf("Discover() backends = %+v, want %+v", discovered[0].Backends, wantBackends)
	}
}

func TestDiscoverReturnsEmptyBackendsWhenNoMatchingEndpointSlicesExist(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}

	discovered := Discover(service, nil)
	if len(discovered) != 1 {
		t.Fatalf("Discover() len = %d, want 1", len(discovered))
	}

	if len(discovered[0].Backends) != 0 {
		t.Fatalf("Discover() backends = %+v, want empty slice", discovered[0].Backends)
	}
}
