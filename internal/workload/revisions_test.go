package workload

import (
	"testing"
	"testing/fstest"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestFinalRevision(t *testing.T) {
	h, r, b := fixture(t)
	base, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*v1.Hermes)
	}{
		{"resources", func(h *v1.Hermes) {
			h.Spec.Resources.Limits = v1.ResourceList{corev1.ResourceCPU: resource.MustParse("3")}
		}},
		{"placement", func(h *v1.Hermes) { h.Spec.Scheduling.NodeSelector = map[string]string{"disk": "ssd"} }},
		{"pull secrets", func(h *v1.Hermes) { h.Spec.Image.PullSecrets = []corev1.LocalObjectReference{{Name: "registry"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := h.DeepCopy()
			tc.change(h)
			got, e := Build(h, r, b, "home")
			if e != nil || got.Revision == base.Revision {
				t.Fatal("change did not roll pod", e)
			}
		})
	}
	h.Labels = map[string]string{"changed": "metadata"}
	h.Spec.Storage.Create.Size = resource.MustParse("20Gi")
	h.Spec.Suspend = true
	h.Spec.Storage.DeletionPolicy = "Delete"
	h.Spec.Network.AdditionalBlockedCIDRs = []string{"192.0.2.0/24"}
	same, err := Build(h, r, b, "home")
	if err != nil || same.Revision != base.Revision {
		t.Fatal("non-Pod input changed revision")
	}
	altered := base.StatefulSet.Spec.Template.DeepCopy()
	altered.Annotations = nil
	if finalRevision(*altered, b.Revision, "new-assets") == finalRevision(*altered, b.Revision, "old-assets") {
		t.Fatal("asset-only upgrade must change final identity")
	}
}

func TestRuntimeScriptOnlyUpgrade(t *testing.T) {
	old := fstest.MapFS{"assets/adapters/v20260914.py": &fstest.MapFile{Data: []byte("old script")}}
	newer := fstest.MapFS{"assets/adapters/v20260914.py": &fstest.MapFile{Data: []byte("new script")}}
	before, after := assetsChecksum(old), assetsChecksum(newer)
	if before == after {
		t.Fatal("same module name must not conceal changed bytes")
	}
	h, r, b := fixture(t)
	out, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	template := out.StatefulSet.Spec.Template
	if finalRevision(template, b.Revision, before) == finalRevision(template, b.Revision, after) {
		t.Fatal("script-only upgrade must change applied identity")
	}
}
