package controller

import (
	"slices"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTTSDependencyRotationAndDisable(t *testing.T) {
	r, h := unit(t, true)
	h.Spec.TTS = &v1.TTSSpec{Enabled: true, Model: "speech", Voice: "default", APIKeySecretRef: &v1.TTSSecretKeyRef{Name: "speech"}}
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(secretNames(h), "speech") {
		t.Fatal("TTS Secret is not indexed for watch")
	}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyNotFound")
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "speech", Namespace: h.Namespace}, Data: map[string][]byte{"TTS_API_KEY": []byte("first-tts-key")}}
	if err := r.Create(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	first := h.Status.AppliedRevision
	if first == "" || *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("TTS installation did not start")
	}
	secret.Data["TTS_API_KEY"] = []byte("rotated-tts-key")
	if err := r.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if h.Status.AppliedRevision == first {
		t.Fatal("TTS key rotation did not roll revision")
	}
	delete(secret.Data, "TTS_API_KEY")
	if err := r.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyKeyMissing")
	if *sts(t, r, h).Spec.Replicas != 0 {
		t.Fatal("revoked TTS credential left workload running")
	}
	h.Spec.TTS.Enabled = false
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 1 || slices.Contains(secretNames(h), "speech") {
		t.Fatal("disabled TTS retained unused dependency")
	}
}
