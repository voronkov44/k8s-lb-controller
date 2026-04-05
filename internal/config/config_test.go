package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
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
	t.Setenv(EnvLogLevel, "DEBUG")

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

	if cfg.LogLevel != LogLevelDebug {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelDebug)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	setConfigEnvToEmpty(t)

	t.Setenv(EnvLeaderElect, "not-a-bool")

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
		"K8S_LB_CONTROLLER_LOG_LEVEL=warn\n")
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
}

func setConfigEnvToEmpty(t *testing.T) {
	t.Helper()

	t.Setenv(EnvMetricsAddr, "")
	t.Setenv(EnvHealthAddr, "")
	t.Setenv(EnvLeaderElect, "")
	t.Setenv(EnvLoadBalancerClass, "")
	t.Setenv(EnvIPPool, "")
	t.Setenv(EnvRequeueAfter, "")
	t.Setenv(EnvLogLevel, "")
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
		EnvLogLevel,
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
