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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/voronkov44/k8s-lb-controller/internal/config"
	"github.com/voronkov44/k8s-lb-controller/internal/provider"
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
