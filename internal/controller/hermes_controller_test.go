package controller

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"strings"
	"testing"
	"time"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/settings"
	"github.com/wbe7/hermes-operator/internal/testfixtures"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var ctx = context.Background()

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{v1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme, networkingv1.AddToScheme, storagev1.AddToScheme} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	return s
}
func networkConfig() settings.NetworkConfig {
	return settings.NetworkConfig{EnforcementConfirmed: true, PodCIDRs: []string{"10.42.0.0/16"}, ServiceCIDRs: []string{"10.43.0.0/16"}, NodeCIDRs: []string{"192.168.1.0/24"}, InfrastructureCIDRs: []string{}, DNS: settings.DNSConfig{ResolverIPs: []string{"10.43.0.10"}}}
}
func source(h *v1.Hermes) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: h.Name + "-hermes-secret", Namespace: h.Namespace, UID: "source-uid"}, Data: map[string][]byte{"TELEGRAM_BOT_TOKEN": []byte("fake-token"), "MODEL_API_KEY": []byte("fake-key")}}
}
func unit(t *testing.T, secrets bool) (*HermesReconciler, *v1.Hermes) {
	t.Helper()
	h := testfixtures.Hermes(t)
	h.Generation = 1
	s := scheme(t)
	b := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v1.Hermes{}, &corev1.Pod{}, &corev1.PersistentVolumeClaim{}).WithObjects(h)
	if secrets {
		b.WithObjects(source(h))
	}
	b.WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if obj.GetUID() == "" {
			obj.SetUID(types.UID("fake-" + obj.GetName()))
		}
		return c.Create(ctx, obj, opts...)
	}})
	k := b.Build()
	return &HermesReconciler{Client: k, APIReader: k, Scheme: s, Network: networkConfig()}, h
}
func runReconcile(t *testing.T, r *HermesReconciler, h *v1.Hermes) {
	t.Helper()
	for i := 0; i < 4; i++ {
		if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
		t.Fatal(err)
	}
}
func sts(t *testing.T, r *HermesReconciler, h *v1.Hermes) *appsv1.StatefulSet {
	t.Helper()
	s := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, s); err != nil {
		t.Fatal(err)
	}
	return s
}
func reason(t *testing.T, h *v1.Hermes, typ, want string) {
	t.Helper()
	c := meta.FindStatusCondition(h.Status.Conditions, typ)
	if c == nil || c.Reason != want || c.ObservedGeneration != h.Generation {
		t.Fatalf("condition %s: %+v want %s generation %d", typ, c, want, h.Generation)
	}
}

// Catches startup without credentials and workload creation before policy succeeds.
func TestDependencyLifecycle(t *testing.T) {
	r, h := unit(t, false)
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyNotFound")
	s := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, s); !apierrors.IsNotFound(err) {
		t.Fatalf("workload started: %v", err)
	}
	if err := r.Create(ctx, source(h)); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("not resumed")
	}
	p := &networkingv1.NetworkPolicy{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(sts(t, r, h)), p); err != nil {
		t.Fatal(err)
	}
	if h.Status.StorageRef == nil || h.Status.AppliedRevision == "" {
		t.Fatal("missing durable identity")
	}
}

func TestEnvtestDependencyLifecycle(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS required")
	}
	e := &envtest.Environment{CRDDirectoryPaths: []string{"../../config/crd/bases"}, ErrorIfCRDPathMissing: true}
	cfg, err := e.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Stop(); err != nil {
			t.Error(err)
		}
	})
	s := scheme(t)
	k, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		t.Fatal(err)
	}
	h := testfixtures.Hermes(t)
	h.UID = ""
	if err := k.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}); err != nil {
		t.Fatal(err)
	}
	if err := k.Create(ctx, h); err != nil {
		t.Fatal(err)
	}
	r := &HermesReconciler{Client: k, APIReader: k, Scheme: s, Network: networkConfig()}
	runReconcile(t, r, h)
	reason(t, h, "DependenciesReady", "DependencyNotFound")
	sec := source(h)
	sec.UID = ""
	if err := k.Create(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	set := sts(t, r, h)
	if set.UID == "" || !metav1.IsControlledBy(set, h) || h.Status.StorageRef.UID == "" {
		t.Fatal("missing real API ownership")
	}
	rv := set.ResourceVersion
	runReconcile(t, r, h)
	if sts(t, r, h).ResourceVersion != rv {
		t.Fatal("metadata-only reconciliation rewrote workload")
	}
	old := h.DeepCopy()
	h.Spec.Model.Name = "updated"
	if err := k.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := k.Update(ctx, old); !apierrors.IsConflict(err) {
		t.Fatalf("expected real resourceVersion conflict: %v", err)
	}
	runReconcile(t, r, h)
	realSecurityAndMetadataRepair(t, r, h)
	realFinalization(t, r, h)
}

