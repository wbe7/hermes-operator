// Package settings validates the operator's installation-specific configuration.
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type DNSPodSelector struct {
	NamespaceLabels map[string]string `json:"namespaceLabels"`
	PodLabels       map[string]string `json:"podLabels"`
}
type DNSConfig struct {
	PodSelector *DNSPodSelector `json:"podSelector,omitempty"`
	ResolverIPs []string        `json:"resolverIPs,omitempty"`
}
type NetworkConfig struct {
	EnforcementConfirmed bool      `json:"enforcementConfirmed"`
	PodCIDRs             []string  `json:"podCIDRs"`
	ServiceCIDRs         []string  `json:"serviceCIDRs"`
	NodeCIDRs            []string  `json:"nodeCIDRs"`
	InfrastructureCIDRs  []string  `json:"infrastructureCIDRs"`
	DNS                  DNSConfig `json:"dns"`
}

// DecodeNetworkConfig accepts the networkPolicy object, in YAML or JSON.
// Nil slices preserve the distinction between missing/null and explicit [].
func DecodeNetworkConfig(data []byte) (NetworkConfig, error) {
	var c NetworkConfig
	// Reject streams: UnmarshalStrict alone silently accepts their first document.
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var first, extra json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return c, fmt.Errorf("decode network config: %w", err)
	}
	if err := dec.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("network config must contain exactly one document")
	}
	if err := yaml.UnmarshalStrict(data, &c); err != nil {
		return c, fmt.Errorf("decode network config: %w", err)
	}
	return c, c.Validate()
}
func (c NetworkConfig) Validate() error {
	if !c.EnforcementConfirmed {
		return fmt.Errorf("enforcementConfirmed must be true")
	}
	for _, group := range []struct {
		name  string
		cidrs []string
		empty bool
	}{{"podCIDRs", c.PodCIDRs, false}, {"serviceCIDRs", c.ServiceCIDRs, false}, {"nodeCIDRs", c.NodeCIDRs, false}, {"infrastructureCIDRs", c.InfrastructureCIDRs, true}} {
		if group.cidrs == nil {
			return fmt.Errorf("%s must be explicitly supplied", group.name)
		}
		if !group.empty && len(group.cidrs) == 0 {
			return fmt.Errorf("%s must be nonempty", group.name)
		}
		for i, s := range group.cidrs {
			p, err := netip.ParsePrefix(s)
			if err != nil || p.Addr().Is4In6() {
				return fmt.Errorf("%s[%d] must be an IPv4 or native IPv6 CIDR", group.name, i)
			}
		}
	}
	if c.DNS.PodSelector == nil && len(c.DNS.ResolverIPs) == 0 {
		return fmt.Errorf("dns must select resolver Pods or exact resolver IPs")
	}
	if s := c.DNS.PodSelector; s != nil {
		for _, group := range []struct {
			name   string
			labels map[string]string
		}{{"namespaceLabels", s.NamespaceLabels}, {"podLabels", s.PodLabels}} {
			if len(group.labels) == 0 {
				return fmt.Errorf("dns.podSelector.%s must be nonempty", group.name)
			}
			selector := &metav1.LabelSelector{MatchLabels: group.labels}
			if len(metav1validation.ValidateLabelSelector(selector, metav1validation.LabelSelectorValidationOptions{}, field.NewPath("dns", "podSelector", group.name))) != 0 {
				return fmt.Errorf("dns.podSelector.%s contains an invalid label", group.name)
			}
		}
	}
	for i, s := range c.DNS.ResolverIPs {
		if _, err := ParseUnicastIP(s); err != nil {
			return fmt.Errorf("dns.resolverIPs[%d] must be a single unicast IP", i)
		}
	}
	return nil
}

// ParseUnicastIP validates exact destinations. Link-local unicast is valid for
// explicit metadata or NodeLocal DNS exceptions. Mapped IPv4 normalizes to /32.
func ParseUnicastIP(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("invalid IP")
	}
	a = a.Unmap()
	if a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a == netip.MustParseAddr("255.255.255.255") {
		return netip.Addr{}, fmt.Errorf("IP must be unicast")
	}
	return a, nil
}
