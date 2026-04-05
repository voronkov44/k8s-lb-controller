package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
	"github.com/f1lzz/k8s-lb-controller/internal/ipam"
	"github.com/f1lzz/k8s-lb-controller/internal/provider"
	servicestatus "github.com/f1lzz/k8s-lb-controller/internal/status"
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

// Reconcile assigns an external IP from the configured pool, syncs provider state, and handles deletion.
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("service", req.NamespacedName.String())

	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get service %s: %w", req.String(), err)
	}

	if service.DeletionTimestamp != nil {
		return r.reconcileDeletingService(ctx, log, service)
	}

	loadBalancerClass := serviceLoadBalancerClass(service)
	if !isManagedLoadBalancerService(service, r.Config.LoadBalancerClass) {
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
			log.Error(err, "no free IPs in pool", "poolSize", len(r.Config.IPPool))
			return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
		}

		return ctrl.Result{}, fmt.Errorf("allocate external IP for service %s: %w", req.String(), err)
	}

	if allocation.Reused {
		log.Info("reused existing external IP", "externalIP", allocation.IP.String())
	} else {
		log.Info("assigned external IP", "externalIP", allocation.IP.String())
	}

	desiredIngress := servicestatus.DesiredLoadBalancerIngress(allocation.IP.String())
	updated, err := servicestatus.UpdateServiceLoadBalancerStatus(ctx, r.Client, service, desiredIngress)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("update status for service %s: %w", req.String(), err)
	}

	if updated {
		log.Info("updated service status", "externalIP", allocation.IP.String())
	}

	providerService := buildProviderService(service, allocation.IP.String())
	if err := r.Provider.Ensure(ctx, providerService); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure provider state for service %s: %w", req.String(), err)
	}

	log.V(1).Info("ensured mock provider state", "externalIP", allocation.IP.String())

	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
}

// SetupWithManager wires the controller into the manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(serviceControllerName).
		For(&corev1.Service{}, builder.WithPredicates(serviceReconcilePredicate())).
		Complete(r)
}

func (r *ServiceReconciler) listServicesForAllocation(ctx context.Context) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList); err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}

func (r *ServiceReconciler) reconcileDeletingService(
	ctx context.Context,
	log logr.Logger,
	service *corev1.Service,
) (ctrl.Result, error) {
	log.Info("service is being deleted")

	if !hasServiceFinalizer(service) {
		return ctrl.Result{}, nil
	}

	serviceRef := provider.ServiceRef{
		Namespace: service.Namespace,
		Name:      service.Name,
	}

	log.Info("cleaning up provider state")
	if err := r.Provider.Delete(ctx, serviceRef); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete provider state for service %s: %w", serviceRef.String(), err)
	}
	log.Info("cleaned up provider state")

	if err := r.removeServiceFinalizer(ctx, service); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer for service %s: %w", serviceRef.String(), err)
	}

	log.Info("removed service finalizer", "finalizer", serviceFinalizer)

	return ctrl.Result{}, nil
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

func buildProviderService(service *corev1.Service, externalIP string) provider.Service {
	ports := make([]provider.ServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, provider.ServicePort{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
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
