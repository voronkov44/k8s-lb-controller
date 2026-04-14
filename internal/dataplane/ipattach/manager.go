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

package ipattach

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
)

const (
	// DefaultCommandPath is the default system command used to manage interface addresses in exec mode.
	DefaultCommandPath = "ip"
	// DefaultCIDRSuffix is the default prefix length used for attached IPv4 addresses.
	DefaultCIDRSuffix = 32
)

// Mode selects which host-side IP attachment backend the dataplane uses.
type Mode string

const (
	// ModeExec keeps the stage-4 command-based attachment path.
	ModeExec Mode = "exec"
	// ModeNetlink uses native Linux netlink calls for IPv4 attachment.
	ModeNetlink Mode = "netlink"
	// DefaultMode prefers the native Linux netlink path and keeps exec available as an explicit fallback.
	DefaultMode = ModeNetlink
)

// Config contains runtime settings for host-side external IP attachment.
type Config struct {
	Enabled     bool
	Mode        Mode
	Interface   string
	CommandPath string
	CIDRSuffix  int
}

// Manager reconciles IPv4 addresses on one host interface.
type Manager interface {
	List(ctx context.Context) ([]netip.Addr, error)
	EnsurePresent(ctx context.Context, addr netip.Addr) (bool, error)
	EnsureAbsent(ctx context.Context, addr netip.Addr) (bool, error)
}

// Runner executes system commands for the exec attachment backend.
type Runner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner executes real system commands through os/exec.
type ExecRunner struct{}

// CombinedOutput runs one command and returns combined stdout/stderr output.
func (ExecRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type netlinkClient interface {
	ListIPv4Addrs(interfaceName string) ([]netip.Prefix, error)
	AddIPv4Addr(interfaceName string, prefix netip.Prefix) error
	DeleteIPv4Addr(interfaceName string, prefix netip.Prefix) error
}

// Dependencies provides optional test hooks for specific attachment backends.
type Dependencies struct {
	Runner        Runner
	NetlinkClient netlinkClient
}

// NewManager creates the configured external IP attachment backend.
func NewManager(cfg Config, deps Dependencies) (Manager, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	if !normalized.Enabled {
		return noopManager{}, nil
	}

	switch normalized.Mode {
	case ModeExec:
		return newExecManager(normalized, deps.Runner), nil
	case ModeNetlink:
		return newNetlinkManager(normalized, deps.NetlinkClient)
	default:
		return nil, fmt.Errorf("unsupported IP attachment mode %q", normalized.Mode)
	}
}

// Normalize returns the canonical lowercase representation of the attachment mode.
func (m Mode) Normalize() Mode {
	return normalizeMode(m)
}

// Valid reports whether the attachment mode is supported.
func (m Mode) Valid() bool {
	switch m.Normalize() {
	case ModeExec, ModeNetlink:
		return true
	default:
		return false
	}
}

func normalizeConfig(cfg Config) (Config, error) {
	normalized := cfg
	normalized.Mode = cfg.Mode.Normalize()
	if normalized.Mode == "" {
		normalized.Mode = DefaultMode
	}

	switch normalized.Mode {
	case ModeExec, ModeNetlink:
	default:
		return Config{}, fmt.Errorf("unsupported IP attachment mode %q", cfg.Mode)
	}

	normalized.Interface = strings.TrimSpace(cfg.Interface)
	normalized.CommandPath = strings.TrimSpace(cfg.CommandPath)
	if normalized.CommandPath == "" {
		normalized.CommandPath = DefaultCommandPath
	}

	if normalized.CIDRSuffix == 0 {
		normalized.CIDRSuffix = DefaultCIDRSuffix
	}
	if normalized.CIDRSuffix < 1 || normalized.CIDRSuffix > 32 {
		return Config{}, fmt.Errorf("CIDR suffix must be between 1 and 32")
	}

	if normalized.Enabled && normalized.Interface == "" {
		return Config{}, fmt.Errorf("interface must not be empty when IP attachment is enabled")
	}

	return normalized, nil
}

func normalizeMode(mode Mode) Mode {
	return Mode(strings.ToLower(strings.TrimSpace(string(mode))))
}

func validateIPv4Addr(addr netip.Addr) error {
	if !addr.IsValid() || !addr.Is4() {
		return fmt.Errorf("external IP %q must be a valid IPv4 address", addr)
	}

	return nil
}

func containsAddr(addresses []netip.Addr, target netip.Addr) bool {
	return slices.ContainsFunc(addresses, func(candidate netip.Addr) bool {
		return candidate == target
	})
}

type noopManager struct{}

func (noopManager) List(context.Context) ([]netip.Addr, error) {
	return nil, nil
}

func (noopManager) EnsurePresent(context.Context, netip.Addr) (bool, error) {
	return false, nil
}

func (noopManager) EnsureAbsent(context.Context, netip.Addr) (bool, error) {
	return false, nil
}
