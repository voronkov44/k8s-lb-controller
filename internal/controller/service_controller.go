package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
)

const serviceControllerName = "service"

// ServiceReconciler watches built-in Services that match the configured load balancer class.
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// Reconcile implements the Phase 1 baseline flow for Services selected by this controller.
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

	log.Info("service matched controller selection",
		"serviceType", service.Spec.Type,
		"loadBalancerClass", loadBalancerClass,
	)

	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
}

// SetupWithManager wires the controller into the manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(serviceControllerName).
		For(&corev1.Service{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
