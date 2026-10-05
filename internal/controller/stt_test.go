package controller

import (
	"slices"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSTTDependencyRotationAndDisable(t *testing.T) {
	r, h := unit(t, true)
	h.Spec.STT = &v1.STTSpec{Enabled: true, Model: "asr", APIKeySecretRef: &v1.STTSecretKeyRef{Name: "transcription"}}
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(secretNames(h), "transcription") {
		t.Fatal("STT Secret is not indexed for watch")
	}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyNotFound")
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "transcription", Namespace: h.Namespace}, Data: map[string][]byte{"STT_API_KEY": []byte("first-stt-key")}}
	if err := r.Create(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	first := h.Status.AppliedRevision
	if first == "" || *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("STT installation did not start")
	}
	secret.Data["STT_API_KEY"] = []byte("rotated-stt-key")
	if err := r.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if h.Status.AppliedRevision == first {
		t.Fatal("STT key rotation did not roll revision")
	}
	delete(secret.Data, "STT_API_KEY")
	if err := r.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyKeyMissing")
	if *sts(t, r, h).Spec.Replicas != 0 {
		t.Fatal("revoked STT credential left workload running")
	}
	h.Spec.STT.Enabled = false
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 1 || slices.Contains(secretNames(h), "transcription") {
		t.Fatal("disabled STT retained unused dependency")
	}
}
