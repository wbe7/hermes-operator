package workload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
)

const RevisionAnnotation = "hermes.wbe7.github.io/revision"

// finalRevision hashes the effective template BEFORE adding this revision to
// annotations and input names. Keep policy, PVC sizing/retention and suspend out.
func finalRevision(template corev1.PodTemplateSpec, bundleRevision, assetsChecksum string) string {
	raw, _ := json.Marshal(struct {
		Template       corev1.PodTemplateSpec
		Bundle, Assets string
	}{template, bundleRevision, assetsChecksum})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}
