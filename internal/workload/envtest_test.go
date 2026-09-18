package workload

import (
	"context"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// This validates API admission only, not scheduling, mounts or gateway runtime.
func TestWorkloadAdmission(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run API admission")
	}
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	})
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	h, r, b := fixture(t)
	ctx := context.Background()
	if err := k.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}); err != nil {
		t.Fatal(err)
	}
	out, err := Build(h, r, b, "home")
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range []client.Object{out.ServiceAccount, out.Service, out.ConfigMap, out.Secret, out.Bootstrap, out.StatefulSet} {
		if err := k.Create(ctx, obj, &client.CreateOptions{DryRun: []string{metav1.DryRunAll}, FieldValidation: metav1.FieldValidationStrict}); err != nil {
			t.Errorf("%T admission: %v", obj, err)
		}
	}
	// StatefulSet admission doesn't enforce the Pod security admission policy;
	// submit the rendered Pod itself under Restricted enforcement as well.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "restricted-workload", Labels: map[string]string{"pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.34"}}}
	if err := k.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "rendered", Namespace: ns.Name}, Spec: out.StatefulSet.Spec.Template.Spec}
	if err := k.Create(ctx, pod, &client.CreateOptions{DryRun: []string{metav1.DryRunAll}, FieldValidation: metav1.FieldValidationStrict}); err != nil {
		t.Fatal(err)
	}
}
