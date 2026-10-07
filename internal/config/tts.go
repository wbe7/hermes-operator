package config

import (
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"math"
	"strings"
)

func TTSEnabled(h *v1.Hermes) bool { return h.Spec.TTS != nil && h.Spec.TTS.Enabled }
func ttsKeyRef(h *v1.Hermes) *corev1.SecretKeySelector {
	if !TTSEnabled(h) || h.Spec.TTS.Auth == "None" {
		return nil
	}
	s := h.Spec.TTS
	name, key := PrimarySecretName(h), "MODEL_API_KEY"
	if s.APIKeySecretRef != nil {
		key = "TTS_API_KEY"
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

// ValidateTTS reuses the shared endpoint/auth/ref contract without inheriting STT
// model-name restrictions or language semantics.
func ValidateTTS(h *v1.Hermes) error {
	s := h.Spec.TTS
	if s == nil {
		return nil
	}
	check := h.DeepCopy()
	check.Spec.STT = &v1.STTSpec{Enabled: s.Enabled, Model: "tts-validation", BaseURL: s.BaseURL, Auth: s.Auth}
	if s.APIKeySecretRef != nil {
		check.Spec.STT.APIKeySecretRef = &v1.STTSecretKeyRef{Name: s.APIKeySecretRef.Name, Key: s.APIKeySecretRef.Key}
	}
	if err := ValidateSTT(check); err != nil {
		return invalid("spec.tts", strings.ReplaceAll(err.Error(), "spec.stt.", ""))
	}
	for path, value := range map[string]string{"model": s.Model, "voice": s.Voice} {
		if (s.Enabled || value != "") && (strings.TrimSpace(value) == "" || len(value) > 256) {
			return invalid("spec.tts."+path, "requires a nonempty value of at most 256 bytes when enabled")
		}
	}
	if s.ResponseMode != "" && s.ResponseMode != "OnRequest" && s.ResponseMode != "VoiceOnly" && s.ResponseMode != "All" {
		return invalid("spec.tts.responseMode", "must be OnRequest, VoiceOnly or All")
	}
	if s.Speed != nil {
		n := *s.Speed
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0.25 || n > 4 {
			return invalid("spec.tts.speed", "must be a number between 0.25 and 4")
		}
	}
	if s.Language != "" && (strings.TrimSpace(s.Language) == "" || len(s.Language) > 64) {
		return invalid("spec.tts.language", "must be a nonempty server-supported language hint of at most 64 bytes")
	}
	if s.Enabled {
		for _, tool := range h.Spec.Tools.Disabled {
			if tool == "tts" {
				return invalid("spec.tools.disabled", "tts conflicts with enabled spec.tts")
			}
		}
	}
	return nil
}

// voiceInput owns the native per-chat mode file independently of config.yaml.
type voiceInput struct {
	Mode  string   `json:"mode"`
	Chats []string `json:"chats"`
}

func renderTTS(h *v1.Hermes, doc *startupInput) {
	s := h.Spec.TTS
	if s == nil {
		s = &v1.TTSSpec{}
	}
	enabled := TTSEnabled(h)
	mode := "off"
	chats := []string{}
	model, voice, endpoint, language := "", "", "", ""
	speed := 1.0
	var key any = ""
	if enabled {
		model, voice, endpoint, language = s.Model, s.Voice, s.BaseURL, s.Language
		if endpoint == "" {
			endpoint = h.Spec.Model.BaseURL
		}
		if s.Speed != nil {
			speed = *s.Speed
		}
		if ttsKeyRef(h) == nil {
			key = "no-key-required"
		} else {
			key = doc.Env["HERMES_TTS_API_KEY"]
		}
		switch s.ResponseMode {
		case "OnRequest":
			mode = "off"
		case "All":
			mode = "all"
		default:
			mode = "voice_only"
		}
	}
	if h.Spec.Telegram != nil {
		chats = append(chats, h.Spec.Telegram.AllowedUserIDs...)
		if h.Spec.Telegram.Groups.Enabled {
			chats = append(chats, h.Spec.Telegram.Groups.AllowedChatIDs...)
		}
	}
	doc.Voice = &voiceInput{Mode: mode, Chats: sortedSet(chats)}
	if ttsKeyRef(h) == nil {
		doc.Env["HERMES_TTS_API_KEY"] = ""
	}
	for path, value := range map[string]any{"tts.provider": "openai", "tts.use_gateway": false, "tts.openai.model": model, "tts.openai.voice": voice, "tts.openai.base_url": endpoint, "tts.openai.api_key": key, "tts.speed": speed, "tts.openai.speed": speed, "tts.openai.language": language, "voice.auto_tts": false} {
		set(doc.Config, path, value)
	}
	disabled := append([]string{}, h.Spec.Tools.Disabled...)
	if !enabled {
		disabled = append(disabled, "tts")
	}
	set(doc.Config, "agent.disabled_toolsets", sortedSet(disabled))
	if enabled && h.Spec.Tools.Enabled != nil {
		set(doc.Config, "platform_toolsets.telegram", sortedSet(append(append([]string{}, h.Spec.Tools.Enabled...), "tts")))
	}
}
