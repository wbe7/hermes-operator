package workload

import (
	"reflect"
	"strings"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	"github.com/wbe7/hermes-operator/internal/network"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	"github.com/wbe7/hermes-operator/internal/testfixtures"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fixture(t *testing.T) (*v1.Hermes, runtimecatalog.Release, config.Bundle) {
	t.Helper()
	h := testfixtures.Hermes(t)
	r, err := runtimecatalog.Resolve(h.Spec.Version)
	if err != nil {
		t.Fatal(err)
	}
	return h, r, config.Bundle{JSON: []byte(`{"schema":1}`), SecretData: map[string][]byte{"token": []byte("fixture-only")}, Revision: "0123456789abcdef"}
}
func TestBuildSecurityAndStartup(t *testing.T) {
	h, r, b := fixture(t)
	out, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	p := out.StatefulSet.Spec.Template.Spec
	c := p.Containers[0]
	sc := p.SecurityContext
	if len(p.Containers) != 1 || len(p.InitContainers) != 0 || *out.StatefulSet.Spec.Replicas != 1 {
		t.Fatal("must run one main container")
	}
	if sc == nil || *sc.RunAsUser != 10000 || *sc.RunAsGroup != 10000 || *sc.FSGroup != 10000 || !*sc.RunAsNonRoot || *sc.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatal("pod hardening")
	}
	cs := c.SecurityContext
	if cs == nil || !*cs.ReadOnlyRootFilesystem || *cs.AllowPrivilegeEscalation || !reflect.DeepEqual(cs.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Fatal("container hardening")
	}
	if *p.AutomountServiceAccountToken || *out.ServiceAccount.AutomountServiceAccountToken || p.HostNetwork || p.HostPID || p.HostIPC || len(c.Ports) > 0 {
		t.Fatal("unneeded privilege or port")
	}
	if p.ServiceAccountName != h.Name+"-hermes" || out.Service.Spec.ClusterIP != "None" || len(out.Service.Spec.Ports) > 0 || out.Service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatal("service / SA contract")
	}
	if !reflect.DeepEqual(c.Command, []string{"/opt/hermes/.venv/bin/python", "-I", "/operator/runtime/bootstrap.py"}) {
		t.Fatalf("restore must run each container start: %v", c.Command)
	}
	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	if env["HOME"] != "/opt/data" || env["HERMES_HOME"] != "/opt/data" {
		t.Fatal("full home")
	}
	mounts := map[string]corev1.VolumeMount{}
	for _, m := range c.VolumeMounts {
		mounts[m.MountPath] = m
		if m.SubPath != "" {
			t.Fatal("partial mount")
		}
	}
	for _, path := range []string{"/operator/config", "/operator/credentials", "/operator/runtime"} {
		if !mounts[path].ReadOnly {
			t.Fatalf("%s must be read-only", path)
		}
	}
	if _, ok := mounts["/opt/data"]; !ok {
		t.Fatal("missing home mount")
	}
	for _, v := range p.Volumes {
		if v.HostPath != nil {
			t.Fatal("host volume")
		}
		if v.Name == "home" && (v.PersistentVolumeClaim == nil || v.PersistentVolumeClaim.ClaimName != "home") {
			t.Fatal("PVC")
		}
		if v.Name == "tmp" && (v.EmptyDir == nil || v.EmptyDir.SizeLimit.Cmp(resource.MustParse("1Gi")) != 0) {
			t.Fatal("bounded tmp")
		}
	}
	if *p.TerminationGracePeriodSeconds != 60 {
		t.Fatal("grace")
	}
	if c.Image != "docker.io/nousresearch/hermes-agent@"+r.ImageDigest {
		t.Fatal("unpinned image")
	}
	for _, obj := range []metav1.Object{out.StatefulSet, out.Service, out.ServiceAccount, out.ConfigMap, out.Secret, out.Bootstrap} {
		refs := obj.GetOwnerReferences()
		if len(refs) != 1 || refs[0].UID != h.UID || !*refs[0].Controller {
			t.Fatal("wrong owner")
		}
	}
	for _, labels := range []map[string]string{out.StatefulSet.Spec.Selector.MatchLabels, out.StatefulSet.Spec.Template.Labels, out.Service.Spec.Selector} {
		if labels[network.InstallationUIDLabel] != string(h.UID) {
			t.Fatal("network isolation selector")
		}
	}
	if !*out.ConfigMap.Immutable || !*out.Secret.Immutable || !*out.Bootstrap.Immutable || !strings.HasSuffix(out.ConfigMap.Name, out.Revision) || !strings.HasSuffix(out.Secret.Name, out.Revision) || out.StatefulSet.Spec.Template.Annotations[RevisionAnnotation] != out.Revision {
		t.Fatal("revision mismatch")
	}
	h.Spec.Suspend = true
	out, err = Build(h, r, b, "home")
	if err != nil || *out.StatefulSet.Spec.Replicas != 0 {
		t.Fatal("suspend")
	}
}
func TestBuildProbes(t *testing.T) {
	h, r, b := fixture(t)
	out, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	c := out.StatefulSet.Spec.Template.Spec.Containers[0]
	for _, tc := range []struct {
		p            *corev1.Probe
		mode         string
		period, fail int32
	}{{c.StartupProbe, "live", 10, 60}, {c.ReadinessProbe, "ready", 10, 3}, {c.LivenessProbe, "live", 30, 3}} {
		if tc.p == nil || tc.p.PeriodSeconds != tc.period || tc.p.FailureThreshold != tc.fail || tc.p.TimeoutSeconds < 5 || !reflect.DeepEqual(tc.p.Exec.Command, []string{"/opt/hermes/.venv/bin/python", "-I", "/operator/runtime/probe.py", tc.mode}) {
			t.Fatalf("probe mismatch: %+v", tc.p)
		}
	}
}
func TestBuildDefaultsAndCopies(t *testing.T) {
	h, r, b := fixture(t)
	h.Spec.Resources.Requests = v1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")}
	h.Spec.Scheduling.NodeSelector = map[string]string{"disk": "ssd"}
	before := h.DeepCopy()
	out, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	c := out.StatefulSet.Spec.Template.Spec.Containers[0]
	if c.Resources.Requests.Cpu().String() != "250m" || c.Resources.Requests.Memory().String() != "512Mi" || c.Resources.Limits.Memory().String() != "2Gi" {
		t.Fatal("key-level defaults")
	}
	out.StatefulSet.Spec.Template.Spec.NodeSelector["disk"] = "changed"
	out.Secret.Data["token"][0] = 'x'
	if !reflect.DeepEqual(h, before) || string(b.SecretData["token"]) != "fixture-only" {
		t.Fatal("builder aliases inputs")
	}
}

func TestMergedResourceValidation(t *testing.T) {
	for _, tc := range []struct {
		name             string
		requests, limits v1.ResourceList
	}{
		{"request exceeds default limit", v1.ResourceList{corev1.ResourceCPU: resource.MustParse("3")}, nil},
		{"limit below default request", nil, v1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")}},
		{"nonpositive", nil, v1.ResourceList{corev1.ResourceCPU: resource.MustParse("0")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, r, b := fixture(t)
			h.Spec.Resources.Requests = tc.requests
			h.Spec.Resources.Limits = tc.limits
			if _, err := Build(h, r, b, "home"); err == nil {
				t.Fatal("invalid effective resource requirements accepted")
			}
		})
	}
}
