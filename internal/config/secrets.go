package config

import (
	"crypto/sha256"
	"fmt"
	"sort"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// DependencyError means a referenced Secret or required key is unavailable.
// It deliberately carries identities only, never Secret contents.
type DependencyError struct {
	Secret types.NamespacedName
	Key    string
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("required Secret %s key %s is missing or empty", e.Secret.String(), e.Key)
}

type binding struct {
	Env string
	Ref corev1.SecretKeySelector
}

func bindings(h *v1.Hermes) []binding {
	name := h.Spec.Credentials.SecretName
	if name == "" {
		name = h.Name + "-hermes-secret"
	}
	ref := func(n, k, def string) corev1.SecretKeySelector {
		if n == "" {
			n = name
		}
		if k == "" {
			k = def
		}
		return corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: n}, Key: k}
	}
	tn, tk := "", ""
	if x := h.Spec.Telegram.BotTokenSecretRef; x != nil {
		tn, tk = x.Name, x.Key
	}
	out := []binding{{"TELEGRAM_BOT_TOKEN", ref(tn, tk, "TELEGRAM_BOT_TOKEN")}}
	if h.Spec.Model.Auth != "None" {
		mn, mk := "", ""
		if x := h.Spec.Model.APIKeySecretRef; x != nil {
			mn, mk = x.Name, x.Key
		}
		out = append(out, binding{"HERMES_MODEL_API_KEY", ref(mn, mk, "MODEL_API_KEY")})
	}
	for env, x := range h.Spec.Credentials.Env {
		out = append(out, binding{env, ref(x.Name, x.Key, "")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Env < out[j].Env })
	return out
}

// SecretRefs resolves convention names; all references are local to h.Namespace.
func SecretRefs(h *v1.Hermes) []corev1.SecretKeySelector {
	out := []corev1.SecretKeySelector{}
	seen := map[string]bool{}
	for _, b := range bindings(h) {
		id := b.Ref.Name + "/" + b.Ref.Key
		if !seen[id] {
			out = append(out, b.Ref)
			seen[id] = true
		}
	}
	return out
}
func credentialName(ref corev1.SecretKeySelector) string {
	sum := sha256.Sum256([]byte(ref.Name + "\x00" + ref.Key))
	return fmt.Sprintf("credential-%x", sum[:16])
}
