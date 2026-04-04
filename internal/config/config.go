package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
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
	// EnvRequeueAfter configures the requeue interval for matching Services.
	EnvRequeueAfter = "K8S_LB_CONTROLLER_REQUEUE_AFTER"
	// EnvLogLevel configures the controller log level.
	EnvLogLevel = "K8S_LB_CONTROLLER_LOG_LEVEL"
)

const (
	// DefaultMetricsAddr is the default bind address for the metrics server.
	DefaultMetricsAddr = ":8080"
	// DefaultHealthAddr is the default bind address for the health server.
	DefaultHealthAddr = ":8081"
	// DefaultLeaderElect is the default leader election setting.
	DefaultLeaderElect = false
	// DefaultLoadBalancerClass is the default Service load balancer class processed by the controller.
	DefaultLoadBalancerClass = "k8s-lb-controller"
	// DefaultRequeueAfter is the default reconciliation requeue interval for matching Services.
	DefaultRequeueAfter = 30 * time.Second
	// DefaultLogLevel is the default structured log level.
	DefaultLogLevel = LogLevelInfo
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
	MetricsAddr       string
	HealthAddr        string
	LeaderElect       bool
	LoadBalancerClass string
	RequeueAfter      time.Duration
	LogLevel          string
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
		MetricsAddr:       stringEnv(EnvMetricsAddr, DefaultMetricsAddr),
		HealthAddr:        stringEnv(EnvHealthAddr, DefaultHealthAddr),
		LoadBalancerClass: stringEnv(EnvLoadBalancerClass, DefaultLoadBalancerClass),
		LogLevel:          normalizeLogLevel(stringEnv(EnvLogLevel, DefaultLogLevel)),
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

	if cfg.LoadBalancerClass == "" {
		return Config{}, fmt.Errorf("%s must not be empty", EnvLoadBalancerClass)
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
