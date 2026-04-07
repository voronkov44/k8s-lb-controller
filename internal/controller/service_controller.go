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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/voronkov44/k8s-lb-controller/internal/backends"
	"github.com/voronkov44/k8s-lb-controller/internal/config"
	"github.com/voronkov44/k8s-lb-controller/internal/ipam"
	controllermetrics "github.com/voronkov44/k8s-lb-controller/internal/metrics"
	"github.com/voronkov44/k8s-lb-controller/internal/provider"
	servicestatus "github.com/voronkov44/k8s-lb-controller/internal/status"
)

const serviceControllerName = "service"

// ServiceReconciler watches built-in Services that match the configured load balancer class.
type ServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   config.Config
	Provider provider.Provider
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch

// Reconcile assigns an external IP from the configured pool, syncs provider state, and handles deletion.
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	controllermetrics.IncServiceReconcile()
	startedAt := time.Now()
	defer func() {
		controllermetrics.ObserveServiceReconcile(time.Since(startedAt), retErr)
	}()

	log := ctrl.LoggerFrom(ctx).WithValues("service", req.String())

	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get service %s: %w", req.String(), err)
	}

	if service.DeletionTimestamp != nil {
		if err := r.reconcileDeletingService(ctx, log, service); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	loadBalancerClass := serviceLoadBalancerClass(service)
	if !isManagedLoadBalancerService(service, r.Config.LoadBalancerClass) {
		if err := r.reconcileUnmanagedService(ctx, log, service); err != nil {
			return ctrl.Result{}, err
		}

		log.V(1).Info("ignoring service because it does not match controller selection",
			"serviceType", service.Spec.Type,
			"loadBalancerClass", loadBalancerClass,
		)
		return ctrl.Result{}, nil
	}

	log.Info("service matched controller selection",
		"serviceType", service.Spec.Type,
		"loadBalancerClass", loadBalancerClass,
	)

	finalizerAdded, err := r.ensureServiceFinalizer(ctx, service)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure finalizer for service %s: %w", req.String(), err)
	}
	if finalizerAdded {
		log.Info("added service finalizer", "finalizer", serviceFinalizer)

		if err := r.Get(ctx, req.NamespacedName, service); err != nil {
			return ctrl.Result{}, fmt.Errorf("refresh service %s after finalizer update: %w", req.String(), err)
		}
	}

	services, err := r.listServicesForAllocation(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("list services for allocation: %w", err)
	}

	allocator := ipam.NewAllocator(r.Config.IPPool, func(candidate *corev1.Service) bool {
		return isManagedLoadBalancerService(candidate, r.Config.LoadBalancerClass)
	})

	allocation, err := allocator.Allocate(service, services)
	if err != nil {
		if errors.Is(err, ipam.ErrNoFreeIP) {
			controllermetrics.RecordIPAllocationExhausted()
			log.Error(err, "no free IPs in pool", "poolSize", len(r.Config.IPPool))
			return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
		}

		controllermetrics.RecordIPAllocationError()
		return ctrl.Result{}, fmt.Errorf("allocate external IP for service %s: %w", req.String(), err)
	}

	if allocation.Reused {
		controllermetrics.RecordIPAllocationReused()
		log.Info("reused existing external IP", "externalIP", allocation.IP.String())
	} else {
		controllermetrics.RecordIPAllocationAllocated()
		log.Info("assigned external IP", "externalIP", allocation.IP.String())
	}

	endpointSlices, err := r.listEndpointSlicesForService(ctx, service)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("list EndpointSlices for service %s: %w", req.String(), err)
	}

	discoveredBackends := backends.Discover(service, endpointSlices)
	providerService := buildProviderService(service, allocation.IP.String(), discoveredBackends)
	providerChanged, err := r.Provider.Ensure(ctx, providerService)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure provider state for service %s: %w", req.String(), err)
	}

	if providerChanged {
		log.Info("ensured provider state",
			"externalIP", allocation.IP.String(),
			"servicePorts", len(providerService.Ports),
			"backendCount", providerBackendCount(providerService),
		)
	} else {
		log.V(1).Info("provider state already up to date",
			"externalIP", allocation.IP.String(),
			"servicePorts", len(providerService.Ports),
			"backendCount", providerBackendCount(providerService),
		)
	}

	desiredIngress := servicestatus.DesiredLoadBalancerIngress(allocation.IP.String())
	updated, err := servicestatus.UpdateServiceLoadBalancerStatus(ctx, r.Client, service, desiredIngress)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("update status for service %s: %w", req.String(), err)
	}

	if updated {
		log.Info("updated service status", "externalIP", allocation.IP.String())
	} else if !finalizerAdded && !providerChanged {
		log.V(1).Info("service reconcile resulted in no changes", "externalIP", allocation.IP.String())
	}

	return ctrl.Result{}, nil
}

// SetupWithManager wires the controller into the manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(serviceControllerName).
		For(&corev1.Service{}, builder.WithPredicates(serviceReconcilePredicate())).
		Watches(
			&discoveryv1.EndpointSlice{},
			handler.EnqueueRequestsFromMapFunc(r.endpointSliceToServiceRequests),
			builder.WithPredicates(endpointSliceReconcilePredicate()),
		).
		Complete(r)
}

func (r *ServiceReconciler) listServicesForAllocation(ctx context.Context) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList); err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}

