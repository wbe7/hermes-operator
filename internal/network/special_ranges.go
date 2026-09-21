package network

import "net/netip"

// IANA snapshots retrieved 2026-09-18, preserved byte-for-byte in testdata:
// https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry-1.csv
// https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry-1.csv
//
// Non-global, deprecated and transition ranges are denied. IPv4 multicast is
// added separately (outside the special-purpose registry). IPv6 is limited to
// 2000::/3 by the public rule: NAT64 (including globally reachable 64:ff9b::/96),
// mapped IPv4, ULA, link-local, multicast and unallocated space cannot bypass it.
// The True global entries nested in denied aggregates are carved here BEFORE
// installation/CR blocks are added. Thus an administrator's explicit deny still
// wins over an IANA carve. TestSnapshotClassification detects registry drift.
func specialRanges() []netip.Prefix {
	denied := prefixes([]string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20",
	})
	global := prefixes([]string{"192.0.0.9/32", "192.0.0.10/32", "2001:1::1/128", "2001:1::2/128", "2001:1::3/128", "2001:3::/32", "2001:4:112::/48", "2001:20::/28", "2001:30::/28"})
	for _, g := range global {
		next := []netip.Prefix{}
		for _, d := range denied {
			next = append(next, subtract(d, g)...)
		}
		denied = next
	}
	return denied
}
func prefixes(values []string) []netip.Prefix {
	out := make([]netip.Prefix, len(values))
	for i, v := range values {
		out[i] = netip.MustParsePrefix(v)
	}
	return out
}

// subtract removes a nested prefix by splitting only its path through the binary
// prefix tree. Addresses are never converted across IP families.
func subtract(base, remove netip.Prefix) []netip.Prefix {
	if base.Addr().BitLen() != remove.Addr().BitLen() || !base.Overlaps(remove) {
		return []netip.Prefix{base}
	}
	if remove.Bits() <= base.Bits() {
		return nil
	}
	bits := base.Bits() + 1
	left := netip.PrefixFrom(base.Addr(), bits)
	raw := base.Addr().As16()
	offset := 0
	if base.Addr().Is4() {
		offset = 12
	}
	raw[offset+(bits-1)/8] |= 1 << uint(7-(bits-1)%8)
	addr := netip.AddrFrom16(raw)
	if base.Addr().Is4() {
		addr = addr.Unmap()
	}
	right := netip.PrefixFrom(addr, bits)
	return append(subtract(left, remove), subtract(right, remove)...)
}
