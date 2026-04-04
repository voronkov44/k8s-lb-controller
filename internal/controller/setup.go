package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
)

// SetupControllers registers all controllers managed by this binary.
func SetupControllers(mgr ctrl.Manager, cfg config.Config) error {
	return (&ServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: cfg,
	}).SetupWithManager(mgr)
}
