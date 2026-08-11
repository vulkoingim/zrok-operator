/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZrokShareSpec defines the desired state of ZrokShare.
type ZrokShareSpec struct {
	// EnvironmentRef references a ZrokEnvironment in the same namespace.
	// +kubebuilder:validation:Required
	EnvironmentRef corev1.LocalObjectReference `json:"environmentRef"`

	// ShareMode selects public or private sharing.
	//   public  — reachable via frontend URL (ephemeral or reserved name)
	//   private — reachable only via zrok access (optional vanity PrivateShareToken)
	// +kubebuilder:default=public
	// +kubebuilder:validation:Enum=public;private
	// +optional
	ShareMode ShareMode `json:"shareMode,omitempty"`

	// BackendMode selects the zrok backend. Default proxy.
	// +kubebuilder:default=proxy
	// +kubebuilder:validation:Enum=proxy;web;caddy;drive;tcpTunnel;udpTunnel;socks
	// +optional
	BackendMode BackendMode `json:"backendMode,omitempty"`

	// Upstream is the in-cluster target to share.
	// +kubebuilder:validation:Required
	Upstream UpstreamSpec `json:"upstream"`

	// NameSelection reserves a sticky public frontend name (zrok v2 reserved name).
	// Omit for ephemeral public shares (random URL, gone when share ends).
	// Only valid with shareMode=public.
	// +optional
	NameSelection *NameSelectionSpec `json:"nameSelection,omitempty"`

	// PrivateShareToken sets a vanity token for private shares (sticky private identifier).
	// Only valid with shareMode=private. Omit for a random private token.
	// +optional
	PrivateShareToken string `json:"privateShareToken,omitempty"`

	// Insecure skips TLS verification when dialing the upstream target.
	// +optional
	Insecure bool `json:"insecure,omitempty"`

	// Closed enables closed permission mode (requires AccessGrants).
	// +optional
	Closed bool `json:"closed,omitempty"`

	// AccessGrants lists zrok account emails allowed to access a closed share.
	// +optional
	AccessGrants []string `json:"accessGrants,omitempty"`

	// BasicAuthSecretRef references a Secret with keys "username" and "password".
	// +optional
	BasicAuthSecretRef *corev1.LocalObjectReference `json:"basicAuthSecretRef,omitempty"`

	// OAuth configures OAuth front-door protection for public shares.
	// +optional
	OAuth *OAuthSpec `json:"oauth,omitempty"`

	// ReclaimPolicy controls whether reserved names are deleted on CR delete.
	// +kubebuilder:default=Delete
	// +kubebuilder:validation:Enum=Delete;Retain
	// +optional
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// UpstreamSpec identifies the backend target.
type UpstreamSpec struct {
	// URL is a full upstream URL, e.g. http://mysvc.default.svc:80
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(https?|tcp|udp)://.+`
	URL string `json:"url"`
}

// NameSelectionSpec selects a reserved name in a namespace (zrok v2).
// Creates the name via CreateShareName and promotes it with reserved=true.
type NameSelectionSpec struct {
	// Namespace is the namespace token (typically "public").
	// +kubebuilder:default=public
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name is the reserved frontend name within the namespace (DNS label).
	// Frontend URL becomes https://<name>.shares.zrok.io (for public namespace).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Name string `json:"name"`
}

// OAuthSpec configures OAuth for a public share.
type OAuthSpec struct {
	// Provider is google or github.
	// +kubebuilder:validation:Enum=google;github
	Provider string `json:"provider"`

	// EmailDomains restricts access to matching email domain globs.
	// +optional
	EmailDomains []string `json:"emailDomains,omitempty"`

	// RefreshInterval is the OAuth session lifetime (Go duration string).
	// +optional
	RefreshInterval string `json:"refreshInterval,omitempty"`
}

// ZrokShareStatus defines the observed state of ZrokShare.
type ZrokShareStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AssignedURL is the primary public frontend URL.
	// +optional
	AssignedURL string `json:"assignedURL,omitempty"`

	// FrontendEndpoints lists all frontend proxy endpoints for the share.
	// +optional
	FrontendEndpoints []string `json:"frontendEndpoints,omitempty"`

	// ShareToken is the zrok share token.
	// +optional
	ShareToken string `json:"shareToken,omitempty"`

	// Reservation describes the frontend identity lifecycle:
	// ephemeral (random public name), reserved (sticky NameSelection), or private.
	// +optional
	// +kubebuilder:validation:Enum=ephemeral;reserved;private
	Reservation string `json:"reservation,omitempty"`

	// Conditions represent the latest available observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=zrokshare
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.shareMode`
// +kubebuilder:printcolumn:name="Reservation",type=string,JSONPath=`.status.reservation`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.assignedURL`
// +kubebuilder:printcolumn:name="Token",type=string,JSONPath=`.status.shareToken`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZrokShare is the Schema for the zrokshares API.
type ZrokShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZrokShareSpec   `json:"spec,omitempty"`
	Status ZrokShareStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZrokShareList contains a list of ZrokShare.
type ZrokShareList struct {
	metav1.TypeMeta `            json:",inline"`
	metav1.ListMeta `            json:"metadata,omitempty"`
	Items           []ZrokShare `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZrokShare{}, &ZrokShareList{})
}
