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

package dataplane

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// EnvHTTPAddr configures the dataplane HTTP listen address.
	EnvHTTPAddr = "K8S_LB_DATAPLANE_HTTP_ADDR"
	// EnvHAProxyConfigPath configures where the rendered HAProxy config is written.
	EnvHAProxyConfigPath = "K8S_LB_DATAPLANE_HAPROXY_CONFIG_PATH"
	// EnvHAProxyValidateCommand configures an optional command used to validate a candidate HAProxy config.
	EnvHAProxyValidateCommand = "K8S_LB_DATAPLANE_HAPROXY_VALIDATE_COMMAND"
	// EnvHAProxyReloadCommand configures an optional command used to reload HAProxy after updating the config.
	EnvHAProxyReloadCommand = "K8S_LB_DATAPLANE_HAPROXY_RELOAD_COMMAND"
	// EnvLogLevel configures the dataplane log level.
	EnvLogLevel = "K8S_LB_DATAPLANE_LOG_LEVEL"
	// EnvGracefulShutdownTimeout configures how long the server waits for in-flight requests to finish on shutdown.
	EnvGracefulShutdownTimeout = "K8S_LB_DATAPLANE_GRACEFUL_SHUTDOWN_TIMEOUT"
)

const (
	// DefaultHTTPAddr is the default listen address for the dataplane API server.
	DefaultHTTPAddr = ":8090"
	// DefaultHAProxyConfigPath is the default path for the rendered HAProxy config file.
	DefaultHAProxyConfigPath = "/tmp/k8s-lb-controller-haproxy.cfg"
	// DefaultHAProxyValidateCommand disables config validation by default.
	DefaultHAProxyValidateCommand = ""
	// DefaultHAProxyReloadCommand disables reload execution by default.
	DefaultHAProxyReloadCommand = ""
	// DefaultLogLevel is the default structured log level.
	DefaultLogLevel = "info"
	// DefaultGracefulShutdownTimeout is the default maximum graceful shutdown time.
	DefaultGracefulShutdownTimeout = 15 * time.Second
)

const (
	// LogLevelDebug enables verbose logging.
	LogLevelDebug = "debug"
	// LogLevelInfo enables informational logging.
	LogLevelInfo = "info"
	// LogLevelWarn enables warning logging.
	LogLevelWarn = "warn"
	// LogLevelError enables error-only logging.
	LogLevelError = "error"
)

var supportedLogLevels = map[string]struct{}{
	LogLevelDebug: {},
	LogLevelInfo:  {},
	LogLevelWarn:  {},
	LogLevelError: {},
}

// Config contains dataplane runtime configuration loaded from environment variables.
type Config struct {
	HTTPAddr                string
	HAProxyConfigPath       string
	HAProxyValidateCommand  string
	HAProxyReloadCommand    string
	LogLevel                string
	GracefulShutdownTimeout time.Duration
}

// LoadConfig reads dataplane configuration from environment variables.
func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:               stringEnv(EnvHTTPAddr, DefaultHTTPAddr),
		HAProxyConfigPath:      stringEnv(EnvHAProxyConfigPath, DefaultHAProxyConfigPath),
		HAProxyValidateCommand: stringEnv(EnvHAProxyValidateCommand, DefaultHAProxyValidateCommand),
		HAProxyReloadCommand:   stringEnv(EnvHAProxyReloadCommand, DefaultHAProxyReloadCommand),
		LogLevel:               normalizeLogLevel(stringEnv(EnvLogLevel, DefaultLogLevel)),
	}

	gracefulShutdownTimeout, err := durationEnv(EnvGracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	if gracefulShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than zero", EnvGracefulShutdownTimeout)
	}
	cfg.GracefulShutdownTimeout = gracefulShutdownTimeout

	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvHTTPAddr)
	}

	if cfg.HAProxyConfigPath == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvHAProxyConfigPath)
	}

	if _, ok := supportedLogLevels[cfg.LogLevel]; !ok {
		return Config{}, fmt.Errorf("%s must be one of: debug, info, warn, error", EnvLogLevel)
	}

	return cfg, nil
}

func stringEnv(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultValue
	}

	return trimmed
}

func durationEnv(key string, defaultValue time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func normalizeLogLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
