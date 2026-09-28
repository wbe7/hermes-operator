package controller

import (
	"context"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	webresources "github.com/wbe7/hermes-operator/internal/web"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func webAbsent(err error) bool {
	return apierrors.IsNotFound(err) || meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}
func (r *HermesReconciler) removeWeb(ctx context.Context, h *v1.Hermes) error {
	for _, obj := range []client.Object{webresources.Route(h), &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: h.Namespace, Name: webresources.Name(h)}}} {
		if err := r.reader().Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if webAbsent(err) {
				continue
			}
			return apiFailure("read web publication for removal", err)
		}
		if !owned(obj, h) {
			continue
		}
		if err := r.deleteUID(ctx, obj); err != nil {
			return err
		}
	}
	h.Status.Web = nil
	return nil
}
func conditionCurrent(obj map[string]any, typ string, generation int64) bool {
	conditions, _, _ := unstructured.NestedSlice(obj, "conditions")
	for _, raw := range conditions {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		gen, _, _ := unstructured.NestedInt64(c, "observedGeneration")
		if c["type"] == typ && c["status"] == "True" && gen == generation {
			return true
		}
	}
	return false
}
func (r *HermesReconciler) preflightWeb(ctx context.Context, h *v1.Hermes) error {
	if !config.WebEnabled(h) {
		return nil
	}
	gateway := &unstructured.Unstructured{}
	gateway.SetGroupVersionKind(webresources.GatewayGVK)
	if err := r.reader().Get(ctx, client.ObjectKey{Namespace: config.GatewayNamespace(h), Name: h.Spec.Web.GatewayRef.Name}, gateway); err != nil {
		if webAbsent(err) {
			return problem("GatewayUnavailable", "Gateway API or the referenced Gateway is unavailable")
		}
		return apiFailure("read Gateway", err)
	}
	if !gateway.GetDeletionTimestamp().IsZero() {
		return problem("GatewayUnavailable", "referenced Gateway is terminating")
	}
	host, _, _ := config.WebAddress(h)
	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	found := false
	for _, raw := range listeners {
		listener, ok := raw.(map[string]any)
		if !ok || listener["name"] != h.Spec.Web.GatewayRef.SectionName {
			continue
		}
		found = true
		port, _, _ := unstructured.NestedInt64(listener, "port")
		tlsMode, _, _ := unstructured.NestedString(listener, "tls", "mode")
		if listener["protocol"] != "HTTPS" || port != 443 || (tlsMode != "" && tlsMode != "Terminate") {
			return problem("InvalidGatewayListener", "web requires a terminating HTTPS listener on port 443")
		}
		pattern, _, _ := unstructured.NestedString(listener, "hostname")
		if pattern != "" && pattern != host && !(strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, pattern[1:])) {
			return problem("GatewayHostnameMismatch", "listener does not accept the declared web hostname")
		}
		from, _, _ := unstructured.NestedString(listener, "allowedRoutes", "namespaces", "from")
		switch from {
		case "All":
		case "Selector":
			raw, ok, _ := unstructured.NestedMap(listener, "allowedRoutes", "namespaces", "selector")
			if !ok {
				return problem("GatewayAttachmentDenied", "listener namespace selector is missing")
			}
			selector := &metav1.LabelSelector{}
			if runtime.DefaultUnstructuredConverter.FromUnstructured(raw, selector) != nil {
				return problem("GatewayAttachmentDenied", "listener namespace selector is invalid")
			}
			sel, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				return problem("GatewayAttachmentDenied", "listener namespace selector is invalid")
			}
			ns := &corev1.Namespace{}
			if err = r.reader().Get(ctx, client.ObjectKey{Name: h.Namespace}, ns); err != nil {
				return apiFailure("read route namespace", err)
			}
			if !sel.Matches(labels.Set(ns.Labels)) {
				return problem("GatewayAttachmentDenied", "namespace does not match listener allowedRoutes")
			}
		case "", "Same":
			if h.Namespace != gateway.GetNamespace() {
				return problem("GatewayAttachmentDenied", "listener permits only its own namespace")
			}
		default:
			return problem("GatewayAttachmentDenied", "unsupported listener namespace policy")
		}
		kinds, _, _ := unstructured.NestedSlice(listener, "allowedRoutes", "kinds")
		if len(kinds) > 0 {
			allowed := false
			for _, raw := range kinds {
				k, ok := raw.(map[string]any)
				if ok && k["kind"] == "HTTPRoute" && (k["group"] == nil || k["group"] == webresources.RouteGVK.Group) {
					allowed = true
				}
			}
			if !allowed {
				return problem("GatewayAttachmentDenied", "listener does not permit HTTPRoute")
			}
		}
	}
	if !found {
		return problem("GatewayListenerMissing", "referenced HTTPS listener is missing")
	}
	status, _, _ := unstructured.NestedMap(gateway.Object, "status")
	if !conditionCurrent(status, "Accepted", gateway.GetGeneration()) || !conditionCurrent(status, "Programmed", gateway.GetGeneration()) {
		return problem("GatewayNotReady", "referenced Gateway has not accepted and programmed its current configuration")
	}
	if err := r.checkWebAddress(ctx, h); err != nil {
		return err
	}
	svc, route := webresources.Build(h)
	for _, obj := range []client.Object{svc, route} {
		if err := r.preflight(ctx, h, obj); err != nil {
			return err
		}
	}
	return nil
}
func overlaps(a, b *v1.Hermes) bool {
	ah, ap, _ := config.WebAddress(a)
	bh, bp, _ := config.WebAddress(b)
	return ah == bh && (ap == bp || ap == "" || bp == "")
}
func (r *HermesReconciler) checkWebAddress(ctx context.Context, h *v1.Hermes) error {
	list := &v1.HermesList{}
	if err := r.reader().List(ctx, list); err != nil {
		return apiFailure("inspect managed web addresses", err)
	}
	// A currently installed owned route wins over a new claimant. Otherwise use a
	// deterministic creation/name ordering, so simultaneous new CRs can converge.
	mine := webresources.Route(h)
	myInstalled := r.reader().Get(ctx, client.ObjectKeyFromObject(mine), mine) == nil && owned(mine, h)
	_, desired := webresources.Build(h)
	if myInstalled {
		wantHosts, _, _ := unstructured.NestedSlice(desired.Object, "spec", "hostnames")
		haveHosts, _, _ := unstructured.NestedSlice(mine.Object, "spec", "hostnames")
		wantRules, _, _ := unstructured.NestedSlice(desired.Object, "spec", "rules")
		haveRules, _, _ := unstructured.NestedSlice(mine.Object, "spec", "rules")
		myInstalled = equality.Semantic.DeepEqual(wantHosts, haveHosts) && equality.Semantic.DeepEqual(wantRules, haveRules)
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.UID == h.UID || other.Spec.Suspend || !other.DeletionTimestamp.IsZero() || !config.WebEnabled(other) || !overlaps(h, other) {
			continue
		}
		existing := webresources.Route(other)
		err := r.reader().Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil && !webAbsent(err) {
			return apiFailure("inspect competing web address", err)
		}
		installed := err == nil && owned(existing, other)
		earlier := other.CreationTimestamp.Before(&h.CreationTimestamp) || (other.CreationTimestamp.Equal(&h.CreationTimestamp) && other.Namespace+"/"+other.Name < h.Namespace+"/"+h.Name)
		if installed || (!myInstalled && earlier) {
			return problem("WebAddressConflict", "another installation claims this web hostname and path")
		}
	}
	return nil
}
func (r *HermesReconciler) applyWeb(ctx context.Context, h *v1.Hermes, desired client.Object) error {
	current := desired.DeepCopyObject().(client.Object)
	err := r.reader().Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if err = r.Create(ctx, desired); err != nil {
			return apiFailure("create web publication", err)
		}
		return nil
	}
	if err != nil {
		return apiFailure("read web publication", err)
	}
	if err = checkOwned(current, h); err != nil {
		return err
	}
	base := current.DeepCopyObject().(client.Object)
	switch want := desired.(type) {
	case *corev1.Service:
		have := current.(*corev1.Service)
		if have.Spec.ClusterIP == corev1.ClusterIPNone || have.Spec.Type != corev1.ServiceTypeClusterIP {
			return problem("ResourceConflict", "web Service must be ClusterIP")
		}
		have.Spec.Selector = want.Spec.Selector
		have.Spec.Ports = want.Spec.Ports
	case *unstructured.Unstructured:
		current.(*unstructured.Unstructured).Object["spec"] = runtime.DeepCopyJSONValue(want.Object["spec"])
	}
	current.SetLabels(desired.GetLabels())
	if equality.Semantic.DeepEqual(base, current) {
		return nil
	}
	if err = r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return apiFailure("update web publication", err)
	}
	return nil
}
func (r *HermesReconciler) reconcileWeb(ctx context.Context, h *v1.Hermes) error {
	if !config.WebEnabled(h) {
		setCondition(h, "WebReady", metav1.ConditionFalse, "Disabled", "web access is disabled")
		return nil
	}
	svc, route := webresources.Build(h)
	for _, obj := range []client.Object{svc, route} {
		if err := r.applyWeb(ctx, h, obj); err != nil {
			return err
		}
	}
	if err := r.reader().Get(ctx, client.ObjectKeyFromObject(route), route); err != nil {
		return apiFailure("observe web route", err)
	}
	_, _, url := config.WebAddress(h)
	h.Status.Web = &v1.WebStatus{URL: url, RouteRef: &v1.ObjectReference{Name: route.GetName(), UID: string(route.GetUID())}}
	if webresources.RouteReady(route, h) {
		setCondition(h, "WebReady", metav1.ConditionTrue, "RouteAccepted", "HTTPS route is accepted; external DNS and TLS are infrastructure responsibilities")
	} else {
		setCondition(h, "WebReady", metav1.ConditionFalse, "RoutePending", "waiting for current HTTPRoute parent conditions")
		setCondition(h, "Ready", metav1.ConditionFalse, "RoutePending", "waiting for current HTTPS route")
	}
	return nil
}
