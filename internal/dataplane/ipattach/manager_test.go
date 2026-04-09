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
	"strings"
	"testing"
)

func TestManagerEnsurePresentAddsAddress(t *testing.T) {
	runner := newFakeRunner()
	manager := newTestManager(t, runner)

	changed, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsurePresent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsurePresent() changed = false, want true")
	}

	if !runner.hasAddr("203.0.113.10") {
		t.Fatal("address was not attached")
	}
}

func TestManagerEnsureAbsentDeletesAddress(t *testing.T) {
	runner := newFakeRunner("203.0.113.10")
	manager := newTestManager(t, runner)

	changed, err := manager.EnsureAbsent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsureAbsent() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureAbsent() changed = false, want true")
	}

	if runner.hasAddr("203.0.113.10") {
		t.Fatal("address was not detached")
	}
}

func TestManagerEnsurePresentIsIdempotent(t *testing.T) {
	runner := newFakeRunner("203.0.113.10")
	manager := newTestManager(t, runner)

	changed, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsurePresent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsurePresent() changed = true, want false")
	}

	if runner.countCommands("addr add") != 0 {
		t.Fatalf("add command count = %d, want 0", runner.countCommands("addr add"))
	}
}

func TestManagerEnsureAbsentIsIdempotent(t *testing.T) {
	runner := newFakeRunner()
	manager := newTestManager(t, runner)

	changed, err := manager.EnsureAbsent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err != nil {
		t.Fatalf("EnsureAbsent() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureAbsent() changed = true, want false")
	}

	if runner.countCommands("addr del") != 0 {
		t.Fatalf("del command count = %d, want 0", runner.countCommands("addr del"))
	}
}

func TestManagerReturnsCommandFailure(t *testing.T) {
	runner := newFakeRunner()
	runner.failAdd = errors.New("boom")
	manager := newTestManager(t, runner)

	_, err := manager.EnsurePresent(context.Background(), netip.MustParseAddr("203.0.113.10"))
	if err == nil {
		t.Fatal("EnsurePresent() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "attach external IP") {
		t.Fatalf("EnsurePresent() error = %v, want attach external IP context", err)
	}
}

type fakeRunner struct {
	addresses map[string]struct{}
	commands  []string
	failAdd   error
	failDel   error
	failList  error
}

func newFakeRunner(initialAddrs ...string) *fakeRunner {
	addresses := make(map[string]struct{}, len(initialAddrs))
	for _, addr := range initialAddrs {
		addresses[addr] = struct{}{}
	}

	return &fakeRunner{addresses: addresses}
}

func (r *fakeRunner) CombinedOutput(_ context.Context, name string, args ...string) ([]byte, error) {
	commandLine := strings.Join(append([]string{name}, args...), " ")
	r.commands = append(r.commands, commandLine)

	switch {
	case slices.Equal(args, []string{"-4", "-o", "addr", "show", "dev", "eth0"}):
		if r.failList != nil {
			return []byte("show failed"), r.failList
		}
		return []byte(r.renderList()), nil
	case len(args) == 5 && args[0] == "addr" && args[1] == "add":
		if r.failAdd != nil {
			return []byte("add failed"), r.failAdd
		}
		addr, err := prefixAddr(args[2])
		if err != nil {
			return nil, err
		}
		r.addresses[addr] = struct{}{}
		return nil, nil
	case len(args) == 5 && args[0] == "addr" && args[1] == "del":
		if r.failDel != nil {
			return []byte("del failed"), r.failDel
		}
		addr, err := prefixAddr(args[2])
		if err != nil {
			return nil, err
		}
		delete(r.addresses, addr)
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", commandLine)
	}
}

func (r *fakeRunner) hasAddr(addr string) bool {
	_, ok := r.addresses[addr]
	return ok
}

func (r *fakeRunner) countCommands(fragment string) int {
	count := 0
	for _, commandLine := range r.commands {
		if strings.Contains(commandLine, fragment) {
			count++
		}
	}

	return count
}

func (r *fakeRunner) renderList() string {
	addresses := make([]string, 0, len(r.addresses))
	for addr := range r.addresses {
		addresses = append(addresses, addr)
	}
	slices.Sort(addresses)

	lines := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		lines = append(lines, fmt.Sprintf("2: eth0    inet %s/32 scope global eth0", addr))
	}

	return strings.Join(lines, "\n")
}

func newTestManager(t *testing.T, runner Runner) *Manager {
	t.Helper()

	manager, err := NewManager(Config{
		Enabled:     true,
		Interface:   "eth0",
		CommandPath: "ip",
		CIDRSuffix:  32,
	}, runner)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	return manager
}

func prefixAddr(value string) (string, error) {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return "", err
	}

	return prefix.Addr().String(), nil
}
