package config

import (
	"net/url"
	"regexp"
	"strings"

	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func STTEnabled(h *v1.Hermes) bool { return h.Spec.STT != nil && h.Spec.STT.Enabled }

// sttKeyRef resolves the credential role independently of the endpoint. A nil
// result means disabled/keyless, never a missing dependency to silently replace.
func sttKeyRef(h *v1.Hermes) *corev1.SecretKeySelector {
	if !STTEnabled(h) || h.Spec.STT.Auth == "None" {
		return nil
	}
	s := h.Spec.STT
	name, key := PrimarySecretName(h), "MODEL_API_KEY"
	if s.APIKeySecretRef != nil {
		key = "STT_API_KEY"
		if s.APIKeySecretRef.Name != "" {
			name = s.APIKeySecretRef.Name
		}
		if s.APIKeySecretRef.Key != "" {
			key = s.APIKeySecretRef.Key
		}
	} else {
		if h.Spec.Model.Auth == "None" {
			return nil
		}
		if ref := h.Spec.Model.APIKeySecretRef; ref != nil {
			if ref.Name != "" {
				name = ref.Name
			}
			if ref.Key != "" {
				key = ref.Key
			}
		}
	}
	return &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: key}
}

var sttLanguage = regexp.MustCompile(`^(auto|[a-z]{2,3})$`)

func ValidateSTT(h *v1.Hermes) error {
	s := h.Spec.STT
	if s == nil {
		return nil
	}
	if (s.Enabled || s.Model != "") && (strings.TrimSpace(s.Model) == "" || len(s.Model) > 256) {
		return invalid("spec.stt.model", "requires a nonempty model of at most 256 bytes when enabled")
	}
	if s.Auth != "" && s.Auth != "Inherit" && s.Auth != "APIKey" && s.Auth != "None" {
		return invalid("spec.stt.auth", "must be Inherit, APIKey or None")
	}
	if s.Auth == "None" && s.APIKeySecretRef != nil {
		return invalid("spec.stt.apiKeySecretRef", "must be absent with auth None")
	}
	if s.Enabled && s.Auth == "APIKey" && s.APIKeySecretRef == nil && h.Spec.Model.Auth == "None" {
		return invalid("spec.stt.apiKeySecretRef", "required when model auth is None and STT auth is APIKey")
	}
	if s.BaseURL != "" {
		u, err := url.Parse(s.BaseURL)
		if err != nil || len(s.BaseURL) > 2048 || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(s.BaseURL, "#") {
			return invalid("spec.stt.baseURL", "requires absolute HTTP(S) URL without credentials, query or fragment")
		}
	}
	if s.Language != "" && !sttLanguage.MatchString(s.Language) {
		return invalid("spec.stt.language", "must be auto or a lowercase two/three-letter language code")
	}
	if ref := s.APIKeySecretRef; ref != nil {
		if ref.Name != "" && len(validation.IsDNS1123Subdomain(ref.Name)) != 0 {
			return invalid("spec.stt.apiKeySecretRef.name", "invalid Secret name")
		}
		if ref.Key != "" && len(validation.IsConfigMapKey(ref.Key)) != 0 {
			return invalid("spec.stt.apiKeySecretRef.key", "invalid Secret key")
		}
	}
	return nil
}

func renderSTT(h *v1.Hermes, doc *startupInput) {
	s := h.Spec.STT
	if s == nil {
		s = &v1.STTSpec{}
	}
	language := s.Language
	if language == "" {
		language = "ru"
	}
	if language == "auto" {
		language = ""
	}
	echo := s.EchoTranscripts == nil || *s.EchoTranscripts
	model, endpoint := "", ""
	var key any = ""
	if STTEnabled(h) {
		model, endpoint = s.Model, s.BaseURL
		if endpoint == "" {
			endpoint = h.Spec.Model.BaseURL
		}
		if sttKeyRef(h) == nil {
			key = "no-key-required"
		} else {
			key = doc.Env["HERMES_STT_API_KEY"]
		}
	}
	if sttKeyRef(h) == nil {
		doc.Env["HERMES_STT_API_KEY"] = ""
	}
	// Do not clear shared OPENAI_API_KEY/VOICE_TOOLS_OPENAI_KEY: other tools
	// may use them. Explicit nested credentials win in the pinned STT resolver.
	doc.Env["HERMES_LOCAL_STT_LANGUAGE"] = ""
	doc.Env["STT_OPENAI_BASE_URL"] = ""
	for path, value := range map[string]any{
		"stt.enabled": STTEnabled(h), "stt_enabled": STTEnabled(h),
		"stt.echo_transcripts": echo, "stt_echo_transcripts": echo,
		"stt.provider": "openai", "stt.use_gateway": false,
		"stt.language": language, "stt.openai.language": language,
		"stt.openai.model": model, "stt.openai.base_url": endpoint, "stt.openai.api_key": key,
	} {
		set(doc.Config, path, value)
	}
}
