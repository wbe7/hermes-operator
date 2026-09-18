package controller

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestRotationMissingDependencyAndSuspend(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	first := sts(t, r, h).Spec.Template.DeepCopy()
	sec := source(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
		t.Fatal(err)
	}
	sec.Data["unused"] = []byte("unused")
	sec.Labels = map[string]string{"label": "value"}
	if err := r.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if sts(t, r, h).Spec.Template.Annotations["hermes.wbe7.github.io/revision"] != first.Annotations["hermes.wbe7.github.io/revision"] {
		t.Fatal("unused rotation rolled Pod")
	}
	sec.Data["MODEL_API_KEY"] = []byte("rotated")
	if err := r.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if sts(t, r, h).Spec.Template.Annotations["hermes.wbe7.github.io/revision"] == first.Annotations["hermes.wbe7.github.io/revision"] {
		t.Fatal("selected rotation did not roll")
	}
	if err := r.Delete(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 0 {
		t.Fatal("old credentials still active")
	}
	_ = claim(t, r, h)
	h.Spec.Suspend = true
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if !meta.IsStatusConditionTrue(h.Status.Conditions, "Suspended") {
		t.Fatal("suspend needs credentials")
	}
	h.Spec.Suspend = false
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	sec = source(h)
	sec.UID = "new-uid"
	if err := r.Create(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("did not resume")
	}
}
func TestInvalidConfigPreservesLastValidWorkload(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	rev := h.Status.AppliedRevision
	h.Spec.Version = "unsupported"
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "Ready", "UnsupportedVersion")
	if *sts(t, r, h).Spec.Replicas != 1 || h.Status.AppliedRevision != rev {
		t.Fatal("invalid spec disturbed last valid workload")
	}
	if h.Status.ObservedGeneration != h.Generation {
		t.Fatal("generation not observed")
	}
}

// A ready condition from a terminal Pod must never make the installation ready.
func TestReadyRequiresRunningCurrentPod(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-0", Namespace: h.Namespace, Annotations: map[string]string{"hermes.wbe7.github.io/revision": h.Status.AppliedRevision}, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}}, Spec: set.Spec.Template.Spec, Status: corev1.PodStatus{Phase: corev1.PodFailed, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if meta.IsStatusConditionTrue(h.Status.Conditions, "Ready") {
		t.Fatal("terminal Pod incorrectly reports ready")
	}
	p.Status.Phase = corev1.PodRunning
	if err := r.Status().Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if !meta.IsStatusConditionTrue(h.Status.Conditions, "Ready") {
		t.Fatal("running current Pod not ready")
	}
}
func TestRolloutRecoveryRetainsTerminatingPodInputs(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	oldRevision := h.Status.AppliedRevision
	oldConfig := set.Spec.Template.Spec.Volumes[2].ConfigMap.Name
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-0", Namespace: h.Namespace, Finalizers: []string{"test/hold"}, Annotations: set.Spec.Template.Annotations, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}}, Spec: set.Spec.Template.Spec}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	h.Spec.Model.Name = "changed"
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
		t.Fatal(err)
	}
	if p.DeletionTimestamp.IsZero() {
		t.Fatal("stuck old Pod not deleted")
	}
	if h.Status.AppliedRevision == oldRevision {
		t.Fatal("workload revision not advanced")
	}
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: h.Namespace, Name: oldConfig}
	if err := r.Get(ctx, key, cm); err != nil {
		t.Fatal("removed inputs while Pod is terminating")
	}
	p.Finalizers = nil
	if err := r.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if err := r.Get(ctx, key, cm); !apierrors.IsNotFound(err) {
		t.Fatalf("unreferenced revision leaked: %v", err)
	}
}

func TestRevisionGCNeverDeletesSourceSecret(t *testing.T) {
	r, h := unit(t, true)
	sec := source(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
		t.Fatal(err)
	}
	sec.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(h, v1.GroupVersion.WithKind("Hermes"))}
	if err := r.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
		t.Fatal("source Secret was garbage collected")
	}
}

func TestSecretUIDAndMissingKey(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	before := h.Status.AppliedRevision
	sec := source(h)
	if err := r.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
		t.Fatal(err)
	}
	delete(sec.Data, "MODEL_API_KEY")
	if err := r.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyKeyMissing")
	if *sts(t, r, h).Spec.Replicas != 0 {
		t.Fatal("missing key left workload active")
	}
	if err := r.Delete(ctx, sec); err != nil {
		t.Fatal(err)
	}
	sec = source(h)
	sec.UID = "replacement-source"
	if err := r.Create(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if h.Status.AppliedRevision == before {
		t.Fatal("same Secret bytes under a different UID did not roll")
	}
}
