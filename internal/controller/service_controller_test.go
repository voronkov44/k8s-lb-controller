package controller

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

func TestServiceReconcilerReconcileReturnsEmptyWhenServiceNotFound(t *testing.T) {
	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "missing"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("status update count = %d, want 0", countingClient.statusUpdates)
	}

	if len(fakeProvider.Snapshot()) != 0 {
		t.Fatalf("provider snapshot = %+v, want empty state", fakeProvider.Snapshot())
	}
}

func TestServiceReconcilerReconcileIgnoresNonMatchingService(t *testing.T) {
	service := newReconcileService("ignored", "default", corev1.ServiceTypeClusterIP, nil)
	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "ignored"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}

	stored := getStoredService(t, countingClient, types.NamespacedName{Namespace: "default", Name: "ignored"})
	if hasServiceFinalizer(stored) {
		t.Fatalf("service finalizers = %v, want no finalizer", stored.Finalizers)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("status update count = %d, want 0", countingClient.statusUpdates)
	}

	if len(fakeProvider.ensureCalls) != 0 {
		t.Fatalf("provider ensure calls = %d, want 0", len(fakeProvider.ensureCalls))
	}
}

func TestServiceReconcilerReconcileEnsuresManagedService(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result != (ctrl.Result{RequeueAfter: 30 * time.Second}) {
		t.Fatalf("Reconcile() result = %+v, want requeue result", result)
	}

	stored := getStoredService(t, countingClient, types.NamespacedName{Namespace: "default", Name: "demo"})
	if !hasServiceFinalizer(stored) {
		t.Fatalf("service finalizers = %v, want %q", stored.Finalizers, serviceFinalizer)
	}

	if len(stored.Status.LoadBalancer.Ingress) != 1 || stored.Status.LoadBalancer.Ingress[0].IP != "203.0.113.10" {
		t.Fatalf("service status ingress = %+v, want IP 203.0.113.10", stored.Status.LoadBalancer.Ingress)
	}

	if countingClient.statusUpdates != 1 {
		t.Fatalf("status update count = %d, want 1", countingClient.statusUpdates)
	}

	snapshot := fakeProvider.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("provider snapshot len = %d, want 1", len(snapshot))
	}

	providerService, ok := snapshot[provider.ServiceRef{Namespace: "default", Name: "demo"}]
	if !ok {
		t.Fatal("provider state missing for default/demo")
	}

	if providerService.ExternalIP != "203.0.113.10" {
		t.Fatalf("provider ExternalIP = %q, want %q", providerService.ExternalIP, "203.0.113.10")
	}

	if len(providerService.Ports) != 1 {
		t.Fatalf("provider Ports len = %d, want 1", len(providerService.Ports))
	}

	if len(providerService.Ports[0].Backends) != 0 {
		t.Fatalf("provider backends = %+v, want empty slice", providerService.Ports[0].Backends)
	}
}

func TestServiceReconcilerReconcileDoesNotRewriteStatusOnSecondPass(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service)
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

	if len(fakeProvider.ensureCalls) != 2 {
		t.Fatalf("provider ensure calls = %d, want 2", len(fakeProvider.ensureCalls))
	}
}

func TestServiceReconcilerReconcileDiscoversEndpointSliceBackends(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	endpointSlice := newEndpointSlice(
		"demo-1",
		"default",
		"demo",
		"http",
		8080,
		[]endpointAddress{{ip: "10.0.0.2", ready: true}, {ip: "10.0.0.1", ready: true}, {ip: "2001:db8::1", ready: true}},
	)
	notReadySlice := newEndpointSlice(
		"demo-2",
		"default",
		"demo",
		"http",
		8080,
		[]endpointAddress{{ip: "10.0.0.3", ready: false}},
	)

	reconciler, _, fakeProvider := newTestServiceReconciler(t, nil, service, endpointSlice, notReadySlice)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	stored, ok := fakeProvider.Get(provider.ServiceRef{Namespace: "default", Name: "demo"})
	if !ok {
		t.Fatal("provider state missing for default/demo")
	}

	wantBackends := []provider.BackendEndpoint{
		{Address: "10.0.0.1", Port: 8080},
		{Address: "10.0.0.2", Port: 8080},
	}
	if len(stored.Ports) != 1 {
		t.Fatalf("provider Ports len = %d, want 1", len(stored.Ports))
	}

	if len(stored.Ports[0].Backends) != len(wantBackends) {
		t.Fatalf("provider backends len = %d, want %d", len(stored.Ports[0].Backends), len(wantBackends))
	}

	for index, backend := range wantBackends {
		if stored.Ports[0].Backends[index] != backend {
			t.Fatalf("provider backend[%d] = %+v, want %+v", index, stored.Ports[0].Backends[index], backend)
		}
	}
}

