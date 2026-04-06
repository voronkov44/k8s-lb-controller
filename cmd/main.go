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
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"

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
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type kubeconfigDiagnosticKind string

const (
	kubeconfigDiagnosticMissingCurrentContext kubeconfigDiagnosticKind = "missingCurrentContext"
	kubeconfigDiagnosticNoKubeconfig          kubeconfigDiagnosticKind = "noKubeconfig"
)

type kubeconfigDiagnostic struct {
	kind              kubeconfigDiagnosticKind
	kubeconfigPath    string
	availableContexts []string
	checkedSources    []string
	suggestedCommand  string
}

type gracefulShutdownObserver struct {
	timeout time.Duration
	started atomic.Bool
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
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

	setupLog.Info("startup beginning")
	logDotEnvStatus(dotEnvLoaded)

	if !likelyInClusterEnvironment() {
		diagnostic, err := diagnoseLocalKubeconfig(os.Args[1:])
		if err != nil {
			setupLog.Error(err, "unable to inspect local kubeconfig")
			os.Exit(1)
		}
		if diagnostic != nil {
			logPreflightKubeconfigDiagnostic(diagnostic)
			os.Exit(1)
		}
	}

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to load Kubernetes client configuration")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: cfg.MetricsAddr, SecureServing: false},
		HealthProbeBindAddress:  cfg.HealthAddr,
		LeaderElection:          cfg.LeaderElect,
		LeaderElectionID:        "ed30ec16.diploma.local",
		GracefulShutdownTimeout: &cfg.GracefulShutdownTimeout,
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

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("manager configuration",
		"metricsAddr", cfg.MetricsAddr,
		"healthAddr", cfg.HealthAddr,
		"leaderElection", cfg.LeaderElect,
		"gracefulShutdownTimeout", cfg.GracefulShutdownTimeout.String(),
		"loadBalancerClass", cfg.LoadBalancerClass,
		"requeueAfter", cfg.RequeueAfter.String(),
		"logLevel", cfg.LogLevel,
		"haproxyConfigPath", cfg.HAProxyConfigPath,
		"haproxyValidateEnabled", cfg.HAProxyValidateCommand != "",
		"haproxyReloadEnabled", cfg.HAProxyReloadCommand != "",
	)

	shutdownCtx := ctrl.SetupSignalHandler()
	shutdownObserver := observeGracefulShutdown(shutdownCtx, cfg.GracefulShutdownTimeout)

	if err := mgr.Start(shutdownCtx); err != nil {
		if shutdownCtx.Err() != nil {
			shutdownObserver.logStarted()
			setupLog.Error(err, "graceful shutdown failed")
		} else {
			setupLog.Error(err, "problem running manager")
		}
		os.Exit(1)
	}

	if shutdownCtx.Err() != nil {
		shutdownObserver.logStarted()
		setupLog.Info("graceful shutdown completed")
	}
}

func observeGracefulShutdown(ctx context.Context, timeout time.Duration) *gracefulShutdownObserver {
	observer := &gracefulShutdownObserver{timeout: timeout}
	go func() {
		<-ctx.Done()
		observer.logStarted()
	}()

	return observer
}

func (o *gracefulShutdownObserver) logStarted() {
	if !o.started.CompareAndSwap(false, true) {
		return
	}

	setupLog.Info("shutdown signal received")
	setupLog.Info("graceful shutdown started", "timeout", o.timeout.String())
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

func logPreflightKubeconfigDiagnostic(diagnostic *kubeconfigDiagnostic) {
	switch diagnostic.kind {
	case kubeconfigDiagnosticMissingCurrentContext:
		fields := []any{
			"kubeconfigPath", diagnostic.kubeconfigPath,
			"availableContexts", diagnostic.availableContexts,
		}
		if diagnostic.suggestedCommand != "" {
			fields = append(fields, "suggestedCommand", diagnostic.suggestedCommand)
		}

		setupLog.Info("kubeconfig found, but current-context is not set", fields...)
	case kubeconfigDiagnosticNoKubeconfig:
		setupLog.Info("no kubeconfig found for local run", "checkedSources", diagnostic.checkedSources)
	}
}

func diagnoseLocalKubeconfig(args []string) (*kubeconfigDiagnostic, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath := kubeconfigFlagValue(args); explicitPath != "" {
		loadingRules.ExplicitPath = explicitPath
	}

	checkedSources := loadingRules.GetLoadingPrecedence()
	existingSources := existingKubeconfigPaths(checkedSources)
	if len(existingSources) == 0 {
		return &kubeconfigDiagnostic{
			kind:           kubeconfigDiagnosticNoKubeconfig,
			checkedSources: checkedSources,
		}, nil
	}

	rawConfig, err := loadingRules.Load()
	if err != nil {
		return nil, err
	}

	if rawConfig.CurrentContext != "" || len(rawConfig.Contexts) == 0 {
		return nil, nil
	}

	availableContexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		availableContexts = append(availableContexts, name)
	}
	sort.Strings(availableContexts)

	if len(availableContexts) == 0 {
		return nil, nil
	}

	return &kubeconfigDiagnostic{
		kind:              kubeconfigDiagnosticMissingCurrentContext,
		kubeconfigPath:    existingSources[0],
		availableContexts: availableContexts,
		checkedSources:    checkedSources,
		suggestedCommand:  fmt.Sprintf("kubectl config use-context %s", availableContexts[0]),
	}, nil
}

func kubeconfigFlagValue(args []string) string {
	for index := range len(args) {
		arg := strings.TrimSpace(args[index])
		switch {
		case strings.HasPrefix(arg, "--kubeconfig="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--kubeconfig="))
		case strings.HasPrefix(arg, "-kubeconfig="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "-kubeconfig="))
		case arg == "--kubeconfig" || arg == "-kubeconfig":
			if index+1 >= len(args) {
				return ""
			}

			return strings.TrimSpace(args[index+1])
		}
	}

	return ""
}

func existingKubeconfigPaths(paths []string) []string {
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		existing = append(existing, path)
	}

	return existing
}

func likelyInClusterEnvironment() bool {
	_, hasServiceHost := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	_, hasServicePort := os.LookupEnv("KUBERNETES_SERVICE_PORT")
	return hasServiceHost && hasServicePort
}
