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

package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/voronkov44/k8s-lb-controller/internal/ipam"
)

const (
	// DotEnvFileName is the default dotenv file loaded by the application.
	DotEnvFileName = ".env"
	// EnvMetricsAddr configures the metrics server bind address.
	EnvMetricsAddr = "K8S_LB_CONTROLLER_METRICS_ADDR"
	// EnvHealthAddr configures the health and readiness probe bind address.
	EnvHealthAddr = "K8S_LB_CONTROLLER_HEALTH_ADDR"
	// EnvLeaderElect enables controller manager leader election.
	EnvLeaderElect = "K8S_LB_CONTROLLER_LEADER_ELECT"
	// EnvLoadBalancerClass configures the Service load balancer class handled by the controller.
	EnvLoadBalancerClass = "K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS"
	// EnvIPPool configures the external IPv4 address pool used for matching Services.
	EnvIPPool = "K8S_LB_CONTROLLER_IP_POOL"
	// EnvRequeueAfter configures how long to wait before retrying a managed Service that could not obtain an IP.
	EnvRequeueAfter = "K8S_LB_CONTROLLER_REQUEUE_AFTER"
	// EnvGracefulShutdownTimeout configures how long the manager waits for runnables to stop after shutdown begins.
	EnvGracefulShutdownTimeout = "K8S_LB_CONTROLLER_GRACEFUL_SHUTDOWN_TIMEOUT"
	// EnvLogLevel configures the controller log level.
	EnvLogLevel = "K8S_LB_CONTROLLER_LOG_LEVEL"
	// EnvProviderMode configures which provider implementation is used at runtime.
	EnvProviderMode = "K8S_LB_CONTROLLER_PROVIDER_MODE"
	// EnvHAProxyConfigPath configures where the rendered HAProxy config is written.
	EnvHAProxyConfigPath = "K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH"
	// EnvHAProxyValidateCommand configures an optional command used to validate a candidate HAProxy config.
	EnvHAProxyValidateCommand = "K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND"
	// EnvHAProxyReloadCommand configures an optional command used to reload HAProxy after updating the config.
	EnvHAProxyReloadCommand = "K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND"
	// EnvDataplaneAPIURL configures the base URL for the remote dataplane API provider.
	EnvDataplaneAPIURL = "K8S_LB_CONTROLLER_DATAPLANE_API_URL"
	// EnvDataplaneAPITimeout configures the HTTP timeout for the remote dataplane API provider.
	EnvDataplaneAPITimeout = "K8S_LB_CONTROLLER_DATAPLANE_API_TIMEOUT"
)

const (
	// DefaultMetricsAddr is the default bind address for the metrics server.
	DefaultMetricsAddr = ":8080"
	// DefaultHealthAddr is the default bind address for the health server.
	DefaultHealthAddr = ":8081"
	// DefaultLeaderElect is the default leader election setting.
	DefaultLeaderElect = false
	// DefaultLoadBalancerClass is the default Service load balancer class processed by the controller.
	DefaultLoadBalancerClass = "iedge.local/service-lb"
	// DefaultIPPool is the default external IPv4 address pool used for managed Services.
	DefaultIPPool = "203.0.113.10,203.0.113.11,203.0.113.12"
	// DefaultRequeueAfter is the default retry interval for managed Services waiting for a free IP.
	DefaultRequeueAfter = 30 * time.Second
	// DefaultGracefulShutdownTimeout is the default maximum graceful shutdown time for the controller manager.
	DefaultGracefulShutdownTimeout = 15 * time.Second
	// DefaultLogLevel is the default structured log level.
	DefaultLogLevel = LogLevelInfo
	// DefaultProviderMode keeps the current local HAProxy provider as the default runtime mode.
	DefaultProviderMode = ProviderModeLocalHAProxy
	// DefaultHAProxyConfigPath is the default path for the rendered HAProxy config file.
	DefaultHAProxyConfigPath = "/tmp/k8s-lb-controller-haproxy.cfg"
	// DefaultHAProxyValidateCommand disables config validation by default.
	DefaultHAProxyValidateCommand = ""
	// DefaultHAProxyReloadCommand disables reload execution by default.
	DefaultHAProxyReloadCommand = ""
	// DefaultDataplaneAPITimeout is the default HTTP timeout for the remote dataplane API provider.
	DefaultDataplaneAPITimeout = 10 * time.Second
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

// Config contains runtime configuration loaded from environment variables.
type Config struct {
	MetricsAddr             string
	HealthAddr              string
	LeaderElect             bool
	LoadBalancerClass       string
	IPPool                  []netip.Addr
	RequeueAfter            time.Duration
	GracefulShutdownTimeout time.Duration
	LogLevel                string
	ProviderMode            ProviderMode
	HAProxyConfigPath       string
	HAProxyValidateCommand  string
	HAProxyReloadCommand    string
	DataplaneAPIURL         string
	DataplaneAPITimeout     time.Duration
}

// ProviderMode selects which runtime provider implementation is active.
type ProviderMode string

const (
	// ProviderModeLocalHAProxy uses the existing local file-based HAProxy provider.
	ProviderModeLocalHAProxy ProviderMode = "local-haproxy"
	// ProviderModeDataplaneAPI uses a remote dataplane API provider.
	ProviderModeDataplaneAPI ProviderMode = "dataplane-api"
)

var supportedProviderModes = map[ProviderMode]struct{}{
	ProviderModeLocalHAProxy: {},
	ProviderModeDataplaneAPI: {},
}

// LoadDotEnv loads variables from .env without overriding the existing environment.
func LoadDotEnv() (bool, error) {
	values, err := godotenv.Read(DotEnvFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("read %s: %w", DotEnvFileName, err)
	}

	for key, value := range values {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return false, fmt.Errorf("set %s from %s: %w", key, DotEnvFileName, err)
		}
	}

	return true, nil
}