func TestServiceReconcilerReconcileUpdatesProviderStateWhenEndpointSliceChanges(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	endpointSlice := newEndpointSlice(
		"demo-1",
		"default",
		"demo",
		"http",
		8080,
		[]endpointAddress{{ip: "10.0.0.1", ready: true}},
	)

	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service, endpointSlice)
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	updatedEndpointSlice := endpointSlice.DeepCopy()
	updatedReady := true
	updatedEndpointSlice.Endpoints = []discoveryv1.Endpoint{
		{
			Addresses: []string{"10.0.0.5"},
			Conditions: discoveryv1.EndpointConditions{
				Ready: &updatedReady,
			},
		},
	}
	if err := countingClient.Update(context.Background(), updatedEndpointSlice); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if len(fakeProvider.ensureCalls) != 2 {
		t.Fatalf("provider ensure calls = %d, want 2", len(fakeProvider.ensureCalls))
	}

	lastEnsure := fakeProvider.ensureCalls[len(fakeProvider.ensureCalls)-1]
	if len(lastEnsure.Ports) != 1 || len(lastEnsure.Ports[0].Backends) != 1 {
		t.Fatalf("last ensure backends = %+v, want one backend", lastEnsure.Ports)
	}

	if lastEnsure.Ports[0].Backends[0] != (provider.BackendEndpoint{Address: "10.0.0.5", Port: 8080}) {
		t.Fatalf("last ensure backend = %+v, want 10.0.0.5:8080", lastEnsure.Ports[0].Backends[0])
	}
}

func TestServiceReconcilerReconcileDoesNotRewriteStatusWhenSameIPHasIPMode(t *testing.T) {
	class := managedServiceClass
	ipMode := corev1.LoadBalancerIPModeVIP
	service := newReconcileServiceWithStatusAndIPMode(
		"demo",
		"default",
		corev1.ServiceTypeLoadBalancer,
		&class,
		"203.0.113.10",
		&ipMode,
	)
	service.Finalizers = []string{serviceFinalizer}

	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("status update count = %d, want 0", countingClient.statusUpdates)
	}

	if len(fakeProvider.ensureCalls) != 1 {
		t.Fatalf("provider ensure calls = %d, want 1", len(fakeProvider.ensureCalls))
	}
}

func TestServiceReconcilerReconcileHandlesExhaustedPoolGracefully(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	occupiedA := newReconcileServiceWithStatus("svc-1", "default", corev1.ServiceTypeLoadBalancer, &class, "203.0.113.10")
	occupiedB := newReconcileServiceWithStatus("svc-2", "default", corev1.ServiceTypeLoadBalancer, &class, "203.0.113.11")
	reconciler, countingClient, fakeProvider := newTestServiceReconciler(t, nil, service, occupiedA, occupiedB)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result != (ctrl.Result{RequeueAfter: 30 * time.Second}) {
		t.Fatalf("Reconcile() result = %+v, want requeue result", result)
	}

	stored := getStoredService(t, countingClient, types.NamespacedName{Namespace: "default", Name: "demo"})
	if !hasServiceFinalizer(stored) {
		t.Fatalf("service finalizers = %v, want %q", stored.Finalizers, serviceFinalizer)
	}

	if len(stored.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("service status ingress = %+v, want empty ingress", stored.Status.LoadBalancer.Ingress)
	}

	if countingClient.statusUpdates != 0 {
		t.Fatalf("status update count = %d, want 0", countingClient.statusUpdates)
	}

	if len(fakeProvider.ensureCalls) != 0 {
		t.Fatalf("provider ensure calls = %d, want 0", len(fakeProvider.ensureCalls))
	}
}

