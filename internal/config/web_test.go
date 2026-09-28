package config

import (
	"encoding/json"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
	"testing"
)

// Decode the public JSON contract, so absent optional channels are tested as real clients send them.
func webFixture(t *testing.T, web string, telegram bool) *v1.Hermes {
	t.Helper()
	h, _ := fixture()
	raw, _ := json.Marshal(h)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	spec := obj["spec"].(map[string]any)
	if !telegram {
		delete(spec, "telegram")
	}
	if web != "" {
		var w any
		if err := json.Unmarshal([]byte(web), &w); err != nil {
			t.Fatal(err)
		}
		spec["web"] = w
	}
	raw, _ = json.Marshal(obj)
	h = &v1.Hermes{}
	if err := json.Unmarshal(raw, h); err != nil {
		t.Fatal(err)
	}
	return h
}

const validWeb = `{"enabled":true,"routing":{"mode":"Path","baseDomain":"agents.example.com"},"gatewayRef":{"name":"external","namespace":"infra-gateway","sectionName":"https"},"network":{"ingressFrom":[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"infra-gateway"}},"podSelector":{"matchLabels":{"gateway.networking.k8s.io/gateway-name":"external"}}}],"trustedProxyCIDRs":["10.42.0.0/16"]}}`

func TestNoChannelsNeedsNoTelegramCredential(t *testing.T) {
	h := webFixture(t, "", false)
	_, r := fixture()
	if err := Validate(h, r); err != nil {
		t.Fatal(err)
	}
	src := sources()
	for _, s := range src {
		delete(s.Data, "TELEGRAM_BOT_TOKEN")
	}
	b, err := Render(h, r, src)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(b.JSON, &doc)
	ch, ok := doc["channels"].(map[string]any)
	if !ok || ch["telegram"] != false || ch["web"] != false {
		t.Fatal("missing disabled channels")
	}
	cfg := doc["config"].(map[string]any)
	if cfg["telegram"].(map[string]any)["enabled"] != false {
		t.Fatal("Telegram remains enabled")
	}
	if doc["env"].(map[string]any)["TELEGRAM_BOT_TOKEN"] != "" {
		t.Fatal("stale bot token not cleared")
	}
}
func TestWebRenderCredentialsAndAddress(t *testing.T) {
	for _, mode := range []string{"Path", "Subdomain"} {
		t.Run(mode, func(t *testing.T) {
			h := webFixture(t, strings.Replace(validWeb, `"Path"`, `"`+mode+`"`, 1), false)
			_, r := fixture()
			src := sources()
			for _, s := range src {
				s.Data["WEB_PASSWORD"] = []byte("SENTINEL-web-password")
				s.Data["WEB_SESSION_SECRET"] = []byte(strings.Repeat("S", 43))
			}
			b, err := Render(h, r, src)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b.JSON), "SENTINEL") {
				t.Fatal("credential leaked")
			}
			var doc map[string]any
			_ = json.Unmarshal(b.JSON, &doc)
			ch, ok := doc["channels"].(map[string]any)
			if !ok || ch["web"] != true || ch["telegram"] != false {
				t.Fatal("web not enabled independently")
			}
			env := doc["env"].(map[string]any)
			want := "https://agents.example.com/maria"
			if mode == "Subdomain" {
				want = "https://maria.agents.example.com"
			}
			if env["HERMES_DASHBOARD_PUBLIC_URL"] != want {
				t.Fatalf("wrong public URL: %v", env["HERMES_DASHBOARD_PUBLIC_URL"])
			}
			if len(b.SecretData) != 3 {
				t.Fatalf("wrong credential roles: %d", len(b.SecretData))
			}
		})
	}
}
func TestRejectUnsafeWebSettings(t *testing.T) {
	cases := []string{
		strings.Replace(validWeb, "agents.example.com", "https://agents.example.com/x", 1),
		strings.Replace(validWeb, `"Path"`, `"Unknown"`, 1),
		strings.Replace(validWeb, `"10.42.0.0/16"`, `"0.0.0.0/0"`, 1),
		strings.Replace(validWeb, `"podSelector":{"matchLabels":{"gateway.networking.k8s.io/gateway-name":"external"}}`, `"podSelector":{}`, 1),
	}
	for _, raw := range cases {
		h := webFixture(t, raw, true)
		_, r := fixture()
		if Validate(h, r) == nil {
			t.Fatal("unsafe Web accepted")
		}
	}
}
func TestDashboardCannotBypassManagedConfig(t *testing.T) {
	h, r := fixture()
	h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"dashboard":{"public_url":"https://evil.invalid"}}`)}
	if Validate(h, r) == nil {
		t.Fatal("dashboard extraConfig accepted")
	}
	h.Spec.ExtraConfig = nil
	h.Spec.ExtraEnv = map[string]string{"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD": "secret"}
	if Validate(h, r) == nil {
		t.Fatal("dashboard env override accepted")
	}
}

func TestWebUsernameMustMatchUpstreamWithoutTrimming(t *testing.T) {
	for _, username := range []string{" admin", "admin ", "   ", "\x1cadmin"} {
		h := webFixture(t, validWeb, false)
		h.Spec.Web.Auth.Username = username
		if err := ValidateWeb(h); err == nil {
			t.Fatalf("trimmed username accepted: %q", username)
		}
	}
}

func TestWebListenerSectionName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"https.agents.example.com", true},
		{strings.Repeat("a", 63) + "." + strings.Repeat("b", 63), true},
		{strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61), true},
		{"", false}, {"HTTPS", false}, {"https..agents", false}, {"-https", false},
		{strings.Repeat("a", 254), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := webFixture(t, validWeb, false)
			h.Spec.Web.GatewayRef.SectionName = tc.name
			if err := ValidateWeb(h); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
