package config

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Bundle is a matched immutable ConfigMap/Secret input pair. JSON is nonsensitive.
type Bundle struct {
	JSON       []byte
	SecretData map[string][]byte
	Revision   string
}
type startupInput struct {
	Schema     int            `json:"schema"`
	Release    string         `json:"release"`
	Config     map[string]any `json:"config"`
	Env        map[string]any `json:"env"`
	OwnedPaths [][]string     `json:"ownedPaths"`
	OwnedEnv   []string       `json:"ownedEnv"`
}

func sortedSet(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return compact(out)
}
func compact(in []string) []string {
	out := []string{}
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
func set(cfg map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		if _, ok := cfg[p].(map[string]any); !ok {
			cfg[p] = map[string]any{}
		}
		cfg = cfg[p].(map[string]any)
	}
	cfg[parts[len(parts)-1]] = value
}
func optional[T ~int32 | ~int64](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}
func defaultNumber(p *int32, d int32) int32 {
	if p == nil {
		return d
	}
	return *p
}

// Empty extra objects are no-ops, not an ownership claim over the parent.
func copyExtra(dst map[string]any, src map[string]any) {
	for k, v := range src {
		if m, ok := v.(map[string]any); ok {
			child := map[string]any{}
			copyExtra(child, m)
			if len(child) > 0 {
				dst[k] = child
			}
		} else {
			dst[k] = v
		}
	}
}
func ownedLeaves(v any, path []string, out *[][]string) {
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		if _, ref := m["credential"]; !(ref && len(m) == 1) {
			keys := []string{}
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				ownedLeaves(m[k], append(append([]string{}, path...), k), out)
			}
			return
		}
	}
	*out = append(*out, append([]string{}, path...))
}
func Render(h *v1.Hermes, r runtimecatalog.Release, secrets map[types.NamespacedName]*corev1.Secret) (Bundle, error) {
	if err := Validate(h, r); err != nil {
		return Bundle{}, err
	}
	doc := startupInput{Schema: 1, Release: r.Version, Config: map[string]any{}, Env: map[string]any{}}
	data := map[string][]byte{}
	identities := []secretIdentity{}
	for _, b := range bindings(h) {
		id := types.NamespacedName{Namespace: h.Namespace, Name: b.Ref.Name}
		s := secrets[id]
		if s == nil || len(s.Data[b.Ref.Key]) == 0 {
			return Bundle{}, &DependencyError{Secret: id, Key: b.Ref.Key}
		}
		name := credentialName(b.Ref)
		data[name] = append([]byte{}, s.Data[b.Ref.Key]...)
		doc.Env[b.Env] = map[string]any{"credential": name}
		identities = append(identities, secretIdentity{Namespace: id.Namespace, Name: id.Name, Key: b.Ref.Key, UID: s.UID})
	}
	cfg, _ := extra(h)
	copyExtra(doc.Config, cfg)
	var modelKey any
	// Resolve by role, never by position in the sorted reference list.
	if h.Spec.Model.Auth == "None" {
		modelKey = "no-key-required"
		doc.Env["HERMES_MODEL_API_KEY"] = ""
	} else {
		modelKey = doc.Env["HERMES_MODEL_API_KEY"]
	}
	// Match the pinned resolver's hostname rule, including subdomains and a
	// trailing DNS dot. Other endpoints must not lend their key to auxiliary
	// OpenRouter clients or overwrite the user's independent OpenRouter setup.
	endpoint, _ := url.Parse(h.Spec.Model.BaseURL) // Validate has checked the URL.
	host := strings.TrimRight(strings.ToLower(endpoint.Hostname()), ".")
	if host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai") {
		doc.Env["OPENROUTER_API_KEY"] = modelKey
		doc.Env["OPENROUTER_BASE_URL"] = ""
	}
	mode := h.Spec.Model.APIMode
	if mode == "" {
		mode = "chat_completions"
	}
	effort := h.Spec.Reasoning.Effort
	if effort == "" {
		effort = "xhigh"
	}
	overrides := map[string]any{}
	for k, v := range h.Spec.Reasoning.Overrides {
		overrides[k] = v
	}
	base := map[string]any{
		"model.default":                      h.Spec.Model.Name,
		"model.provider":                     "custom",
		"model.base_url":                     h.Spec.Model.BaseURL,
		"model.api_mode":                     mode,
		"model.api_key":                      modelKey,
		"model.api":                          nil,
		"model.context_length":               optional(h.Spec.Model.ContextLength),
		"agent.reasoning_effort":             effort,
		"agent.reasoning_overrides":          overrides,
		"agent.max_turns":                    defaultNumber(h.Spec.Agent.MaxTurns, 50),
		"agent.run_budget_seconds":           defaultNumber(h.Spec.Agent.RunBudgetSeconds, 600),
		"agent.disabled_toolsets":            sortedSet(h.Spec.Tools.Disabled),
		"terminal.backend":                   "local",
		"terminal.cwd":                       "/opt/data/workspace",
		"terminal.timeout":                   defaultNumber(h.Spec.Terminal.TimeoutSeconds, 300),
		"memory.memory_char_limit":           optional(h.Spec.Memory.CharLimit),
		"memory.user_char_limit":             optional(h.Spec.Memory.UserCharLimit),
		"platform_toolsets.telegram":         nil,
		"gateway.multiplex_profiles":         false,
		"gateway.proxy_url":                  "",
		"gateway.proxy_key":                  nil,
		"gateway.relay_url":                  "",
		"gateway.unauthorized_dm_behavior":   "ignore",
		"gateway.platforms.telegram.enabled": true,
		"gateway.platforms.telegram.token":   doc.Env["TELEGRAM_BOT_TOKEN"],
	}
	if h.Spec.Tools.Enabled != nil {
		base["platform_toolsets.telegram"] = sortedSet(h.Spec.Tools.Enabled)
	}
	ids := sortedSet(h.Spec.Telegram.AllowedUserIDs)
	chats := []string{"__operator_dm_only__"}
	if h.Spec.Telegram.Groups.Enabled {
		chats = sortedSet(h.Spec.Telegram.Groups.AllowedChatIDs)
	}
	tg := map[string]any{
		"allow_from":               ids,
		"group_allow_from":         ids,
		"allowed_chats":            chats,
		"group_allowed_chats":      []string{},
		"guest_mode":               false,
		"dm_policy":                "allowlist",
		"group_policy":             "allowlist",
		"allow_admin_from":         ids,
		"group_allow_admin_from":   ids,
		"unauthorized_dm_behavior": "ignore",
	}
	for k, v := range tg {
		base["gateway.platforms.telegram.extra."+k] = v
		base["telegram.extra."+k] = v
		base["telegram."+k] = v
	}
	base["telegram.enabled"] = true
	for _, p := range otherPlatforms {
		base["gateway.platforms."+p+".enabled"] = false
		base[p+".enabled"] = false
	}
	for k, v := range base {
		set(doc.Config, k, v)
	}
	for k, v := range h.Spec.ExtraEnv {
		doc.Env[k] = v
	}
	// These precedence paths must also reset a pre-operator .env.
	for k, v := range map[string]string{
		"CUSTOM_BASE_URL":              "",
		"GATEWAY_MULTIPLEX_PROFILES":   "false",
		"TELEGRAM_ALLOWED_USERS":       strings.Join(ids, ","),
		"TELEGRAM_ALLOW_ALL_USERS":     "false",
		"TELEGRAM_GROUP_ALLOWED_USERS": "",
		"TELEGRAM_GROUP_ALLOWED_CHATS": "",
		"TELEGRAM_ALLOW_BOTS":          "false",
		"TELEGRAM_GUEST_MODE":          "false",
		"TELEGRAM_ALLOWED_CHATS":       strings.Join(chats, ","),
		"GATEWAY_ALLOWED_USERS":        "",
		"GATEWAY_ALLOW_ALL_USERS":      "false",
		"GATEWAY_ALLOW_BOTS":           "false",
		"GATEWAY_PROXY_URL":            "",
		"GATEWAY_PROXY_KEY":            "",
		"GATEWAY_RELAY_URL":            "",
		"TERMINAL_ENV":                 "local",
	} {
		doc.Env[k] = v
	}
	ownedLeaves(doc.Config, nil, &doc.OwnedPaths)
	for k := range doc.Env {
		doc.OwnedEnv = append(doc.OwnedEnv, k)
	}
	sort.Strings(doc.OwnedEnv)
	raw, err := json.Marshal(doc)
	if err != nil {
		return Bundle{}, invalid("spec", "cannot encode startup configuration")
	}
	if len(raw) > 256*1024 {
		return Bundle{}, invalid("spec", "rendered configuration exceeds 256 KiB")
	}
	revision := computeRevision(h, r, raw, data, identities)
	return Bundle{JSON: raw, SecretData: data, Revision: revision}, nil
}