func TestPolicyFailurePreventsWorkload(t *testing.T) {
	r, h := unit(t, true)
	underlying := r.Client
	r.Client = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if _, ok := obj.(*networkingv1.NetworkPolicy); ok {
			return apierrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, obj.GetName(), fmt.Errorf("private upstream response"))
		}
		return c.Create(ctx, obj, opts...)
	}})
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
		t.Fatal(err)
	}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)})
	if err == nil || strings.Contains(err.Error(), "private upstream") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
	set := &appsv1.StatefulSet{}
	if err := underlying.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, set); !apierrors.IsNotFound(err) {
		t.Fatal("started workload despite policy failure")
	}
	r.Client = underlying
	runReconcile(t, r, h)
	if *sts(t, r, h).Spec.Replicas != 1 {
		t.Fatal("failed to recover")
	}
}

func TestUncachedSecretsAndOwnershipConflicts(t *testing.T) {
	r, h := unit(t, true)
	underlying := r.Client
	r.Client = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		if _, ok := obj.(*corev1.Secret); ok {
			t.Fatal("typed Secret read through cached client")
		}
		return c.Get(ctx, key, obj, opts...)
	}})
	foreign := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: h.Name + "-hermes", Namespace: h.Namespace, UID: "foreign"}}
	if err := r.Create(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	reason(t, h, "Ready", "ResourceConflict")
	claims := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, claims); err != nil {
		t.Fatal(err)
	}
	if len(claims.Items) != 0 {
		t.Fatal("partial apply before conflict validation")
	}
	if err := r.Delete(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if h.Status.AppliedRevision == "" {
		t.Fatal("did not recover")
	}
}

func TestPatchConflictRecoversWithoutDuplicateClaim(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	uid := h.Status.StorageRef.UID
	h.Spec.Model.Name = "changed"
	h.Generation++
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	underlying := r.Client
	fail := true
	r.Client = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
		if _, ok := obj.(*appsv1.StatefulSet); ok && fail {
			fail = false
			return apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "statefulsets"}, obj.GetName(), fmt.Errorf("simulated concurrent writer"))
		}
		return c.Patch(ctx, obj, patch, opts...)
	}})
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err == nil {
		t.Fatal("expected retryable conflict")
	}
	runReconcile(t, r, h)
	if h.Status.StorageRef.UID != uid {
		t.Fatal("claim identity changed after conflict")
	}
	claims := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, claims); err != nil {
		t.Fatal(err)
	}
	if len(claims.Items) != 1 {
		t.Fatal("duplicate claims")
	}
}

