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
	"slices"
	"strings"
)

type execManager struct {
	config Config
	runner Runner
}

func newExecManager(cfg Config, runner Runner) Manager {
	if runner == nil {
		runner = ExecRunner{}
	}

	return &execManager{
		config: cfg,
		runner: runner,
	}
}

func (m *execManager) List(ctx context.Context) ([]netip.Addr, error) {
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

func (m *execManager) EnsurePresent(ctx context.Context, addr netip.Addr) (bool, error) {
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
		return ensureExecStateAfterCommand(ctx, m, addr, true, "attach external IP", err, output)
	}

	return true, nil
}

func (m *execManager) EnsureAbsent(ctx context.Context, addr netip.Addr) (bool, error) {
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
		return ensureExecStateAfterCommand(ctx, m, addr, false, "detach external IP", err, output)
	}

	return true, nil
}

func ensureExecStateAfterCommand(ctx context.Context, manager *execManager, addr netip.Addr, wantPresent bool, action string, commandErr error, output []byte) (bool, error) {
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
