package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func sources() map[types.NamespacedName]*corev1.Secret {
	return map[types.NamespacedName]*corev1.Secret{{Namespace: "tenant", Name: "maria-hermes-secret"}: {ObjectMeta: metav1.ObjectMeta{UID: "source-uid"}, Data: map[string][]byte{"MODEL_API_KEY": []byte("SENTINEL${HOME}"), "TELEGRAM_BOT_TOKEN": []byte("SENTINEL-token"), "UNUSED": []byte("unused")}}}
}

func TestOpenRouterCredentialsAreScopedToItsHostname(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		owned    bool
	}{
		{"https://inference.invalid/v1", false},
		{"https://openrouter.ai.attacker.invalid/v1", false},
		{"https://proxy.invalid/openrouter.ai/v1", false},
		{"https://openrouter.ai/api/v1", true},
		{"https://API.OPENROUTER.AI.:443/api/v1", true},
	} {
		for _, auth := range []string{"APIKey", "None"} {
			t.Run(tc.endpoint+"/"+auth, func(t *testing.T) {
				h, r := fixture()
				h.Spec.Model.BaseURL, h.Spec.Model.Auth = tc.endpoint, auth
				b, err := Render(h, r, sources())
				if err != nil {
					t.Fatal(err)
				}
				var doc startupInput
				if err := json.Unmarshal(b.JSON, &doc); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"OPENROUTER_API_KEY", "OPENROUTER_BASE_URL"} {
					if _, present := doc.Env[key]; present != tc.owned {
						t.Fatalf("%s owned=%v, want %v", key, present, tc.owned)
					}
				}
				if tc.owned && auth == "None" && doc.Env["OPENROUTER_API_KEY"] != "no-key-required" {
					t.Fatal("no-auth endpoint retained credentials")
				}
			})
		}
	}
}
func TestDefaultRefsAndRender(t *testing.T) {
	h, r := fixture()
	refs := SecretRefs(h)
	if len(refs) != 2 {
		t.Fatalf("refs: %v", refs)
	}
	for _, ref := range refs {
		if ref.Name != "maria-hermes-secret" {
			t.Fatal(ref)
		}
	}
	b, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.SecretData) != 2 || strings.Contains(string(b.JSON), "SENTINEL") {
		t.Fatal("secret leak or unexpected data")
	}
	var doc map[string]any
	if json.Unmarshal(b.JSON, &doc) != nil {
		t.Fatal("invalid JSON")
	}
	for _, key := range []string{"MESSAGING_CWD", "TERMINAL_CWD"} {
		if _, present := doc["env"].(map[string]any)[key]; present {
			t.Fatalf("deprecated cwd variable emitted: %s", key)
		}
	}
	cfg := doc["config"].(map[string]any)
	agent := cfg["agent"].(map[string]any)
	if agent["reasoning_effort"] != "xhigh" || len(agent["reasoning_overrides"].(map[string]any)) != 0 {
		t.Fatal(agent)
	}
}
func TestExplicitRefsAndNoAuth(t *testing.T) {
	h, r := fixture()
	h.Spec.Credentials.SecretName = "common"
	h.Spec.Model.APIKeySecretRef = &v1.ModelSecretKeyRef{Name: "model", Key: "key"}
	h.Spec.Telegram.BotTokenSecretRef = &v1.TelegramSecretKeyRef{Key: "bot"}
	refs := SecretRefs(h)
	found := map[string]string{}
	for _, ref := range refs {
		found[ref.Name] = ref.Key
	}
	if found["model"] != "key" || found["common"] != "bot" {
		t.Fatal(found)
	}
	h.Spec.Model.Auth = "None"
	h.Spec.Model.APIKeySecretRef = nil
	refs = SecretRefs(h)
	if len(refs) != 1 || refs[0].Key != "bot" {
		t.Fatal(refs)
	}
	h.Spec.Credentials.SecretName = ""
	h.Spec.Telegram.BotTokenSecretRef = nil
	b, err := Render(h, r, sources())
	if err != nil || len(b.SecretData) != 1 {
		t.Fatalf("%v %v", b, err)
	}
}
func TestMissingDependencyTypedAndNamespaced(t *testing.T) {
	h, r := fixture()
	s := sources()
	s[types.NamespacedName{Namespace: "other", Name: "maria-hermes-secret"}] = s[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}]
	delete(s, types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"})
	_, err := Render(h, r, s)
	var dep *DependencyError
	if !errors.As(err, &dep) {
		t.Fatalf("%T %v", err, err)
	}
	s = sources()
	s[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}].Data["MODEL_API_KEY"] = nil
	_, err = Render(h, r, s)
	if !errors.As(err, &dep) || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal(err)
	}
}

func TestMappingDefaultsAndOptionalRemoval(t *testing.T) {
	h, r := fixture()
	b, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	var d startupInput
	if err = json.Unmarshal(b.JSON, &d); err != nil {
		t.Fatal(err)
	}
	get := func(path string) any {
		var value any = d.Config
		for _, part := range strings.Split(path, ".") {
			m, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("missing ancestor: %s", path)
			}
			var found bool
			value, found = m[part]
			if !found {
				t.Fatalf("missing managed path: %s", path)
			}
		}
		return value
	}
	for path, want := range map[string]any{"model.context_length": nil, "model.api": nil, "platform_toolsets.telegram": nil, "memory.memory_char_limit": nil, "memory.user_char_limit": nil, "agent.max_turns": float64(50), "agent.run_budget_seconds": float64(600), "terminal.timeout": float64(300), "terminal.backend": "local", "terminal.cwd": "/opt/data/workspace"} {
		if got := get(path); got != want {
			t.Fatalf("%s: got %#v", path, got)
		}
	}
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"compression":{},"agent":{},"safe":{"empty":{}}}`)}
	empty, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	if b.Revision != empty.Revision {
		t.Fatal("empty objects changed input")
	}
}

func TestExtraNumbersRemainExact(t *testing.T) {
	h, r := fixture()
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"safe_counter":9007199254740993}`)}
	b, err := Render(h, r, sources())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b.JSON), `"safe_counter":9007199254740993`) {
		t.Fatal("extra integer precision lost")
	}
}
