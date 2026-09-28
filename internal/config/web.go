package config

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/netip"
	"strings"
)

func WebEnabled(h *v1.Hermes) bool { return h.Spec.Web != nil && h.Spec.Web.Enabled }
func PrimarySecretName(h *v1.Hermes) string {
	if h.Spec.Credentials.SecretName != "" {
		return h.Spec.Credentials.SecretName
	}
	return h.Name + "-hermes-secret"
}
func WebAddress(h *v1.Hermes) (host, prefix, url string) {
	if !WebEnabled(h) {
		return
	}
	r := h.Spec.Web.Routing
	name := r.Name
	if name == "" {
		name = h.Name
	}
	host = r.BaseDomain
	if r.Mode == "Subdomain" {
		host = name + "." + host
	} else {
		prefix = "/" + name
	}
	return host, prefix, "https://" + host + prefix
}
func GatewayNamespace(h *v1.Hermes) string {
	if h.Spec.Web.GatewayRef.Namespace != "" {
		return h.Spec.Web.GatewayRef.Namespace
	}
	return h.Namespace
}
func boundedSelector(s *metav1.LabelSelector) bool {
	if s == nil || (len(s.MatchLabels) == 0 && len(s.MatchExpressions) == 0) {
		return false
	}
	_, err := metav1.LabelSelectorAsSelector(s)
	return err == nil
}
func ValidateWeb(h *v1.Hermes) error {
	if !WebEnabled(h) {
		return nil
	}
	w := h.Spec.Web
	if w.Routing.Mode != "Subdomain" && w.Routing.Mode != "Path" {
		return invalid("spec.web.routing.mode", "must be Subdomain or Path")
	}
	name := w.Routing.Name
	if name == "" {
		name = h.Name
	}
	host, _, _ := WebAddress(h)
	if len(validation.IsDNS1123Label(name)) != 0 || len(validation.IsDNS1123Subdomain(w.Routing.BaseDomain)) != 0 || len(validation.IsDNS1123Subdomain(host)) != 0 || !strings.Contains(w.Routing.BaseDomain, ".") {
		return invalid("spec.web.routing", "requires a valid DNS hostname and name")
	}
	if len(validation.IsDNS1123Subdomain(w.GatewayRef.Name)) != 0 || len(validation.IsDNS1123Label(GatewayNamespace(h))) != 0 || len(validation.IsDNS1123Label(w.GatewayRef.SectionName)) != 0 {
		return invalid("spec.web.gatewayRef", "requires Gateway name, namespace and HTTPS listener")
	}
	if len(w.Auth.Username) > 128 || strings.ContainsAny(w.Auth.Username, "\x00\r\n") {
		return invalid("spec.web.auth.username", "invalid username")
	}
	n := w.Network
	if len(n.IngressFrom) == 0 || len(n.IngressFrom) > 32 || len(n.TrustedProxyCIDRs) == 0 || len(n.TrustedProxyCIDRs) > 32 {
		return invalid("spec.web.network", "requires bounded ingress sources and trusted proxies")
	}
	for _, peer := range n.IngressFrom {
		if peer.IPBlock != nil {
			p, err := netip.ParsePrefix(peer.IPBlock.CIDR)
			if err != nil || p.Bits() == 0 || p.Addr().Is4In6() || peer.NamespaceSelector != nil || peer.PodSelector != nil || len(peer.IPBlock.Except) > 0 {
				return invalid("spec.web.network.ingressFrom", "requires one bounded CIDR or namespace and pod selectors")
			}
		} else if !boundedSelector(peer.NamespaceSelector) || !boundedSelector(peer.PodSelector) {
			return invalid("spec.web.network.ingressFrom", "requires nonempty namespace and pod selectors")
		}
	}
	for _, raw := range n.TrustedProxyCIDRs {
		p, err := netip.ParsePrefix(raw)
		if err != nil || p.Bits() == 0 || p.Addr().Is4In6() {
			return invalid("spec.web.network.trustedProxyCIDRs", "requires bounded native IPv4/IPv6 CIDRs")
		}
	}
	return nil
}
func renderWeb(h *v1.Hermes, doc *startupInput) {
	// Clear all competing persisted native auth settings, including when disabled.
	for _, k := range []string{"HERMES_DASHBOARD_PUBLIC_URL", "HERMES_DASHBOARD_BASIC_AUTH_USERNAME", "HERMES_DASHBOARD_BASIC_AUTH_PASSWORD_HASH", "HERMES_DASHBOARD_SESSION_TOKEN"} {
		doc.Env[k] = ""
	}
	for _, path := range []string{"dashboard.basic_auth.username", "dashboard.basic_auth.password", "dashboard.basic_auth.password_hash", "dashboard.basic_auth.secret", "dashboard.public_url"} {
		set(doc.Config, path, "")
	}
	set(doc.Config, "dashboard.trusted_proxies", []string{})
	if !WebEnabled(h) {
		doc.Env["HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"] = ""
		doc.Env["HERMES_DASHBOARD_BASIC_AUTH_SECRET"] = ""
		return
	}
	username := h.Spec.Web.Auth.Username
	if username == "" {
		username = "admin"
	}
	_, _, url := WebAddress(h)
	doc.Env["HERMES_DASHBOARD_PUBLIC_URL"] = url
	doc.Env["HERMES_DASHBOARD_BASIC_AUTH_USERNAME"] = username
	set(doc.Config, "dashboard.public_url", url)
	set(doc.Config, "dashboard.basic_auth.username", username)
	set(doc.Config, "dashboard.basic_auth.password", doc.Env["HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"])
	set(doc.Config, "dashboard.basic_auth.secret", doc.Env["HERMES_DASHBOARD_BASIC_AUTH_SECRET"])
	set(doc.Config, "dashboard.trusted_proxies", h.Spec.Web.Network.TrustedProxyCIDRs)
}