// Load reads controller configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		MetricsAddr:            stringEnv(EnvMetricsAddr, DefaultMetricsAddr),
		HealthAddr:             stringEnv(EnvHealthAddr, DefaultHealthAddr),
		LoadBalancerClass:      stringEnv(EnvLoadBalancerClass, DefaultLoadBalancerClass),
		LogLevel:               normalizeLogLevel(stringEnv(EnvLogLevel, DefaultLogLevel)),
		ProviderMode:           normalizeProviderMode(stringEnv(EnvProviderMode, string(DefaultProviderMode))),
		HAProxyConfigPath:      stringEnv(EnvHAProxyConfigPath, DefaultHAProxyConfigPath),
		HAProxyValidateCommand: stringEnv(EnvHAProxyValidateCommand, DefaultHAProxyValidateCommand),
		HAProxyReloadCommand:   stringEnv(EnvHAProxyReloadCommand, DefaultHAProxyReloadCommand),
	}

	leaderElect, err := boolEnv(EnvLeaderElect, DefaultLeaderElect)
	if err != nil {
		return Config{}, err
	}
	cfg.LeaderElect = leaderElect

	requeueAfter, err := durationEnv(EnvRequeueAfter, DefaultRequeueAfter)
	if err != nil {
		return Config{}, err
	}
	if requeueAfter <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than zero", EnvRequeueAfter)
	}
	cfg.RequeueAfter = requeueAfter

	gracefulShutdownTimeout, err := durationEnv(EnvGracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	if gracefulShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than zero", EnvGracefulShutdownTimeout)
	}
	cfg.GracefulShutdownTimeout = gracefulShutdownTimeout

	dataplaneAPITimeout, err := durationEnv(EnvDataplaneAPITimeout, DefaultDataplaneAPITimeout)
	if err != nil {
		return Config{}, err
	}
	if dataplaneAPITimeout <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than zero", EnvDataplaneAPITimeout)
	}
	cfg.DataplaneAPITimeout = dataplaneAPITimeout

	if cfg.LoadBalancerClass == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvLoadBalancerClass)
	}

	if _, ok := supportedProviderModes[cfg.ProviderMode]; !ok {
		return Config{}, fmt.Errorf(
			"%s must be one of: %s, %s",
			EnvProviderMode,
			ProviderModeLocalHAProxy,
			ProviderModeDataplaneAPI,
		)
	}

	if cfg.ProviderMode == ProviderModeLocalHAProxy && cfg.HAProxyConfigPath == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvHAProxyConfigPath)
	}

	ipPool, err := ipam.ParsePool(stringEnv(EnvIPPool, DefaultIPPool))
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", EnvIPPool, err)
	}
	cfg.IPPool = ipPool

	if _, ok := supportedLogLevels[cfg.LogLevel]; !ok {
		return Config{}, fmt.Errorf("%s must be one of: debug, info, warn, error", EnvLogLevel)
	}

	if cfg.ProviderMode == ProviderModeDataplaneAPI {
		cfg.DataplaneAPIURL, err = dataplaneAPIURLFromEnv()
		if err != nil {
			return Config{}, err
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

func normalizeProviderMode(mode string) ProviderMode {
	return ProviderMode(strings.ToLower(strings.TrimSpace(mode)))
}

func dataplaneAPIURLFromEnv() (string, error) {
	value, ok := os.LookupEnv(EnvDataplaneAPIURL)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf(
			"%s must not be empty when %s=%s",
			EnvDataplaneAPIURL,
			EnvProviderMode,
			ProviderModeDataplaneAPI,
		)
	}

	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", EnvDataplaneAPIURL, err)
	}

	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute URL", EnvDataplaneAPIURL)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", EnvDataplaneAPIURL)
	}

	return parsed.String(), nil
}
