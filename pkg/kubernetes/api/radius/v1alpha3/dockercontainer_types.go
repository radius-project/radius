// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DockerContainerSpec defines the desired state of an DockerContainer
type DockerContainerSpec struct {
	Image            string            `json:"image"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	//+kubebuilder:default=1
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=100
	// Number of replicas to launch for the DockerContainer
	Replicas int `json:"replicas,omitempty"`

	// Ports specifies ports to bind to the executable.
	Ports []DockerContainerPort `json:"ports,omitempty"`
}

type DockerContainerPort struct {
	Port          *int `json:"port"`
	ContainerPort *int `json:"containerPort"`
	Dynamic       bool `json:"dynamic"`
}

type DockerContainerStatus struct {
	FinishTimestamp *metav1.Time    `json:"finishTimestamp,omitempty"`
	Replicas        []ReplicaStatus `json:"replicas,omitempty"`
}

type DockerReplicaPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type DockerReplicaStatus struct {
	// The process ID
	PID int `json:"pid"`

	// Exit code of a process
	ExitCode int `json:"exitCode,omitempty"`

	LogFile string `json:"logfile,omitempty"`

	Ports []DockerReplicaPort `json:"ports,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Container",type="string",JSONPath=".spec.executable"
//+kubebuilder:printcolumn:name="Args",type="string",JSONPath=".spec.args"

type DockerContainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerContainerSpec   `json:"spec,omitempty"`
	Status DockerContainerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type DockerContainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerContainer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DockerContainer{}, &DockerContainerList{})
}
