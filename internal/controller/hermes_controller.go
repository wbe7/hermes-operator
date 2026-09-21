// Package controller reconciles namespace-local Hermes resources.
package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	"github.com/wbe7/hermes-operator/internal/network"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	"github.com/wbe7/hermes-operator/internal/settings"
	"github.com/wbe7/hermes-operator/internal/workload"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const Finalizer = "hermes.wbe7.github.io/storage-protection"
const deletionPolicyAnnotation = "hermes.wbe7.github.io/deletion-policy"
const retryDelay = 10 * time.Second

type HermesReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
	Network   settings.NetworkConfig
}
type conditionError struct{ reason, message string }

func (e *conditionError) Error() string    { return e.message }
func problem(reason, message string) error { return &conditionError{reason, message} }

// API errors can include submitted fields, including Secret bytes. Only expose
// the operation and Kubernetes reason to status, logs and the retry machinery.
func apiFailure(operation string, err error) error {
	return fmt.Errorf("%s failed (%s)", operation, apierrors.ReasonForError(err))
}
func (r *HermesReconciler) reader() client.Reader { return r.APIReader }
func owned(obj client.Object, h *v1.Hermes) bool {
	ref := metav1.GetControllerOf(obj)
	return ref != nil && ref.UID == h.UID && ref.APIVersion == v1.GroupVersion.String() && ref.Kind == "Hermes" && ref.Name == h.Name
}
func checkOwned(obj client.Object, h *v1.Hermes) error {
	if !owned(obj, h) {
		return problem("ResourceConflict", "a resource with the required name belongs to another owner")
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return problem("ResourceConflict", "a required resource is terminating")
	}
	return nil
}
func (r *HermesReconciler) deleteUID(ctx context.Context, obj client.Object) error {
	uid := obj.GetUID()
	if uid == "" {
		return problem("ResourceConflict", "refusing deletion without a UID")
	}
	if err := r.Delete(ctx, obj, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
		return apiFailure("delete resource", err)
	}
	return nil
}

// +kubebuilder:rbac:groups=hermes.wbe7.github.io,resources=hermes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=hermes.wbe7.github.io,resources=hermes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hermes.wbe7.github.io,resources=hermes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps;secrets;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

