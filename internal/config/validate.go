package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/runtimecatalog"
)

// Version-specific exclusions supplement the exact leaves rendered below. Root
// aliases and auth/provider routing containers can bypass a seemingly safe leaf.
var protected = []string{
	"model",
	"provider",
	"base_url",
	"api_key",
	"api_mode",
	"max_turns",
	"providers",
	"custom_providers",
	"agent.reasoning_effort",
	"agent.reasoning_overrides",
	"agent.max_turns",
	"agent.run_budget_seconds",
	"agent.disabled_toolsets",
	"agent.system_prompt",
	"display.personality",
	"display.personalities",
	"personalities",
	"soul",
	"system_prompt",
	"terminal.backend",
	"terminal.cwd",
	"terminal.timeout",
	"platform_toolsets.telegram",
	"memory.memory_char_limit",
	"memory.user_char_limit",
	"memory.memory_enabled",
	"memory.user_profile_enabled",
	"gateway.platforms",
	"gateway.telegram",
	"gateway.proxy_url",
	"gateway.proxy_key",
	"gateway.relay_url",
	"gateway.multiplex_profiles",
	"gateway.unauthorized_dm_behavior",
	"telegram",
	"platforms",
	"profiles",
	"web",
	"web_ui",
	"webhook",
	"api_server",
}
var otherPlatforms = strings.Fields("local discord whatsapp whatsapp_cloud slack signal mattermost matrix homeassistant email sms dingtalk api_server webhook msgraph_webhook feishu wecom wecom_callback weixin bluebubbles qqbot yuanbao relay a2a buzz google_chat irc line ntfy photon raft simplex teams")
var reserved = map[string]bool{}

