package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

// SetupControllers registers all controllers managed by this binary.
func SetupControllers(mgr ctrl.Manager, cfg config.Config, serviceProvider provider.Provider) error {
	if serviceProvider == nil {
		return fmt.Errorf("service provider is nil")
	}

	return (&ServiceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   cfg,
		Provider: serviceProvider,
	}).SetupWithManager(mgr)
}
