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
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	setDataplaneEnvToEmpty(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.HTTPAddr != DefaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, DefaultHTTPAddr)
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

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}

	if cfg.GracefulShutdownTimeout != DefaultGracefulShutdownTimeout {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvHTTPAddr, ":18080")
	t.Setenv(EnvHAProxyConfigPath, "/tmp/dataplane.cfg")
	t.Setenv(EnvHAProxyValidateCommand, "haproxy -c -f {{config}}")
	t.Setenv(EnvHAProxyReloadCommand, "service haproxy reload")
	t.Setenv(EnvLogLevel, "DEBUG")
	t.Setenv(EnvGracefulShutdownTimeout, "20s")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":18080")
	}

	if cfg.HAProxyConfigPath != "/tmp/dataplane.cfg" {
		t.Fatalf("HAProxyConfigPath = %q, want %q", cfg.HAProxyConfigPath, "/tmp/dataplane.cfg")
	}

	if cfg.HAProxyValidateCommand != "haproxy -c -f {{config}}" {
		t.Fatalf("HAProxyValidateCommand = %q, want %q", cfg.HAProxyValidateCommand, "haproxy -c -f {{config}}")
	}

	if cfg.HAProxyReloadCommand != "service haproxy reload" {
		t.Fatalf("HAProxyReloadCommand = %q, want %q", cfg.HAProxyReloadCommand, "service haproxy reload")
	}

	if cfg.LogLevel != LogLevelDebug {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelDebug)
	}

	if cfg.GracefulShutdownTimeout != 20*time.Second {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, 20*time.Second)
	}
}

func TestLoadConfigRejectsInvalidLogLevel(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvLogLevel, "trace")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want non-nil")
	}
}

func TestLoadConfigRejectsInvalidGracefulShutdownTimeout(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvGracefulShutdownTimeout, "0s")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want non-nil")
	}
}

func setDataplaneEnvToEmpty(t *testing.T) {
	t.Helper()

	t.Setenv(EnvHTTPAddr, "")
	t.Setenv(EnvHAProxyConfigPath, "")
	t.Setenv(EnvHAProxyValidateCommand, "")
	t.Setenv(EnvHAProxyReloadCommand, "")
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvGracefulShutdownTimeout, "")
}
