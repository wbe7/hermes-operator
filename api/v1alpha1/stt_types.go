package v1alpha1

// STTSpec configures the pinned Hermes OpenAI-compatible transcription backend.
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.model)",message="stt.model is required when enabled"
// +kubebuilder:validation:XValidation:rule="self.auth != 'None' || !has(self.apiKeySecretRef)",message="STT apiKeySecretRef must be absent with auth None"
// +kubebuilder:validation:XValidation:rule="!has(self.baseURL) || (isURL(self.baseURL) && (url(self.baseURL).getScheme() == 'http' || url(self.baseURL).getScheme() == 'https') && size(url(self.baseURL).getHost()) > 0 && !self.baseURL.contains('@') && !self.baseURL.contains('?') && !self.baseURL.contains('#'))",message="STT baseURL must be absolute HTTP(S) without credentials, query or fragment"
type STTSpec struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`\S`
	// +kubebuilder:validation:XValidation:rule="!(self in ['whisper-large-v3', 'whisper-large-v3-turbo', 'distil-whisper-large-v3-en'])",message="pinned Hermes rewrites this model to whisper-1; use a different server-side model alias"
	Model string `json:"model,omitempty"`
	// BaseURL inherits model.baseURL when omitted.
	// +kubebuilder:validation:MaxLength=2048
	BaseURL string `json:"baseURL,omitempty"`
	// Inherit uses an explicit STT reference first, otherwise model authentication.
	// +kubebuilder:validation:Enum=Inherit;APIKey;None
	// +kubebuilder:default=Inherit
	Auth            string           `json:"auth,omitempty"`
	APIKeySecretRef *STTSecretKeyRef `json:"apiKeySecretRef,omitempty"`
	// Auto omits the language hint sent to the transcription server.
	// +kubebuilder:validation:Pattern=`^(auto|[a-z]{2,3})$`
	// +kubebuilder:default=ru
	Language string `json:"language,omitempty"`
	// +kubebuilder:default=true
	EchoTranscripts *bool `json:"echoTranscripts,omitempty"`
}

type STTSecretKeyRef struct {
	// Name defaults to the installation's primary Secret, in the same namespace.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[-._a-zA-Z0-9]+$`
	// +kubebuilder:default=STT_API_KEY
	Key string `json:"key,omitempty"`
}
