package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func withSTT(t *testing.T, h *v1.Hermes, raw string) {
	t.Helper()
	b, _ := json.Marshal(h.Spec)
	var spec map[string]any
	_ = json.Unmarshal(b, &spec)
	var stt any
	if err := json.Unmarshal([]byte(raw), &stt); err != nil {
		t.Fatal(err)
	}
	spec["stt"] = stt
	b, _ = json.Marshal(spec)
	if err := json.Unmarshal(b, &h.Spec); err != nil {
		t.Fatal(err)
	}
}

func TestSTTRenderAndInheritance(t *testing.T) {
	for _, tc := range []struct {
		name, raw, url, language    string
		enabled, echo, ownKey, none bool
	}{
		{name: "absent", raw: `null`, language: "ru", echo: true},
		{name: "disabled-unused-key", raw: `{"enabled":false,"apiKeySecretRef":{"name":"missing"}}`, language: "ru", echo: true},
		{name: "inherited", raw: `{"enabled":true,"model":"qwen3-asr-1.7b"}`, enabled: true, language: "ru", echo: true},
		{name: "url-only", raw: `{"enabled":true,"model":"qwen3-asr-1.7b","baseURL":"https://asr.invalid/v1"}`, url: "https://asr.invalid/v1", enabled: true, language: "ru", echo: true},
		{name: "key-only", raw: `{"enabled":true,"model":"qwen3-asr-1.7b","apiKeySecretRef":{}}`, enabled: true, language: "ru", echo: true, ownKey: true},
		{name: "auto-no-echo", raw: `{"enabled":true,"model":"qwen3-asr-1.7b","language":"auto","echoTranscripts":false}`, enabled: true},
		{name: "none", raw: `{"enabled":true,"model":"qwen3-asr-1.7b","auth":"None"}`, enabled: true, language: "ru", echo: true, none: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, r := fixture()
			withSTT(t, h, tc.raw)
			src := sources()
			src[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}].Data["STT_API_KEY"] = []byte("separate-stt-sentinel")
			b, err := Render(h, r, src)
			if err != nil {
				t.Fatal(err)
			}
			var doc startupInput
			_ = json.Unmarshal(b.JSON, &doc)
			stt, ok := doc.Config["stt"].(map[string]any)
			if !ok {
				t.Fatal("STT is not managed")
			}
			if stt["enabled"] != tc.enabled || doc.Config["stt_enabled"] != tc.enabled {
				t.Fatal("enabled/legacy alias", stt)
			}
			if stt["echo_transcripts"] != tc.echo || doc.Config["stt_echo_transcripts"] != tc.echo {
				t.Fatal("echo/legacy alias", stt)
			}
			if stt["language"] != tc.language || doc.Env["HERMES_LOCAL_STT_LANGUAGE"] != "" {
				t.Fatal("language", stt)
			}
			if stt["use_gateway"] != false {
				t.Fatal("stored managed selection can override provider")
			}
			openai := stt["openai"].(map[string]any)
			if !tc.enabled {
				if openai["api_key"] != "" {
					t.Fatal("disabled credential retained")
				}
				return
			}
			wantURL := tc.url
			if wantURL == "" {
				wantURL = h.Spec.Model.BaseURL
			}
			if stt["provider"] != "openai" || openai["model"] != "qwen3-asr-1.7b" || openai["base_url"] != wantURL || openai["language"] != tc.language {
				t.Fatal(stt)
			}
			key := openai["api_key"]
			if tc.none {
				if key != "no-key-required" {
					t.Fatal("None leaked a key")
				}
			} else if tc.ownKey {
				ref := key.(map[string]any)["credential"].(string)
				if string(b.SecretData[ref]) != "separate-stt-sentinel" {
					t.Fatal("wrong STT key")
				}
			} else if !reflect.DeepEqual(key, doc.Env["HERMES_MODEL_API_KEY"]) {
				t.Fatal("did not inherit model key")
			}
			if strings.Contains(string(b.JSON), "sentinel") || strings.Contains(string(b.JSON), "SENTINEL") {
				t.Fatal("credential leak")
			}
		})
	}
}

