package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func TestStatefulSetUpdateStrategyIsRestored(t *testing.T) {
	for _, drift := range []appsv1.StatefulSetUpdateStrategy{
		{Type: appsv1.OnDeleteStatefulSetStrategyType},
		{Type: appsv1.RollingUpdateStatefulSetStrategyType, RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(1))}},
	} {
		t.Run(string(drift.Type), func(t *testing.T) {
			r, h := unit(t, true)
			runReconcile(t, r, h)
			set := sts(t, r, h)
			set.Spec.UpdateStrategy = drift
			if err := r.Update(ctx, set); err != nil {
				t.Fatal(err)
			}
			runReconcile(t, r, h)
			strategy := sts(t, r, h).Spec.UpdateStrategy
			if strategy.Type != appsv1.RollingUpdateStatefulSetStrategyType || (strategy.RollingUpdate != nil && ptr.Deref(strategy.RollingUpdate.Partition, int32(0)) != 0) {
				t.Fatal("rollout strategy drift retained")
			}
		})
	}
}

func TestUpdateStrategyDefaultsDoNotCauseRepeatedWrites(t *testing.T) {
	r, h := unit(t, true)
	runReconcile(t, r, h)
	set := sts(t, r, h)
	set.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType, RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(0)), MaxUnavailable: ptr.To(intstr.FromInt32(1))}}
	if err := r.Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	version := sts(t, r, h).ResourceVersion
	runReconcile(t, r, h)
	if sts(t, r, h).ResourceVersion != version {
		t.Fatal("server defaults caused a redundant workload patch")
	}
}
