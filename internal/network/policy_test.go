package network

import (
	"encoding/csv"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"

	v1alpha1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/settings"
	"github.com/wbe7/hermes-operator/internal/testfixtures"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func clusterConfig() settings.NetworkConfig {
	return settings.NetworkConfig{EnforcementConfirmed: true, PodCIDRs: []string{"10.244.0.1/16", "2600:100::/40"}, ServiceCIDRs: []string{"10.96.0.0/12"}, NodeCIDRs: []string{"192.168.1.0/24"}, InfrastructureCIDRs: []string{"8.8.4.0/24"}, DNS: settings.DNSConfig{PodSelector: &settings.DNSPodSelector{NamespaceLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"}, PodLabels: map[string]string{"k8s-app": "kube-dns"}}, ResolverIPs: []string{"10.96.0.10"}}}
}

// Matches only numeric peers: selectors are verified separately and cannot make
// an arbitrary external destination pass this address matrix.
func allows(p *networkingv1.NetworkPolicy, ip string, port int32, proto corev1.Protocol) bool {
	a := netip.MustParseAddr(ip)
	for _, r := range p.Spec.Egress {
		ports := len(r.Ports) == 0
		for _, v := range r.Ports {
			if v.Protocol != nil && *v.Protocol == proto && v.Port != nil && v.Port.IntVal == port {
				ports = true
			}
		}
		if !ports {
			continue
		}
		for _, peer := range r.To {
			if peer.IPBlock == nil {
				continue
			}
			b := peer.IPBlock
			match := netip.MustParsePrefix(b.CIDR).Contains(a)
			for _, e := range b.Except {
				if netip.MustParsePrefix(e).Contains(a) {
					match = false
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
func TestAddressMatrix(t *testing.T) {
	h := testfixtures.Hermes(t)
	h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: "10.20.30.40", Ports: []v1alpha1.NetworkPort{{Port: 8000}}}, {IP: "fd00::7", Ports: []v1alpha1.NetworkPort{{Port: 9999, Protocol: corev1.ProtocolUDP}}}}
	h.Spec.Network.AdditionalBlockedCIDRs = []string{"9.9.9.0/24"}
	p, err := Build(h, clusterConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ip    string
		port  int32
		proto corev1.Protocol
		want  bool
	}{
		{"8.8.8.8", 443, "TCP", true}, {"8.8.8.8", 999, "UDP", true}, {"2606:4700:4700::1111", 443, "TCP", true},
		{"10.20.30.40", 8000, "TCP", true}, {"10.20.30.40", 8001, "TCP", false}, {"10.20.30.40", 8000, "UDP", false}, {"10.20.30.41", 8000, "TCP", false},
		{"fd00::7", 9999, "UDP", true}, {"fd00::8", 9999, "UDP", false}, {"fd00::7", 9999, "TCP", false},
		{"10.96.0.10", 53, "UDP", true}, {"10.96.0.10", 53, "TCP", true}, {"10.96.0.10", 54, "TCP", false}, {"10.96.0.10", 53, "SCTP", false}, {"10.96.0.11", 53, "UDP", false},
	} {
		if got := allows(p, tc.ip, tc.port, tc.proto); got != tc.want {
			t.Errorf("%s:%d/%s got %v", tc.ip, tc.port, tc.proto, got)
		}
	}
	for _, ip := range []string{"0.1.2.3", "10.1.1.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.1.1", "192.168.2.1", "192.0.0.8", "192.88.99.2", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "8.8.4.1", "9.9.9.9", "2600:100::1", "::", "::1", "fc00::1", "fe80::1", "ff02::1", "64:ff9b::808:808", "64:ff9b:1::1", "::ffff:8.8.8.8", "2001::1", "2001:2::1", "2001:10::1", "2001:db8::1", "2002:808:808::1", "3fff::1"} {
		if allows(p, ip, 443, "TCP") {
			t.Errorf("must deny %s", ip)
		}
	}
	for _, ip := range []string{"192.0.0.9", "192.0.0.10", "192.31.196.1", "192.52.193.1", "192.175.48.1", "2001:1::1", "2001:1::2", "2001:1::3", "2001:3::1", "2001:4:112::1", "2001:20::1", "2001:30::1", "2620:4f:8000::1"} {
		if !allows(p, ip, 443, "TCP") {
			t.Errorf("global exception denied %s", ip)
		}
	}
	if len(p.Spec.Ingress) != 0 || !reflect.DeepEqual(p.Spec.PolicyTypes, []networkingv1.PolicyType{"Ingress", "Egress"}) {
		t.Fatal("missing default isolation")
	}
	if p.Spec.PodSelector.MatchLabels[InstallationUIDLabel] != string(h.UID) {
		t.Fatal("UID selector missing")
	}
	for _, r := range p.Spec.Egress {
		for _, peer := range r.To {
			if peer.IPBlock != nil {
				base := netip.MustParsePrefix(peer.IPBlock.CIDR)
				for _, s := range peer.IPBlock.Except {
					ex := netip.MustParsePrefix(s)
					if base.Addr().BitLen() != ex.Addr().BitLen() || base.Bits() >= ex.Bits() || !base.Contains(ex.Addr()) {
						t.Errorf("invalid except %s in %s", s, base)
					}
				}
			} else {
				if peer.PodSelector == nil || peer.NamespaceSelector == nil || len(r.Ports) != 2 {
					t.Fatal("broad DNS peer")
				}
			}
		}
	}
}
func TestInputValidation(t *testing.T) {
	for _, ip := range []string{"0.0.0.0", "127.0.0.1", "224.0.0.1", "::", "::1", "ff02::1", "10.1.1.1/32", "example.org", "fe80::1%eth0", "::ffff:10.0.0.1%eth0", "::ffff:127.0.0.1", "255.255.255.255"} {
		h := testfixtures.Hermes(t)
		h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: ip}}
		if _, err := Build(h, clusterConfig()); err == nil {
			t.Errorf("accepted %s", ip)
		}
	}
	for _, port := range []v1alpha1.NetworkPort{{Port: 0}, {Port: 65536}, {Port: 53, Protocol: "ICMP"}} {
		h := testfixtures.Hermes(t)
		h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: "10.1.1.1", Ports: []v1alpha1.NetworkPort{port}}}
		if _, err := Build(h, clusterConfig()); err == nil {
			t.Errorf("accepted port %+v", port)
		}
	}
	h := testfixtures.Hermes(t)
	h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: "10.1.1.1", Ports: []v1alpha1.NetworkPort{}}}
	if _, err := Build(h, clusterConfig()); err == nil {
		t.Error("empty explicit ports accepted")
	}
	h.Spec.Network.AllowPrivate = nil
	h.Spec.Network.AdditionalBlockedCIDRs = []string{"bad"}
	if _, err := Build(h, clusterConfig()); err == nil {
		t.Error("invalid blocked CIDR accepted")
	}
}
func TestOverlappingAndUniversalBlocks(t *testing.T) {
	h := testfixtures.Hermes(t)
	h.Spec.Network.AdditionalBlockedCIDRs = []string{"0.0.0.0/0", "2000::/3"}
	h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: "10.1.1.1"}}
	p, err := Build(h, clusterConfig())
	if err != nil {
		t.Fatal(err)
	}
	if allows(p, "8.8.8.8", 443, "TCP") || allows(p, "2001:1::1", 443, "TCP") {
		t.Fatal("universal block bypass")
	}
	if !allows(p, "10.1.1.1", 1234, "SCTP") {
		t.Fatal("explicit exception lost")
	}
	h.Spec.Network.AdditionalBlockedCIDRs = []string{"192.0.0.9/32", "2001:20::/28"}
	p, err = Build(h, clusterConfig())
	if err != nil {
		t.Fatal(err)
	}
	if allows(p, "192.0.0.9", 443, "TCP") || allows(p, "2001:20::1", 443, "TCP") {
		t.Fatal("global carve bypassed configured block")
	}
}

