// Package web builds Gateway API resources without requiring Gateway CRDs in the manager cache.
package web

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	"github.com/wbe7/hermes-operator/internal/network"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"maps"
)

var RouteGVK = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}
var GatewayGVK = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"}

const Port int32 = 9119

func Name(h *v1.Hermes) string { return h.Name + "-hermes-web" }
func Route(h *v1.Hermes) *unstructured.Unstructured {
	r := &unstructured.Unstructured{}
	r.SetGroupVersionKind(RouteGVK)
	r.SetName(Name(h))
	r.SetNamespace(h.Namespace)
	return r
}
func Build(h *v1.Hermes) (*corev1.Service, *unstructured.Unstructured) {
	if !config.WebEnabled(h) {
		return nil, nil
	}
	labels := map[string]string{network.InstallationUIDLabel: string(h.UID)}
	meta := metav1.ObjectMeta{Name: Name(h), Namespace: h.Namespace, Labels: maps.Clone(labels), OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(h, v1.GroupVersion.WithKind("Hermes"))}}
	svc := &corev1.Service{ObjectMeta: meta, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: maps.Clone(labels), Ports: []corev1.ServicePort{{Name: "http", Port: Port, TargetPort: intstr.FromInt32(Port), Protocol: corev1.ProtocolTCP}}}}
	route := Route(h)
	route.SetLabels(maps.Clone(labels))
	route.SetOwnerReferences(meta.OwnerReferences)
	host, prefix, _ := config.WebAddress(h)
	// Replace rather than append trusted metadata; remove client-supplied aliases.
	header := map[string]any{"set": []any{map[string]any{"name": "X-Forwarded-Proto", "value": "https"}, map[string]any{"name": "X-Forwarded-Host", "value": host}}, "remove": []any{"Forwarded", "X-Forwarded-Prefix"}}
	filters := []any{map[string]any{"type": "RequestHeaderModifier", "requestHeaderModifier": header}}
	path := "/"
	if prefix != "" {
		path = prefix
		header["remove"] = []any{"Forwarded"}
		header["set"] = append(header["set"].([]any), map[string]any{"name": "X-Forwarded-Prefix", "value": prefix})
		filters = append(filters, map[string]any{"type": "URLRewrite", "urlRewrite": map[string]any{"path": map[string]any{"type": "ReplacePrefixMatch", "replacePrefixMatch": "/"}}})
	}
	route.Object["spec"] = map[string]any{"parentRefs": []any{map[string]any{"group": GatewayGVK.Group, "kind": "Gateway", "name": h.Spec.Web.GatewayRef.Name, "namespace": config.GatewayNamespace(h), "sectionName": h.Spec.Web.GatewayRef.SectionName}}, "hostnames": []any{host}, "rules": []any{map[string]any{"matches": []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": path}}}, "filters": filters, "timeouts": map[string]any{"request": "0s"}, "backendRefs": []any{map[string]any{"name": svc.Name, "port": int64(Port)}}}}}
	return svc, route
}
func RouteReady(route *unstructured.Unstructured, h *v1.Hermes) bool {
	parents, _, _ := unstructured.NestedSlice(route.Object, "status", "parents")
	for _, raw := range parents {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := p["parentRef"].(map[string]any)
		if !ok {
			continue
		}
		ns, _ := ref["namespace"].(string)
		if ns == "" {
			ns = route.GetNamespace()
		}
		if ref["name"] != h.Spec.Web.GatewayRef.Name || ns != config.GatewayNamespace(h) || ref["sectionName"] != h.Spec.Web.GatewayRef.SectionName {
			continue
		}
		if group, ok := ref["group"]; ok && group != GatewayGVK.Group {
			continue
		}
		if kind, ok := ref["kind"]; ok && kind != "Gateway" {
			continue
		}
		conditions, _, _ := unstructured.NestedSlice(p, "conditions")
		accepted, resolved := false, false
		for _, raw := range conditions {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			gen, _, _ := unstructured.NestedInt64(c, "observedGeneration")
			if c["status"] != "True" || gen != route.GetGeneration() {
				continue
			}
			if c["type"] == "Accepted" {
				accepted = true
			}
			if c["type"] == "ResolvedRefs" {
				resolved = true
			}
		}
		if accepted && resolved {
			return true
		}
	}
	return false
}
