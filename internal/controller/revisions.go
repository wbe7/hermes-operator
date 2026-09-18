package controller

import (
	"context"
	"strings"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Retain inputs referenced by the desired StatefulSet or any live/terminating
// Pod of that workload. Secret listing uses metadata only, never typed cache.
func (r *HermesReconciler) collectRevisions(ctx context.Context, h *v1.Hermes, set *appsv1.StatefulSet) error {
	keepCM, keepSecret := map[string]bool{}, map[string]bool{}
	add := func(spec corev1.PodSpec) {
		for _, v := range spec.Volumes {
			if v.ConfigMap != nil {
				keepCM[v.ConfigMap.Name] = true
			}
			if v.Secret != nil {
				keepSecret[v.Secret.SecretName] = true
			}
		}
	}
	for _, ref := range config.SecretRefs(h) {
		keepSecret[ref.Name] = true
	}
	add(set.Spec.Template.Spec)
	pods, err := r.workloadPods(ctx, h, set)
	if err != nil {
		return err
	}
	for _, p := range pods {
		add(p.Spec)
	}
	for _, kind := range []string{"ConfigMap", "Secret"} {
		list := &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: kind + "List"})
		if err = r.reader().List(ctx, list, client.InNamespace(h.Namespace)); err != nil {
			return apiFailure("list revision metadata", err)
		}
		for i := range list.Items {
			obj := &list.Items[i]
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: kind})
			if kind == "Secret" && !strings.HasPrefix(obj.Name, h.Name+"-sec-") {
				continue
			}
			if kind == "ConfigMap" && !strings.HasPrefix(obj.Name, h.Name+"-cfg-") && !strings.HasPrefix(obj.Name, h.Name+"-rt-") {
				continue
			}
			if !owned(obj, h) || !obj.DeletionTimestamp.IsZero() {
				continue
			}
			if kind == "ConfigMap" && keepCM[obj.Name] || kind == "Secret" && keepSecret[obj.Name] {
				continue
			}
			if err = r.deleteUID(ctx, obj); err != nil {
				return err
			}
		}
	}
	return nil
}