func (r *HermesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reconcileErr error) {
	if r.APIReader == nil {
		return ctrl.Result{}, errors.New("uncached APIReader is required")
	}
	h := &v1.Hermes{}
	if err := r.reader().Get(ctx, req.NamespacedName, h); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, apiFailure("read Hermes", err)
	}
	if !h.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, h)
	}
	// Every exit that can retain an applied workload, including failed partial
	// applies and status writes, must enforce revocation of its actual sources.
	// Inspect after apply so valid replacements can recover from revoked sources;
	// old Pods are checked independently of the new StatefulSet template.
	defer func() {
		if !h.Spec.Suspend {
			if err := r.checkAppliedDependencies(ctx, h); err != nil {
				result, reconcileErr = r.stopAndFail(ctx, h, "DependenciesReady", err)
			}
		}
	}()
	if !controllerutil.ContainsFinalizer(h, Finalizer) {
		base := h.DeepCopy()
		controllerutil.AddFinalizer(h, Finalizer)
		if err := r.Patch(ctx, h, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			return ctrl.Result{}, apiFailure("protect Hermes", err)
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}
	if h.Spec.Suspend {
		_, err := r.stop(ctx, h)
		if err != nil {
			return r.failed(ctx, h, "Ready", err)
		}
		setCondition(h, "Suspended", metav1.ConditionTrue, "Suspended", "installation is suspended")
		setCondition(h, "Ready", metav1.ConditionFalse, "Suspended", "installation is suspended")
		return r.saveStatus(ctx, h)
	}
	setCondition(h, "Suspended", metav1.ConditionFalse, "Reconciled", "installation is active")
	secrets, err := r.secrets(ctx, h)
	if err != nil {
		return r.stopAndFail(ctx, h, "DependenciesReady", err)
	}
	release, err := runtimecatalog.Resolve(h.Spec.Version)
	if err != nil {
		return r.configurationFailed(ctx, h, problem("UnsupportedVersion", "Hermes version is not supported"))
	}
	if err = config.Validate(h, release); err != nil {
		return r.configurationFailed(ctx, h, problem("InvalidConfiguration", "configuration is invalid"))
	}
	policy, err := network.Build(h, r.Network)
	if err != nil {
		return r.configurationFailed(ctx, h, problem("InvalidConfiguration", "network configuration is invalid"))
	}
	setCondition(h, "ConfigurationReady", metav1.ConditionTrue, "Reconciled", "configuration is valid")
	bundle, err := config.Render(h, release, secrets)
	if err != nil {
		var missing *config.DependencyError
		if errors.As(err, &missing) {
			return r.stopAndFail(ctx, h, "DependenciesReady", problem("DependencyKeyMissing", "a required Secret key is missing or empty"))
		}
		return r.configurationFailed(ctx, h, problem("InvalidConfiguration", "configuration cannot be rendered"))
	}
	setCondition(h, "DependenciesReady", metav1.ConditionTrue, "Reconciled", "referenced credentials are available")
	resources, err := workload.Build(h, release, bundle, claimName(h))
	if err != nil {
		return r.configurationFailed(ctx, h, problem("InvalidConfiguration", "workload configuration is invalid"))
	}
	// Source identities are nonsecret metadata attached to the immutable
	// snapshot. They remain available when a later desired spec changes refs.
	resources.Secret.Annotations = map[string]string{sourceRefsAnnotation: encodeSourceRefs(h, secrets)}
	// Validate storage and all generated object identities before any partial apply.
	if err = r.validateStorage(ctx, h); err != nil {
		return r.stopAndFail(ctx, h, "StorageReady", err)
	}
	objects := []client.Object{policy, resources.ServiceAccount, resources.Service, resources.ConfigMap, resources.Secret, resources.Bootstrap, resources.StatefulSet}
	for _, obj := range objects {
		if err = r.preflight(ctx, h, obj); err != nil {
			return r.failed(ctx, h, "Ready", err)
		}
	}
	if err = r.ensureStorage(ctx, h); err != nil {
		return r.stopAndFail(ctx, h, "StorageReady", err)
	}
	// Persist PVC identity before a workload can use it; a crash is reconstructible
	// from the creation annotation and cannot authorize a replacement claim delete.
	if _, err = r.saveStatus(ctx, h); err != nil {
		return ctrl.Result{}, err
	}
	if err = r.apply(ctx, h, policy); err != nil {
		return r.stopAndFail(ctx, h, "NetworkPolicyReady", err)
	}
	setCondition(h, "NetworkPolicyReady", metav1.ConditionTrue, "Reconciled", "isolation policy is applied")
	for _, obj := range objects[1:] {
		if err = r.apply(ctx, h, obj); err != nil {
			return r.failed(ctx, h, "Ready", err)
		}
	}
	current := &appsv1.StatefulSet{}
	if err = r.reader().Get(ctx, client.ObjectKeyFromObject(resources.StatefulSet), current); err != nil {
		return ctrl.Result{}, apiFailure("read workload", err)
	}
	if err = checkOwned(current, h); err != nil {
		return r.failed(ctx, h, "Ready", err)
	}
	h.Status.AppliedRevision = resources.Revision
	h.Status.ResolvedImage = resources.StatefulSet.Spec.Template.Spec.Containers[0].Image
	h.Status.WorkloadRef = &v1.ObjectReference{Name: current.Name, UID: string(current.UID)}
	if err = r.observePods(ctx, h, current, resources.Revision); err != nil {
		return r.failed(ctx, h, "Ready", err)
	}
	if err = r.collectRevisions(ctx, h, current); err != nil {
		return r.failed(ctx, h, "Ready", err)
	}
	return r.saveStatus(ctx, h)
}

