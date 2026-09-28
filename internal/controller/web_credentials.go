package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"unicode/utf8"
)

func (r *HermesReconciler) ensureWebCredentials(ctx context.Context, h *v1.Hermes) error {
	if !config.WebEnabled(h) {
		return nil
	}
	s := &corev1.Secret{}
	err := r.reader().Get(ctx, client.ObjectKey{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}, s)
	create := apierrors.IsNotFound(err)
	if err != nil && !create {
		return apiFailure("read web credential source", err)
	}
	if create {
		s = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}, Type: corev1.SecretTypeOpaque}
	}
	if !s.DeletionTimestamp.IsZero() {
		return problem("DependencyNotFound", "web credential source is terminating")
	}
	missing := []string{}
	for _, key := range []string{"WEB_PASSWORD", "WEB_SESSION_SECRET"} {
		value, exists := s.Data[key]
		if !exists {
			missing = append(missing, key)
			continue
		}
		if len(value) == 0 || !utf8.Valid(value) || strings.ContainsRune(string(value), '\x00') || (key == "WEB_SESSION_SECRET" && len(value) < 16) {
			return problem("InvalidWebCredentials", "web credential keys must contain valid nonempty text and a signing key of at least 16 bytes")
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if s.Immutable != nil && *s.Immutable {
		return problem("ImmutableWebCredentials", "immutable credential source must supply both web keys")
	}
	base := s.DeepCopy()
	if s.Data == nil {
		s.Data = map[string][]byte{}
	}
	for _, key := range missing {
		value := make([]byte, 32)
		if _, err = rand.Read(value); err != nil {
			return problem("CredentialGenerationFailed", "cannot generate web credentials")
		}
		s.Data[key] = []byte(base64.RawURLEncoding.EncodeToString(value))
	}
	if create {
		err = r.Create(ctx, s)
	} else {
		err = r.Patch(ctx, s, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	}
	if err != nil {
		return apiFailure("save web credential source", err)
	}
	return nil
}
