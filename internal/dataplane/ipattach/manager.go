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
	// DefaultCommandPath is the default system command used to manage interface addresses.
	DefaultCommandPath = "ip"
	// DefaultCIDRSuffix is the default prefix length used for attached IPv4 addresses.
	DefaultCIDRSuffix = 32
)

// Config contains runtime settings for command-based external IP attachment.
type Config struct {
	Enabled     bool
	Interface   string
	CommandPath string
	CIDRSuffix  int
}

// Runner executes system commands for the attachment manager.
type Runner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner executes real system commands through os/exec.
type ExecRunner struct{}

// CombinedOutput runs one command and returns combined stdout/stderr output.
func (ExecRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Manager reconciles IPv4 addresses on one host interface through the `ip` command.
type Manager struct {
	config Config
	runner Runner
}

// NewManager creates a command-based attachment manager with validated settings.
func NewManager(cfg Config, runner Runner) (*Manager, error) {
	commandPath := strings.TrimSpace(cfg.CommandPath)
	if commandPath == "" {
		commandPath = DefaultCommandPath
	}

	interfaceName := strings.TrimSpace(cfg.Interface)
	if cfg.Enabled && interfaceName == "" {
		return nil, fmt.Errorf("interface must not be empty when IP attachment is enabled")
	}

	cidrSuffix := cfg.CIDRSuffix
	if cidrSuffix == 0 {
		cidrSuffix = DefaultCIDRSuffix
	}
	if cidrSuffix < 1 || cidrSuffix > 32 {
		return nil, fmt.Errorf("CIDR suffix must be between 1 and 32")
	}

	if runner == nil {
		runner = ExecRunner{}
	}

	return &Manager{
		config: Config{
			Enabled:     cfg.Enabled,
			Interface:   interfaceName,
			CommandPath: commandPath,
			CIDRSuffix:  cidrSuffix,
		},
		runner: runner,
	}, nil
}

// Enabled reports whether the manager actively mutates host addresses.
func (m *Manager) Enabled() bool {
	return m != nil && m.config.Enabled
}

// List returns the current IPv4 addresses attached to the configured interface.
func (m *Manager) List(ctx context.Context) ([]netip.Addr, error) {
	if !m.Enabled() {
		return nil, nil
	}

	output, err := m.runner.CombinedOutput(ctx, m.config.CommandPath, "-4", "-o", "addr", "show", "dev", m.config.Interface)
	if err != nil {
		return nil, commandError("list interface addresses", err, output)
	}

	addresses, err := parseInterfaceAddrs(output)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(addresses, func(left, right netip.Addr) int {
		return left.Compare(right)
	})

	return addresses, nil
}

// EnsurePresent makes one IPv4 address present on the configured interface.
func (m *Manager) EnsurePresent(ctx context.Context, addr netip.Addr) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}
	if err := validateIPv4Addr(addr); err != nil {
		return false, err
	}

	current, err := m.List(ctx)
	if err != nil {
		return false, err
	}
	if containsAddr(current, addr) {
		return false, nil
	}

	output, err := m.runner.CombinedOutput(
		ctx,
		m.config.CommandPath,
		"addr",
		"add",
		fmt.Sprintf("%s/%d", addr, m.config.CIDRSuffix),
		"dev",
		m.config.Interface,
	)
	if err != nil {
		return ensureStateAfterCommand(ctx, m, addr, true, "attach external IP", err, output)
	}

	return true, nil
}

// EnsureAbsent makes one IPv4 address absent from the configured interface.
func (m *Manager) EnsureAbsent(ctx context.Context, addr netip.Addr) (bool, error) {
	if !m.Enabled() {
		return false, nil
	}
	if err := validateIPv4Addr(addr); err != nil {
		return false, err
	}

	current, err := m.List(ctx)
	if err != nil {
		return false, err
	}
	if !containsAddr(current, addr) {
		return false, nil
	}

	output, err := m.runner.CombinedOutput(
		ctx,
		m.config.CommandPath,
		"addr",
		"del",
		fmt.Sprintf("%s/%d", addr, m.config.CIDRSuffix),
		"dev",
		m.config.Interface,
	)
	if err != nil {
		return ensureStateAfterCommand(ctx, m, addr, false, "detach external IP", err, output)
	}

	return true, nil
}

func ensureStateAfterCommand(ctx context.Context, manager *Manager, addr netip.Addr, wantPresent bool, action string, commandErr error, output []byte) (bool, error) {
	current, err := manager.List(ctx)
	if err == nil && containsAddr(current, addr) == wantPresent {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", commandFailureMessage(action, output), err)
	}

	return false, commandError(action, commandErr, output)
}

func commandFailureMessage(action string, output []byte) string {
	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		return action
	}

	return fmt.Sprintf("%s: %s", action, trimmedOutput)
}

func commandError(action string, err error, output []byte) error {
	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		return fmt.Errorf("%s: %w", action, err)
	}

	return fmt.Errorf("%s: %w: %s", action, err, trimmedOutput)
}

func parseInterfaceAddrs(output []byte) ([]netip.Addr, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	addresses := make([]netip.Addr, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		for index, field := range fields {
			if field != "inet" || index+1 >= len(fields) {
				continue
			}

			prefix, err := netip.ParsePrefix(fields[index+1])
			if err != nil {
				return nil, fmt.Errorf("parse interface address %q: %w", fields[index+1], err)
			}
			if prefix.Addr().Is4() {
				addresses = append(addresses, prefix.Addr())
			}
			break
		}
	}

	return addresses, nil
}

func containsAddr(addresses []netip.Addr, target netip.Addr) bool {
	return slices.ContainsFunc(addresses, func(candidate netip.Addr) bool {
		return candidate == target
	})
}

func validateIPv4Addr(addr netip.Addr) error {
	if !addr.IsValid() || !addr.Is4() {
		return fmt.Errorf("external IP %q must be a valid IPv4 address", addr)
	}

	return nil
}
