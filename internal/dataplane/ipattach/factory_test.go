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

import "testing"

func TestNewManagerSelectsExecMode(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled:   true,
		Mode:      ModeExec,
		Interface: "eth0",
	}, Dependencies{Runner: newFakeRunner()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, ok := manager.(*execManager); !ok {
		t.Fatalf("manager type = %T, want *execManager", manager)
	}
}

func TestNewManagerSelectsNetlinkMode(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled:   true,
		Mode:      ModeNetlink,
		Interface: "eth0",
	}, Dependencies{NetlinkClient: newFakeNetlinkClient()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, ok := manager.(*netlinkManager); !ok {
		t.Fatalf("manager type = %T, want *netlinkManager", manager)
	}
}

func TestNewManagerDefaultsToNetlinkMode(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled:   true,
		Interface: "eth0",
	}, Dependencies{NetlinkClient: newFakeNetlinkClient()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, ok := manager.(*netlinkManager); !ok {
		t.Fatalf("manager type = %T, want *netlinkManager", manager)
	}
}

func TestNewManagerRejectsInvalidMode(t *testing.T) {
	_, err := NewManager(Config{
		Enabled:   true,
		Mode:      Mode("mystery"),
		Interface: "eth0",
	}, Dependencies{})
	if err == nil {
		t.Fatal("NewManager() error = nil, want non-nil")
	}
}

func TestNewManagerDisabledReturnsNoopManager(t *testing.T) {
	manager, err := NewManager(Config{}, Dependencies{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, ok := manager.(noopManager); !ok {
		t.Fatalf("manager type = %T, want noopManager", manager)
	}
}
