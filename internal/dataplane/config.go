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
	"strconv"
	"strings"
	"time"

	"github.com/voronkov44/k8s-lb-controller/internal/dataplane/ipattach"
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
	// EnvHAProxyPIDFile configures the pidfile shared with the HAProxy runtime process.
	EnvHAProxyPIDFile = "K8S_LB_DATAPLANE_HAPROXY_PID_FILE"
	// EnvLogLevel configures the dataplane log level.
	EnvLogLevel = "K8S_LB_DATAPLANE_LOG_LEVEL"
	// EnvGracefulShutdownTimeout configures how long the server waits for in-flight requests to finish on shutdown.
	EnvGracefulShutdownTimeout = "K8S_LB_DATAPLANE_GRACEFUL_SHUTDOWN_TIMEOUT"
	// EnvIPAttachEnabled enables command-based host-side external IP attachment.
	EnvIPAttachEnabled = "K8S_LB_DATAPLANE_IP_ATTACH_ENABLED"
	// EnvInterface configures which host interface receives external IPs.
	EnvInterface = "K8S_LB_DATAPLANE_INTERFACE"
	// EnvIPCommand configures the command path used for external IP management.
	EnvIPCommand = "K8S_LB_DATAPLANE_IP_COMMAND"
	// EnvIPCIDRSuffix configures the prefix length used when attaching external IPv4 addresses.
	EnvIPCIDRSuffix = "K8S_LB_DATAPLANE_IP_CIDR_SUFFIX"
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
	// DefaultHAProxyPIDFile leaves pidfile-based bootstrap reload detection disabled by default.
	DefaultHAProxyPIDFile = ""
	// DefaultLogLevel is the default structured log level.
	DefaultLogLevel = "info"
	// DefaultGracefulShutdownTimeout is the default maximum graceful shutdown time.
	DefaultGracefulShutdownTimeout = 15 * time.Second
	// DefaultIPAttachEnabled keeps host integration opt-in outside deployed dataplane mode.
	DefaultIPAttachEnabled = false
	// DefaultInterface requires deployments to choose the host interface explicitly when IP attachment is enabled.
	DefaultInterface = ""
	// DefaultIPCommand is the default executable used to manage interface addresses.
	DefaultIPCommand = ipattach.DefaultCommandPath
	// DefaultIPCIDRSuffix uses /32 host routes for attached IPv4 addresses.
	DefaultIPCIDRSuffix = ipattach.DefaultCIDRSuffix
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
	HAProxyPIDFile          string
	LogLevel                string
	GracefulShutdownTimeout time.Duration
	IPAttachEnabled         bool
	Interface               string
	IPCommand               string
	IPCIDRSuffix            int
}

// LoadConfig reads dataplane configuration from environment variables.
func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:               stringEnv(EnvHTTPAddr, DefaultHTTPAddr),
		HAProxyConfigPath:      stringEnv(EnvHAProxyConfigPath, DefaultHAProxyConfigPath),
		HAProxyValidateCommand: stringEnv(EnvHAProxyValidateCommand, DefaultHAProxyValidateCommand),
		HAProxyReloadCommand:   stringEnv(EnvHAProxyReloadCommand, DefaultHAProxyReloadCommand),
		HAProxyPIDFile:         stringEnv(EnvHAProxyPIDFile, DefaultHAProxyPIDFile),
		LogLevel:               normalizeLogLevel(stringEnv(EnvLogLevel, DefaultLogLevel)),
		Interface:              stringEnv(EnvInterface, DefaultInterface),
		IPCommand:              stringEnv(EnvIPCommand, DefaultIPCommand),
	}

	gracefulShutdownTimeout, err := durationEnv(EnvGracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	if gracefulShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than zero", EnvGracefulShutdownTimeout)
	}
	cfg.GracefulShutdownTimeout = gracefulShutdownTimeout

	ipAttachEnabled, err := boolEnv(EnvIPAttachEnabled, DefaultIPAttachEnabled)
	if err != nil {
		return Config{}, err
	}
	cfg.IPAttachEnabled = ipAttachEnabled

	ipCIDRSuffix, err := intEnv(EnvIPCIDRSuffix, DefaultIPCIDRSuffix)
	if err != nil {
		return Config{}, err
	}
	cfg.IPCIDRSuffix = ipCIDRSuffix

	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvHTTPAddr)
	}

	if cfg.HAProxyConfigPath == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvHAProxyConfigPath)
	}

	if _, ok := supportedLogLevels[cfg.LogLevel]; !ok {
		return Config{}, fmt.Errorf("%s must be one of: debug, info, warn, error", EnvLogLevel)
	}

	if cfg.IPCIDRSuffix < 1 || cfg.IPCIDRSuffix > 32 {
		return Config{}, fmt.Errorf("%s must be between 1 and 32", EnvIPCIDRSuffix)
	}

	if cfg.IPAttachEnabled {
		if cfg.Interface == "" {
			return Config{}, fmt.Errorf("%s must not be empty when %s=true", EnvInterface, EnvIPAttachEnabled)
		}
		if cfg.IPCommand == "" {
			return Config{}, fmt.Errorf("%s must not be empty when %s=true", EnvIPCommand, EnvIPAttachEnabled)
		}
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

func boolEnv(key string, defaultValue bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func intEnv(key string, defaultValue int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func normalizeLogLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
