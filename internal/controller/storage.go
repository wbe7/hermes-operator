package controller

import (
	"context"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const CreatorUIDAnnotation = "hermes.wbe7.github.io/creator-uid"

func claimName(h *v1.Hermes) string {
	if h.Spec.Storage.ExistingClaim != "" {
		return h.Spec.Storage.ExistingClaim
	}
	return h.Name + "-hermes-data"
}
func claimKey(h *v1.Hermes) types.NamespacedName {
	return types.NamespacedName{Namespace: h.Namespace, Name: claimName(h)}
}
func (r *HermesReconciler) validateStorage(ctx context.Context, h *v1.Hermes) error {
	if (h.Spec.Storage.Create == nil) == (h.Spec.Storage.ExistingClaim == "") {
		return problem("InvalidConfiguration", "exactly one storage source is required")
	}
	list := &v1.HermesList{}
	if err := r.reader().List(ctx, list, client.InNamespace(h.Namespace)); err != nil {
		return apiFailure("inspect storage references", err)
	}
	for _, other := range list.Items {
		if other.UID != h.UID && claimName(&other) == claimName(h) {
			return problem("ResourceConflict", "another Hermes references this claim")
		}
	}
	p := &corev1.PersistentVolumeClaim{}
	if err := r.reader().Get(ctx, claimKey(h), p); err != nil {
		if apierrors.IsNotFound(err) {
			if h.Status.StorageRef != nil {
				return problem("StorageIdentityMismatch", "recorded persistent volume claim is missing")
			}
			if h.Spec.Storage.Create != nil {
				return nil
			}
			return problem("DependencyNotFound", "referenced persistent volume claim is missing")
		}
		return apiFailure("inspect storage", err)
	}
	if !p.DeletionTimestamp.IsZero() {
		return problem("StorageIdentityMismatch", "persistent volume claim is terminating")
	}
	if h.Status.StorageRef != nil && (h.Status.StorageRef.Name != p.Name || h.Status.StorageRef.UID != string(p.UID)) {
		return problem("StorageIdentityMismatch", "persistent volume claim identity changed")
	}
	if p.Spec.VolumeMode != nil && *p.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return problem("InvalidConfiguration", "persistent volume claim must use Filesystem mode")
	}
	if h.Spec.Storage.Create != nil {
		if p.Annotations[CreatorUIDAnnotation] != string(h.UID) || len(p.OwnerReferences) != 0 {
			return problem("StorageIdentityMismatch", "persistent volume claim was not created by this installation")
		}
		if h.Spec.Storage.Create.Size.Cmp(*p.Spec.Resources.Requests.Storage()) < 0 {
			return problem("InvalidConfiguration", "persistent volume claim cannot shrink")
		}
	}
	return nil
}
func (r *HermesReconciler) ensureStorage(ctx context.Context, h *v1.Hermes) error {
	p := &corev1.PersistentVolumeClaim{}
	err := r.reader().Get(ctx, claimKey(h), p)
	if apierrors.IsNotFound(err) && h.Spec.Storage.Create != nil {
		if h.Status.StorageRef != nil {
			return problem("StorageIdentityMismatch", "recorded persistent volume claim is missing")
		}
		c := h.Spec.Storage.Create
		p = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: claimName(h), Namespace: h.Namespace, Annotations: map[string]string{CreatorUIDAnnotation: string(h.UID)}}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: c.StorageClassName, AccessModes: []corev1.PersistentVolumeAccessMode{c.AccessMode}, VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem), Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: c.Size.DeepCopy()}}}}
		if p.Spec.AccessModes[0] == "" {
			p.Spec.AccessModes[0] = corev1.ReadWriteOnce
		}
		if err = r.Create(ctx, p); err != nil {
			return apiFailure("create persistent volume claim", err)
		}
	} else if err != nil {
		return apiFailure("read persistent volume claim", err)
	}
	// Recheck identity on the fresh read; a replacement between preflight and apply
	// must not be resized or recorded as the original claim.
	if h.Status.StorageRef != nil && (h.Status.StorageRef.Name != p.Name || h.Status.StorageRef.UID != string(p.UID)) {
		return problem("StorageIdentityMismatch", "persistent volume claim identity changed")
	}
	origin := "Existing"
	if h.Spec.Storage.Create != nil {
		origin = "Created"
		if p.Annotations[CreatorUIDAnnotation] != string(h.UID) || len(p.OwnerReferences) != 0 {
			return problem("StorageIdentityMismatch", "persistent volume claim provenance changed")
		}
	}
	h.Status.StorageRef = &v1.StorageReference{Name: p.Name, UID: string(p.UID), Origin: origin}
	reason := "StoragePending"
	status := metav1.ConditionFalse
	message := "claim is waiting for binding; workload may start for delayed binding"
	if p.Status.Phase == corev1.ClaimBound {
		status = metav1.ConditionTrue
		reason = "Reconciled"
		message = "persistent storage is bound"
	}
	if c := h.Spec.Storage.Create; c != nil && c.Size.Cmp(*p.Spec.Resources.Requests.Storage()) > 0 {
		class := &storagev1.StorageClass{}
		if p.Spec.StorageClassName == nil || *p.Spec.StorageClassName == "" {
			setCondition(h, "StorageReady", metav1.ConditionFalse, "ResizeUnsupported", "storage class does not support expansion")
			return nil
		}
		if err = r.reader().Get(ctx, types.NamespacedName{Name: *p.Spec.StorageClassName}, class); err != nil {
			if !apierrors.IsNotFound(err) {
				return apiFailure("read storage class", err)
			}
			setCondition(h, "StorageReady", metav1.ConditionFalse, "ResizeUnsupported", "storage class is unavailable")
			return nil
		}
		if class.AllowVolumeExpansion == nil || !*class.AllowVolumeExpansion {
			setCondition(h, "StorageReady", metav1.ConditionFalse, "ResizeUnsupported", "storage class does not support expansion")
			return nil
		}
		base := p.DeepCopy()
		p.Spec.Resources.Requests[corev1.ResourceStorage] = c.Size.DeepCopy()
		if err = r.Patch(ctx, p, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
				setCondition(h, "StorageReady", metav1.ConditionFalse, "ResizeUnsupported", "storage provider rejected expansion")
				return nil
			}
			return apiFailure("expand persistent volume claim", err)
		}
	}
	if c := h.Spec.Storage.Create; c != nil && c.Size.Cmp(*p.Status.Capacity.Storage()) > 0 && p.Status.Phase == corev1.ClaimBound {
		status = metav1.ConditionFalse
		reason = "ResizePending"
		message = "waiting for volume expansion"
	}
	setCondition(h, "StorageReady", status, reason, message)
	return nil
}
func (r *HermesReconciler) workloadPods(ctx context.Context, h *v1.Hermes, set *appsv1.StatefulSet) ([]corev1.Pod, error) {
	list := &corev1.PodList{}
	if err := r.reader().List(ctx, list, client.InNamespace(h.Namespace)); err != nil {
		return nil, apiFailure("inspect workload Pods", err)
	}
	out := []corev1.Pod{}
	for _, p := range list.Items {
		ref := metav1.GetControllerOf(&p)
		matches := ref != nil && ref.UID == set.UID && ref.Kind == "StatefulSet" && ref.Name == set.Name && ref.APIVersion == "apps/v1"
		if p.Name == set.Name+"-0" && !matches {
			return nil, problem("ResourceConflict", "workload Pod belongs to another owner")
		}
		if matches {
			out = append(out, p)
		}
	}
	return out, nil
}

