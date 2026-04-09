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
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"testing"
)

func TestNetlinkManagerEnsurePresentAddsAddress(t *testing.T) {
	client := newFakeNetlinkClient()
	manager := newTestNetlinkManager(t, client)

	changed, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsurePresent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsurePresent() changed = false, want true")
	}
	if !client.hasAddr("eth0", "203.0.113.10") {
		t.Fatal("address was not attached")
	}
}

func TestNetlinkManagerEnsureAbsentDeletesAddress(t *testing.T) {
	client := newFakeNetlinkClient("eth0", "203.0.113.10")
	manager := newTestNetlinkManager(t, client)

	changed, err := manager.EnsureAbsent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsureAbsent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureAbsent() changed = false, want true")
	}
	if client.hasAddr("eth0", "203.0.113.10") {
		t.Fatal("address was not detached")
	}
}

func TestNetlinkManagerEnsurePresentIsIdempotent(t *testing.T) {
	client := newFakeNetlinkClient("eth0", "203.0.113.10")
	manager := newTestNetlinkManager(t, client)

	changed, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsurePresent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsurePresent() changed = true, want false")
	}
	if got := client.operationCount("add"); got != 0 {
		t.Fatalf("AddIPv4Addr() count = %d, want 0", got)
	}
}

func TestNetlinkManagerEnsureAbsentIsIdempotent(t *testing.T) {
	client := newFakeNetlinkClient()
	manager := newTestNetlinkManager(t, client)

	changed, err := manager.EnsureAbsent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsureAbsent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureAbsent() changed = true, want false")
	}
	if got := client.operationCount("delete"); got != 0 {
		t.Fatalf("DeleteIPv4Addr() count = %d, want 0", got)
	}
}

func TestNetlinkManagerReturnsOperationFailure(t *testing.T) {
	client := newFakeNetlinkClient()
	client.addErr = errors.New("boom")
	manager := newTestNetlinkManager(t, client)

	_, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err == nil {
		t.Fatal("EnsurePresent() error = nil, want non-nil")
	}
}

func newTestNetlinkManager(t *testing.T, client netlinkClient) Manager {
	t.Helper()

	manager, err := NewManager(Config{
		Enabled:    true,
		Mode:       ModeNetlink,
		Interface:  "eth0",
		CIDRSuffix: 32,
	}, Dependencies{NetlinkClient: client})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	return manager
}

type fakeNetlinkClient struct {
	byInterface map[string]map[netip.Prefix]struct{}
	operations  []string
	addErr      error
	deleteErr   error
	listErr     error
}

func newFakeNetlinkClient(initial ...string) *fakeNetlinkClient {
	client := &fakeNetlinkClient{
		byInterface: map[string]map[netip.Prefix]struct{}{},
	}

	for index := 0; index+1 < len(initial); index += 2 {
		client.addPrefix(initial[index], netip.MustParsePrefix(initial[index+1]+"/32"))
	}

	return client
}

func (c *fakeNetlinkClient) ListIPv4Addrs(interfaceName string) ([]netip.Prefix, error) {
	c.operations = append(c.operations, "list")
	if c.listErr != nil {
		return nil, c.listErr
	}

	prefixes := make([]netip.Prefix, 0, len(c.byInterface[interfaceName]))
	for prefix := range c.byInterface[interfaceName] {
		prefixes = append(prefixes, prefix)
	}
	slices.SortFunc(prefixes, func(left, right netip.Prefix) int {
		if left.Addr() == right.Addr() {
			return left.Bits() - right.Bits()
		}
		return left.Addr().Compare(right.Addr())
	})

	return prefixes, nil
}

func (c *fakeNetlinkClient) AddIPv4Addr(interfaceName string, prefix netip.Prefix) error {
	c.operations = append(c.operations, "add")
	if c.addErr != nil {
		return c.addErr
	}

	c.addPrefix(interfaceName, prefix)
	return nil
}

func (c *fakeNetlinkClient) DeleteIPv4Addr(interfaceName string, prefix netip.Prefix) error {
	c.operations = append(c.operations, "delete")
	if c.deleteErr != nil {
		return c.deleteErr
	}

	if addresses, ok := c.byInterface[interfaceName]; ok {
		delete(addresses, prefix)
	}
	return nil
}

func (c *fakeNetlinkClient) addPrefix(interfaceName string, prefix netip.Prefix) {
	if _, ok := c.byInterface[interfaceName]; !ok {
		c.byInterface[interfaceName] = make(map[netip.Prefix]struct{})
	}
	c.byInterface[interfaceName][prefix] = struct{}{}
}

func (c *fakeNetlinkClient) hasAddr(interfaceName, addr string) bool {
	addresses := c.byInterface[interfaceName]
	for prefix := range addresses {
		if prefix.Addr().String() == addr {
			return true
		}
	}

	return false
}

func (c *fakeNetlinkClient) operationCount(operation string) int {
	count := 0
	for _, candidate := range c.operations {
		if candidate == operation {
			count++
		}
	}

	return count
}

func (c *fakeNetlinkClient) String() string {
	return fmt.Sprintf("%v", c.byInterface)
}
