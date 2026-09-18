// Package network compiles isolation policy without contacting Kubernetes or DNS.
package network

import (
	"fmt"
	"maps"
	"net/netip"
	"sort"

	v1alpha1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/settings"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const InstallationUIDLabel = "hermes.wbe7.github.io/installation-uid"

// Build returns the desired policy. Its existence does not attest CNI enforcement.
func Build(h *v1alpha1.Hermes, cluster settings.NetworkConfig) (*networkingv1.NetworkPolicy, error) {
	if h == nil || h.UID == "" {
		return nil, fmt.Errorf("Hermes UID is required for policy isolation")
	}
	if err := cluster.Validate(); err != nil {
		return nil, err
	}
	denied := specialRanges()
	for _, cidrs := range [][]string{cluster.PodCIDRs, cluster.ServiceCIDRs, cluster.NodeCIDRs, cluster.InfrastructureCIDRs, h.Spec.Network.AdditionalBlockedCIDRs} {
		for i, s := range cidrs {
			p, err := netip.ParsePrefix(s)
			if err != nil || p.Addr().Is4In6() {
				return nil, fmt.Errorf("network CIDR at index %d must be IPv4 or native IPv6", i)
			}
			denied = append(denied, p.Masked())
		}
	}
	denied = compact(denied)
	p := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: h.Name + "-hermes", Namespace: h.Namespace,
			Labels:          map[string]string{InstallationUIDLabel: string(h.UID)},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(h, v1alpha1.GroupVersion.WithKind("Hermes"))},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{InstallationUIDLabel: string(h.UID)}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
		},
	}
	for _, root := range []string{"0.0.0.0/0", "2000::/3"} {
		base := netip.MustParsePrefix(root)
		except := []string{}
		blocked := false
		for _, d := range denied {
			if d.Addr().BitLen() != base.Addr().BitLen() {
				continue
			}
			if d.Bits() <= base.Bits() && d.Contains(base.Addr()) {
				blocked = true
				break
			}
			if base.Contains(d.Addr()) {
				except = append(except, d.String())
			}
		}
		if !blocked {
			p.Spec.Egress = append(p.Spec.Egress, networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: root, Except: except}}}})
		}
	}
	dns := networkingv1.NetworkPolicyEgressRule{Ports: []networkingv1.NetworkPolicyPort{port(53, corev1.ProtocolUDP), port(53, corev1.ProtocolTCP)}}
	if s := cluster.DNS.PodSelector; s != nil {
		dns.To = append(dns.To, networkingv1.NetworkPolicyPeer{NamespaceSelector: &metav1.LabelSelector{MatchLabels: maps.Clone(s.NamespaceLabels)}, PodSelector: &metav1.LabelSelector{MatchLabels: maps.Clone(s.PodLabels)}})
	}
	for _, s := range cluster.DNS.ResolverIPs {
		a, _ := settings.ParseUnicastIP(s)
		dns.To = append(dns.To, exactPeer(a))
	}
	p.Spec.Egress = append(p.Spec.Egress, dns)
	for i, e := range h.Spec.Network.AllowPrivate {
		a, err := settings.ParseUnicastIP(e.IP)
		if err != nil {
			return nil, fmt.Errorf("network.allowPrivate[%d].ip must be one unicast IP", i)
		}
		if e.Ports != nil && len(e.Ports) == 0 {
			return nil, fmt.Errorf("network.allowPrivate[%d].ports must be nonempty when supplied", i)
		}
		rule := networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{exactPeer(a)}}
		for j, v := range e.Ports {
			proto := v.Protocol
			if proto == "" {
				proto = corev1.ProtocolTCP
			}
			if v.Port < 1 || v.Port > 65535 || (proto != corev1.ProtocolTCP && proto != corev1.ProtocolUDP && proto != corev1.ProtocolSCTP) {
				return nil, fmt.Errorf("network.allowPrivate[%d].ports[%d] has invalid port or protocol", i, j)
			}
			rule.Ports = append(rule.Ports, port(v.Port, proto))
		}
		p.Spec.Egress = append(p.Spec.Egress, rule)
	}
	return p, nil
}
func exactPeer(a netip.Addr) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: netip.PrefixFrom(a, a.BitLen()).String()}}
}
func port(n int32, proto corev1.Protocol) networkingv1.NetworkPolicyPort {
	p := intstr.FromInt32(n)
	return networkingv1.NetworkPolicyPort{Port: &p, Protocol: &proto}
}

// compact normalizes and removes redundant contained prefixes. Sorting by family
// and prefix length ensures every potential containing prefix precedes its child.
func compact(input []netip.Prefix) []netip.Prefix {
	for i := range input {
		input[i] = input[i].Masked()
	}
	sort.Slice(input, func(i, j int) bool {
		a, b := input[i], input[j]
		if a.Addr().BitLen() != b.Addr().BitLen() {
			return a.Addr().BitLen() < b.Addr().BitLen()
		}
		if a.Bits() != b.Bits() {
			return a.Bits() < b.Bits()
		}
		return a.Addr().Less(b.Addr())
	})
	out := []netip.Prefix{}
	for _, p := range input {
		contained := false
		for _, q := range out {
			if q.Addr().BitLen() == p.Addr().BitLen() && q.Contains(p.Addr()) {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, p)
		}
	}
	return out
}
