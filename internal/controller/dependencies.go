package controller

import (
	"context"
	"encoding/json"
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
		s, ok := out[key]
		if !ok {
			s = &corev1.Secret{}
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
		if len(s.Data[ref.Key]) == 0 {
			return nil, problem("DependencyKeyMissing", "a required Secret key is missing or empty")
		}
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
		if index == secretIndex {
			revisions := &metav1.PartialObjectMetadataList{}
			revisions.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "SecretList"})
			if err := r.List(ctx, revisions, client.InNamespace(obj.GetNamespace()), client.MatchingFields{revisionSourceIndex: obj.GetName()}); err == nil {
				for i := range revisions.Items {
					revision := &revisions.Items[i]
					owner := metav1.GetControllerOf(revision)
					if owner == nil || owner.Kind != "Hermes" || owner.APIVersion != v1.GroupVersion.String() {
						continue
					}
					key := types.NamespacedName{Namespace: revision.Namespace, Name: owner.Name}
					if !seen[key] {
						seen[key] = true
						out = append(out, reconcile.Request{NamespacedName: key})
					}
				}
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
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), secret, revisionSourceIndex, revisionSourceNames); err != nil {
		return err
	}
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

const sourceRefsAnnotation = "hermes.wbe7.github.io/source-refs"
const revisionSourceIndex = "hermes.revisionSourceName"

type sourceRef struct {
	Name string    `json:"name"`
	Key  string    `json:"key"`
	UID  types.UID `json:"uid"`
}

func encodeSourceRefs(h *v1.Hermes, secrets map[types.NamespacedName]*corev1.Secret) string {
	refs := []sourceRef{}
	for _, ref := range config.SecretRefs(h) {
		s := secrets[types.NamespacedName{Namespace: h.Namespace, Name: ref.Name}]
		refs = append(refs, sourceRef{Name: ref.Name, Key: ref.Key, UID: s.UID})
	}
	raw, _ := json.Marshal(refs)
	return string(raw)
}
func decodeSourceRefs(obj client.Object) ([]sourceRef, error) {
	var refs []sourceRef
	if err := json.Unmarshal([]byte(obj.GetAnnotations()[sourceRefsAnnotation]), &refs); err != nil || len(refs) == 0 {
		return nil, problem("DependencyIdentityUnknown", "applied credential dependency identity is unavailable")
	}
	for _, ref := range refs {
		if ref.Name == "" || ref.Key == "" || ref.UID == "" {
			return nil, problem("DependencyIdentityUnknown", "applied credential dependency identity is incomplete")
		}
	}
	return refs, nil
}
func revisionSourceNames(obj client.Object) []string {
	refs, err := decodeSourceRefs(obj)
	if err != nil {
		return nil
	}
	names := []string{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !seen[ref.Name] {
			seen[ref.Name] = true
			names = append(names, ref.Name)
		}
	}
	return names
}

// The reconcile exit guard checks applied dependencies for this and every other
// failure path, including failures after a partial apply.
func (r *HermesReconciler) configurationFailed(ctx context.Context, h *v1.Hermes, configurationErr error) (ctrl.Result, error) {
	return r.failed(ctx, h, "ConfigurationReady", configurationErr)
}

// The Pod may be on an older revision than the StatefulSet, so inspect both;
// source values never enter this metadata.
func (r *HermesReconciler) checkAppliedDependencies(ctx context.Context, h *v1.Hermes) error {
	set := &appsv1.StatefulSet{}
	err := r.reader().Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, set)
	if apierrors.IsNotFound(err) {
		if h.Status.WorkloadRef == nil {
			return nil
		}
		set.Name = h.Status.WorkloadRef.Name
		set.Namespace = h.Namespace
		set.UID = types.UID(h.Status.WorkloadRef.UID)
	} else if err != nil {
		return apiFailure("read applied workload dependencies", err)
	} else if err = checkOwned(set, h); err != nil {
		return err
	}
	pods, err := r.workloadPods(ctx, h, set)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	add := func(spec corev1.PodSpec) {
		for _, v := range spec.Volumes {
			if v.Secret != nil {
				names[v.Secret.SecretName] = true
			}
		}
	}
	add(set.Spec.Template.Spec)
	for _, pod := range pods {
		add(pod.Spec)
	}
	sources := map[string]*corev1.Secret{}
	for name := range names {
		revision := &metav1.PartialObjectMetadata{}
		revision.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Secret"})
		if err = r.reader().Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: name}, revision); err != nil {
			if apierrors.IsNotFound(err) {
				return problem("DependencyNotFound", "an applied credential snapshot is missing")
			}
			return apiFailure("read applied credential metadata", err)
		}
		if err = checkOwned(revision, h); err != nil {
			return err
		}
		refs, err := decodeSourceRefs(revision)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			source, ok := sources[ref.Name]
			if !ok {
				source = &corev1.Secret{}
				if err = r.reader().Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: ref.Name}, source); err != nil {
					if apierrors.IsNotFound(err) {
						return problem("DependencyNotFound", "an applied credential source is missing")
					}
					return apiFailure("read applied credential source", err)
				}
				sources[ref.Name] = source
			}
			if !source.DeletionTimestamp.IsZero() || source.UID != ref.UID {
				return problem("DependencyNotFound", "an applied credential source was replaced or is terminating")
			}
			if len(source.Data[ref.Key]) == 0 {
				return problem("DependencyKeyMissing", "an applied credential key is missing or empty")
			}
		}
	}
	return nil
}
