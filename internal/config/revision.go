package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	"k8s.io/apimachinery/pkg/types"
)

type secretIdentity struct {
	Namespace, Name, Key string
	UID                  types.UID
}

func computeRevision(h *v1.Hermes, r runtimecatalog.Release, input []byte, data map[string][]byte, ids []secretIdentity) string {
	image := h.Spec.Image
	if image.Repository == "" {
		image.Repository = "docker.io/nousresearch/hermes-agent"
	}
	if image.Digest == "" {
		image.Digest = r.ImageDigest
	}
	if image.PullPolicy == "" {
		image.PullPolicy = "IfNotPresent"
	}
	raw, _ := json.Marshal(struct {
		Input   json.RawMessage
		Data    map[string][]byte
		Sources []secretIdentity
		Release runtimecatalog.Release
		Image   v1.ImageSpec
	}{input, data, ids, r, image})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}
