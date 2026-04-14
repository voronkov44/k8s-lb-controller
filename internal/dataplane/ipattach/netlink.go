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
)

type netlinkManager struct {
	config Config
	client netlinkClient
}

func newNetlinkManager(cfg Config, client netlinkClient) (Manager, error) {
	if client == nil {
		var err error
		client, err = newDefaultNetlinkClient()
		if err != nil {
			return nil, err
		}
	}

	return &netlinkManager{
		config: cfg,
		client: client,
	}, nil
}

func (m *netlinkManager) List(context.Context) ([]netip.Addr, error) {
	prefixes, err := m.client.ListIPv4Addrs(m.config.Interface)
	if err != nil {
		return nil, fmt.Errorf("list interface addresses: %w", err)
	}

	addresses := make([]netip.Addr, 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix.Addr().Is4() && prefix.Bits() == m.config.CIDRSuffix {
			addresses = append(addresses, prefix.Addr())
		}
	}

	slices.SortFunc(addresses, func(left, right netip.Addr) int {
		return left.Compare(right)
	})

	return addresses, nil
}

func (m *netlinkManager) EnsurePresent(ctx context.Context, addr netip.Addr) (bool, error) {
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

	prefix := netip.PrefixFrom(addr, m.config.CIDRSuffix)
	if err := m.client.AddIPv4Addr(m.config.Interface, prefix); err != nil {
		return ensureNetlinkStateAfterOperation(ctx, m, addr, true, "attach external IP", err)
	}

	return true, nil
}

func (m *netlinkManager) EnsureAbsent(ctx context.Context, addr netip.Addr) (bool, error) {
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

	prefix := netip.PrefixFrom(addr, m.config.CIDRSuffix)
	if err := m.client.DeleteIPv4Addr(m.config.Interface, prefix); err != nil {
		return ensureNetlinkStateAfterOperation(ctx, m, addr, false, "detach external IP", err)
	}

	return true, nil
}

func ensureNetlinkStateAfterOperation(ctx context.Context, manager *netlinkManager, addr netip.Addr, wantPresent bool, action string, operationErr error) (bool, error) {
	current, err := manager.List(ctx)
	if err == nil && containsAddr(current, addr) == wantPresent {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", action, err)
	}

	return false, fmt.Errorf("%s: %w", action, operationErr)
}