func TestServiceReconcilerReconcileDeletesManagedService(t *testing.T) {
	class := managedServiceClass
	now := metav1.NewTime(time.Now())
	service := newDeletingReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class, &now)
	fakeProvider := newFakeProvider()
	if err := fakeProvider.Ensure(context.Background(), provider.Service{
		Namespace:         "default",
		Name:              "demo",
		LoadBalancerClass: class,
		ExternalIP:        "203.0.113.10",
	}); err != nil {
		t.Fatalf("Ensure() seed error = %v", err)
	}

	reconciler, countingClient, _ := newTestServiceReconciler(t, fakeProvider, service)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}

	deletedService := &corev1.Service{}
	err = countingClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "demo"}, deletedService)
	if err == nil {
		t.Fatal("Get() error = nil, want not found after finalizer removal")
	}

	if len(fakeProvider.deleteCalls) != 1 {
		t.Fatalf("provider delete calls = %d, want 1", len(fakeProvider.deleteCalls))
	}

	if _, ok := fakeProvider.Get(provider.ServiceRef{Namespace: "default", Name: "demo"}); ok {
		t.Fatal("provider state still present after delete")
	}
}

func TestServiceReconcilerReconcileProviderEnsureErrorReturnsError(t *testing.T) {
	class := managedServiceClass
	service := newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class)
	fakeProvider := newFakeProvider()
	fakeProvider.ensureErr = errors.New("provider ensure failed")

	reconciler, countingClient, _ := newTestServiceReconciler(t, fakeProvider, service)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	}); err == nil {
		t.Fatal("Reconcile() error = nil, want non-nil")
	}

	stored := getStoredService(t, countingClient, types.NamespacedName{Namespace: "default", Name: "demo"})
	if !hasServiceFinalizer(stored) {
		t.Fatalf("service finalizers = %v, want %q", stored.Finalizers, serviceFinalizer)
	}
}

func TestServiceReconcilerReconcileProviderDeleteErrorKeepsFinalizer(t *testing.T) {
	class := managedServiceClass
	now := metav1.NewTime(time.Now())
	service := newDeletingReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class, &now)
	fakeProvider := newFakeProvider()
	fakeProvider.deleteErr = errors.New("provider delete failed")

	reconciler, countingClient, _ := newTestServiceReconciler(t, fakeProvider, service)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "demo"},
	}); err == nil {
		t.Fatal("Reconcile() error = nil, want non-nil")
	}

	stored := getStoredService(t, countingClient, types.NamespacedName{Namespace: "default", Name: "demo"})
	if !hasServiceFinalizer(stored) {
		t.Fatalf("service finalizers = %v, want finalizer to remain", stored.Finalizers)
	}

	if len(fakeProvider.deleteCalls) != 1 {
		t.Fatalf("provider delete calls = %d, want 1", len(fakeProvider.deleteCalls))
	}
}

func TestServiceReconcilerEndpointSliceToServiceRequests(t *testing.T) {
	reconciler := &ServiceReconciler{}

	requests := reconciler.endpointSliceToServiceRequests(context.Background(), &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-1",
			Namespace: "default",
			Labels: map[string]string{
				discoveryv1.LabelServiceName: "demo",
			},
		},
	})

	if len(requests) != 1 {
		t.Fatalf("endpointSliceToServiceRequests() len = %d, want 1", len(requests))
	}

	if requests[0].NamespacedName != (types.NamespacedName{Namespace: "default", Name: "demo"}) {
		t.Fatalf("endpointSliceToServiceRequests() = %+v, want default/demo", requests[0].NamespacedName)
	}

	requests = reconciler.endpointSliceToServiceRequests(context.Background(), &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-label",
			Namespace: "default",
		},
	})
	if len(requests) != 0 {
		t.Fatalf("endpointSliceToServiceRequests() len = %d, want 0", len(requests))
	}
}

