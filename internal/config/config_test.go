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
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"
)

func TestLoadDefaults(t *testing.T) {
	setConfigEnvToEmpty(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MetricsAddr != DefaultMetricsAddr {
		t.Fatalf("MetricsAddr = %q, want %q", cfg.MetricsAddr, DefaultMetricsAddr)
	}

	if cfg.HealthAddr != DefaultHealthAddr {
		t.Fatalf("HealthAddr = %q, want %q", cfg.HealthAddr, DefaultHealthAddr)
	}

	if cfg.LeaderElect != DefaultLeaderElect {
		t.Fatalf("LeaderElect = %t, want %t", cfg.LeaderElect, DefaultLeaderElect)
	}

	if cfg.LoadBalancerClass != DefaultLoadBalancerClass {
		t.Fatalf("LoadBalancerClass = %q, want %q", cfg.LoadBalancerClass, DefaultLoadBalancerClass)
	}

	wantPool := []netip.Addr{
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
		netip.MustParseAddr("203.0.113.12"),
	}
	if !slices.Equal(cfg.IPPool, wantPool) {
		t.Fatalf("IPPool = %v, want %v", cfg.IPPool, wantPool)
	}

	if cfg.RequeueAfter != DefaultRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", cfg.RequeueAfter, DefaultRequeueAfter)
	}

	if cfg.GracefulShutdownTimeout != DefaultGracefulShutdownTimeout {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}

	if cfg.ProviderMode != DefaultProviderMode {
		t.Fatalf("ProviderMode = %q, want %q", cfg.ProviderMode, DefaultProviderMode)
	}

	if cfg.HAProxyConfigPath != DefaultHAProxyConfigPath {
		t.Fatalf("HAProxyConfigPath = %q, want %q", cfg.HAProxyConfigPath, DefaultHAProxyConfigPath)
	}

	if cfg.HAProxyValidateCommand != DefaultHAProxyValidateCommand {
		t.Fatalf("HAProxyValidateCommand = %q, want %q", cfg.HAProxyValidateCommand, DefaultHAProxyValidateCommand)
	}

	if cfg.HAProxyReloadCommand != DefaultHAProxyReloadCommand {
		t.Fatalf("HAProxyReloadCommand = %q, want %q", cfg.HAProxyReloadCommand, DefaultHAProxyReloadCommand)
	}

	if cfg.DataplaneAPIURL != "" {
		t.Fatalf("DataplaneAPIURL = %q, want empty string", cfg.DataplaneAPIURL)
	}

	if cfg.DataplaneAPITimeout != DefaultDataplaneAPITimeout {
		t.Fatalf("DataplaneAPITimeout = %s, want %s", cfg.DataplaneAPITimeout, DefaultDataplaneAPITimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	setConfigEnvToEmpty(t)

	t.Setenv(EnvMetricsAddr, ":18080")
	t.Setenv(EnvHealthAddr, ":18081")
	t.Setenv(EnvLeaderElect, "true")
	t.Setenv(EnvLoadBalancerClass, "example.local/lb")
	t.Setenv(EnvIPPool, "203.0.113.20, 203.0.113.21 , ,203.0.113.22")
	t.Setenv(EnvRequeueAfter, "45s")
	t.Setenv(EnvGracefulShutdownTimeout, "20s")
	t.Setenv(EnvLogLevel, "DEBUG")
	t.Setenv(EnvProviderMode, string(ProviderModeLocalHAProxy))
	t.Setenv(EnvHAProxyConfigPath, "/var/run/haproxy/controller.cfg")
	t.Setenv(EnvHAProxyValidateCommand, "haproxy -c -f {{config}}")
	t.Setenv(EnvHAProxyReloadCommand, "service haproxy reload")
	t.Setenv(EnvDataplaneAPITimeout, "12s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MetricsAddr != ":18080" {
		t.Fatalf("MetricsAddr = %q, want %q", cfg.MetricsAddr, ":18080")
	}

	if cfg.HealthAddr != ":18081" {
		t.Fatalf("HealthAddr = %q, want %q", cfg.HealthAddr, ":18081")
	}

	if !cfg.LeaderElect {
		t.Fatal("LeaderElect = false, want true")
	}

	if cfg.LoadBalancerClass != "example.local/lb" {
		t.Fatalf("LoadBalancerClass = %q, want %q", cfg.LoadBalancerClass, "example.local/lb")
	}

	wantPool := []netip.Addr{
		netip.MustParseAddr("203.0.113.20"),
		netip.MustParseAddr("203.0.113.21"),
		netip.MustParseAddr("203.0.113.22"),
	}
	if !slices.Equal(cfg.IPPool, wantPool) {
		t.Fatalf("IPPool = %v, want %v", cfg.IPPool, wantPool)
	}

	if cfg.RequeueAfter != 45*time.Second {
		t.Fatalf("RequeueAfter = %s, want %s", cfg.RequeueAfter, 45*time.Second)
	}

	if cfg.GracefulShutdownTimeout != 20*time.Second {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, 20*time.Second)
	}

	if cfg.LogLevel != LogLevelDebug {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelDebug)
	}

	if cfg.ProviderMode != ProviderModeLocalHAProxy {
		t.Fatalf("ProviderMode = %q, want %q", cfg.ProviderMode, ProviderModeLocalHAProxy)
	}

	if cfg.HAProxyConfigPath != "/var/run/haproxy/controller.cfg" {
		t.Fatalf("HAProxyConfigPath = %q, want %q", cfg.HAProxyConfigPath, "/var/run/haproxy/controller.cfg")
	}

	if cfg.HAProxyValidateCommand != "haproxy -c -f {{config}}" {
		t.Fatalf("HAProxyValidateCommand = %q, want %q", cfg.HAProxyValidateCommand, "haproxy -c -f {{config}}")
	}

	if cfg.HAProxyReloadCommand != "service haproxy reload" {
		t.Fatalf("HAProxyReloadCommand = %q, want %q", cfg.HAProxyReloadCommand, "service haproxy reload")
	}

	if cfg.DataplaneAPIURL != "" {
		t.Fatalf("DataplaneAPIURL = %q, want empty string", cfg.DataplaneAPIURL)
	}

	if cfg.DataplaneAPITimeout != 12*time.Second {
		t.Fatalf("DataplaneAPITimeout = %s, want %s", cfg.DataplaneAPITimeout, 12*time.Second)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	setConfigEnvToEmpty(t)

	t.Setenv(EnvLeaderElect, "not-a-bool")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadDataplaneAPIProviderMode(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvProviderMode, string(ProviderModeDataplaneAPI))
	t.Setenv(EnvDataplaneAPIURL, "https://dataplane.example.local:8443/api/v1")
	t.Setenv(EnvDataplaneAPITimeout, "7s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ProviderMode != ProviderModeDataplaneAPI {
		t.Fatalf("ProviderMode = %q, want %q", cfg.ProviderMode, ProviderModeDataplaneAPI)
	}

	if cfg.DataplaneAPIURL != "https://dataplane.example.local:8443/api/v1" {
		t.Fatalf("DataplaneAPIURL = %q, want %q", cfg.DataplaneAPIURL, "https://dataplane.example.local:8443/api/v1")
	}

	if cfg.DataplaneAPITimeout != 7*time.Second {
		t.Fatalf("DataplaneAPITimeout = %s, want %s", cfg.DataplaneAPITimeout, 7*time.Second)
	}
}

func TestLoadRejectsDataplaneAPIModeWithoutURL(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvProviderMode, string(ProviderModeDataplaneAPI))

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidDataplaneAPITimeout(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvDataplaneAPITimeout, "not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidDataplaneAPIURL(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvProviderMode, string(ProviderModeDataplaneAPI))
	t.Setenv(EnvDataplaneAPIURL, "://bad-url")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidProviderMode(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvProviderMode, "invalid-mode")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidGracefulShutdownTimeout(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvGracefulShutdownTimeout, "not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsNonPositiveGracefulShutdownTimeout(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvGracefulShutdownTimeout, "0s")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsDuplicateIPPoolAddresses(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvIPPool, "203.0.113.10,203.0.113.10")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidIPPoolAddresses(t *testing.T) {
	setConfigEnvToEmpty(t)
	t.Setenv(EnvIPPool, "203.0.113.10,not-an-ip")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadDotEnvReturnsFalseWhenFileIsMissing(t *testing.T) {
	unsetConfigEnv(t)
	chdir(t, t.TempDir())

	loaded, err := LoadDotEnv()
	if err != nil {
		t.Fatalf("LoadDotEnv() error = %v", err)
	}

	if loaded {
		t.Fatal("LoadDotEnv() loaded = true, want false")
	}
}

func TestLoadDotEnvLoadsFileWithoutOverridingEnvironment(t *testing.T) {
	unsetConfigEnv(t)

	dir := t.TempDir()
	chdir(t, dir)

	content := []byte("K8S_LB_CONTROLLER_METRICS_ADDR=:19090\n" +
		"K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS=from-dotenv\n" +
		"K8S_LB_CONTROLLER_IP_POOL=203.0.113.30,203.0.113.31\n" +
		"K8S_LB_CONTROLLER_LOG_LEVEL=warn\n" +
		"K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH=/tmp/from-dotenv.cfg\n")
	if err := os.WriteFile(filepath.Join(dir, DotEnvFileName), content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := os.Setenv(EnvLoadBalancerClass, "from-env"); err != nil {
		t.Fatalf("Setenv() error = %v", err)
	}

	loaded, err := LoadDotEnv()
	if err != nil {
		t.Fatalf("LoadDotEnv() error = %v", err)
	}

	if !loaded {
		t.Fatal("LoadDotEnv() loaded = false, want true")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MetricsAddr != ":19090" {
		t.Fatalf("MetricsAddr = %q, want %q", cfg.MetricsAddr, ":19090")
	}

	if cfg.LoadBalancerClass != "from-env" {
		t.Fatalf("LoadBalancerClass = %q, want %q", cfg.LoadBalancerClass, "from-env")
	}

	wantPool := []netip.Addr{
		netip.MustParseAddr("203.0.113.30"),
		netip.MustParseAddr("203.0.113.31"),
	}
	if !slices.Equal(cfg.IPPool, wantPool) {
		t.Fatalf("IPPool = %v, want %v", cfg.IPPool, wantPool)
	}

	if cfg.LogLevel != LogLevelWarn {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelWarn)
	}

	if cfg.HAProxyConfigPath != "/tmp/from-dotenv.cfg" {
		t.Fatalf("HAProxyConfigPath = %q, want %q", cfg.HAProxyConfigPath, "/tmp/from-dotenv.cfg")
	}
}

func TestManagerManifestGracefulShutdownConfiguration(t *testing.T) {
	manifestPath := repoPath(t, "config", "manager", "manager.yaml")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", manifestPath, err)
	}

	deploymentDoc, ok := findYAMLDocumentByKind(string(content), "Deployment")
	if !ok {
		t.Fatalf("deployment document not found in %s", manifestPath)
	}

	var deployment struct {
		Spec struct {
			Template struct {
				Spec struct {
					TerminationGracePeriodSeconds *int64 `yaml:"terminationGracePeriodSeconds"`
					Containers                    []struct {
						Env []struct {
							Name  string `yaml:"name"`
							Value string `yaml:"value"`
						} `yaml:"env"`
					} `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(deploymentDoc), &deployment); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if deployment.Spec.Template.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("terminationGracePeriodSeconds = nil, want explicit value")
	}

	shutdownEnv, ok := findEnvValue(deployment.Spec.Template.Spec.Containers, EnvGracefulShutdownTimeout)
	if !ok {
		t.Fatalf("env %s not found in manager manifest", EnvGracefulShutdownTimeout)
	}

	shutdownTimeout, err := time.ParseDuration(shutdownEnv)
	if err != nil {
		t.Fatalf("ParseDuration(%q) error = %v", shutdownEnv, err)
	}

	if shutdownTimeout != DefaultGracefulShutdownTimeout {
		t.Fatalf("shutdown timeout = %s, want %s", shutdownTimeout, DefaultGracefulShutdownTimeout)
	}

	if *deployment.Spec.Template.Spec.TerminationGracePeriodSeconds < int64(shutdownTimeout.Seconds()) {
		t.Fatalf(
			"terminationGracePeriodSeconds = %d, want >= %d",
			*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds,
			int64(shutdownTimeout.Seconds()),
		)
	}
}

func setConfigEnvToEmpty(t *testing.T) {
	t.Helper()

	t.Setenv(EnvMetricsAddr, "")
	t.Setenv(EnvHealthAddr, "")
	t.Setenv(EnvLeaderElect, "")
	t.Setenv(EnvLoadBalancerClass, "")
	t.Setenv(EnvIPPool, "")
	t.Setenv(EnvRequeueAfter, "")
	t.Setenv(EnvGracefulShutdownTimeout, "")
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvProviderMode, "")
	t.Setenv(EnvHAProxyConfigPath, "")
	t.Setenv(EnvHAProxyValidateCommand, "")
	t.Setenv(EnvHAProxyReloadCommand, "")
	t.Setenv(EnvDataplaneAPIURL, "")
	t.Setenv(EnvDataplaneAPITimeout, "")
}

func unsetConfigEnv(t *testing.T) {
	t.Helper()

	keys := []string{
		EnvMetricsAddr,
		EnvHealthAddr,
		EnvLeaderElect,
		EnvLoadBalancerClass,
		EnvIPPool,
		EnvRequeueAfter,
		EnvGracefulShutdownTimeout,
		EnvLogLevel,
		EnvProviderMode,
		EnvHAProxyConfigPath,
		EnvHAProxyValidateCommand,
		EnvHAProxyReloadCommand,
		EnvDataplaneAPIURL,
		EnvDataplaneAPITimeout,
	}

	saved := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			v := value
			saved[key] = &v
		} else {
			saved[key] = nil
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q) error = %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if saved[key] == nil {
				_ = os.Unsetenv(key)
				continue
			}

			_ = os.Setenv(key, *saved[key])
		}
	})
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func repoPath(t *testing.T, pathElements ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	elements := []string{filepath.Dir(file), "..", ".."}
	elements = append(elements, pathElements...)
	return filepath.Clean(filepath.Join(elements...))
}

func findYAMLDocumentByKind(content, kind string) (string, bool) {
	for doc := range strings.SplitSeq(content, "\n---\n") {
		if strings.Contains(doc, "\nkind: "+kind+"\n") || strings.HasPrefix(doc, "kind: "+kind+"\n") {
			return doc, true
		}
	}

	return "", false
}

func findEnvValue(containers []struct {
	Env []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"env"`
}, envName string) (string, bool) {
	for _, container := range containers {
		for _, envVar := range container.Env {
			if envVar.Name == envName {
				return envVar.Value, true
			}
		}
	}

	return "", false
}
