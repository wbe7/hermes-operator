package controller

import (
	"context"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"strings"
	"testing"
)

func TestWebCredentialsGeneratedOnceAndPreserveSource(t *testing.T) {
	r, h := unit(t, true)
	h.Spec.Web = &v1.WebSpec{Enabled: true}
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}
	if err := r.Get(ctx, key, s); err != nil {
		t.Fatal(err)
	}
	if len(s.Data["WEB_PASSWORD"]) < 43 || len(s.Data["WEB_SESSION_SECRET"]) < 43 || string(s.Data["MODEL_API_KEY"]) != "fake-key" || len(s.OwnerReferences) != 0 {
		t.Fatal("credential ownership/entropy failure")
	}
	password, signing := string(s.Data["WEB_PASSWORD"]), string(s.Data["WEB_SESSION_SECRET"])
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, key, s); err != nil {
		t.Fatal(err)
	}
	if password != string(s.Data["WEB_PASSWORD"]) || signing != string(s.Data["WEB_SESSION_SECRET"]) {
		t.Fatal("credential changed at reconcile")
	}
}
func TestWebCredentialsRespectProvidedImmutableAndEmpty(t *testing.T) {
	for _, mode := range []string{"provided", "immutable-missing", "empty", "password-trim", "signing-blank"} {
		t.Run(mode, func(t *testing.T) {
			r, h := unit(t, true)
			h.Spec.Web = &v1.WebSpec{Enabled: true}
			s := source(h)
			_ = r.Get(ctx, client.ObjectKeyFromObject(s), s)
			if mode == "provided" {
				s.Data["WEB_PASSWORD"] = []byte("chosen-password")
				s.Data["WEB_SESSION_SECRET"] = []byte(strings.Repeat("s", 32))
			}
			if mode == "empty" {
				s.Data["WEB_PASSWORD"] = []byte{}
			}
			if mode == "password-trim" {
				s.Data["WEB_PASSWORD"] = []byte(" chosen-password ")
			}
			if mode == "signing-blank" {
				s.Data["WEB_SESSION_SECRET"] = []byte(strings.Repeat(" ", 32))
			}
			if mode == "provided" || mode == "immutable-missing" {
				s.Immutable = ptr.To(true)
			}
			if err := r.Update(ctx, s); err != nil {
				t.Fatal(err)
			}
			err := r.ensureWebCredentials(ctx, h)
			if (err == nil) != (mode == "provided") {
				t.Fatalf("unexpected result mode=%s err=%v", mode, err)
			}
			_ = r.Get(ctx, client.ObjectKeyFromObject(s), s)
			if mode == "provided" && string(s.Data["WEB_PASSWORD"]) != "chosen-password" {
				t.Fatal("provided password changed")
			}
			if mode != "provided" && mode != "signing-blank" {
				if _, ok := s.Data["WEB_SESSION_SECRET"]; ok {
					t.Fatal("partial update on invalid secret")
				}
			}
		})
	}
}
func TestNoChannelNoAuthDoesNotRequireSecret(t *testing.T) {
	r, h := unit(t, false)
	h.Spec.Telegram = nil
	h.Spec.Model.Auth = "None"
	if err := r.Update(ctx, h); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, h)
	if sts(t, r, h) == nil {
		t.Fatal("no-channel workload missing")
	}
	for _, c := range h.Status.Conditions {
		if c.Reason == "DependencyIdentityUnknown" || c.Reason == "DependencyKeyMissing" {
			t.Fatal("empty credential set rejected")
		}
	}
}

func TestWebCredentialsCreateAndDisabled(t *testing.T) {
	r, h := unit(t, false)
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}
	if err := r.Get(ctx, key, s); !apierrors.IsNotFound(err) {
		t.Fatal("disabled Web created a Secret")
	}
	enableWeb(h)
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, key, s); err != nil {
		t.Fatal(err)
	}
	if len(s.Data) != 2 || len(s.OwnerReferences) != 0 {
		t.Fatal("unexpected generated source")
	}
}

func TestWebCredentialConflictPreservesConcurrentUpdate(t *testing.T) {
	r, h := unit(t, true)
	enableWeb(h)
	original := r.Client
	r.Client = interceptor.NewClient(original.(client.WithWatch), interceptor.Funcs{Patch: func(c context.Context, cl client.WithWatch, obj client.Object, p client.Patch, opts ...client.PatchOption) error {
		s := &corev1.Secret{}
		if err := cl.Get(c, client.ObjectKeyFromObject(obj), s); err != nil {
			return err
		}
		s.Data["CONCURRENT_KEY"] = []byte("keep")
		if err := cl.Update(c, s); err != nil {
			return err
		}
		return cl.Patch(c, obj, p, opts...)
	}})
	if err := r.ensureWebCredentials(ctx, h); err == nil {
		t.Fatal("stale patch accepted")
	}
	r.Client = original
	if err := r.ensureWebCredentials(ctx, h); err != nil {
		t.Fatal(err)
	}
	s := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: h.Namespace, Name: config.PrimarySecretName(h)}, s); err != nil {
		t.Fatal(err)
	}
	if string(s.Data["CONCURRENT_KEY"]) != "keep" || len(s.Data["WEB_PASSWORD"]) == 0 {
		t.Fatal("concurrent data lost")
	}
}