// stop waits for API-observed Pod disappearance, including terminating Pods.
// No zero-grace/force deletion; finalization cannot race a still-running poller.
func (r *HermesReconciler) stop(ctx context.Context, h *v1.Hermes) (bool, error) {
	set := &appsv1.StatefulSet{}
	err := r.reader().Get(ctx, types.NamespacedName{Name: h.Name + "-hermes", Namespace: h.Namespace}, set)
	if apierrors.IsNotFound(err) {
		set.Name = h.Name + "-hermes"
		set.Namespace = h.Namespace
		if h.Status.WorkloadRef != nil {
			set.Name = h.Status.WorkloadRef.Name
			set.UID = types.UID(h.Status.WorkloadRef.UID)
		}
	} else if err != nil {
		return false, apiFailure("read workload for stop", err)
	} else {
		if !owned(set, h) {
			return false, problem("ResourceConflict", "workload belongs to another owner")
		}
		if set.DeletionTimestamp.IsZero() && (set.Spec.Replicas == nil || *set.Spec.Replicas != 0) {
			base := set.DeepCopy()
			set.Spec.Replicas = ptr.To(int32(0))
			if err = r.Patch(ctx, set, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return false, apiFailure("stop workload", err)
			}
		}
	}
	pods, err := r.workloadPods(ctx, h, set)
	if err != nil {
		return false, err
	}
	for i := range pods {
		if pods[i].DeletionTimestamp.IsZero() {
			if err = r.deleteUID(ctx, &pods[i]); err != nil {
				return false, err
			}
		}
	}
	return len(pods) == 0, nil
}
func (r *HermesReconciler) finalize(ctx context.Context, h *v1.Hermes) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(h, Finalizer) {
		return ctrl.Result{}, nil
	}
	if h.Annotations[deletionPolicyAnnotation] == "" {
		base := h.DeepCopy()
		if h.Annotations == nil {
			h.Annotations = map[string]string{}
		}
		policy := "Retain"
		if h.Spec.Storage.Create != nil && h.Spec.Storage.DeletionPolicy == "Delete" {
			policy = "Delete"
		}
		h.Annotations[deletionPolicyAnnotation] = policy
		if err := r.Patch(ctx, h, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			return ctrl.Result{}, apiFailure("freeze deletion policy", err)
		}
		return ctrl.Result{RequeueAfter: retryDelay}, nil
	}
	stopped, err := r.stop(ctx, h)
	if err != nil {
		return r.failed(ctx, h, "Ready", err)
	}
	if !stopped {
		return ctrl.Result{RequeueAfter: retryDelay}, nil
	}
	if h.Annotations[deletionPolicyAnnotation] == "Delete" && h.Spec.Storage.Create != nil {
		p := &corev1.PersistentVolumeClaim{}
		err = r.reader().Get(ctx, claimKey(h), p)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, apiFailure("inspect finalizing claim", err)
		}
		if err == nil {
			if p.Annotations[CreatorUIDAnnotation] != string(h.UID) || len(p.OwnerReferences) != 0 {
				return r.failed(ctx, h, "StorageReady", problem("StorageIdentityMismatch", "refusing to delete a claim with different provenance"))
			}
			if h.Status.StorageRef == nil {
				h.Status.StorageRef = &v1.StorageReference{Name: p.Name, UID: string(p.UID), Origin: "Created"}
				return r.saveStatus(ctx, h)
			}
			if h.Status.StorageRef.Name != p.Name || h.Status.StorageRef.UID != string(p.UID) || h.Status.StorageRef.Origin != "Created" {
				return r.failed(ctx, h, "StorageReady", problem("StorageIdentityMismatch", "refusing to delete a replacement claim"))
			}
			if p.DeletionTimestamp.IsZero() {
				if err = r.deleteUID(ctx, p); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: retryDelay}, nil
		}
	}
	base := h.DeepCopy()
	controllerutil.RemoveFinalizer(h, Finalizer)
	if err = r.Patch(ctx, h, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return ctrl.Result{}, apiFailure("release finalizer", err)
	}
	return ctrl.Result{}, nil
}
