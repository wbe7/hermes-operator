package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Hermes represents one managed Hermes installation.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=hms
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 40",message="metadata.name must be at most 40 characters"
type Hermes struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HermesSpec   `json:"spec"`
	Status            HermesStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type HermesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hermes `json:"items"`
}

// HermesSpec is the declarative startup contract.
// +kubebuilder:validation:XValidation:rule="has(self.storage.create) != has(self.storage.existingClaim)",message="exactly one of storage.create or storage.existingClaim is required"
type HermesSpec struct {
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// +kubebuilder:default:={}
	Image       ImageSpec       `json:"image,omitempty"`
	Credentials CredentialsSpec `json:"credentials,omitempty"`
	Model       ModelSpec       `json:"model"`
	// +kubebuilder:default:={}
	Reasoning ReasoningSpec `json:"reasoning,omitempty"`
	Telegram  TelegramSpec  `json:"telegram"`
	// +kubebuilder:default:={}
	Agent AgentSpec `json:"agent,omitempty"`
	// +kubebuilder:default:={}
	Terminal TerminalSpec `json:"terminal,omitempty"`
	Tools    ToolsSpec    `json:"tools,omitempty"`
	Memory   MemorySpec   `json:"memory,omitempty"`
	// +kubebuilder:default:={}
	Resources ResourcesSpec `json:"resources,omitempty"`
	// +kubebuilder:default:={}
	Scheduling SchedulingSpec `json:"scheduling,omitempty"`
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	ExtraConfig *runtime.RawExtension `json:"extraConfig,omitempty"`
	// +kubebuilder:validation:MaxProperties=64
	ExtraEnv map[string]string `json:"extraEnv,omitempty"`
	Storage  StorageSpec       `json:"storage"`
	Network  NetworkSpec       `json:"network,omitempty"`
}

type ImageSpec struct {
	// +kubebuilder:default="docker.io/nousresearch/hermes-agent"
	Repository string `json:"repository,omitempty"`
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	Digest string `json:"digest,omitempty"`
	// +kubebuilder:validation:Enum=IfNotPresent;Always
	// +kubebuilder:default=IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

type SecretKeyRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

type ModelSecretKeyRef struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=MODEL_API_KEY
	Key string `json:"key,omitempty"`
}

type TelegramSecretKeyRef struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=TELEGRAM_BOT_TOKEN
	Key string `json:"key,omitempty"`
}

type CredentialsSpec struct {
	SecretName string `json:"secretName,omitempty"`
	// +kubebuilder:validation:MaxProperties=64
	Env map[string]SecretKeyRef `json:"env,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.provider != 'auto'",message="model.provider auto is not supported"
// +kubebuilder:validation:XValidation:rule="self.provider != 'custom' || has(self.baseURL)",message="model.baseURL is required for custom provider"
// +kubebuilder:validation:XValidation:rule="self.auth != 'None' || (self.provider == 'custom' && has(self.baseURL))",message="auth None requires a custom endpoint"
// +kubebuilder:validation:XValidation:rule="self.auth != 'None' || !has(self.apiKeySecretRef)",message="apiKeySecretRef must be absent when auth is None"
// +kubebuilder:validation:XValidation:rule="!has(self.baseURL) || (isURL(self.baseURL) && (url(self.baseURL).getScheme() == 'http' || url(self.baseURL).getScheme() == 'https') && size(url(self.baseURL).getHost()) > 0 && !self.baseURL.contains('@') && !self.baseURL.contains('?') && !self.baseURL.contains('#'))",message="baseURL must be an absolute HTTP(S) URL without userinfo, query, or fragment"
type ModelSpec struct {
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Pattern=`^https?://[^/?#@]+(?::[0-9]+)?(?:/[^?#]*)?$`
	// +kubebuilder:validation:MaxLength=2048
	BaseURL string `json:"baseURL,omitempty"`
	// +kubebuilder:validation:Enum=chat_completions;responses;anthropic_messages
	APIMode string `json:"apiMode,omitempty"`
	// +kubebuilder:validation:Enum=APIKey;None
	// +kubebuilder:default=APIKey
	Auth            string             `json:"auth,omitempty"`
	APIKeySecretRef *ModelSecretKeyRef `json:"apiKeySecretRef,omitempty"`
	// +kubebuilder:validation:Minimum=1
	ContextLength *int64 `json:"contextLength,omitempty"`
}

type ReasoningSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default=xhigh
	Effort    string            `json:"effort,omitempty"`
	Overrides map[string]string `json:"overrides,omitempty"`
}

type TelegramSpec struct {
	BotTokenSecretRef *TelegramSecretKeyRef `json:"botTokenSecretRef,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^[1-9][0-9]*$`
	AllowedUserIDs []string `json:"allowedUserIDs"`
	// +kubebuilder:default:={}
	Groups TelegramGroupsSpec `json:"groups,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.enabled) && self.enabled) ? (has(self.allowedChatIDs) && size(self.allowedChatIDs) > 0) : (!has(self.allowedChatIDs) || size(self.allowedChatIDs) == 0)",message="allowedChatIDs must be nonempty only when groups are enabled"
type TelegramGroupsSpec struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:MaxItems=256
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^-[1-9][0-9]*$`
	AllowedChatIDs []string `json:"allowedChatIDs,omitempty"`
}

