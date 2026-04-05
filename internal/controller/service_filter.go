package controller

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func isManagedLoadBalancerService(service *corev1.Service, expectedLoadBalancerClass string) bool {
	if service == nil {
		return false
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}

	if service.Spec.LoadBalancerClass == nil {
		return false
	}

	return *service.Spec.LoadBalancerClass == expectedLoadBalancerClass
}

func serviceLoadBalancerClass(service *corev1.Service) string {
	if service == nil || service.Spec.LoadBalancerClass == nil {
		return ""
	}

	return *service.Spec.LoadBalancerClass
}

func serviceReconcilePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldService, oldOK := e.ObjectOld.(*corev1.Service)
			newService, newOK := e.ObjectNew.(*corev1.Service)
			if !oldOK || !newOK {
				return true
			}

			if oldService.Generation != newService.Generation {
				return true
			}

			return deletionTimestampChanged(oldService, newService)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

func endpointSliceReconcilePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldEndpointSlice, oldOK := e.ObjectOld.(*discoveryv1.EndpointSlice)
			newEndpointSlice, newOK := e.ObjectNew.(*discoveryv1.EndpointSlice)
			if !oldOK || !newOK {
				return true
			}

			return oldEndpointSlice.ResourceVersion != newEndpointSlice.ResourceVersion
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

func deletionTimestampChanged(oldService, newService *corev1.Service) bool {
	switch {
	case oldService == nil && newService == nil:
		return false
	case oldService == nil:
		return newService.DeletionTimestamp != nil
	case newService == nil:
		return oldService.DeletionTimestamp != nil
	case oldService.DeletionTimestamp == nil && newService.DeletionTimestamp == nil:
		return false
	case oldService.DeletionTimestamp == nil || newService.DeletionTimestamp == nil:
		return true
	default:
		return !oldService.DeletionTimestamp.Equal(newService.DeletionTimestamp)
	}
}
