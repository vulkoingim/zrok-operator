package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZrokEnvironmentSpec defines the desired state of ZrokEnvironment.
type ZrokEnvironmentSpec struct {
	// ApiEndpoint is the zrok controller API URL.
	// Defaults to https://api-v2.zrok.io when empty. Set for self-hosted instances.
	// +optional
	ApiEndpoint string `json:"apiEndpoint,omitempty"`

	// UniqueID prefixes the remote zrok environment host and description
	// (`{uniqueID}/zrok-operator/{namespace}/{name}`). When empty, defaults to
	// the UUID (metadata.uid) of the kube-system Namespace:
	//
	//	kubectl get ns kube-system -o jsonpath='{.metadata.uid}'
	//
	// Applied at Enable; changing this later does not rename an existing remote env.
	// +optional
	UniqueID string `json:"uniqueID,omitempty"`

	// EnableTokenSecretRef references a Secret key containing the zrok account enable token.
	// +kubebuilder:validation:Required
	EnableTokenSecretRef corev1.SecretKeySelector `json:"enableTokenSecretRef"`

	// Agent configures the per-environment zrok2 agent Deployment.
	// +optional
	Agent AgentSpec `json:"agent,omitempty"`

	// ReclaimPolicy controls whether the remote zrok environment is disabled on delete.
	// +kubebuilder:default=Delete
	// +kubebuilder:validation:Enum=Delete;Retain
	// +optional
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// AgentSpec configures the zrok2 agent data-plane Deployment.
type AgentSpec struct {
	// Image is the container image for the agent. Defaults to openziti/zrok2 pinned version.
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas is the desired agent replica count. Must be 1 until agent HA is supported.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ConsolePort is the agent HTTP console / gRPC-gateway port.
	// +kubebuilder:default=8888
	// +optional
	ConsolePort int32 `json:"consolePort,omitempty"`

	// Persistence configures the PVC that stores ~/.zrok2 state.
	// +optional
	Persistence AgentPersistence `json:"persistence,omitempty"`

	// Resources for the agent container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentPersistence configures PVC storage for the agent HOME directory.
type AgentPersistence struct {
	// Size of the PVC. Defaults to 1Gi.
	// +optional
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName for the PVC. Uses cluster default when unset.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// ZrokEnvironmentStatus defines the observed state of ZrokEnvironment.
type ZrokEnvironmentStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// EnvZID is the Ziti identity of the enabled zrok environment.
	// +optional
	EnvZID string `json:"envZID,omitempty"`

	// UniqueID is the prefix used at Enable (spec.uniqueID, or kube-system Namespace UUID).
	// +optional
	UniqueID string `json:"uniqueID,omitempty"`

	// AgentService is the cluster DNS name of the agent Service.
	// +optional
	AgentService string `json:"agentService,omitempty"`

	// AgentReady reports whether the agent Deployment is available.
	// +optional
	AgentReady bool `json:"agentReady,omitempty"`

	// Conditions represent the latest available observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=zrokkenv
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Agent",type=boolean,JSONPath=`.status.agentReady`
// +kubebuilder:printcolumn:name="EnvZID",type=string,JSONPath=`.status.envZID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZrokEnvironment is the Schema for the zrokenvironments API.
type ZrokEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZrokEnvironmentSpec   `json:"spec,omitempty"`
	Status ZrokEnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZrokEnvironmentList contains a list of ZrokEnvironment.
type ZrokEnvironmentList struct {
	metav1.TypeMeta `                  json:",inline"`
	metav1.ListMeta `                  json:"metadata,omitempty"`
	Items           []ZrokEnvironment `json:"items"`
}

func init() {
	registerTypes(&ZrokEnvironment{}, &ZrokEnvironmentList{})
}
