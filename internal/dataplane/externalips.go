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

package dataplane

import (
	"fmt"
	"net/netip"
	"slices"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

func desiredExternalIPs(services []provider.Service) ([]netip.Addr, error) {
	unique := make(map[netip.Addr]struct{})
	for _, service := range services {
		if service.ExternalIP == "" {
			continue
		}

		addr, err := netip.ParseAddr(service.ExternalIP)
		if err != nil {
			return nil, fmt.Errorf("parse desired external IP %q for %s: %w", service.ExternalIP, service.Ref(), err)
		}
		if !addr.Is4() {
			return nil, fmt.Errorf("desired external IP %q for %s must be IPv4", service.ExternalIP, service.Ref())
		}

		unique[addr] = struct{}{}
	}

	addresses := make([]netip.Addr, 0, len(unique))
	for addr := range unique {
		addresses = append(addresses, addr)
	}

	slices.SortFunc(addresses, func(left, right netip.Addr) int {
		return left.Compare(right)
	})

	return addresses, nil
}

func diffExternalIPs(left, right []netip.Addr) []netip.Addr {
	rightSet := make(map[netip.Addr]struct{}, len(right))
	for _, addr := range right {
		rightSet[addr] = struct{}{}
	}

	diff := make([]netip.Addr, 0, len(left))
	for _, addr := range left {
		if _, ok := rightSet[addr]; ok {
			continue
		}

		diff = append(diff, addr)
	}

	return diff
}