// Every saved IANA range is checked at both boundaries, resolving nested rows by
// longest prefix. Unknown/deprecated reachability is denied; native global IPv6
// is the explicit supported family boundary even for globally reachable NAT64.
func TestSnapshotClassification(t *testing.T) {
	type entry struct {
		prefix netip.Prefix
		global bool
	}
	var registry []entry
	for _, name := range []string{"testdata/iana-ipv4.csv", "testdata/iana-ipv6.csv"} {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(f).ReadAll()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows[1:] {
			for _, block := range strings.Split(row[0], ",") {
				block = strings.Fields(block)[0]
				registry = append(registry, entry{netip.MustParsePrefix(block), row[8] == "True"})
			}
		}
	}
	p, err := Build(testfixtures.Hermes(t), clusterConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range registry {
		last := e.prefix.Addr().As16()
		offset := 0
		if e.prefix.Addr().Is4() {
			offset = 12
		}
		for bit := e.prefix.Bits(); bit < e.prefix.Addr().BitLen(); bit++ {
			last[offset+bit/8] |= 1 << uint(7-bit%8)
		}
		end := netip.AddrFrom16(last)
		if e.prefix.Addr().Is4() {
			end = end.Unmap()
		}
		for _, a := range []netip.Addr{e.prefix.Addr(), end} {
			want := false
			longest := -1
			for _, r := range registry {
				if r.prefix.Contains(a) && r.prefix.Bits() > longest {
					want = r.global
					longest = r.prefix.Bits()
				}
			}
			if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
				want = false
			}
			if got := allows(p, a.String(), 443, "TCP"); got != want {
				t.Errorf("registry %s boundary %s: got %v want %v", e.prefix, a, got, want)
			}
		}
	}
}
func TestPolicyIdentityAndInputOwnership(t *testing.T) {
	h := testfixtures.Hermes(t)
	c := clusterConfig()
	before := h.DeepCopy()
	p, err := Build(h, c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != h.Name+"-hermes" || p.Namespace != h.Namespace || len(p.OwnerReferences) != 1 || p.OwnerReferences[0].UID != h.UID || !(*p.OwnerReferences[0].Controller) {
		t.Fatal("incorrect policy identity")
	}
	second, err := Build(h, c)
	if err != nil || !reflect.DeepEqual(p, second) {
		t.Fatal("non-deterministic build")
	}
	if !reflect.DeepEqual(h, before) {
		t.Fatal("mutated CR")
	}
	for i := range p.Spec.Egress {
		for j := range p.Spec.Egress[i].To {
			peer := &p.Spec.Egress[i].To[j]
			if peer.PodSelector != nil {
				peer.PodSelector.MatchLabels["k8s-app"] = "changed"
			}
		}
	}
	if c.DNS.PodSelector.PodLabels["k8s-app"] != "kube-dns" {
		t.Fatal("aliased input selectors")
	}
	h.UID = ""
	if _, err := Build(h, c); err == nil {
		t.Fatal("empty UID accepted")
	}
	if _, err := Build(nil, c); err == nil {
		t.Fatal("nil Hermes accepted")
	}
	c.EnforcementConfirmed = false
	if _, err := Build(before, c); err == nil {
		t.Fatal("invalid cluster settings accepted")
	}
}
func TestInstallerBlocksAndPrefixNormalization(t *testing.T) {
	c := clusterConfig()
	c.InfrastructureCIDRs = []string{"192.0.0.9/32", "2001:20::1/28", "8.8.8.8/24", "8.8.8.0/25", "8.8.8.8/24"}
	p, err := Build(testfixtures.Hermes(t), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"192.0.0.9", "2001:20::1", "8.8.8.1", "8.8.8.255"} {
		if allows(p, ip, 443, "TCP") {
			t.Errorf("infrastructure bypass %s", ip)
		}
	}
	for _, r := range p.Spec.Egress {
		for _, peer := range r.To {
			if peer.IPBlock == nil {
				continue
			}
			for i, s := range peer.IPBlock.Except {
				prefix := netip.MustParsePrefix(s)
				if prefix != prefix.Masked() {
					t.Fatal("unnormalized prefix")
				}
				for j, other := range peer.IPBlock.Except {
					if i != j && prefix.Overlaps(netip.MustParsePrefix(other)) {
						t.Fatal("overlapping exclusions")
					}
				}
			}
		}
	}
}

func TestDNSPodSelection(t *testing.T) {
	c := clusterConfig()
	c.DNS.ResolverIPs = nil
	p, err := Build(testfixtures.Hermes(t), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		namespace, pod string
		port           int32
		proto          corev1.Protocol
		want           bool
	}{
		{"kube-system", "kube-dns", 53, "TCP", true}, {"kube-system", "kube-dns", 53, "UDP", true},
		{"kube-system", "kube-dns", 54, "TCP", false}, {"kube-system", "kube-dns", 53, "SCTP", false},
		{"tenant", "kube-dns", 53, "UDP", false}, {"kube-system", "other", 53, "UDP", false},
	} {
		allowed := false
		for _, r := range p.Spec.Egress {
			for _, peer := range r.To {
				if peer.NamespaceSelector == nil || peer.PodSelector == nil {
					continue
				}
				ns, err := metav1.LabelSelectorAsSelector(peer.NamespaceSelector)
				if err != nil {
					t.Fatal(err)
				}
				pods, err := metav1.LabelSelectorAsSelector(peer.PodSelector)
				if err != nil {
					t.Fatal(err)
				}
				if !ns.Matches(labels.Set{"kubernetes.io/metadata.name": tc.namespace}) || !pods.Matches(labels.Set{"k8s-app": tc.pod}) {
					continue
				}
				for _, port := range r.Ports {
					if *port.Protocol == tc.proto && port.Port.IntVal == tc.port {
						allowed = true
					}
				}
			}
		}
		if allowed != tc.want {
			t.Errorf("namespace=%s pod=%s port=%d proto=%s: got %v", tc.namespace, tc.pod, tc.port, tc.proto, allowed)
		}
	}
}
func TestMappedExplicitExceptionNormalization(t *testing.T) {
	h := testfixtures.Hermes(t)
	h.Spec.Network.AllowPrivate = []v1alpha1.PrivateNetworkException{{IP: "::ffff:10.2.3.4", Ports: []v1alpha1.NetworkPort{{Port: 7777, Protocol: "SCTP"}}}}
	c := clusterConfig()
	c.DNS.ResolverIPs = []string{"fd00::53"}
	p, err := Build(h, c)
	if err != nil {
		t.Fatal(err)
	}
	if !allows(p, "10.2.3.4", 7777, "SCTP") || allows(p, "10.2.3.5", 7777, "SCTP") || allows(p, "10.2.3.4", 7777, "TCP") {
		t.Fatal("mapped IPv4 exception is not exact")
	}
	if !allows(p, "fd00::53", 53, "UDP") || !allows(p, "fd00::53", 53, "TCP") || allows(p, "fd00::53", 54, "TCP") || allows(p, "fd00::54", 53, "UDP") {
		t.Fatal("IPv6 DNS is not exact")
	}
}
