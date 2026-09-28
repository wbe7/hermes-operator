package controller

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	webresources "github.com/wbe7/hermes-operator/internal/web"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func enableWeb(h *v1.Hermes) {
	h.Spec.Web = &v1.WebSpec{Enabled: true, Routing: v1.WebRoutingSpec{Mode: "Path", BaseDomain: "agents.example.com"}, GatewayRef: v1.WebGatewayRef{Name: "external", Namespace: "infra", SectionName: "https"}, Network: v1.WebNetworkSpec{IngressFrom: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "infra"}}, PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"gateway": "external"}}}}, TrustedProxyCIDRs: []string{"10.42.0.0/16"}}}
}
func addGateway(t *testing.T, r *HermesReconciler, h *v1.Hermes) {
	t.Helper()
	g := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway", "metadata": map[string]any{"name": "external", "namespace": "infra", "generation": int64(1)}, "spec": map[string]any{"listeners": []any{map[string]any{"name": "https", "protocol": "HTTPS", "port": int64(443), "hostname": "*.example.com", "tls": map[string]any{"mode": "Terminate"}, "allowedRoutes": map[string]any{"namespaces": map[string]any{"from": "All"}}}}}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Programmed", "status": "True", "observedGeneration": int64(1)}, map[string]any{"type": "Accepted", "status": "True", "observedGeneration": int64(1)}}}}}
	if err := r.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
}
func TestWebRouteLifecyclePreservesSource(t *testing.T) {
	r, h := unit(t, true)
	enableWeb(h)
	addGateway(t, r, h)
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.preflightWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.reconcileWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	route := webresources.Route(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(route), route); err != nil {
		t.Fatal(err)
	}
	if h.Status.Web == nil || h.Status.Web.URL != "https://agents.example.com/"+h.Name {
		t.Fatal("missing web status")
	}
	h.Spec.Web.Enabled = false
	if err := r.removeWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(route), route); !apierrors.IsNotFound(err) {
		t.Fatal("route retained")
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}, secret); err != nil || len(secret.Data["WEB_PASSWORD"]) == 0 {
		t.Fatal("source credential removed")
	}
}
func TestMissingGatewayRejectsPublication(t *testing.T) {
	r, h := unit(t, true)
	enableWeb(h)
	if r.preflightWeb(ctx, h) == nil {
		t.Fatal("missing Gateway accepted")
	}
}
func TestInvalidWebChangeRemovesOldPublication(t *testing.T) {
	r, h := unit(t, true)
	enableWeb(h)
	addGateway(t, r, h)
	if err := r.reconcileWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, err := r.saveStatus(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
		t.Fatal(err)
	}
	enableWeb(h)
	h.Spec.Web.Routing.BaseDomain = "invalid/domain"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	route := webresources.Route(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(route), route); !apierrors.IsNotFound(err) {
		t.Fatal("invalid desired config left old publication")
	}
	current := &v1.Hermes{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Web != nil {
		t.Fatal("removed publication retains URL in status")
	}
}

func TestWebPreflightListenerFailures(t *testing.T) {
	for _, mode := range []string{"http", "missing-section", "namespace", "stale", "host"} {
		t.Run(mode, func(t *testing.T) {
			r, h := unit(t, true)
			enableWeb(h)
			addGateway(t, r, h)
			g := &unstructured.Unstructured{}
			g.SetGroupVersionKind(webresources.GatewayGVK)
			if err := r.Get(ctx, client.ObjectKey{Namespace: "infra", Name: "external"}, g); err != nil {
				t.Fatal(err)
			}
			listeners, _, _ := unstructured.NestedSlice(g.Object, "spec", "listeners")
			l := listeners[0].(map[string]any)
			switch mode {
			case "http":
				l["protocol"] = "HTTP"
			case "missing-section":
				l["name"] = "other"
			case "namespace":
				l["allowedRoutes"] = map[string]any{"namespaces": map[string]any{"from": "Same"}}
			case "stale":
				g.SetGeneration(2)
			case "host":
				l["hostname"] = "other.example.com"
			}
			_ = unstructured.SetNestedSlice(g.Object, listeners, "spec", "listeners")
			if err := r.Update(ctx, g); err != nil {
				t.Fatal(err)
			}
			if err := r.preflightWeb(ctx, h); err == nil {
				t.Fatal("unsafe/unready listener accepted")
			}
		})
	}
}

func TestWebAddressConflictAndForeignCleanup(t *testing.T) {
	r, h := unit(t, true)
	enableWeb(h)
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.reconcileWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	other := h.DeepCopy()
	other.Name = "other"
	other.UID = "other-uid"
	other.ResourceVersion = ""
	other.Spec.Web.Routing.Name = h.Name
	if err := r.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := r.checkWebAddress(ctx, other); err == nil {
		t.Fatal("duplicate URL accepted")
	}
	// A foreign object with our derived name must never be deleted.
	route := webresources.Route(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(route), route); err != nil {
		t.Fatal(err)
	}
	route.SetOwnerReferences(nil)
	if err := r.Update(ctx, route); err != nil {
		t.Fatal(err)
	}
	if err := r.removeWeb(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(route), route); err != nil {
		t.Fatal("foreign route removed")
	}
}
