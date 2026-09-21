// Package workload builds desired objects without API calls or cleanup side effects.
package workload

import (
	"fmt"
	"maps"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	"github.com/wbe7/hermes-operator/internal/network"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// Resources is one consistent desired workload. Revision, not Bundle.Revision,
// is the identity the controller must match against the running Pod and status.
type Resources struct {
	StatefulSet    *appsv1.StatefulSet
	Service        *corev1.Service
	ServiceAccount *corev1.ServiceAccount
	ConfigMap      *corev1.ConfigMap
	Secret         *corev1.Secret
	Bootstrap      *corev1.ConfigMap
	Revision       string
}

func Build(h *v1.Hermes, release runtimecatalog.Release, bundle config.Bundle, claimName string) (Resources, error) {
	if h == nil || h.UID == "" || h.Name == "" || h.Namespace == "" || claimName == "" || bundle.Revision == "" {
		return Resources{}, fmt.Errorf("Hermes identity, claim and bundle revision are required")
	}
	if release.UID <= 0 || release.GID <= 0 {
		return Resources{}, fmt.Errorf("runtime UID/GID must be non-root")
	}
	scripts, items, err := bootstrapData(release.Adapter)
	if err != nil {
		return Resources{}, err
	}
	// Own all mutable maps/slices; callers may cache or reuse source objects.
	h = h.DeepCopy()
	name := h.Name + "-hermes"
	labels := map[string]string{network.InstallationUIDLabel: string(h.UID)}
	metadata := func(n string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: n, Namespace: h.Namespace, Labels: maps.Clone(labels), OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(h, v1.GroupVersion.WithKind("Hermes"))}}
	}
	repository, digest := release.ResolveImage(h.Spec.Image.Repository, h.Spec.Image.Digest)
	pull := h.Spec.Image.PullPolicy
	if pull == "" {
		pull = corev1.PullIfNotPresent
	}
	resources := resourceDefaults(h.Spec.Resources)
	for _, list := range []corev1.ResourceList{resources.Requests, resources.Limits} {
		for key, value := range list {
			if value.Sign() <= 0 {
				return Resources{}, fmt.Errorf("resource %s must be positive", key)
			}
		}
	}
	for key, request := range resources.Requests {
		if limit, ok := resources.Limits[key]; ok && request.Cmp(limit) > 0 {
			return Resources{}, fmt.Errorf("resource %s request exceeds effective limit", key)
		}
	}
	assetsChecksum := RuntimeAssetsChecksum()
	bootstrapName := h.Name + "-rt-" + assetsChecksum[:16]
	c := corev1.Container{
		Name: "hermes", Image: repository + "@" + digest, ImagePullPolicy: pull,
		Command:         []string{"/opt/hermes/.venv/bin/python", "-I", "/operator/runtime/bootstrap.py"},
		WorkingDir:      "/opt/hermes",
		Env:             []corev1.EnvVar{{Name: "HOME", Value: "/opt/data"}, {Name: "HERMES_HOME", Value: "/opt/data"}, {Name: "PYTHONDONTWRITEBYTECODE", Value: "1"}},
		SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
		Resources:       resources,
		VolumeMounts:    []corev1.VolumeMount{{Name: "home", MountPath: "/opt/data"}, {Name: "tmp", MountPath: "/tmp"}, {Name: "config", MountPath: "/operator/config", ReadOnly: true}, {Name: "credentials", MountPath: "/operator/credentials", ReadOnly: true}, {Name: "runtime", MountPath: "/operator/runtime", ReadOnly: true}},
		StartupProbe:    probe("live", 10, 60), ReadinessProbe: probe("ready", 10, 3), LivenessProbe: probe("live", 30, 3),
	}
	template := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: maps.Clone(labels)}, Spec: corev1.PodSpec{
		ServiceAccountName: name, AutomountServiceAccountToken: ptr.To(false), EnableServiceLinks: ptr.To(false), TerminationGracePeriodSeconds: ptr.To(int64(60)),
		SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(release.UID), RunAsGroup: ptr.To(release.GID), FSGroup: ptr.To(release.GID), FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
		Containers:      []corev1.Container{c}, ImagePullSecrets: h.Spec.Image.PullSecrets, NodeSelector: h.Spec.Scheduling.NodeSelector, Tolerations: h.Spec.Scheduling.Tolerations, Affinity: h.Spec.Scheduling.Affinity,
		Volumes: []corev1.Volume{
			{Name: "home", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName}}},
			{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse("1Gi"))}}},
			{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{DefaultMode: ptr.To(int32(0444))}}},
			{Name: "credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{DefaultMode: ptr.To(int32(0440))}}},
			{Name: "runtime", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: bootstrapName}, Items: items, DefaultMode: ptr.To(int32(0444))}}},
		},
	}}
	revision := finalRevision(template, bundle.Revision, assetsChecksum)
	configName, secretName := h.Name+"-cfg-"+revision, h.Name+"-sec-"+revision
	template.Annotations = map[string]string{RevisionAnnotation: revision}
	template.Spec.Volumes[2].ConfigMap.Name = configName
	template.Spec.Volumes[3].Secret.SecretName = secretName
	replicas := int32(1)
	if h.Spec.Suspend {
		replicas = 0
	}
	data := map[string][]byte{}
	for k, v := range bundle.SecretData {
		data[k] = append([]byte(nil), v...)
	}
	return Resources{
		Revision:       revision,
		StatefulSet:    &appsv1.StatefulSet{ObjectMeta: metadata(name), Spec: appsv1.StatefulSetSpec{ServiceName: name, Replicas: ptr.To(replicas), Selector: &metav1.LabelSelector{MatchLabels: maps.Clone(labels)}, Template: template, UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType}}},
		Service:        &corev1.Service{ObjectMeta: metadata(name), Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: corev1.ClusterIPNone, Selector: maps.Clone(labels)}},
		ServiceAccount: &corev1.ServiceAccount{ObjectMeta: metadata(name), AutomountServiceAccountToken: ptr.To(false)},
		ConfigMap:      &corev1.ConfigMap{ObjectMeta: metadata(configName), Immutable: ptr.To(true), Data: map[string]string{"input.json": string(bundle.JSON)}},
		Secret:         &corev1.Secret{ObjectMeta: metadata(secretName), Immutable: ptr.To(true), Type: corev1.SecretTypeOpaque, Data: data},
		Bootstrap:      &corev1.ConfigMap{ObjectMeta: metadata(bootstrapName), Immutable: ptr.To(true), Data: scripts},
	}, nil
}
func probe(mode string, period, fail int32) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"/opt/hermes/.venv/bin/python", "-I", "/operator/runtime/probe.py", mode}}}, PeriodSeconds: period, TimeoutSeconds: 5, FailureThreshold: fail, SuccessThreshold: 1}
}
func resourceDefaults(in v1.ResourcesSpec) corev1.ResourceRequirements {
	out := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("512Mi"), corev1.ResourceEphemeralStorage: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("2Gi"), corev1.ResourceEphemeralStorage: resource.MustParse("2Gi")}}
	for k, v := range in.Requests {
		out.Requests[k] = v.DeepCopy()
	}
	for k, v := range in.Limits {
		out.Limits[k] = v.DeepCopy()
	}
	return out
}
