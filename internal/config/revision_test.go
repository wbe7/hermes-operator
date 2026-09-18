package config

import (
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestRevisionInputs(t *testing.T) {
	h, r := fixture()
	s := sources()
	b, err := Render(h, r, s)
	if err != nil {
		t.Fatal(err)
	}
	h.Annotations = map[string]string{"ignored": "changed"}
	src := s[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}]
	src.ResourceVersion = "999"
	src.Data["UNUSED"] = []byte("changed")
	same, _ := Render(h, r, s)
	if same.Revision != b.Revision {
		t.Fatal("irrelevant change revised")
	}
	for _, change := range []func(){func() { h.Spec.Model.Name = "new" }, func() { src.Data["MODEL_API_KEY"] = []byte("new") }, func() { src.UID = "new" }, func() { r.Adapter += "-new" }, func() { h.Spec.Image.Repository = "mirror.invalid/hermes" }} {
		before, _ := Render(h, r, s)
		change()
		after, err := Render(h, r, s)
		if err != nil {
			t.Fatal(err)
		}
		if before.Revision == after.Revision {
			t.Fatal("relevant change ignored")
		}
	}
}
func TestNormalizedOrderingAndExplicitDefaults(t *testing.T) {
	h, r := fixture()
	h.Spec.ExtraEnv = map[string]string{"ZZ": "z", "AA": "a"}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"safe":{"z":1,"a":2}}`)}
	h.Spec.Telegram.AllowedUserIDs = []string{"456", "123"}
	before, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"safe":{"a":2,"z":1}}`)}
	h.Spec.Telegram.AllowedUserIDs = []string{"123", "456"}
	h.Spec.Reasoning.Effort = "xhigh"
	h.Spec.Model.Auth = "APIKey"
	h.Spec.Model.APIMode = "chat_completions"
	h.Spec.Image.Repository = "docker.io/nousresearch/hermes-agent"
	h.Spec.Image.Digest = r.ImageDigest
	h.Spec.Image.PullPolicy = "IfNotPresent"
	after, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision {
		t.Fatal("equivalent inputs changed revision")
	}
}
func TestSelectedKeyIdentityChangesRevision(t *testing.T) {
	h, r := fixture()
	s := sources()
	before, _ := Render(h, r, s)
	src := s[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}]
	src.Data["alias"] = append([]byte{}, src.Data["MODEL_API_KEY"]...)
	h.Spec.Model.APIKeySecretRef = &v1.ModelSecretKeyRef{Key: "alias"}
	after, err := Render(h, r, s)
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision == after.Revision {
		t.Fatal("source key identity ignored")
	}
}