// Real manager/cache/watch/Lease exercise: the second process must reconstruct
// state and Secret watch must remain metadata-only across leader restarts.
func TestEnvtestManagerRestart(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS required")
	}
	ctrl.SetLogger(logr.Discard())
	e := &envtest.Environment{CRDDirectoryPaths: []string{"../../config/crd/bases"}, ErrorIfCRDPathMissing: true}
	cfg, err := e.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Stop(); err != nil {
			t.Error(err)
		}
	})
	s := scheme(t)
	k, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		t.Fatal(err)
	}
	h := testfixtures.Hermes(t)
	h.UID = ""
	if err := k.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}); err != nil {
		t.Fatal(err)
	}
	if err := k.Create(ctx, h); err != nil {
		t.Fatal(err)
	}
	start := func() (ctrl.Manager, func()) {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{Controller: controllerconfig.Controller{SkipNameValidation: ptr.To(true)}, Scheme: s, Cache: cache.Options{ReaderFailOnMissingInformer: true}, Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0", LeaderElection: true, LeaderElectionID: "test-hermes", LeaderElectionNamespace: h.Namespace, LeaderElectionReleaseOnCancel: true, LeaseDuration: ptr.To(3 * time.Second), RenewDeadline: ptr.To(2 * time.Second), RetryPeriod: ptr.To(300 * time.Millisecond)})
		if err != nil {
			t.Fatal(err)
		}
		r := &HermesReconciler{Network: networkConfig()}
		if err := r.SetupWithManager(mgr); err != nil {
			t.Fatal(err)
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- mgr.Start(runCtx) }()
		select {
		case <-mgr.Elected():
		case <-time.After(10 * time.Second):
			cancel()
			t.Fatal("leader not elected")
		}
		stop := func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("manager did not stop")
			}
		}
		return mgr, stop
	}
	waitFor := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("controller did not converge")
	}
	mgr, stop := start()
	waitFor(func() bool {
		if err := k.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
			return false
		}
		c := meta.FindStatusCondition(h.Status.Conditions, "DependenciesReady")
		return c != nil && c.Reason == "DependencyNotFound"
	})
	sec := source(h)
	sec.UID = ""
	if err := k.Create(ctx, sec); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool {
		if err := k.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
			return false
		}
		return h.Status.AppliedRevision != ""
	})
	if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(sec), &corev1.Secret{}); err == nil {
		t.Fatal("typed Secret informer was accidentally installed")
	}
	revision, uid := h.Status.AppliedRevision, h.Status.StorageRef.UID
	stop()
	// Change while no leader exists; startup enqueue alone must recover.
	if err := k.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
		t.Fatal(err)
	}
	sec.Data["MODEL_API_KEY"] = []byte("restart-key")
	if err := k.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	_, stop = start()
	defer stop()
	waitFor(func() bool {
		if err := k.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
			return false
		}
		return h.Status.AppliedRevision != revision
	})
	if h.Status.StorageRef.UID != uid {
		t.Fatal("restart replaced PVC")
	}
	// Desired refs move away from the running snapshot while the spec is invalid.
	// The old source event must still enqueue this CR through revision metadata.
	next := source(h)
	next.Name = "next-source"
	next.UID = ""
	if err := k.Create(ctx, next); err != nil {
		t.Fatal(err)
	}
	h.Spec.Credentials.SecretName = next.Name
	h.Spec.Version = "unsupported"
	if err := k.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool {
		if err := k.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
			return false
		}
		c := meta.FindStatusCondition(h.Status.Conditions, "ConfigurationReady")
		return c != nil && c.ObservedGeneration == h.Generation && c.Reason == "UnsupportedVersion"
	})
	if err := k.Delete(ctx, sec); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool {
		set := &appsv1.StatefulSet{}
		return k.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, set) == nil && set.Spec.Replicas != nil && *set.Spec.Replicas == 0
	})
}

func TestWorkloadRepairsAddedContainersAndScheduling(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	set.Spec.Template.Spec.Containers = append(set.Spec.Template.Spec.Containers, corev1.Container{Name: "unexpected", Image: "unexpected"})
	set.Spec.Template.Spec.NodeSelector = map[string]string{"unexpected": "node"}
	if err := r.Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	set = sts(t, r, h)
	if len(set.Spec.Template.Spec.Containers) != 1 || len(set.Spec.Template.Spec.NodeSelector) != 0 {
		t.Fatal("owned workload drift was not repaired")
	}
}

func TestGenerationChangeDuringReconcileCannotStartStaleWorkload(t *testing.T) {
	r, h := unit(t, true)
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)}); err != nil {
		t.Fatal(err)
	}
	underlying := r.Client
	reads := 0
	r.APIReader = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		if _, ok := obj.(*v1.Hermes); ok {
			reads++
			if reads == 2 {
				changed := &v1.Hermes{}
				if err := c.Get(ctx, key, changed); err != nil {
					return err
				}
				changed.Spec.Suspend = true
				changed.Generation++
				if err := c.Update(ctx, changed); err != nil {
					return err
				}
			}
		}
		return c.Get(ctx, key, obj, opts...)
	}})
	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)})
	set := &appsv1.StatefulSet{}
	if err := underlying.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.Name + "-hermes"}, set); !apierrors.IsNotFound(err) {
		t.Fatal("stale reconcile started workload after observing changed generation")
	}
}