func (r *HermesReconciler) preflight(ctx context.Context, h *v1.Hermes, desired client.Object) error {
	current := desired.DeepCopyObject().(client.Object)
	err := r.reader().Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return apiFailure("inspect resource", err)
	}
	if err = checkOwned(current, h); err != nil {
		return err
	}
	switch want := desired.(type) {
	case *corev1.Secret:
		have := current.(*corev1.Secret)
		if !equality.Semantic.DeepEqual(want.Data, have.Data) || have.Immutable == nil || !*have.Immutable {
			return problem("ResourceConflict", "immutable credential revision differs from expected content")
		}
	case *corev1.ConfigMap:
		have := current.(*corev1.ConfigMap)
		if !equality.Semantic.DeepEqual(want.Data, have.Data) || len(have.BinaryData) != 0 || have.Immutable == nil || !*have.Immutable {
			return problem("ResourceConflict", "immutable configuration revision differs from expected content")
		}
	}
	return nil
}
func (r *HermesReconciler) apply(ctx context.Context, h *v1.Hermes, desired client.Object) error {
	current := desired.DeepCopyObject().(client.Object)
	err := r.reader().Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if err = r.Create(ctx, desired); err != nil {
			return apiFailure("create resource", err)
		}
		return nil
	}
	if err != nil {
		return apiFailure("read resource", err)
	}
	if err = checkOwned(current, h); err != nil {
		return err
	}
	base := current.DeepCopyObject().(client.Object)
	switch want := desired.(type) {
	case *corev1.ConfigMap:
		return r.preflight(ctx, h, desired)
	case *corev1.Secret:
		if err = r.preflight(ctx, h, desired); err != nil {
			return err
		}
		if current.GetAnnotations() == nil {
			current.SetAnnotations(map[string]string{})
		}
		current.GetAnnotations()[sourceRefsAnnotation] = want.Annotations[sourceRefsAnnotation]
	case *appsv1.StatefulSet:
		have := current.(*appsv1.StatefulSet)
		have.Spec.Replicas = ptr.To(*want.Spec.Replicas)
		if have.Spec.Template.Annotations[workload.RevisionAnnotation] != want.Spec.Template.Annotations[workload.RevisionAnnotation] || !templateMatches(want.Spec.Template, have.Spec.Template) {
			have.Spec.Template = *want.Spec.Template.DeepCopy()
		}
	case *networkingv1.NetworkPolicy:
		current.(*networkingv1.NetworkPolicy).Spec = *want.Spec.DeepCopy()
	case *corev1.Service:
		have := current.(*corev1.Service)
		if have.Spec.ClusterIP != corev1.ClusterIPNone {
			return problem("ResourceConflict", "workload Service is not headless")
		}
		have.Spec.Selector = want.Spec.Selector
		have.Spec.Ports = want.Spec.Ports
	case *corev1.ServiceAccount:
		current.(*corev1.ServiceAccount).AutomountServiceAccountToken = ptr.To(false)
	}
	current.SetLabels(desired.GetLabels())
	if equality.Semantic.DeepEqual(base, current) {
		return nil
	}
	if err = r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return apiFailure("patch resource", err)
	}
	return nil
}

// DeepDerivative ignores server default fields, but also accepts extra slices
// and maps. Check operator-controlled optional fields and lengths explicitly.
func templateMatches(want, have corev1.PodTemplateSpec) bool {
	w, a := want.Spec, have.Spec
	if !podSecurityMatches(w, a) {
		return false
	}
	if len(w.Containers) != len(a.Containers) || len(w.InitContainers) != len(a.InitContainers) || len(w.Volumes) != len(a.Volumes) || len(w.ImagePullSecrets) != len(a.ImagePullSecrets) || !equality.Semantic.DeepEqual(w.NodeSelector, a.NodeSelector) || !equality.Semantic.DeepEqual(w.Affinity, a.Affinity) || !equality.Semantic.DeepEqual(w.Tolerations, a.Tolerations) || w.NodeName != a.NodeName || len(w.HostAliases) != len(a.HostAliases) {
		return false
	}
	for i, c := range w.Containers {
		actual := a.Containers[i]
		if len(c.Env) != len(actual.Env) || len(c.EnvFrom) != len(actual.EnvFrom) || len(c.VolumeMounts) != len(actual.VolumeMounts) || len(c.VolumeDevices) != len(actual.VolumeDevices) || len(c.Ports) != len(actual.Ports) || !equality.Semantic.DeepEqual(c.Resources, actual.Resources) {
			return false
		}
	}
	return equality.Semantic.DeepDerivative(want, have)
}

