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

package main

import (
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
	"github.com/f1lzz/k8s-lb-controller/internal/controller"
	controllermetrics "github.com/f1lzz/k8s-lb-controller/internal/metrics"
	haproxyprovider "github.com/f1lzz/k8s-lb-controller/internal/provider/haproxy"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func main() {
	dotEnvLoaded, err := config.LoadDotEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to load %s: %v\n", config.DotEnvFileName, err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to load configuration: %v\n", err)
		os.Exit(1)
	}

	if err := configureLogger(cfg.LogLevel); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to configure logger: %v\n", err)
		os.Exit(1)
	}

	logDotEnvStatus(dotEnvLoaded)

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to load Kubernetes client configuration")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.HealthAddr,
		LeaderElection:         cfg.LeaderElect,
		LeaderElectionID:       "ed30ec16.diploma.local",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	serviceProvider, err := haproxyprovider.NewProvider(haproxyprovider.Config{
		ConfigPath:      cfg.HAProxyConfigPath,
		ValidateCommand: cfg.HAProxyValidateCommand,
		ReloadCommand:   cfg.HAProxyReloadCommand,
	})
	if err != nil {
		setupLog.Error(err, "unable to create HAProxy provider")
		os.Exit(1)
	}
	instrumentedProvider := controllermetrics.WrapProvider(serviceProvider)

	if err := controller.SetupControllers(mgr, cfg, instrumentedProvider); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager",
		"metricsAddr", cfg.MetricsAddr,
		"healthAddr", cfg.HealthAddr,
		"leaderElection", cfg.LeaderElect,
		"loadBalancerClass", cfg.LoadBalancerClass,
		"requeueAfter", cfg.RequeueAfter.String(),
		"logLevel", cfg.LogLevel,
		"haproxyConfigPath", cfg.HAProxyConfigPath,
		"haproxyValidateEnabled", cfg.HAProxyValidateCommand != "",
		"haproxyReloadEnabled", cfg.HAProxyReloadCommand != "",
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func logDotEnvStatus(dotEnvLoaded bool) {
	if dotEnvLoaded {
		setupLog.Info("loaded environment variables from dotenv file", "path", config.DotEnvFileName)
		return
	}

	setupLog.Info("dotenv file not found, using environment variables and defaults", "path", config.DotEnvFileName)
}

func configureLogger(levelName string) error {
	level, err := parseLogLevel(levelName)
	if err != nil {
		return err
	}

	opts := ctrlzap.Options{
		Development: level == zapcore.DebugLevel,
		Level:       level,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}

	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseFlagOptions(&opts)))
	return nil
}

func parseLogLevel(levelName string) (zapcore.Level, error) {
	switch levelName {
	case config.LogLevelDebug:
		return zapcore.DebugLevel, nil
	case config.LogLevelInfo:
		return zapcore.InfoLevel, nil
	case config.LogLevelWarn:
		return zapcore.WarnLevel, nil
	case config.LogLevelError:
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level %q", levelName)
	}
}