func TestDependencyIndexOnlyEnqueuesAffectedNamespace(t *testing.T) {
	h := testfixtures.Hermes(t)
	h.Spec.Credentials.SecretName = "shared"
	other := h.DeepCopy()
	other.Name = "other"
	other.UID = "other"
	other.Spec.Credentials.SecretName = "different"
	remote := h.DeepCopy()
	remote.Namespace = "another"
	remote.UID = "remote"
	s := scheme(t)
	k := fake.NewClientBuilder().WithScheme(s).WithObjects(h, other, remote).WithIndex(&v1.Hermes{}, secretIndex, secretNames).Build()
	r := &HermesReconciler{Client: k, APIReader: k}
	requests := r.mapDependency(secretIndex)(ctx, &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: h.Namespace}})
	if len(requests) != 1 || requests[0].NamespacedName != client.ObjectKeyFromObject(h) {
		t.Fatalf("unrelated installations enqueued: %v", requests)
	}
}

func TestNetworkOnlyUpdateDoesNotRollWorkload(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	revision := h.Status.AppliedRevision
	key := client.ObjectKeyFromObject(sts(t, r, h))
	before := &networkingv1.NetworkPolicy{}
	if err := r.Get(ctx, key, before); err != nil {
		t.Fatal(err)
	}
	r.Network.InfrastructureCIDRs = []string{"8.8.8.0/24"}
	runReconcile(t, r, h)
	after := &networkingv1.NetworkPolicy{}
	if err := r.Get(ctx, key, after); err != nil {
		t.Fatal(err)
	}
	if equality.Semantic.DeepEqual(before.Spec, after.Spec) {
		t.Fatal("network configuration was not applied")
	}
	if h.Status.AppliedRevision != revision {
		t.Fatal("network-only change rolled workload")
	}
}

func TestSecurityContextDriftIsRepaired(t *testing.T) {
	cases := map[string]func(*corev1.PodSpec){
		"root-container":   func(p *corev1.PodSpec) { p.Containers[0].SecurityContext.RunAsUser = ptr.To(int64(0)) },
		"nonroot-disabled": func(p *corev1.PodSpec) { p.Containers[0].SecurityContext.RunAsNonRoot = ptr.To(false) },
		"unconfined-container": func(p *corev1.PodSpec) {
			p.Containers[0].SecurityContext.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}
		},
		"added-capability": func(p *corev1.PodSpec) {
			p.Containers[0].SecurityContext.Capabilities.Add = []corev1.Capability{"SYS_ADMIN"}
		},
		"privileged-container": func(p *corev1.PodSpec) { p.Containers[0].SecurityContext.Privileged = ptr.To(true) },
		"unmasked-proc":        func(p *corev1.PodSpec) { p.Containers[0].SecurityContext.ProcMount = ptr.To(corev1.UnmaskedProcMount) },
		"pod-sysctl": func(p *corev1.PodSpec) {
			p.SecurityContext.Sysctls = []corev1.Sysctl{{Name: "net.ipv4.ip_forward", Value: "1"}}
		},
		"pod-added-groups": func(p *corev1.PodSpec) { p.SecurityContext.SupplementalGroups = []int64{0} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r, h := unit(t, true)
			runReconcile(t, r, h)
			set := sts(t, r, h)
			desired := set.Spec.Template.Spec.DeepCopy()
			mutate(&set.Spec.Template.Spec)
			if err := r.Update(ctx, set); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			got := sts(t, r, h).Spec.Template.Spec
			if !equality.Semantic.DeepEqual(desired.SecurityContext, got.SecurityContext) || !equality.Semantic.DeepEqual(desired.Containers[0].SecurityContext, got.Containers[0].SecurityContext) {
				t.Fatal("unsafe securityContext drift survived reconciliation")
			}
		})
	}
}

