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
