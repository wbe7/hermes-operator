package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/wbe7/hermes-operator/internal/network"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestApplyAndPreflightFailureCannotRetainRevokedSource(t *testing.T) {
	for _, failure := range []string{"statefulset-apply", "preflight-conflict"} {
		for _, revoke := range []string{"deleted", "empty-key", "replaced-uid"} {
			t.Run(failure+"/"+revoke, func(t *testing.T) {
				r, h := unit(t, true)
				runReconcile(t, r, h)
				before := h.Status.AppliedRevision
				next := source(h)
				next.Name = "next-source"
				next.UID = "next-uid"
				if err := r.Create(ctx, next); err != nil {
					t.Fatal(err)
				}
				h.Spec.Credentials.SecretName = next.Name
				h.Generation++
				if err := r.Update(ctx, h); err != nil {
					t.Fatal(err)
				}
				if failure == "preflight-conflict" {
					svc := &corev1.Service{}
					if err := r.Get(ctx, client.ObjectKey{Namespace: h.Namespace, Name: h.Name + "-hermes"}, svc); err != nil {
						t.Fatal(err)
					}
					svc.OwnerReferences = nil
					if err := r.Update(ctx, svc); err != nil {
						t.Fatal(err)
					}
				} else {
					// A real API may reject an otherwise representable template, e.g. admission
					// policy. Inject that API rejection, permitting the revocation scale-down.
					r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
						Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
							if set, ok := obj.(*appsv1.StatefulSet); ok && ptr.Deref(set.Spec.Replicas, int32(0)) > 0 {
								return apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "statefulsets"}, set.Name, fmt.Errorf("synthetic admission rejection"))
							}
							return c.Patch(ctx, obj, patch, opts...)
						},
					})
				}
				reconcile := func() {
					_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(h)})
					if err := r.Get(ctx, client.ObjectKeyFromObject(h), h); err != nil {
						t.Fatal(err)
					}
				}
				reconcile()
				if *sts(t, r, h).Spec.Replicas != 1 || h.Status.AppliedRevision != before {
					t.Fatal("intact old workload was disturbed")
				}
				old := source(h)
				if err := r.Get(ctx, client.ObjectKeyFromObject(old), old); err != nil {
					t.Fatal(err)
				}
				switch revoke {
				case "empty-key":
					old.Data["MODEL_API_KEY"] = nil
					if err := r.Update(ctx, old); err != nil {
						t.Fatal(err)
					}
				default:
					if err := r.Delete(ctx, old); err != nil {
						t.Fatal(err)
					}
					if revoke == "replaced-uid" {
						old.ResourceVersion = ""
						old.UID = "replacement-uid"
						if err := r.Create(ctx, old); err != nil {
							t.Fatal(err)
						}
					}
				}
				reconcile()
				if *sts(t, r, h).Spec.Replicas != 0 {
					t.Fatal("failed desired apply retained revoked applied source")
				}
				if meta.IsStatusConditionTrue(h.Status.Conditions, "DependenciesReady") {
					t.Fatal("revoked dependency reported Ready")
				}
			})
		}
	}
}

func TestLivePodIsolationDriftIsRejected(t *testing.T) {
	for name, mutate := range map[string]func(*corev1.Pod){
		"missing-selection-label": func(p *corev1.Pod) { delete(p.Labels, network.InstallationUIDLabel) },
		"changed-selection-label": func(p *corev1.Pod) { p.Labels[network.InstallationUIDLabel] = "other" },
		"host-network":            func(p *corev1.Pod) { p.Spec.HostNetwork = true },
		"host-pid":                func(p *corev1.Pod) { p.Spec.HostPID = true },
		"host-ipc":                func(p *corev1.Pod) { p.Spec.HostIPC = true },
		"token-automount":         func(p *corev1.Pod) { p.Spec.AutomountServiceAccountToken = ptr.To(true) },
		"injected-host-volume": func(p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}})
		},
		"changed-volume-source": func(p *corev1.Pod) { p.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "other-home" },
		"writable-input-mount":  func(p *corev1.Pod) { p.Spec.Containers[0].VolumeMounts[2].ReadOnly = false },
		"injected-mount": func(p *corev1.Pod) {
			p.Spec.Containers[0].VolumeMounts = append(p.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "home", MountPath: "/unexpected"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, h := unit(t, true)
			runReconcile(t, r, h)
			set := sts(t, r, h)
			p := &corev1.Pod{ObjectMeta: *set.Spec.Template.ObjectMeta.DeepCopy(), Spec: *set.Spec.Template.Spec.DeepCopy(), Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
			p.Name = set.Name + "-0"
			p.Namespace = h.Namespace
			p.Finalizers = []string{"test/hold"}
			p.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}
			mutate(p)
			if err := r.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			if err := r.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				t.Fatal(err)
			}
			if p.DeletionTimestamp.IsZero() || meta.IsStatusConditionTrue(h.Status.Conditions, "Ready") {
				t.Fatal("isolation drift retained or marked Ready")
			}
		})
	}
}
