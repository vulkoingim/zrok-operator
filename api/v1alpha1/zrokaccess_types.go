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

// ZrokAccessSpec defines a private share access (consumer side).
type ZrokAccessSpec struct {
	// EnvironmentRef references a ZrokEnvironment in the same namespace.
	// +kubebuilder:validation:Required
	EnvironmentRef corev1.LocalObjectReference `json:"environmentRef"`

	// ShareToken is the private share token to access.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ShareToken string `json:"shareToken"`

	// BindAddress is the local bind address for the access proxy (inside the agent).
	// +kubebuilder:default="0.0.0.0:0"
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// ZrokAccessStatus defines the observed state of ZrokAccess.
type ZrokAccessStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// FrontendEndpoint is where the access is bound (when known).
	// +optional
	FrontendEndpoint string `json:"frontendEndpoint,omitempty"`

	// AccessToken is the agent access token.
	// +optional
	AccessToken string `json:"accessToken,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=zrokaccess
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Token",type=string,JSONPath=`.spec.shareToken`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZrokAccess is the Schema for private share access.
type ZrokAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZrokAccessSpec   `json:"spec,omitempty"`
	Status ZrokAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZrokAccessList contains a list of ZrokAccess.
type ZrokAccessList struct {
	metav1.TypeMeta `             json:",inline"`
	metav1.ListMeta `             json:"metadata,omitempty"`
	Items           []ZrokAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZrokAccess{}, &ZrokAccessList{})
}
