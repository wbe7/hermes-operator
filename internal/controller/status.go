package controller

import (
	"context"
	"errors"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/network"
	"github.com/wbe7/hermes-operator/internal/workload"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func setCondition(h *v1.Hermes, typ string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&h.Status.Conditions, metav1.Condition{Type: typ, Status: status, Reason: reason, Message: message, ObservedGeneration: h.Generation})
}
func (r *HermesReconciler) saveStatus(ctx context.Context, h *v1.Hermes) (ctrl.Result, error) {
	h.Status.ObservedGeneration = h.Generation
	current := &v1.Hermes{}
	if err := r.reader().Get(ctx, client.ObjectKeyFromObject(h), current); err != nil {
		return ctrl.Result{}, apiFailure("read status", err)
	}
	if current.UID != h.UID || current.Generation != h.Generation || current.DeletionTimestamp.IsZero() != h.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, errors.New("Hermes changed during reconciliation; retry from current state")
	}
	if equality.Semantic.DeepEqual(h.Status, current.Status) {
		return ctrl.Result{RequeueAfter: retryDelay}, nil
	}
	base := current.DeepCopy()
	current.Status = *h.Status.DeepCopy()
	if err := r.Status().Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return ctrl.Result{}, apiFailure("patch status", err)
	}
	return ctrl.Result{RequeueAfter: retryDelay}, nil
}
func (r *HermesReconciler) failed(ctx context.Context, h *v1.Hermes, typ string, err error) (ctrl.Result, error) {
	var p *conditionError
	if !errors.As(err, &p) {
		setCondition(h, typ, metav1.ConditionFalse, "ReconcileError", "Kubernetes operation failed; reconciliation will retry")
		setCondition(h, "Ready", metav1.ConditionFalse, "ReconcileError", "Kubernetes operation failed; reconciliation will retry")
		_, saveErr := r.saveStatus(ctx, h)
		if saveErr != nil {
			return ctrl.Result{}, saveErr
		}
		return ctrl.Result{}, err
	}
	setCondition(h, typ, metav1.ConditionFalse, p.reason, p.message)
	setCondition(h, "Ready", metav1.ConditionFalse, p.reason, p.message)
	setCondition(h, "Degraded", metav1.ConditionTrue, p.reason, p.message)
	return r.saveStatus(ctx, h)
}
func (r *HermesReconciler) stopAndFail(ctx context.Context, h *v1.Hermes, typ string, err error) (ctrl.Result, error) {
	if _, stopErr := r.stop(ctx, h); stopErr != nil {
		return r.failed(ctx, h, typ, stopErr)
	}
	return r.failed(ctx, h, typ, err)
}
func (r *HermesReconciler) observePods(ctx context.Context, h *v1.Hermes, set *appsv1.StatefulSet, revision string) error {
	pods, err := r.workloadPods(ctx, h, set)
	if err != nil {
		return err
	}
	ready := false
	reason, message := "RolloutInProgress", "waiting for the current workload revision"
	for i := range pods {
		p := &pods[i]
		if !p.DeletionTimestamp.IsZero() {
			continue
		}
		if p.Annotations[workload.RevisionAnnotation] != revision || p.Labels[network.InstallationUIDLabel] != string(h.UID) || !podIsolationMatches(set.Spec.Template.Spec, p.Spec) {
			if err = r.deleteUID(ctx, p); err != nil {
				return err
			}
			continue
		}
		reason, message = "GatewayNotReady", "gateway readiness has not succeeded"
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue && p.Status.Phase == corev1.PodRunning && len(pods) == 1 {
				ready = true
			}
		}
	}
	if ready {
		setCondition(h, "Ready", metav1.ConditionTrue, "Reconciled", "current workload revision is ready")
		setCondition(h, "Degraded", metav1.ConditionFalse, "Reconciled", "installation is healthy")
	} else {
		setCondition(h, "Ready", metav1.ConditionFalse, reason, message)
		setCondition(h, "Degraded", metav1.ConditionFalse, "Reconciled", "waiting for workload readiness")
	}
	return nil
}
