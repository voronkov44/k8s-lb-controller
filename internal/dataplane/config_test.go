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

	"github.com/voronkov44/k8s-lb-controller/internal/dataplane/ipattach"
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

	if cfg.HAProxyPIDFile != DefaultHAProxyPIDFile {
		t.Fatalf("HAProxyPIDFile = %q, want %q", cfg.HAProxyPIDFile, DefaultHAProxyPIDFile)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}

	if cfg.GracefulShutdownTimeout != DefaultGracefulShutdownTimeout {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	}

	if cfg.IPAttachEnabled != DefaultIPAttachEnabled {
		t.Fatalf("IPAttachEnabled = %t, want %t", cfg.IPAttachEnabled, DefaultIPAttachEnabled)
	}

	if cfg.IPAttachMode != DefaultIPAttachMode {
		t.Fatalf("IPAttachMode = %q, want %q", cfg.IPAttachMode, DefaultIPAttachMode)
	}

	if cfg.Interface != DefaultInterface {
		t.Fatalf("Interface = %q, want %q", cfg.Interface, DefaultInterface)
	}

	if cfg.IPCommand != DefaultIPCommand {
		t.Fatalf("IPCommand = %q, want %q", cfg.IPCommand, DefaultIPCommand)
	}

	if cfg.IPCIDRSuffix != DefaultIPCIDRSuffix {
		t.Fatalf("IPCIDRSuffix = %d, want %d", cfg.IPCIDRSuffix, DefaultIPCIDRSuffix)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvHTTPAddr, ":18080")
	t.Setenv(EnvHAProxyConfigPath, "/tmp/dataplane.cfg")
	t.Setenv(EnvHAProxyValidateCommand, "haproxy -c -f {{config}}")
	t.Setenv(EnvHAProxyReloadCommand, "service haproxy reload")
	t.Setenv(EnvHAProxyPIDFile, "/run/haproxy.pid")
	t.Setenv(EnvLogLevel, "DEBUG")
	t.Setenv(EnvGracefulShutdownTimeout, "20s")
	t.Setenv(EnvIPAttachEnabled, "true")
	t.Setenv(EnvIPAttachMode, "exec")
	t.Setenv(EnvInterface, "eth0")
	t.Setenv(EnvIPCommand, "/sbin/ip")
	t.Setenv(EnvIPCIDRSuffix, "32")

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

	if cfg.HAProxyPIDFile != "/run/haproxy.pid" {
		t.Fatalf("HAProxyPIDFile = %q, want %q", cfg.HAProxyPIDFile, "/run/haproxy.pid")
	}

	if cfg.LogLevel != LogLevelDebug {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelDebug)
	}

	if cfg.GracefulShutdownTimeout != 20*time.Second {
		t.Fatalf("GracefulShutdownTimeout = %s, want %s", cfg.GracefulShutdownTimeout, 20*time.Second)
	}

	if !cfg.IPAttachEnabled {
		t.Fatal("IPAttachEnabled = false, want true")
	}

	if cfg.IPAttachMode != ipattach.ModeExec {
		t.Fatalf("IPAttachMode = %q, want %q", cfg.IPAttachMode, ipattach.ModeExec)
	}

	if cfg.Interface != "eth0" {
		t.Fatalf("Interface = %q, want %q", cfg.Interface, "eth0")
	}

	if cfg.IPCommand != "/sbin/ip" {
		t.Fatalf("IPCommand = %q, want %q", cfg.IPCommand, "/sbin/ip")
	}

	if cfg.IPCIDRSuffix != 32 {
		t.Fatalf("IPCIDRSuffix = %d, want %d", cfg.IPCIDRSuffix, 32)
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

func TestLoadConfigAcceptsNetlinkModeWithoutCommand(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvIPAttachEnabled, "true")
	t.Setenv(EnvIPAttachMode, "netlink")
	t.Setenv(EnvInterface, "eth0")
	t.Setenv(EnvIPCommand, "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.IPAttachMode != ipattach.ModeNetlink {
		t.Fatalf("IPAttachMode = %q, want %q", cfg.IPAttachMode, ipattach.ModeNetlink)
	}
}

func TestLoadConfigRejectsIPAttachWithoutInterface(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvIPAttachEnabled, "true")
	t.Setenv(EnvInterface, " ")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want non-nil")
	}
}

func TestLoadConfigRejectsInvalidIPCIDRSuffix(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvIPCIDRSuffix, "33")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want non-nil")
	}
}

func TestLoadConfigRejectsInvalidIPAttachBool(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvIPAttachEnabled, "definitely")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want non-nil")
	}
}

func TestLoadConfigRejectsInvalidIPAttachMode(t *testing.T) {
	setDataplaneEnvToEmpty(t)
	t.Setenv(EnvIPAttachMode, "mystery")

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
	t.Setenv(EnvHAProxyPIDFile, "")
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvGracefulShutdownTimeout, "")
	t.Setenv(EnvIPAttachEnabled, "")
	t.Setenv(EnvIPAttachMode, "")
	t.Setenv(EnvInterface, "")
	t.Setenv(EnvIPCommand, "")
	t.Setenv(EnvIPCIDRSuffix, "")
}
