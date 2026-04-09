//go:build linux

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
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"
)

type linuxNetlinkClient struct{}

func newDefaultNetlinkClient() (netlinkClient, error) {
	return linuxNetlinkClient{}, nil
}

func (linuxNetlinkClient) ListIPv4Addrs(interfaceName string) ([]netip.Prefix, error) {
	link, err := netlink.LinkByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("lookup interface %q: %w", interfaceName, err)
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list IPv4 addresses for interface %q: %w", interfaceName, err)
	}

	prefixes := make([]netip.Prefix, 0, len(addrs))
	for _, addr := range addrs {
		if addr.IPNet == nil {
			continue
		}

		prefix, err := prefixFromIPNet(addr.IPNet)
		if err != nil {
			return nil, fmt.Errorf("convert address %q on interface %q: %w", addr.IPNet.String(), interfaceName, err)
		}

		if prefix.Addr().Is4() {
			prefixes = append(prefixes, prefix)
		}
	}

	return prefixes, nil
}

func (linuxNetlinkClient) AddIPv4Addr(interfaceName string, prefix netip.Prefix) error {
	link, err := netlink.LinkByName(interfaceName)
	if err != nil {
		return fmt.Errorf("lookup interface %q: %w", interfaceName, err)
	}

	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: prefixToIPNet(prefix)}); err != nil {
		return fmt.Errorf("add IPv4 address %s to interface %q: %w", prefix, interfaceName, err)
	}

	return nil
}

func (linuxNetlinkClient) DeleteIPv4Addr(interfaceName string, prefix netip.Prefix) error {
	link, err := netlink.LinkByName(interfaceName)
	if err != nil {
		return fmt.Errorf("lookup interface %q: %w", interfaceName, err)
	}

	if err := netlink.AddrDel(link, &netlink.Addr{IPNet: prefixToIPNet(prefix)}); err != nil {
		return fmt.Errorf("delete IPv4 address %s from interface %q: %w", prefix, interfaceName, err)
	}

	return nil
}

func prefixToIPNet(prefix netip.Prefix) *net.IPNet {
	return &net.IPNet{
		IP:   net.IP(prefix.Addr().AsSlice()),
		Mask: net.CIDRMask(prefix.Bits(), 32),
	}
}

func prefixFromIPNet(ipNet *net.IPNet) (netip.Prefix, error) {
	addr, ok := netip.AddrFromSlice(ipNet.IP)
	if !ok {
		return netip.Prefix{}, fmt.Errorf("invalid IP %q", ipNet.IP.String())
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return netip.Prefix{}, fmt.Errorf("unsupported mask size %d", bits)
	}

	return netip.PrefixFrom(addr.Unmap(), ones), nil
}