func TestLegitimateSecurityDefaultsAreIdempotent(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	set.Spec.Template.Spec.Containers[0].SecurityContext.Privileged = ptr.To(false)
	set.Spec.Template.Spec.Containers[0].SecurityContext.ProcMount = ptr.To(corev1.DefaultProcMount)
	set.Spec.Template.Spec.SecurityContext.SupplementalGroupsPolicy = ptr.To(corev1.SupplementalGroupsPolicyMerge)
	if err := r.Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	rv := set.ResourceVersion
	runReconcile(t, r, h)
	if sts(t, r, h).ResourceVersion != rv {
		t.Fatal("legitimate default values caused repeated template patch")
	}
}

func TestBackfillDependencyMetadataDoesNotRollWorkload(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	rv, revision := set.ResourceVersion, h.Status.AppliedRevision
	sec := &corev1.Secret{}
	key := types.NamespacedName{Namespace: h.Namespace, Name: set.Spec.Template.Spec.Volumes[3].Secret.SecretName}
	if err := r.APIReader.Get(ctx, key, sec); err != nil {
		t.Fatal(err)
	}
	data := sec.DeepCopy().Data
	sec.Annotations = nil
	if err := r.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if sts(t, r, h).ResourceVersion != rv || h.Status.AppliedRevision != revision {
		t.Fatal("backfilling provenance rolled workload")
	}
	if err := r.APIReader.Get(ctx, key, sec); err != nil {
		t.Fatal(err)
	}
	if sec.Annotations[sourceRefsAnnotation] == "" || !equality.Semantic.DeepEqual(data, sec.Data) {
		t.Fatal("backfill missing or altered credential snapshot")
	}
	if strings.Contains(sec.Annotations[sourceRefsAnnotation], "fake-token") || strings.Contains(sec.Annotations[sourceRefsAnnotation], "fake-key") {
		t.Fatal("secret values entered metadata")
	}
}

func realSecurityAndMetadataRepair(t *testing.T, r *HermesReconciler, h *v1.Hermes) {
	t.Helper()
	set := sts(t, r, h)
	set.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = ptr.To(int64(0))
	set.Spec.Template.Spec.Containers[0].SecurityContext.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}
	set.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Add = []corev1.Capability{"SYS_ADMIN"}
	if err := r.Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	set = sts(t, r, h)
	sc := set.Spec.Template.Spec.Containers[0].SecurityContext
	if sc.RunAsUser != nil || sc.SeccompProfile != nil || len(sc.Capabilities.Add) != 0 {
		t.Fatal("API-admitted security drift was not repaired")
	}
	rv := set.ResourceVersion
	snapshot := &corev1.Secret{}
	key := types.NamespacedName{Namespace: h.Namespace, Name: set.Spec.Template.Spec.Volumes[3].Secret.SecretName}
	if err := r.APIReader.Get(ctx, key, snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Annotations = nil
	if err := r.Update(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if sts(t, r, h).ResourceVersion != rv {
		t.Fatal("security defaults or metadata backfill rewrote StatefulSet")
	}
	if err := r.APIReader.Get(ctx, key, snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Annotations[sourceRefsAnnotation] == "" {
		t.Fatal("existing snapshot source refs were not reconstructed")
	}
}

func TestAppliedDependencyIndexSurvivesDesiredRefChange(t *testing.T) {
	h := testfixtures.Hermes(t)
	h.Spec.Credentials.SecretName = "next"
	revision := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: h.Name + "-sec-previous", Namespace: h.Namespace, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(h, v1.GroupVersion.WithKind("Hermes"))}, Annotations: map[string]string{sourceRefsAnnotation: `[{"name":"old","key":"MODEL_API_KEY","uid":"old-uid"}]`}}}
	s := scheme(t)
	k := fake.NewClientBuilder().WithScheme(s).WithObjects(h, revision).WithIndex(&v1.Hermes{}, secretIndex, secretNames).WithIndex(&corev1.Secret{}, revisionSourceIndex, revisionSourceNames).Build()
	r := &HermesReconciler{Client: k, APIReader: k}
	requests := r.mapDependency(secretIndex)(ctx, &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: h.Namespace}})
	if len(requests) != 1 || requests[0].NamespacedName != client.ObjectKeyFromObject(h) {
		t.Fatalf("applied source did not enqueue its installation: %v", requests)
	}
}
