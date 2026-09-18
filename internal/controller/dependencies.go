package controller

import (
	"context"
	"time"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const secretIndex = "hermes.secretName"
const claimIndex = "hermes.claimName"

func (r *HermesReconciler) secrets(ctx context.Context, h *v1.Hermes) (map[types.NamespacedName]*corev1.Secret, error) {
	out := map[types.NamespacedName]*corev1.Secret{}
	for _, ref := range config.SecretRefs(h) {
		key := types.NamespacedName{Namespace: h.Namespace, Name: ref.Name}
		if _, ok := out[key]; ok {
			continue
		}
		s := &corev1.Secret{}
		if err := r.reader().Get(ctx, key, s); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, problem("DependencyNotFound", "a referenced Secret is missing")
			}
			return nil, apiFailure("read credential dependency", err)
		}
		if !s.DeletionTimestamp.IsZero() {
			return nil, problem("DependencyNotFound", "a referenced Secret is terminating")
		}
		out[key] = s
	}
	return out, nil
}
func secretNames(obj client.Object) []string {
	h := obj.(*v1.Hermes)
	out := []string{}
	seen := map[string]bool{}
	for _, ref := range config.SecretRefs(h) {
		if !seen[ref.Name] {
			out = append(out, ref.Name)
			seen[ref.Name] = true
		}
	}
	return out
}
func (r *HermesReconciler) mapDependency(index string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		list := &v1.HermesList{}
		if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace()), client.MatchingFields{index: obj.GetName()}); err != nil {
			return nil
		}
		out := []reconcile.Request{}
		seen := map[types.NamespacedName]bool{}
		for _, h := range list.Items {
			key := client.ObjectKeyFromObject(&h)
			out = append(out, reconcile.Request{NamespacedName: key})
			seen[key] = true
		}
		if ref := metav1.GetControllerOf(obj); ref != nil && ref.Kind == "Hermes" && ref.APIVersion == v1.GroupVersion.String() {
			key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: ref.Name}
			if !seen[key] {
				out = append(out, reconcile.Request{NamespacedName: key})
			}
		}
		return out
	}
}
func (r *HermesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.Network.Validate(); err != nil {
		return err
	}
	r.Client = mgr.GetClient()
	r.APIReader = mgr.GetAPIReader()
	r.Scheme = mgr.GetScheme()
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Hermes{}, secretIndex, secretNames); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Hermes{}, claimIndex, func(obj client.Object) []string { return []string{claimName(obj.(*v1.Hermes))} }); err != nil {
		return err
	}
	secret := &metav1.PartialObjectMetadata{}
	secret.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
	return ctrl.NewControllerManagedBy(mgr).For(&v1.Hermes{}).Owns(&appsv1.StatefulSet{}).Owns(&corev1.Service{}).Owns(&corev1.ServiceAccount{}).Owns(&corev1.ConfigMap{}).Owns(&networkingv1.NetworkPolicy{}).
		Watches(secret, handler.EnqueueRequestsFromMapFunc(r.mapDependency(secretIndex)), builder.OnlyMetadata).
		Watches(&corev1.PersistentVolumeClaim{}, handler.EnqueueRequestsFromMapFunc(r.mapDependency(claimIndex))).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.mapPod)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1, RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](time.Second, time.Minute)}).Complete(r)
}
func (r *HermesReconciler) mapPod(ctx context.Context, obj client.Object) []reconcile.Request {
	ref := metav1.GetControllerOf(obj)
	if ref == nil || ref.Kind != "StatefulSet" || ref.APIVersion != "apps/v1" {
		return nil
	}
	set := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: ref.Name}, set); err != nil || set.UID != ref.UID {
		return nil
	}
	owner := metav1.GetControllerOf(set)
	if owner == nil || owner.Kind != "Hermes" || owner.APIVersion != v1.GroupVersion.String() {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: owner.Name}}}
}
