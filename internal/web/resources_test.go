package web

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/testfixtures"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func TestBuildRoutes(t *testing.T) {
	for _, mode := range []string{"Subdomain", "Path"} {
		t.Run(mode, func(t *testing.T) {
			h := testfixtures.Hermes(t)
			h.Spec.Web = &v1.WebSpec{Enabled: true, Routing: v1.WebRoutingSpec{Mode: mode, BaseDomain: "agents.example.com", Name: "one"}, GatewayRef: v1.WebGatewayRef{Name: "external", Namespace: "infra", SectionName: "https"}}
			svc, route := Build(h)
			if svc.Spec.Ports[0].Port != 9119 || svc.Spec.Selector["hermes.wbe7.github.io/installation-uid"] != string(h.UID) {
				t.Fatal("incorrect backend")
			}
			hosts, _, _ := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
			want := "agents.example.com"
			if mode == "Subdomain" {
				want = "one." + want
			}
			if len(hosts) != 1 || hosts[0] != want {
				t.Fatal(hosts)
			}
			rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
			rule := rules[0].(map[string]any)
			filters := rule["filters"].([]any)
			if mode == "Path" && len(filters) != 2 {
				t.Fatal("prefix rewrite missing")
			}
			header := filters[0].(map[string]any)["requestHeaderModifier"].(map[string]any)
			if len(header["set"].([]any)) == 0 {
				t.Fatal("trusted metadata not set")
			}
			parent := map[string]any{"name": "external", "namespace": "infra", "sectionName": "https"}
			route.SetGeneration(4)
			condition := func(typ string, generation int64) map[string]any {
				return map[string]any{"type": typ, "status": "True", "observedGeneration": generation}
			}
			set := func(gen int64) {
				_ = unstructured.SetNestedSlice(route.Object, []any{map[string]any{"parentRef": parent, "conditions": []any{condition("Accepted", gen), condition("ResolvedRefs", gen)}}}, "status", "parents")
			}
			set(3)
			if RouteReady(route, h) {
				t.Fatal("stale status accepted")
			}
			set(4)
			if !RouteReady(route, h) {
				t.Fatal("current parent status rejected")
			}
			parent["sectionName"] = "wrong"
			set(4)
			if RouteReady(route, h) {
				t.Fatal("wrong listener accepted")
			}
		})
	}
}