func init() {
	for _, p := range otherPlatforms {
		protected = append(protected, p, "gateway."+p)
	}
	for _, k := range strings.Fields("HOME HERMES_HOME HERMES_PROFILE HERMES_DEFAULT_PROFILE PATH PYTHON_DOTENV_DISABLED GATEWAY_MULTIPLEX_PROFILES HERMES_MODEL_API_KEY CUSTOM_BASE_URL OPENROUTER_BASE_URL HERMES_MODEL HERMES_PROVIDER HERMES_INFERENCE_PROVIDER HERMES_BASE_URL HERMES_REASONING_EFFORT TELEGRAM_BOT_TOKEN TELEGRAM_ALLOWED_USERS TELEGRAM_ALLOW_ALL_USERS TELEGRAM_GROUP_ALLOWED_USERS TELEGRAM_GROUP_ALLOWED_CHATS TELEGRAM_ALLOW_BOTS TELEGRAM_ALLOWED_CHATS TELEGRAM_GUEST_MODE TERMINAL_CWD TERMINAL_ENV TERMINAL_BACKEND TERMINAL_TIMEOUT GATEWAY_ALLOW_ALL_USERS GATEWAY_ALLOWED_USERS GATEWAY_ALLOW_BOTS GATEWAY_PROXY_URL GATEWAY_PROXY_KEY GATEWAY_RELAY_URL MESSAGING_CWD OPENROUTER_API_KEY") {
		reserved[k] = true
	}
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var userID = regexp.MustCompile(`^[1-9][0-9]*$`)
var chatID = regexp.MustCompile(`^-[1-9][0-9]*$`)

func invalid(path, reason string) error { return fmt.Errorf("%s: %s", path, reason) }
func reservedEnv(k string) bool {
	return reserved[k] || strings.HasPrefix(k, "PYTHON") || strings.HasPrefix(k, "LD_") || strings.HasPrefix(k, "XDG_") || strings.HasPrefix(k, "HERMES_OPERATOR_")
}
func secretField(k string) bool {
	k = strings.ToLower(k)
	return k == "api_key" || k == "api" || k == "token" || k == "bot_token" || k == "password" || k == "secret" || k == "access_token" || k == "client_secret" || k == "key_cmd" || k == "key_env" || k == "api_key_env" || k == "credential"
}
func extra(h *v1.Hermes) (map[string]any, error) {
	out := map[string]any{}
	if h.Spec.ExtraConfig != nil && len(h.Spec.ExtraConfig.Raw) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(h.Spec.ExtraConfig.Raw))
		decoder.UseNumber()
		if err := decoder.Decode(&out); err != nil || out == nil {
			return nil, invalid("spec.extraConfig", "must be a JSON object")
		}
	}
	return out, nil
}
func walkExtra(h *v1.Hermes, value any, path []string) error {
	if len(path) > 16 {
		return invalid("spec.extraConfig."+strings.Join(path, "."), "maximum depth exceeded")
	}
	if object, ok := value.(map[string]any); ok {
		for k, v := range object {
			if k == "" || strings.ContainsAny(k, "\x00\r\n") {
				return invalid("spec.extraConfig", "invalid key")
			}
			if err := walkExtra(h, v, append(append([]string{}, path...), k)); err != nil {
				return err
			}
		}
		return nil
	}
	p := strings.Join(path, ".")
	for _, blocked := range protected {
		if p == blocked || strings.HasPrefix(p, blocked+".") || strings.HasPrefix(blocked, p+".") {
			return invalid("spec.extraConfig."+p, "conflicts with managed or personal path")
		}
	}
	if len(path) > 0 && secretField(path[len(path)-1]) && value != nil {
		str, ok := value.(string)
		if !ok || !strings.HasPrefix(str, "${") || !strings.HasSuffix(str, "}") {
			return invalid("spec.extraConfig."+p, "credential literals are forbidden")
		}
		if _, ok := h.Spec.Credentials.Env[str[2:len(str)-1]]; !ok {
			return invalid("spec.extraConfig."+p, "requires a credentials.env reference")
		}
	}
	// Lists are ownership leaves, but credential literals can be nested within them.
	if list, ok := value.([]any); ok {
		for _, v := range list {
			if err := walkExtra(h, v, append(path, "[]")); err != nil {
				return err
			}
		}
	}
	return nil
}
func Validate(h *v1.Hermes, r runtimecatalog.Release) error {
	if h.Spec.Version != r.Version || r.Version != "v2026.9.14" {
		return invalid("spec.version", "UnsupportedVersion")
	}
	if h.Spec.Image.Digest != "" && h.Spec.Image.Digest != r.ImageDigest {
		return invalid("spec.image.digest", "must match release digest")
	}
	if h.Spec.Model.Provider != "custom" {
		return invalid("spec.model.provider", "UnsupportedProvider")
	}
	if strings.TrimSpace(h.Spec.Model.Name) == "" {
		return invalid("spec.model.name", "required")
	}
	u, err := url.Parse(h.Spec.Model.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(h.Spec.Model.BaseURL, "#") {
		return invalid("spec.model.baseURL", "requires absolute HTTP(S) URL without credentials, query or fragment")
	}
	if h.Spec.Model.APIMode != "" && h.Spec.Model.APIMode != "chat_completions" {
		return invalid("spec.model.apiMode", "unsupported custom API mode")
	}
	if h.Spec.Model.Auth != "" && h.Spec.Model.Auth != "APIKey" && h.Spec.Model.Auth != "None" {
		return invalid("spec.model.auth", "unsupported auth mode")
	}
	if h.Spec.Model.Auth == "None" && h.Spec.Model.APIKeySecretRef != nil {
		return invalid("spec.model.apiKeySecretRef", "must be absent with auth None")
	}
	for path, value := range map[string]*int64{"spec.model.contextLength": h.Spec.Model.ContextLength, "spec.memory.charLimit": h.Spec.Memory.CharLimit, "spec.memory.userCharLimit": h.Spec.Memory.UserCharLimit} {
		if value != nil && *value < 1 {
			return invalid(path, "must be positive")
		}
	}
	for path, value := range map[string]*int32{"spec.agent.maxTurns": h.Spec.Agent.MaxTurns, "spec.agent.runBudgetSeconds": h.Spec.Agent.RunBudgetSeconds} {
		if value != nil && *value < 0 {
			return invalid(path, "must be nonnegative")
		}
	}
	if h.Spec.Terminal.TimeoutSeconds != nil && *h.Spec.Terminal.TimeoutSeconds < 1 {
		return invalid("spec.terminal.timeoutSeconds", "must be positive")
	}
	for key, value := range h.Spec.Reasoning.Overrides {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return invalid("spec.reasoning.overrides", "model and effort must be nonempty")
		}
	}
	if h.Spec.Tools.Enabled != nil && len(h.Spec.Tools.Enabled) == 0 || len(h.Spec.Tools.Enabled) > 128 || len(h.Spec.Tools.Disabled) > 128 {
		return invalid("spec.tools", "invalid toolset count")
	}
	if len(h.Spec.Telegram.AllowedUserIDs) == 0 || len(h.Spec.Telegram.AllowedUserIDs) > 256 {
		return invalid("spec.telegram.allowedUserIDs", "requires 1 to 256 IDs")
	}
	for _, id := range h.Spec.Telegram.AllowedUserIDs {
		if !userID.MatchString(id) {
			return invalid("spec.telegram.allowedUserIDs", "invalid sender ID")
		}
	}
	g := h.Spec.Telegram.Groups
	if g.Enabled != (len(g.AllowedChatIDs) > 0) || len(g.AllowedChatIDs) > 256 {
		return invalid("spec.telegram.groups.allowedChatIDs", "nonempty only when enabled")
	}
	for _, id := range g.AllowedChatIDs {
		if !chatID.MatchString(id) {
			return invalid("spec.telegram.groups.allowedChatIDs", "invalid group ID")
		}
	}
	for _, name := range h.Spec.Tools.Enabled {
		for _, off := range h.Spec.Tools.Disabled {
			if name == off {
				return invalid("spec.tools", "enabled and disabled overlap")
			}
		}
	}
	if len(h.Spec.ExtraEnv) > 64 || len(h.Spec.Credentials.Env) > 64 {
		return invalid("spec.extraEnv", "too many environment entries")
	}
	for k := range h.Spec.ExtraEnv {
		if !envName.MatchString(k) || reservedEnv(k) {
			return invalid("spec.extraEnv."+k, "reserved or invalid environment name")
		}
		if _, ok := h.Spec.Credentials.Env[k]; ok {
			return invalid("spec.extraEnv."+k, "duplicates credentials.env")
		}
		if secretField(strings.ToLower(k)) || strings.HasSuffix(k, "_API_KEY") || strings.HasSuffix(k, "_TOKEN") || strings.HasSuffix(k, "_PASSWORD") || strings.HasSuffix(k, "_SECRET") {
			return invalid("spec.extraEnv."+k, "use credentials.env")
		}
	}
	for k, ref := range h.Spec.Credentials.Env {
		if !envName.MatchString(k) || reservedEnv(k) || ref.Key == "" {
			return invalid("spec.credentials.env."+k, "reserved name or missing key")
		}
	}
	cfg, err := extra(h)
	if err != nil {
		return err
	}
	if err = walkExtra(h, cfg, nil); err != nil {
		return err
	}
	raw, err := json.Marshal(h.Spec)
	if err != nil || len(raw) > 256*1024 {
		return invalid("spec", "configuration exceeds 256 KiB")
	}
	return nil
}