func newTestServiceReconciler(
	t *testing.T,
	serviceProvider provider.Provider,
	objects ...runtime.Object,
) (*ServiceReconciler, *countingClient, *fakeProvider) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&corev1.Service{})
	if len(objects) > 0 {
		clientBuilder = clientBuilder.WithRuntimeObjects(objects...)
	}

	countingClient := newCountingClient(clientBuilder.Build())

	var fakeProviderImpl *fakeProvider
	if serviceProvider == nil {
		fakeProviderImpl = newFakeProvider()
		serviceProvider = fakeProviderImpl
	} else if typedProvider, ok := serviceProvider.(*fakeProvider); ok {
		fakeProviderImpl = typedProvider
	}

	reconciler := &ServiceReconciler{
		Client:   countingClient,
		Scheme:   scheme,
		Provider: serviceProvider,
		Config: config.Config{
			LoadBalancerClass: managedServiceClass,
			IPPool: []netip.Addr{
				netip.MustParseAddr("203.0.113.10"),
				netip.MustParseAddr("203.0.113.11"),
			},
			RequeueAfter: 30 * time.Second,
		},
	}

	return reconciler, countingClient, fakeProviderImpl
}

func getStoredService(t *testing.T, k8sClient client.Client, key types.NamespacedName) *corev1.Service {
	t.Helper()

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), key, service); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	return service
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
			Ports: []corev1.ServicePort{
				{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80},
			},
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
	service.Finalizers = []string{serviceFinalizer}
	return service
}

type endpointAddress struct {
	ip    string
	ready bool
}

func newEndpointSlice(
	name, namespace, serviceName, portName string,
	port int32,
	addresses []endpointAddress,
) *discoveryv1.EndpointSlice {
	endpoints := make([]discoveryv1.Endpoint, 0, len(addresses))
	for _, address := range addresses {
		ready := address.ready
		endpoints = append(endpoints, discoveryv1.Endpoint{
			Addresses: []string{address.ip},
			Conditions: discoveryv1.EndpointConditions{
				Ready: &ready,
			},
		})
	}

	portNameCopy := portName
	portCopy := port
	protocol := corev1.ProtocolTCP

	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				discoveryv1.LabelServiceName: serviceName,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports: []discoveryv1.EndpointPort{
			{
				Name:     &portNameCopy,
				Port:     &portCopy,
				Protocol: &protocol,
			},
		},
		Endpoints: endpoints,
	}
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

type fakeProvider struct {
	mu          sync.Mutex
	services    map[provider.ServiceRef]provider.Service
	ensureCalls []provider.Service
	deleteCalls []provider.ServiceRef
	ensureErr   error
	deleteErr   error
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		services: make(map[provider.ServiceRef]provider.Service),
	}
}

func (p *fakeProvider) Ensure(_ context.Context, service provider.Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureCalls = append(p.ensureCalls, service.DeepCopy())
	if p.ensureErr != nil {
		return p.ensureErr
	}

	p.services[service.Ref()] = service.DeepCopy()
	return nil
}

func (p *fakeProvider) Delete(_ context.Context, ref provider.ServiceRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.deleteCalls = append(p.deleteCalls, ref)
	if p.deleteErr != nil {
		return p.deleteErr
	}

	delete(p.services, ref)
	return nil
}

func (p *fakeProvider) Get(ref provider.ServiceRef) (provider.Service, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	service, ok := p.services[ref]
	if !ok {
		return provider.Service{}, false
	}

	return service.DeepCopy(), true
}

func (p *fakeProvider) Snapshot() map[provider.ServiceRef]provider.Service {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshot := make(map[provider.ServiceRef]provider.Service, len(p.services))
	for ref, service := range p.services {
		snapshot[ref] = service.DeepCopy()
	}

	return snapshot
}