// These defaults are semantically defined by the Kubernetes API; all other
// security fields, including absent fields that override Pod inheritance, are
// compared exactly. Never use DeepDerivative for a securityContext.
func normalizeContainerSecurity(in *corev1.SecurityContext) *corev1.SecurityContext {
	out := in.DeepCopy()
	if out == nil {
		out = &corev1.SecurityContext{}
	}
	if out.Privileged == nil {
		out.Privileged = ptr.To(false)
	}
	if out.ProcMount == nil {
		out.ProcMount = ptr.To(corev1.DefaultProcMount)
	}
	return out
}
func normalizePodSecurity(in *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	out := in.DeepCopy()
	if out == nil {
		out = &corev1.PodSecurityContext{}
	}
	if out.SupplementalGroupsPolicy == nil {
		out.SupplementalGroupsPolicy = ptr.To(corev1.SupplementalGroupsPolicyMerge)
	}
	return out
}

// Live Pods have node assignment and other non-template fields. Compare only
// the controlled security contexts here, also for a Pod carrying the expected
// revision annotation: annotations alone cannot prove effective isolation.
func podSecurityMatches(want, have corev1.PodSpec) bool {
	if !equality.Semantic.DeepEqual(normalizePodSecurity(want.SecurityContext), normalizePodSecurity(have.SecurityContext)) {
		return false
	}
	for _, pair := range [][2][]corev1.Container{{want.Containers, have.Containers}, {want.InitContainers, have.InitContainers}} {
		if len(pair[0]) != len(pair[1]) {
			return false
		}
		for i, c := range pair[0] {
			if c.Name != pair[1][i].Name || !equality.Semantic.DeepEqual(normalizeContainerSecurity(c.SecurityContext), normalizeContainerSecurity(pair[1][i].SecurityContext)) {
				return false
			}
		}
	}
	return true
}

// Live isolation ignores node assignment, default tolerations and other scheduling
// fields, but never accepts admission-added access to host or credential volumes.
func podIsolationMatches(want, have corev1.PodSpec) bool {
	if !podSecurityMatches(want, have) ||
		want.HostNetwork != have.HostNetwork || want.HostPID != have.HostPID || want.HostIPC != have.HostIPC ||
		ptr.Deref(want.ShareProcessNamespace, false) != ptr.Deref(have.ShareProcessNamespace, false) ||
		ptr.Deref(want.HostUsers, true) != ptr.Deref(have.HostUsers, true) ||
		ptr.Deref(want.AutomountServiceAccountToken, true) != ptr.Deref(have.AutomountServiceAccountToken, true) ||
		ptr.Deref(want.EnableServiceLinks, true) != ptr.Deref(have.EnableServiceLinks, true) ||
		want.ServiceAccountName != have.ServiceAccountName ||
		len(have.EphemeralContainers) != 0 ||
		!equality.Semantic.DeepEqual(want.Volumes, have.Volumes) {
		return false
	}
	for _, pair := range [][2][]corev1.Container{{want.Containers, have.Containers}, {want.InitContainers, have.InitContainers}} {
		for i, c := range pair[0] {
			actual := pair[1][i]
			if !equality.Semantic.DeepEqual(c.VolumeMounts, actual.VolumeMounts) ||
				!equality.Semantic.DeepEqual(c.VolumeDevices, actual.VolumeDevices) ||
				!equality.Semantic.DeepEqual(c.Ports, actual.Ports) {
				return false
			}
		}
	}
	return true
}
