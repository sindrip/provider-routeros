package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderConfigSpec defines one RouterOS REST endpoint and its local
// authentication and TLS material.
// +kubebuilder:validation:XValidation:rule="!has(self.tls) || self.endpoint.startsWith('https://')",message="tls settings require an https endpoint"
// +kubebuilder:validation:XValidation:rule="!has(self.requestTimeout) || duration(self.requestTimeout) >= duration('5s')",message="requestTimeout must be at least 5s"
type ProviderConfigSpec struct {
	// Endpoint is the RouterOS REST base URL, including its http or https scheme.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.startsWith('http://') || self.startsWith('https://')",message="endpoint must use http or https"
	Endpoint string `json:"endpoint"`

	Credentials ProviderCredentials `json:"credentials"`

	// TLS controls HTTPS server verification. Omit it for normal system trust.
	// +optional
	TLS *ProviderTLSConfig `json:"tls,omitempty"`

	// RequestTimeout bounds one RouterOS REST request. Values below five
	// seconds are rejected. The default is 59 seconds.
	// +optional
	RequestTimeout *metav1.Duration `json:"requestTimeout,omitempty"`
}

// ProviderCredentials selects username and password keys in one Secret in the
// ProviderConfig's namespace.
type ProviderCredentials struct {
	SecretRef ProviderCredentialSecretReference `json:"secretRef"`
}

// ProviderCredentialSecretReference selects RouterOS basic-auth credentials.
// The Secret is always resolved in the ProviderConfig's namespace.
type ProviderCredentialSecretReference struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +kubebuilder:default=username
	// +kubebuilder:validation:MinLength=1
	UsernameKey string `json:"usernameKey,omitempty"`

	// +kubebuilder:default=password
	// +kubebuilder:validation:MinLength=1
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ProviderTLSConfig configures HTTPS server verification.
// +kubebuilder:validation:XValidation:rule="!has(self.insecureSkipVerify) || !self.insecureSkipVerify || !has(self.caSecretRef)",message="insecureSkipVerify and caSecretRef are mutually exclusive"
type ProviderTLSConfig struct {
	// InsecureSkipVerify explicitly disables certificate verification.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// CASecretRef selects a PEM CA bundle from a Secret in the
	// ProviderConfig's namespace.
	// +optional
	CASecretRef *xpv2.LocalSecretKeySelector `json:"caSecretRef,omitempty"`
}

// ProviderConfigStatus reflects the observed state of a ProviderConfig.
type ProviderConfigStatus struct {
	xpv2.ProviderConfigStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,routeros}

// ProviderConfig configures RouterOS access for resources in the same
// namespace.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains ProviderConfig objects.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,routeros}

// ProviderConfigUsage records a resource's use of a namespaced ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains ProviderConfigUsage objects.
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}
