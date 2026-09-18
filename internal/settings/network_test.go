package settings

import (
	"strings"
	"testing"
)

const configYAML = `enforcementConfirmed: true
podCIDRs: [10.244.0.0/16]
serviceCIDRs: [10.96.0.0/12]
nodeCIDRs: [192.168.1.0/24]
infrastructureCIDRs: []
dns:
  podSelector:
    namespaceLabels: {kubernetes.io/metadata.name: kube-system}
    podLabels: {k8s-app: kube-dns}
  resolverIPs: [10.96.0.10]
`

func TestDecodeNetworkConfig(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"valid", configYAML, true},
		{"omitted infrastructure", strings.ReplaceAll(configYAML, "infrastructureCIDRs: []\n", ""), false},
		{"null infrastructure", strings.ReplaceAll(configYAML, "infrastructureCIDRs: []", "infrastructureCIDRs: null"), false},
		{"empty pods", strings.ReplaceAll(configYAML, "[10.244.0.0/16]", "[]"), false},
		{"invalid cidr", strings.ReplaceAll(configYAML, "10.244.0.0/16", "oops"), false},
		{"unconfirmed", strings.ReplaceAll(configYAML, "Confirmed: true", "Confirmed: false"), false},
		{"unknown", configYAML + "typo: true\n", false},
		{"multiple documents", configYAML + "---\ntypo: true\n", false},
		{"trailing JSON", `{"enforcementConfirmed":true,"podCIDRs":["10.0.0.0/8"],"serviceCIDRs":["10.96.0.0/12"],"nodeCIDRs":["192.168.1.0/24"],"infrastructureCIDRs":[],"dns":{"resolverIPs":["10.96.0.10"]}} {}`, false},
		{"duplicate", configYAML + "enforcementConfirmed: true\n", false},
		{"empty dns", strings.Split(configYAML, "dns:")[0] + "dns: {}\n", false},
		{"namespace only", strings.ReplaceAll(configYAML, "    podLabels: {k8s-app: kube-dns}\n", ""), false},
		{"invalid label", strings.ReplaceAll(configYAML, "k8s-app", "bad key"), false},
		{"bad resolver", strings.ReplaceAll(configYAML, "10.96.0.10", "127.0.0.1"), false},
		{"resolver cidr", strings.ReplaceAll(configYAML, "10.96.0.10", "10.96.0.10/32"), false},
		{"resolver only", strings.Split(configYAML, "dns:")[0] + "dns: {resolverIPs: [169.254.20.10]}\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeNetworkConfig([]byte(tc.text))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