func TestSTTAuthAndReferences(t *testing.T) {
	for _, tc := range []struct {
		name, modelAuth, raw string
		valid, noKey         bool
	}{
		{"inherit-none", "None", `{"enabled":true,"model":"asr"}`, true, true},
		{"key-overrides-none", "None", `{"enabled":true,"model":"asr","apiKeySecretRef":{"name":"asr","key":"custom"}}`, true, false},
		{"explicit-api-without-source", "None", `{"enabled":true,"model":"asr","auth":"APIKey"}`, false, false},
		{"none-with-reference", "APIKey", `{"enabled":true,"model":"asr","auth":"None","apiKeySecretRef":{}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, r := fixture()
			h.Spec.Model.Auth = tc.modelAuth
			withSTT(t, h, tc.raw)
			src := sources()
			src[types.NamespacedName{Namespace: "tenant", Name: "asr"}] = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{UID: "asr-uid"}, Data: map[string][]byte{"custom": []byte("asr-key")}}
			b, err := Render(h, r, src)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if !tc.valid {
				return
			}
			var doc startupInput
			_ = json.Unmarshal(b.JSON, &doc)
			key := doc.Config["stt"].(map[string]any)["openai"].(map[string]any)["api_key"]
			if tc.noKey {
				if key != "no-key-required" {
					t.Fatal(key)
				}
			} else {
				ref := key.(map[string]any)["credential"].(string)
				if string(b.SecretData[ref]) != "asr-key" {
					t.Fatal("wrong reference")
				}
			}
		})
	}
	h, r := fixture()
	h.Spec.Model.APIKeySecretRef = &v1.ModelSecretKeyRef{Name: "custom-model", Key: "custom"}
	withSTT(t, h, `{"enabled":true,"model":"asr"}`)
	src := sources()
	src[types.NamespacedName{Namespace: "tenant", Name: "custom-model"}] = &corev1.Secret{Data: map[string][]byte{"custom": []byte("inherited-custom")}}
	b, err := Render(h, r, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(SecretRefs(h)) != 2 || len(b.SecretData) != 2 {
		t.Fatal("inheritance must deduplicate refs")
	}
	withSTT(t, h, `{"enabled":true,"model":"asr","apiKeySecretRef":{"name":"absent"}}`)
	_, err = Render(h, r, src)
	var dependency *DependencyError
	if !errors.As(err, &dependency) || dependency.Secret.Name != "absent" {
		t.Fatalf("explicit missing key silently fell back: %v", err)
	}
}

func TestSTTValidation(t *testing.T) {
	for _, raw := range []string{`{"enabled":true}`, `{"enabled":true,"model":" "}`, `{"enabled":true,"model":"asr","auth":"Bad"}`, `{"enabled":true,"model":"asr","baseURL":"https://u:p@host/v1"}`, `{"enabled":true,"model":"asr","baseURL":"https://host/v1?key=abc"}`, `{"enabled":true,"model":"asr","language":"russian"}`} {
		h, r := fixture()
		withSTT(t, h, raw)
		if Validate(h, r) == nil {
			t.Fatalf("accepted invalid STT %s", raw)
		}
	}
	for _, raw := range []string{`{"stt_enabled":true}`, `{"stt_echo_transcripts":false}`, `{"stt":{"provider":"nous"}}`, `{"stt":{"use_gateway":true}}`, `{"stt":{"openai":{"model":"other"}}}`} {
		h, r := fixture()
		h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(raw)}
		if Validate(h, r) == nil {
			t.Fatalf("accepted competing STT config %s", raw)
		}
	}
	for _, key := range []string{"HERMES_STT_API_KEY", "HERMES_LOCAL_STT_LANGUAGE", "STT_OPENAI_BASE_URL"} {
		h, r := fixture()
		h.Spec.ExtraEnv = map[string]string{key: "override"}
		if Validate(h, r) == nil {
			t.Fatalf("accepted competing STT env %s", key)
		}
	}
}
