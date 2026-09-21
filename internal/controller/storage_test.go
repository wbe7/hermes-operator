package controller

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func claim(t *testing.T, r *HermesReconciler, h *v1.Hermes) *corev1.PersistentVolumeClaim {
	t.Helper()
	p := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes-data"}, p); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestStoragePendingAndProvenance(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	p := claim(t, r, h)
	if len(p.OwnerReferences) != 0 || p.Annotations[CreatorUIDAnnotation] != string(h.UID) {
		t.Fatal("unsafe PVC provenance")
	}
	if *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("delayed binding deadlocked")
	}
	reason(t, h, "StorageReady", "StoragePending")
}
func TestExistingNeverMutatedAndExclusive(t *testing.T) {
	r, h := unit(t, true)
	h.Spec.Storage = v1.StorageSpec{ExistingClaim: "external"}
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	p := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: h.Namespace, UID: "external-uid"}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}}}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	rv := p.ResourceVersion
	runReconcile(t, r, h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
		t.Fatal(err)
	}
	if rv != p.ResourceVersion {
		t.Fatal("existing PVC mutated")
	}
	other := h.DeepCopy()
	other.Name = "other"
	other.UID = "other-uid"
	other.ResourceVersion = ""
	other.Finalizers = nil
	other.Status = v1.HermesStatus{}
	if err := r.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "Ready", "ResourceConflict")
	if *sts(t, r, h).Spec.Replicas != 0 {
		t.Fatal("shared home still running")
	}
}
func TestRetainedClaimNotAdopted(t *testing.T) {
	r, h := unit(t, true)
	p := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: h.Name + "-hermes-data", Namespace: h.Namespace, UID: "old-claim", Annotations: map[string]string{CreatorUIDAnnotation: "old-cr"}}}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "Ready", "StorageIdentityMismatch")
}
func TestResizeUnsupportedPreservesRequest(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	h.Spec.Storage.Create.Size = resource.MustParse("20Gi")
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "StorageReady", "ResizeUnsupported")
	p := claim(t, r, h)
	if p.Spec.Resources.Requests.Storage().Cmp(resource.MustParse("10Gi")) != 0 {
		t.Fatal("unsupported resize mutated claim")
	}
}
func TestFinalizerRetentionAndIdentity(t *testing.T) {
	for _, policy := range []string{"Retain", "Delete"} {
		t.Run(policy, func(t *testing.T) {
			r, h := unit(t, true)
			h.Spec.Storage.DeletionPolicy = policy
			if err := r.Update(ctx, h); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			p := claim(t, r, h)
			set := sts(t, r, h)
			set.UID = "set-uid"
			if err := r.Update(ctx, set); err != nil {
				t.Fatal(err)
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-0", Namespace: h.Namespace, UID: "pod-uid", Finalizers: []string{"test/hold"}, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "test"}}}}
			if err := r.Create(ctx, pod); err != nil {
				t.Fatal(err)
			}
			if err := r.Delete(ctx, h); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				t.Fatal("PVC removed before Pod stopped")
			}
			if err := r.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
				t.Fatal(err)
			}
			pod.Finalizers = nil
			if err := r.Update(ctx, pod); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4; i++ {
				if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
					t.Fatal(err)
				}
			}
			err := r.Get(ctx, client.ObjectKeyFromObject(p), p)
			if policy == "Retain" && err != nil {
				t.Fatal(err)
			}
			if policy == "Delete" && !apierrors.IsNotFound(err) {
				t.Fatalf("delete claim: %v", err)
			}
		})
	}
}
func TestReplacementClaimNeverDeleted(t *testing.T) {
	r, h := unit(t, true)
	h.Spec.Storage.DeletionPolicy = "Delete"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	p := claim(t, r, h)
	h.Status.StorageRef.UID = "original-uid"
	if err := r.Status().Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	p.UID = "replacement-uid"
	if err := r.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "Ready", "StorageIdentityMismatch")
	if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRecordedClaimIsNotRecreated(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	p := claim(t, r, h)
	if err := r.Delete(ctx, p); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "StorageReady", "StorageIdentityMismatch")
	if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); !apierrors.IsNotFound(err) {
		t.Fatal("lost persistent data silently replaced with an empty claim")
	}
}
func TestOrphanPodBlocksFinalizationWithoutWorkloadStatus(t *testing.T) {
	r, h := unit(t, false)
	runReconcile(t, r, h)
	h.Spec.Storage.DeletionPolicy = "Retain"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: h.Name + "-hermes-0", Namespace: h.Namespace}}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, h); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
		t.Fatal("finalizer released with an unaccounted Pod")
	}
}

func TestSupportedResizeChangesOnlyRequest(t *testing.T) {
	r, h := unit(t, true)
	class := "expandable"
	h.Spec.Storage.Create.StorageClassName = &class
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: class}, Provisioner: "fake.test", AllowVolumeExpansion: ptr.To(true)}
	if err := r.Create(ctx, sc); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	p := claim(t, r, h)
	before := p.DeepCopy()
	p.Status.Phase = corev1.ClaimBound
	p.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}
	if err := r.Status().Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	rev := h.Status.AppliedRevision
	h.Spec.Storage.Create.Size = resource.MustParse("20Gi")
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	p = claim(t, r, h)
	if p.Spec.Resources.Requests.Storage().Cmp(resource.MustParse("20Gi")) != 0 || p.UID != before.UID || !equality.Semantic.DeepEqual(p.Annotations, before.Annotations) || len(p.OwnerReferences) != 0 {
		t.Fatal("unsafe expansion")
	}
	if h.Status.AppliedRevision != rev {
		t.Fatal("storage resize restarted workload")
	}
	reason(t, h, "StorageReady", "ResizePending")
}

// envtest has no StatefulSet/GC/PVC protection controllers. This test creates
// the child Pod and completes protection finalizers explicitly, while every
// operator write/delete still goes through the real API server.
func realFinalization(t *testing.T, r *HermesReconciler, h *v1.Hermes) {
	t.Helper()
	h.Spec.Storage.DeletionPolicy = "Delete"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	set := sts(t, r, h)
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-0", Namespace: h.Namespace, Finalizers: []string{"test/hold"}, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}}, Spec: set.Spec.Template.Spec}
	p.Spec.NodeName = "envtest-node"
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	home := claim(t, r, h)
	if err := r.Delete(ctx, h); err != nil {
		t.Fatal(err)
	}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}
	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(home), home); err != nil || !home.DeletionTimestamp.IsZero() {
		t.Fatal("claim deletion preceded Pod shutdown")
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
		t.Fatal(err)
	}
	if p.DeletionTimestamp.IsZero() || p.DeletionGracePeriodSeconds == nil || *p.DeletionGracePeriodSeconds == 0 {
		t.Fatal("Pod was not gracefully stopped")
	}
	p.Finalizers = nil
	if err := r.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	// No kubelet runs in envtest: simulate its completed graceful termination.
	if err := r.Delete(ctx, p, &client.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0)), Preconditions: &metav1.Preconditions{UID: &p.UID}}); err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
		t.Fatal(err)
	}
	h.Spec.Storage.DeletionPolicy = "Retain"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	err := r.Get(ctx, client.ObjectKeyFromObject(home), home)
	if err == nil {
		if home.DeletionTimestamp.IsZero() {
			t.Fatal("frozen Delete policy changed")
		}
		home.Finalizers = nil
		if err := r.Update(ctx, home); err != nil {
			t.Fatal(err)
		}
	} else if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); !apierrors.IsNotFound(err) {
		t.Fatalf("finalization incomplete: %v", err)
	}
}