type AgentSpec struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=50
	MaxTurns *int32 `json:"maxTurns,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=600
	RunBudgetSeconds *int32 `json:"runBudgetSeconds,omitempty"`
}

type TerminalSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=300
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.enabled) || !has(self.disabled) || !self.enabled.exists(x, x in self.disabled)",message="enabled and disabled toolsets must not overlap"
type ToolsSpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=128
	// +listType=set
	Enabled []string `json:"enabled,omitempty"`
	// +kubebuilder:validation:MaxItems=128
	// +listType=set
	Disabled []string `json:"disabled,omitempty"`
}

type MemorySpec struct {
	// +kubebuilder:validation:Minimum=1
	CharLimit *int64 `json:"charLimit,omitempty"`
	// +kubebuilder:validation:Minimum=1
	UserCharLimit *int64 `json:"userCharLimit,omitempty"`
}

// ResourcesSpec limits the container's standard Kubernetes resources.
// +kubebuilder:validation:XValidation:rule="!has(self.requests) || !has(self.limits) || self.requests.all(k, !(k in self.limits) || quantity(self.requests[k]).compareTo(quantity(self.limits[k])) <= 0)",message="each resource request must be less than or equal to its limit"
type ResourcesSpec struct {
	// +kubebuilder:default:={cpu:100m,memory:512Mi,ephemeral-storage:256Mi}
	Requests ResourceList `json:"requests,omitempty"`
	// +kubebuilder:default:={cpu:"2",memory:2Gi,ephemeral-storage:2Gi}
	Limits ResourceList `json:"limits,omitempty"`
}

// ResourceList retains Kubernetes quantity and resource-name semantics while
// bounding CEL validation cost.
// +kubebuilder:validation:MaxProperties=8
type ResourceList map[corev1.ResourceName]resource.Quantity

type SchedulingSpec struct {
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity     *corev1.Affinity    `json:"affinity,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="has(self.create) != has(self.existingClaim)",message="exactly one storage source is required"
// +kubebuilder:validation:XValidation:rule="self.deletionPolicy != 'Delete' || has(self.create)",message="deletionPolicy Delete is only valid with storage.create"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.create) || has(self.create)",message="storage source is immutable"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.existingClaim) || self.existingClaim == oldSelf.existingClaim",message="storage source is immutable"
type StorageSpec struct {
	Create *StorageCreateSpec `json:"create,omitempty"`
	// +kubebuilder:validation:MinLength=1
	ExistingClaim string `json:"existingClaim,omitempty"`
	// +kubebuilder:validation:Enum=Retain;Delete
	// +kubebuilder:default=Retain
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="quantity(self.size).isGreaterThan(quantity('0'))",message="storage size must be positive"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.size) || quantity(self.size).compareTo(quantity(oldSelf.size)) >= 0",message="storage size cannot shrink"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.storageClassName) ? (has(self.storageClassName) && self.storageClassName == oldSelf.storageClassName) : !has(self.storageClassName)",message="storageClassName is immutable"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.accessMode) || self.accessMode == oldSelf.accessMode",message="accessMode is immutable"
type StorageCreateSpec struct {
	// +kubebuilder:default="10Gi"
	Size             resource.Quantity `json:"size,omitempty"`
	StorageClassName *string           `json:"storageClassName,omitempty"`
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteOncePod
	// +kubebuilder:default=ReadWriteOnce
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

type NetworkSpec struct {
	// +kubebuilder:validation:MaxItems=128
	AllowPrivate []PrivateNetworkException `json:"allowPrivate,omitempty"`
	// +kubebuilder:validation:MaxItems=128
	// +listType=set
	// +kubebuilder:validation:items:MaxLength=49
	// +kubebuilder:validation:XValidation:rule="self.all(c, isCIDR(c))",message="additionalBlockedCIDRs entries must be CIDRs"
	AdditionalBlockedCIDRs []string `json:"additionalBlockedCIDRs,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="isIP(self.ip)",message="ip must be one IP address, not a CIDR or hostname"
type PrivateNetworkException struct {
	// +kubebuilder:validation:MaxLength=45
	IP string `json:"ip"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Ports []NetworkPort `json:"ports,omitempty"`
}

type NetworkPort struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default=TCP
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

type HermesStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	AppliedRevision    string             `json:"appliedRevision,omitempty"`
	ResolvedImage      string             `json:"resolvedImage,omitempty"`
	WorkloadRef        *ObjectReference   `json:"workloadRef,omitempty"`
	StorageRef         *StorageReference  `json:"storageRef,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type ObjectReference struct {
	Name string `json:"name"`
	UID  string `json:"uid,omitempty"`
}
type StorageReference struct {
	Name   string `json:"name"`
	UID    string `json:"uid,omitempty"`
	Origin string `json:"origin,omitempty"`
}

func init() { SchemeBuilder.Register(&Hermes{}, &HermesList{}) }
