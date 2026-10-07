package config

import (
	"encoding/json"
	"errors"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"math"
	"strings"
	"testing"
)

func withTTS(t *testing.T, h *v1.Hermes, raw string) {
	t.Helper()
	b, _ := json.Marshal(h)
	var doc map[string]any
	_ = json.Unmarshal(b, &doc)
	var s any
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	doc["spec"].(map[string]any)["tts"] = s
	b, _ = json.Marshal(doc)
	if err := json.Unmarshal(b, h); err != nil {
		t.Fatal(err)
	}
}
func TestTTSRender(t *testing.T) {
	for _, tc := range []struct {
		name, raw, mode, key, url, language string
		enabled                             bool
		speed                               float64
	}{
		{"absent", `null`, "off", "", "", "", false, 1},
		{"inherited", `{"enabled":true,"model":"fish-s2-pro","voice":"default"}`, "voice_only", "SENTINEL${HOME}", "https://inference.example/v1", "", true, 1},
		{"override", `{"enabled":true,"model":"fish-s2-pro","voice":"default","baseURL":"https://speech.example/v1","apiKeySecretRef":{},"responseMode":"All","speed":1.25,"language":"ru"}`, "all", "tts-secret", "https://speech.example/v1", "ru", true, 1.25},
		{"keyless", `{"enabled":true,"model":"fish-s2-pro","voice":"default","auth":"None","responseMode":"OnRequest"}`, "off", "no-key-required", "https://inference.example/v1", "", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, r := fixture()
			h.Spec.Model.BaseURL = "https://inference.example/v1"
			withTTS(t, h, tc.raw)
			src := sources()
			src[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}].Data["TTS_API_KEY"] = []byte("tts-secret")
			b, err := Render(h, r, src)
			if err != nil {
				t.Fatal(err)
			}
			var d map[string]any
			_ = json.Unmarshal(b.JSON, &d)
			c := d["config"].(map[string]any)
			s, ok := c["tts"].(map[string]any)
			if !ok {
				t.Fatal("missing managed TTS")
			}
			o := s["openai"].(map[string]any)
			if s["provider"] != "openai" || s["use_gateway"] != false || o["speed"] != tc.speed || o["language"] != tc.language {
				t.Fatal("native config", s)
			}
			if o["base_url"] != tc.url {
				t.Fatal("endpoint", o)
			}
			key := o["api_key"]
			if ref, ok := key.(map[string]any); ok {
				key = string(b.SecretData[ref["credential"].(string)])
			}
			if key != tc.key {
				t.Fatal("wrong key selection")
			}
			voice := d["voice"].(map[string]any)
			if voice["mode"] != tc.mode {
				t.Fatal("voice mode", voice)
			}
			if c["voice"].(map[string]any)["auto_tts"] != false {
				t.Fatal("unsafe automatic fallback")
			}
			disabled := c["agent"].(map[string]any)["disabled_toolsets"].([]any)
			has := false
			for _, v := range disabled {
				has = has || v == "tts"
			}
			if has == tc.enabled {
				t.Fatal("tool availability")
			}
			if strings.Contains(string(b.JSON), "tts-secret") {
				t.Fatal("credential leaked")
			}
		})
	}
}
func TestTTSValidation(t *testing.T) {
	for _, raw := range []string{`{"enabled":true}`, `{"enabled":true,"model":"m"}`, `{"enabled":true,"model":"m","voice":" "}`, `{"model":"m","voice":"v","speed":0.1}`, `{"speed":5}`, `{"responseMode":"bad"}`, `{"auth":"None","apiKeySecretRef":{}}`, `{"baseURL":"https://u:p@host/v1"}`} {
		h, r := fixture()
		withTTS(t, h, raw)
		if Validate(h, r) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestTTSAuthAndReferences(t *testing.T) {
	for _, tc := range []struct {
		name, modelAuth, raw string
		valid, noKey         bool
	}{
		{"inherit-none", "None", `{"enabled":true,"model":"speech","voice":"default"}`, true, true},
		{"key-overrides-none", "None", `{"enabled":true,"model":"speech","voice":"default","apiKeySecretRef":{"name":"asr","key":"custom"}}`, true, false},
		{"explicit-api-without-source", "None", `{"enabled":true,"model":"speech","voice":"default","auth":"APIKey"}`, false, false},
		{"none-with-reference", "APIKey", `{"enabled":true,"model":"speech","voice":"default","auth":"None","apiKeySecretRef":{}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, r := fixture()
			h.Spec.Model.Auth = tc.modelAuth
			withTTS(t, h, tc.raw)
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
			key := doc.Config["tts"].(map[string]any)["openai"].(map[string]any)["api_key"]
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
	withTTS(t, h, `{"enabled":true,"model":"speech","voice":"default"}`)
	src := sources()
	src[types.NamespacedName{Namespace: "tenant", Name: "custom-model"}] = &corev1.Secret{Data: map[string][]byte{"custom": []byte("inherited-custom")}}
	b, err := Render(h, r, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(SecretRefs(h)) != 2 || len(b.SecretData) != 2 {
		t.Fatal("inheritance must deduplicate refs")
	}
	withTTS(t, h, `{"enabled":true,"model":"speech","voice":"default","apiKeySecretRef":{"name":"absent"}}`)
	_, err = Render(h, r, src)
	var dependency *DependencyError
	if !errors.As(err, &dependency) || dependency.Secret.Name != "absent" {
		t.Fatalf("explicit missing key silently fell back: %v", err)
	}
}

func TestTTSProtectedConfigAndToolConflict(t *testing.T) {
	for _, raw := range []string{`{"tts":{"provider":"edge"}}`, `{"tts":{"openai":{"voice":"other"}}}`, `{"voice":{"auto_tts":true}}`} {
		h, r := fixture()
		h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(raw)}
		if Validate(h, r) == nil {
			t.Fatalf("accepted conflicting config %s", raw)
		}
	}
	h, r := fixture()
	withTTS(t, h, `{"enabled":true,"model":"m","voice":"v"}`)
	h.Spec.Tools.Disabled = []string{"tts"}
	if Validate(h, r) == nil {
		t.Fatal("accepted disabled TTS toolset")
	}
	h.Spec.Tools.Disabled = nil
	for _, n := range []float64{math.NaN(), math.Inf(1), 0.0, 4.01} {
		h.Spec.TTS.Speed = &n
		if Validate(h, r) == nil {
			t.Fatal("accepted invalid speed")
		}
	}
	if !reservedEnv("HERMES_TTS_API_KEY") {
		t.Fatal("managed key can be overridden")
	}
}
