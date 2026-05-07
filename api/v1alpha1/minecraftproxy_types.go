package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Edition representa la edición de Minecraft soportada.
// +kubebuilder:validation:Enum=java;bedrock
type Edition string

const (
	EditionJava Edition = "java"

	EditionBedrock Edition = "bedrock"
)

type MinecraftProxySpec struct {
	// +kubebuilder:validation:Required
	Edition Edition `json:"edition"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Required
	Backend BackendSpec `json:"backend"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	MaxPlayers int32 `json:"maxPlayers,omitempty"`

	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	AssignedPort int32 `json:"assignedPort,omitempty"`

	// +optional
	RateLimit *RateLimitSpec `json:"rateLimit,omitempty"`
}

type BackendSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ServicePort int32 `json:"servicePort,omitempty"`

	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type RateLimitSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	ConnectionsPerMinute int32 `json:"connectionsPerMinute,omitempty"`
}

type MinecraftProxyStatus struct {
	// +kubebuilder:default=false
	Ready bool `json:"ready,omitempty"`

	ActiveConnections int32 `json:"activeConnections,omitempty"`

	// +optional
	AssignedPort int32 `json:"assignedPort,omitempty"`

	// +optional
	Edition Edition `json:"edition,omitempty"`

	// +optional
	LastConnected *metav1.Time `json:"lastConnected,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Edition",type=string,JSONPath=`.spec.edition`
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.hostname`
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.backend.serviceName`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.status.assignedPort`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Connections",type=integer,JSONPath=`.status.activeConnections`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type MinecraftProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MinecraftProxySpec   `json:"spec,omitempty"`
	Status MinecraftProxyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type MinecraftProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MinecraftProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MinecraftProxy{}, &MinecraftProxyList{})
}

func DefaultServicePort(edition Edition) int32 {
	switch edition {
	case EditionBedrock:
		return 19132
	default:
		return 25565
	}
}
