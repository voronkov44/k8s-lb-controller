package ipam

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// ParsePool parses a comma-separated list of IPv4 addresses into a validated pool.
func ParsePool(raw string) ([]netip.Addr, error) {
	parts := strings.Split(raw, ",")
	pool := make([]netip.Addr, 0, len(parts))
	seen := make(map[netip.Addr]struct{}, len(parts))

	for index, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}

		addr, err := netip.ParseAddr(candidate)
		if err != nil {
			return nil, fmt.Errorf("item %d %q is not a valid IP address: %w", index+1, candidate, err)
		}

		if !addr.Is4() {
			return nil, fmt.Errorf("item %d %q must be an IPv4 address", index+1, candidate)
		}

		if _, ok := seen[addr]; ok {
			return nil, fmt.Errorf("duplicate IPv4 address %q", candidate)
		}

		seen[addr] = struct{}{}
		pool = append(pool, addr)
	}

	if len(pool) == 0 {
		return nil, fmt.Errorf("must contain at least one IPv4 address")
	}

	return pool, nil
}

// Contains reports whether the pool contains the provided IPv4 address.
func Contains(pool []netip.Addr, addr netip.Addr) bool {
	return slices.Contains(pool, addr)
}
