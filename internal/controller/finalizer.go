package controller

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const serviceFinalizer = "iedge.local/service-lb-finalizer"

func hasServiceFinalizer(service *corev1.Service) bool {
	if service == nil {
		return false
	}

	return controllerutil.ContainsFinalizer(service, serviceFinalizer)
}