func (r *ServiceReconciler) listEndpointSlicesForService(
	ctx context.Context,
	service *corev1.Service,
) ([]discoveryv1.EndpointSlice, error) {
	if service == nil {
		return nil, nil
	}

	endpointSliceList := &discoveryv1.EndpointSliceList{}
	if err := r.List(
		ctx,
		endpointSliceList,
		client.InNamespace(service.Namespace),
		client.MatchingLabels{discoveryv1.LabelServiceName: service.Name},
	); err != nil {
		return nil, err
	}

	return endpointSliceList.Items, nil
}

func (r *ServiceReconciler) reconcileDeletingService(
	ctx context.Context,
	log logr.Logger,
	service *corev1.Service,
) error {
	log.Info("service is being deleted")

	if !hasServiceFinalizer(service) {
		return nil
	}

	serviceRef := provider.ServiceRef{
		Namespace: service.Namespace,
		Name:      service.Name,
	}

	log.Info("cleaning up provider state")
	providerChanged, err := r.Provider.Delete(ctx, serviceRef)
	if err != nil {
		return fmt.Errorf("delete provider state for service %s: %w", serviceRef.String(), err)
	}
	if providerChanged {
		log.Info("cleaned up provider state")
	} else {
		log.V(1).Info("provider state already absent during deletion")
	}

	if err := r.removeServiceFinalizer(ctx, service); err != nil {
		return fmt.Errorf("remove finalizer for service %s: %w", serviceRef.String(), err)
	}

	log.Info("removed service finalizer", "finalizer", serviceFinalizer)

	return nil
}

func (r *ServiceReconciler) reconcileUnmanagedService(
	ctx context.Context,
	log logr.Logger,
	service *corev1.Service,
) error {
	if service == nil {
		return nil
	}

	ownedStatus := servicestatus.HasLoadBalancerIngressIPInPool(service, r.Config.IPPool)
	if !hasServiceFinalizer(service) && !ownedStatus {
		return nil
	}

	serviceRef := provider.ServiceRef{
		Namespace: service.Namespace,
		Name:      service.Name,
	}

	log.Info("service no longer matches controller selection, cleaning up managed state")

	providerChanged, err := r.Provider.Delete(ctx, serviceRef)
	if err != nil {
		return fmt.Errorf("delete provider state for unmanaged service %s: %w", serviceRef.String(), err)
	}

	if providerChanged {
		log.Info("cleaned up provider state for unmanaged service")
	} else {
		log.V(1).Info("provider state already absent for unmanaged service")
	}

	statusWasCleared := false
	if hasServiceFinalizer(service) || ownedStatus {
		statusWasCleared, err = servicestatus.UpdateServiceLoadBalancerStatus(ctx, r.Client, service, nil)
		if err != nil {
			return fmt.Errorf("clear load balancer status for unmanaged service %s: %w", serviceRef.String(), err)
		}

		if statusWasCleared {
			log.Info("cleared service load balancer status")
		}
	}

	if !hasServiceFinalizer(service) {
		return nil
	}

	if statusWasCleared {
		if err := r.Get(ctx, client.ObjectKeyFromObject(service), service); err != nil {
			return fmt.Errorf("refresh unmanaged service %s after status update: %w", serviceRef.String(), err)
		}
	}

	if err := r.removeServiceFinalizer(ctx, service); err != nil {
		return fmt.Errorf("remove finalizer for unmanaged service %s: %w", serviceRef.String(), err)
	}

	log.Info("removed service finalizer", "finalizer", serviceFinalizer)

	return nil
}

func (r *ServiceReconciler) ensureServiceFinalizer(ctx context.Context, service *corev1.Service) (bool, error) {
	if hasServiceFinalizer(service) {
		return false, nil
	}

	original := service.DeepCopy()
	controllerutil.AddFinalizer(service, serviceFinalizer)

	if err := r.Patch(ctx, service, client.MergeFrom(original)); err != nil {
		return false, err
	}

	return true, nil
}

func (r *ServiceReconciler) removeServiceFinalizer(ctx context.Context, service *corev1.Service) error {
	if !hasServiceFinalizer(service) {
		return nil
	}

	original := service.DeepCopy()
	controllerutil.RemoveFinalizer(service, serviceFinalizer)

	return r.Patch(ctx, service, client.MergeFrom(original))
}

func (r *ServiceReconciler) endpointSliceToServiceRequests(
	_ context.Context,
	obj client.Object,
) []reconcile.Request {
	endpointSlice, ok := obj.(*discoveryv1.EndpointSlice)
	if !ok || endpointSlice == nil {
		return nil
	}

	serviceName := endpointSlice.Labels[discoveryv1.LabelServiceName]
	if serviceName == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: endpointSlice.Namespace,
			Name:      serviceName,
		},
	}}
}

func buildProviderService(
	service *corev1.Service,
	externalIP string,
	discoveredBackends []backends.ServicePortBackends,
) provider.Service {
	ports := make([]provider.ServicePort, 0, len(discoveredBackends))
	for _, port := range discoveredBackends {
		ports = append(ports, provider.ServicePort{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			Backends:   append([]provider.BackendEndpoint(nil), port.Backends...),
		})
	}

	return provider.Service{
		Namespace:         service.Namespace,
		Name:              service.Name,
		LoadBalancerClass: serviceLoadBalancerClass(service),
		ExternalIP:        externalIP,
		Ports:             ports,
	}
}

func providerBackendCount(service provider.Service) int {
	total := 0
	for _, port := range service.Ports {
		total += len(port.Backends)
	}

	return total
}
