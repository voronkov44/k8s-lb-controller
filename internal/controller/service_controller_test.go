package controller

import (
	"context"
	"net/netip"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
)

func TestServiceReconcilerReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	requeueAfter := 30 * time.Second
	class := "iedge.local/service-lb"
	ipPool := []netip.Addr{
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
	}
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name              string
		objects           []runtime.Object
		request           types.NamespacedName
		wantResult        ctrl.Result
		wantStatusIP      string
		wantStatusUpdates int
	}{
		{
			name: "returns empty result when service is not found",
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "missing",
			},
			wantResult: ctrl.Result{},
		},
		{
			name: "ignores non matching service",
			objects: []runtime.Object{
				newReconcileService("ignored", "default", corev1.ServiceTypeClusterIP, nil),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "ignored",
			},
			wantResult: ctrl.Result{},
		},
		{
			name: "skips deleting service",
			objects: []runtime.Object{
				newDeletingReconcileService("deleting", "default", corev1.ServiceTypeLoadBalancer, &class, &now),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "deleting",
			},
			wantResult: ctrl.Result{},
		},
		{
			name: "assigns external IP to matching service",
			objects: []runtime.Object{
				newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "demo",
			},
			wantResult:        ctrl.Result{RequeueAfter: requeueAfter},
			wantStatusIP:      "203.0.113.10",
			wantStatusUpdates: 1,
		},
		{
			name: "handles exhausted pool gracefully",
			objects: []runtime.Object{
				newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class),
				newReconcileServiceWithStatus("svc-1", "default", corev1.ServiceTypeLoadBalancer, &class, "203.0.113.10"),
				newReconcileServiceWithStatus("svc-2", "default", corev1.ServiceTypeLoadBalancer, &class, "203.0.113.11"),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "demo",
			},
			wantResult: ctrl.Result{RequeueAfter: requeueAfter},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&corev1.Service{})
			if len(tt.objects) > 0 {
				clientBuilder = clientBuilder.WithRuntimeObjects(tt.objects...)
			}

			countingClient := newCountingClient(clientBuilder.Build())
			reconciler := &ServiceReconciler{
				Client: countingClient,
				Scheme: scheme,
				Config: config.Config{
					LoadBalancerClass: class,
					IPPool:            ipPool,
					RequeueAfter:      requeueAfter,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: tt.request})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			if result != tt.wantResult {
				t.Fatalf("Reconcile() result = %+v, want %+v", result, tt.wantResult)
			}

			if countingClient.statusUpdates != tt.wantStatusUpdates {
				t.Fatalf("status update count = %d, want %d", countingClient.statusUpdates, tt.wantStatusUpdates)
			}

			if tt.wantStatusIP == "" {
				return
			}

			stored := &corev1.Service{}
			if err := countingClient.Get(context.Background(), tt.request, stored); err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if len(stored.Status.LoadBalancer.Ingress) != 1 {
				t.Fatalf("stored ingress len = %d, want 1", len(stored.Status.LoadBalancer.Ingress))
			}

			if stored.Status.LoadBalancer.Ingress[0].IP != tt.wantStatusIP {
				t.Fatalf("stored ingress IP = %q, want %q", stored.Status.LoadBalancer.Ingress[0].IP, tt.wantStatusIP)
			}
		})
	}
}

func TestServiceReconcilerReconcileDoesNotRewriteStatusOnSecondPass(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	class := "iedge.local/service-lb"
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&corev1.Service{}).
		WithRuntimeObjects(service).
		Build()
	countingClient := newCountingClient(baseClient)

	reconciler := &ServiceReconciler{
		Client: countingClient,
		Scheme: scheme,
		Config: config.Config{
			LoadBalancerClass: class,
			IPPool: []netip.Addr{
				netip.MustParseAddr("203.0.113.10"),
				netip.MustParseAddr("203.0.113.11"),
			},
			RequeueAfter: 30 * time.Second,
		},
	}

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	if countingClient.statusUpdates != 1 {
		t.Fatalf("first status update count = %d, want 1", countingClient.statusUpdates)
	}

	countingClient.statusUpdates = 0

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("second status update count = %d, want 0", countingClient.statusUpdates)
	}
}

func TestServiceReconcilerReconcileDoesNotRewriteStatusWhenSameIPHasIPMode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	class := "iedge.local/service-lb"
	ipMode := corev1.LoadBalancerIPModeVIP
	service := newReconcileServiceWithStatusAndIPMode(
		"demo",
		"default",
		corev1.ServiceTypeLoadBalancer,
		&class,
		"203.0.113.10",
		&ipMode,
	)

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&corev1.Service{}).
		WithRuntimeObjects(service).
		Build()
	countingClient := newCountingClient(baseClient)

	reconciler := &ServiceReconciler{
		Client: countingClient,
		Scheme: scheme,
		Config: config.Config{
			LoadBalancerClass: class,
			IPPool: []netip.Addr{
				netip.MustParseAddr("203.0.113.10"),
				netip.MustParseAddr("203.0.113.11"),
			},
			RequeueAfter: 30 * time.Second,
		},
	}

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("status update count = %d, want 0", countingClient.statusUpdates)
	}
}

func newReconcileService(name, namespace string, serviceType corev1.ServiceType, loadBalancerClass *string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:              serviceType,
			LoadBalancerClass: loadBalancerClass,
		},
	}
}

func newReconcileServiceWithStatus(name, namespace string, serviceType corev1.ServiceType, loadBalancerClass *string, ingressIP string) *corev1.Service {
	service := newReconcileService(name, namespace, serviceType, loadBalancerClass)
	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ingressIP}}
	return service
}

func newReconcileServiceWithStatusAndIPMode(
	name, namespace string,
	serviceType corev1.ServiceType,
	loadBalancerClass *string,
	ingressIP string,
	ipMode *corev1.LoadBalancerIPMode,
) *corev1.Service {
	service := newReconcileService(name, namespace, serviceType, loadBalancerClass)
	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{
		IP:     ingressIP,
		IPMode: ipMode,
	}}
	return service
}

func newDeletingReconcileService(
	name, namespace string,
	serviceType corev1.ServiceType,
	loadBalancerClass *string,
	deletedAt *metav1.Time,
) *corev1.Service {
	service := newReconcileService(name, namespace, serviceType, loadBalancerClass)
	service.DeletionTimestamp = deletedAt
	service.Finalizers = []string{"test.finalizer"}
	return service
}

type countingClient struct {
	client.Client
	statusUpdates int
}

func newCountingClient(base client.Client) *countingClient {
	return &countingClient{Client: base}
}

func (c *countingClient) Status() client.SubResourceWriter {
	return &countingStatusWriter{
		SubResourceWriter: c.Client.Status(),
		statusUpdates:     &c.statusUpdates,
	}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	statusUpdates *int
}

func (w *countingStatusWriter) Update(
	ctx context.Context,
	obj client.Object,
	opts ...client.SubResourceUpdateOption,
) error {
	if _, ok := obj.(*corev1.Service); ok {
		*w.statusUpdates = *w.statusUpdates + 1
	}

	return w.SubResourceWriter.Update(ctx, obj, opts...)
}
