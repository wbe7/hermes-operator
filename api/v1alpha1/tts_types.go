package v1alpha1

// TTSSpec configures the pinned Hermes OpenAI-compatible speech synthesis backend.
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.voice)",message="tts.voice is required when enabled"
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.model)",message="tts.model is required when enabled"
// +kubebuilder:validation:XValidation:rule="self.auth != 'None' || !has(self.apiKeySecretRef)",message="TTS apiKeySecretRef must be absent with auth None"
// +kubebuilder:validation:XValidation:rule="!has(self.baseURL) || (isURL(self.baseURL) && (url(self.baseURL).getScheme() == 'http' || url(self.baseURL).getScheme() == 'https') && size(url(self.baseURL).getHost()) > 0 && !self.baseURL.contains('@') && !self.baseURL.contains('?') && !self.baseURL.contains('#'))",message="TTS baseURL must be absolute HTTP(S) without credentials, query or fragment"
type TTSSpec struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`\S`
	Model string `json:"model,omitempty"`
	// BaseURL inherits model.baseURL when omitted.
	// +kubebuilder:validation:MaxLength=2048
	BaseURL string `json:"baseURL,omitempty"`
	// Inherit uses an explicit TTS reference first, otherwise model authentication.
	// +kubebuilder:validation:Enum=Inherit;APIKey;None
	// +kubebuilder:default=Inherit
	Auth            string           `json:"auth,omitempty"`
	APIKeySecretRef *TTSSecretKeyRef `json:"apiKeySecretRef,omitempty"`
	// Voice is a provider-specific voice name or registered ID.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`\S`
	Voice string `json:"voice,omitempty"`
	// +kubebuilder:validation:Enum=OnRequest;VoiceOnly;All
	// +kubebuilder:default=VoiceOnly
	ResponseMode string `json:"responseMode,omitempty"`
	// Speed is a fractional playback multiplier, constrained by admission.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Minimum=0.25
	// +kubebuilder:validation:Maximum=4
	// +kubebuilder:default=1
	Speed *float64 `json:"speed,omitempty"`
	// Language is forwarded as native lang_code; support depends on the server.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`\S`
	Language string `json:"language,omitempty"`
}

type TTSSecretKeyRef struct {
	// Name defaults to the installation's primary Secret, in the same namespace.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[-._a-zA-Z0-9]+$`
	// +kubebuilder:default=TTS_API_KEY
	Key string `json:"key,omitempty"`
}
