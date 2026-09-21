package controller

import (
	"testing"

	"github.com/wbe7/hermes-operator/internal/network"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestInvalidSpecRetainsWorkloadOnlyWithIntactNetworkPolicy(t *testing.T) {
	for _, drift := range []string{"intact", "missing", "allow-all", "selector", "foreign", "terminating", "invalid-network"} {
		t.Run(drift, func(t *testing.T) {
			r, h := unit(t, true)
			runReconcile(t, r, h)
			original := h.Spec.DeepCopy()
			set := sts(t, r, h)
			homeUID := h.Status.StorageRef.UID
			pod := &corev1.Pod{ObjectMeta: *set.Spec.Template.ObjectMeta.DeepCopy(), Spec: *set.Spec.Template.Spec.DeepCopy(), Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
			pod.Name = set.Name + "-0"
			pod.Namespace = h.Namespace
			pod.Finalizers = []string{"test/hold"}
			pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))}
			if err := r.Create(ctx, pod); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			if !meta.IsStatusConditionTrue(h.Status.Conditions, "Ready") {
				t.Fatal("baseline not ready")
			}
			policy := &networkingv1.NetworkPolicy{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(set), policy); err != nil {
				t.Fatal(err)
			}
			h.Spec.Version = "unsupported"
			h.Generation++
			switch drift {
			case "allow-all":
				policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
			case "selector":
				policy.Spec.PodSelector.MatchLabels[network.InstallationUIDLabel] = "other"
			case "foreign":
				policy.OwnerReferences = nil
			case "terminating":
				policy.Finalizers = []string{"test/hold"}
			case "invalid-network":
				h.Spec.Network.AdditionalBlockedCIDRs = []string{"invalid"}
			}
			if err := r.Update(ctx, policy); err != nil {
				t.Fatal(err)
			}
			if drift == "missing" || drift == "terminating" {
				if err := r.Delete(ctx, policy); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Update(ctx, h); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			if err := r.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
				t.Fatal(err)
			}
			if drift == "intact" {
				if *sts(t, r, h).Spec.Replicas != 1 || !pod.DeletionTimestamp.IsZero() {
					t.Fatal("intact old workload stopped")
				}
				return
			}
			if *sts(t, r, h).Spec.Replicas != 0 || pod.DeletionTimestamp.IsZero() {
				t.Fatal("unsafe old workload was retained")
			}
			if meta.IsStatusConditionTrue(h.Status.Conditions, "NetworkPolicyReady") || meta.IsStatusConditionTrue(h.Status.Conditions, "Ready") {
				t.Fatal("unsafe workload reported ready")
			}
			if h.Status.StorageRef.UID != homeUID {
				t.Fatal("persistent home changed")
			}
			// A valid desired state recreates policy and resumes the same home.
			if drift == "foreign" || drift == "terminating" {
				return
			} // External ownership/deletion must be resolved separately.
			h.Spec = *original
			h.Generation++
			if err := r.Update(ctx, h); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			if *sts(t, r, h).Spec.Replicas != 1 || h.Status.StorageRef.UID != homeUID {
				t.Fatal("valid configuration did not recover")
			}
			if err := r.Get(ctx, client.ObjectKeyFromObject(set), policy); err != nil {
				t.Fatal(err)
			}
			if policy.Spec.PodSelector.MatchLabels[network.InstallationUIDLabel] != string(h.UID) || len(policy.Spec.Egress[0].To) == 0 {
				t.Fatal("policy not restored")
			}
		})
	}
}
