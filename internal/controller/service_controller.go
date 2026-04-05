package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
	"github.com/f1lzz/k8s-lb-controller/internal/ipam"
	servicestatus "github.com/f1lzz/k8s-lb-controller/internal/status"
)

const serviceControllerName = "service"

// ServiceReconciler watches built-in Services that match the configured load balancer class.
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch

// Reconcile assigns an external IP from the configured pool to matching Services and updates status.
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get service %s: %w", req.String(), err)
	}

	loadBalancerClass := serviceLoadBalancerClass(service)
	if !isManagedLoadBalancerService(service, r.Config.LoadBalancerClass) {
		log.V(1).Info("ignoring service because it does not match controller selection",
			"serviceType", service.Spec.Type,
			"loadBalancerClass", loadBalancerClass,
		)
		return ctrl.Result{}, nil
	}

	if service.DeletionTimestamp != nil {
		log.V(1).Info("service is being deleted, skipping Phase 2 processing")
		return ctrl.Result{}, nil
	}

	log.Info("service matched controller selection",
		"serviceType", service.Spec.Type,
		"loadBalancerClass", loadBalancerClass,
	)

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

	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
}

// SetupWithManager wires the controller into the manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(serviceControllerName).
		For(&corev1.Service{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *ServiceReconciler) listServicesForAllocation(ctx context.Context) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList); err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}
