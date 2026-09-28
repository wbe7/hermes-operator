package v1alpha1

import networkingv1 "k8s.io/api/networking/v1"

// WebSpec publishes the stock dashboard through an administrator-provided HTTPS Gateway.
// +kubebuilder:validation:XValidation:rule="!self.enabled || (has(self.routing) && has(self.gatewayRef) && has(self.network))",message="enabled web requires routing, gatewayRef and network"
type WebSpec struct {
	// +kubebuilder:default=false
	Enabled    bool           `json:"enabled,omitempty"`
	Routing    WebRoutingSpec `json:"routing,omitempty,omitzero"`
	GatewayRef WebGatewayRef  `json:"gatewayRef,omitempty,omitzero"`
	Auth       WebAuthSpec    `json:"auth,omitempty,omitzero"`
	Network    WebNetworkSpec `json:"network,omitempty,omitzero"`
}
type WebRoutingSpec struct {
	// +kubebuilder:validation:Enum=Subdomain;Path
	Mode string `json:"mode"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	BaseDomain string `json:"baseDomain"`
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name,omitempty"`
}
type WebGatewayRef struct {
	// +kubebuilder:validation:MinLength=1
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	// +kubebuilder:validation:MinLength=1
	SectionName string `json:"sectionName"`
}
type WebAuthSpec struct {
	// +kubebuilder:default=admin
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Username string `json:"username,omitempty"`
}
type WebNetworkSpec struct {
	// IngressFrom contains bounded Gateway sources, used only on TCP 9119.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	IngressFrom []networkingv1.NetworkPolicyPeer `json:"ingressFrom"`
	// TrustedProxyCIDRs authorizes forwarded HTTPS metadata, never an unbounded /0.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=49
	TrustedProxyCIDRs []string `json:"trustedProxyCIDRs"`
}
type WebStatus struct {
	URL      string           `json:"url,omitempty"`
	RouteRef *ObjectReference `json:"routeRef,omitempty"`
}
