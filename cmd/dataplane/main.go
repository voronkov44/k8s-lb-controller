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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	controllerconfig "github.com/voronkov44/k8s-lb-controller/internal/config"
	"github.com/voronkov44/k8s-lb-controller/internal/dataplane"
	"github.com/voronkov44/k8s-lb-controller/internal/dataplane/ipattach"
)

func main() {
	dotEnvLoaded, err := controllerconfig.LoadDotEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to load %s: %v\n", controllerconfig.DotEnvFileName, err)
		os.Exit(1)
	}

	cfg, err := dataplane.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to load dataplane configuration: %v\n", err)
		os.Exit(1)
	}

	logger, err := configureLogger(cfg.LogLevel)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to configure logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("dataplane startup beginning")
	logDotEnvStatus(logger, dotEnvLoaded)

	ipManager, err := ipattach.NewManager(ipattach.Config{
		Enabled:     cfg.IPAttachEnabled,
		Mode:        cfg.IPAttachMode,
		Interface:   cfg.Interface,
		CommandPath: cfg.IPCommand,
		CIDRSuffix:  cfg.IPCIDRSuffix,
	}, ipattach.Dependencies{})
	if err != nil {
		logger.Error("unable to create IP attachment manager", "error", err.Error())
		os.Exit(1)
	}

	engine, err := dataplane.NewEngine(dataplane.EngineConfig{
		ConfigPath:      cfg.HAProxyConfigPath,
		ValidateCommand: cfg.HAProxyValidateCommand,
		ReloadCommand:   cfg.HAProxyReloadCommand,
		PIDFile:         cfg.HAProxyPIDFile,
		IPManager:       ipManager,
	})
	if err != nil {
		logger.Error("unable to create dataplane engine", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("dataplane configuration",
		"httpAddr", cfg.HTTPAddr,
		"gracefulShutdownTimeout", cfg.GracefulShutdownTimeout.String(),
		"logLevel", cfg.LogLevel,
		"haproxyConfigPath", cfg.HAProxyConfigPath,
		"haproxyPIDFile", cfg.HAProxyPIDFile,
		"haproxyValidateEnabled", cfg.HAProxyValidateCommand != "",
		"haproxyReloadEnabled", cfg.HAProxyReloadCommand != "",
		"ipAttachEnabled", cfg.IPAttachEnabled,
		"ipAttachMode", cfg.IPAttachMode,
		"interface", cfg.Interface,
		"ipCommand", cfg.IPCommand,
		"ipCIDRSuffix", cfg.IPCIDRSuffix,
		"maxRequestBodyBytes", dataplane.DefaultMaxRequestBodyBytes,
	)

	if err := engine.Bootstrap(context.Background()); err != nil {
		logger.Error("unable to bootstrap dataplane runtime state", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("dataplane runtime state bootstrapped")

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           dataplane.NewHandler(engine, logger, dataplane.HandlerConfig{}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("dataplane HTTP server listening", "httpAddr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}

		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		if err != nil {
			logger.Error("dataplane HTTP server stopped unexpectedly", "error", err.Error())
			os.Exit(1)
		}
	case <-serverCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("graceful shutdown completed")
}

func configureLogger(levelName string) (*slog.Logger, error) {
	level, err := parseLogLevel(levelName)
	if err != nil {
		return nil, err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger, nil
}

func parseLogLevel(levelName string) (slog.Level, error) {
	switch levelName {
	case dataplane.LogLevelDebug:
		return slog.LevelDebug, nil
	case dataplane.LogLevelInfo:
		return slog.LevelInfo, nil
	case dataplane.LogLevelWarn:
		return slog.LevelWarn, nil
	case dataplane.LogLevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q", levelName)
	}
}

func logDotEnvStatus(logger *slog.Logger, dotEnvLoaded bool) {
	if dotEnvLoaded {
		logger.Info("loaded environment variables from dotenv file", "path", controllerconfig.DotEnvFileName)
		return
	}

	logger.Info("dotenv file not found, using environment variables and defaults", "path", controllerconfig.DotEnvFileName)
}
