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

package ipam

import (
	"net/netip"
	"slices"
	"testing"
)

func TestParsePoolTrimsSkipsEmptyItemsAndPreservesOrder(t *testing.T) {
	pool, err := ParsePool(" 203.0.113.10 , ,203.0.113.11, 203.0.113.12 ")
	if err != nil {
		t.Fatalf("ParsePool() error = %v", err)
	}

	want := []netip.Addr{
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
		netip.MustParseAddr("203.0.113.12"),
	}
	if !slices.Equal(pool, want) {
		t.Fatalf("ParsePool() = %v, want %v", pool, want)
	}
}

func TestParsePoolRejectsIPv6Addresses(t *testing.T) {
	if _, err := ParsePool("203.0.113.10,2001:db8::1"); err == nil {
		t.Fatal("ParsePool() error = nil, want non-nil")
	}
}
