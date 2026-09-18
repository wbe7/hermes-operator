package config

import (
	"strings"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func fixture() (*v1.Hermes, runtimecatalog.Release) {
	r, _ := runtimecatalog.Resolve("v2026.9.14")
	return &v1.Hermes{ObjectMeta: metav1.ObjectMeta{Name: "maria", Namespace: "tenant"}, Spec: v1.HermesSpec{Version: r.Version, Model: v1.ModelSpec{Provider: "custom", Name: "test-model", BaseURL: "https://inference.invalid/v1"}, Telegram: v1.TelegramSpec{AllowedUserIDs: []string{"123"}}}}, r
}
func TestRejectConflictingExtraConfig(t *testing.T) {
	for _, raw := range []string{`{"model":{"default":"SENTINEL"}}`, `{"agent":"SENTINEL"}`, `{"display":{"personality":"SENTINEL"}}`, `{"agent":{"system_prompt":"SENTINEL"}}`, `{"gateway":{"platforms":{"telegram":{"extra":{"guest_mode":true}}}}}`, `{"auxiliary":{"vision":{"api_key":"SENTINEL"}}}`, `{"terminal":{"cwd":"SENTINEL"}}`, `{"custom_providers":[{"api_key":"SENTINEL"}]}`} {
		t.Run(raw, func(t *testing.T) {
			h, r := fixture()
			h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(raw)}
			err := Validate(h, r)
			if err == nil || !strings.Contains(err.Error(), "spec.extraConfig") || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("unsafe validation result: %v", err)
			}
		})
	}
}
func TestEnvironmentValidation(t *testing.T) {
	for _, name := range []string{"HERMES_MODEL_API_KEY", "CUSTOM_BASE_URL", "PYTHON_DOTENV_DISABLED", "TELEGRAM_ALLOW_ALL_USERS", "TERMINAL_CWD", "PATH", "XDG_CONFIG_HOME"} {
		h, r := fixture()
		h.Spec.ExtraEnv = map[string]string{name: "SENTINEL"}
		if err := Validate(h, r); err == nil || strings.Contains(err.Error(), "SENTINEL") {
			t.Fatalf("accepted %s", name)
		}
	}
	h, r := fixture()
	h.Spec.ExtraEnv = map[string]string{"SEARCH_TOKEN": "SENTINEL"}
	h.Spec.Credentials.Env = map[string]v1.SecretKeyRef{"SEARCH_TOKEN": {Key: "token"}}
	if Validate(h, r) == nil {
		t.Fatal("duplicate env accepted")
	}
	h.Spec.Credentials.Env = nil
	h.Spec.ExtraEnv = map[string]string{"OPENAI_API_KEY": "SENTINEL"}
	if Validate(h, r) == nil {
		t.Fatal("credential literal accepted")
	}
}
func TestSafeEmptyObjectsDoNotAcquireOwnership(t *testing.T) {
	h, r := fixture()
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"compression":{},"agent":{},"safe":{"nested":42}}`)}
	if err := Validate(h, r); err != nil {
		t.Fatal(err)
	}
}
func TestProviderAndModeMatrix(t *testing.T) {
	for _, provider := range []string{"auto", "openrouter", "anthropic", "unknown"} {
		h, r := fixture()
		h.Spec.Model.Provider = provider
		if Validate(h, r) == nil {
			t.Fatal("unverified provider accepted")
		}
	}
	for _, mode := range []string{"responses", "anthropic_messages", "bad"} {
		h, r := fixture()
		h.Spec.Model.APIMode = mode
		if Validate(h, r) == nil {
			t.Fatal("unverified mode accepted")
		}
	}
}
func TestExtraConfigLimitsAndCredentialReference(t *testing.T) {
	h, r := fixture()
	h.Spec.Credentials.Env = map[string]v1.SecretKeyRef{"VISION_API_KEY": {Key: "vision"}}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"auxiliary":{"vision":{"api_key":"${VISION_API_KEY}"}}}`)}
	if err := Validate(h, r); err != nil {
		t.Fatal(err)
	}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(strings.Repeat(`{"x":`, 17) + `1` + strings.Repeat(`}`, 17))}
	if Validate(h, r) == nil {
		t.Fatal("deep tree accepted")
	}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"large":"` + strings.Repeat("x", 256*1024) + `"}`)}
	if Validate(h, r) == nil {
		t.Fatal("oversized input accepted")
	}
}
