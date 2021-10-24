// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExecutableSpec defines the desired state of an Executable
type ExecutableSpec struct {
	// Path to Executable binary
	Executable string `json:"executable"`

	// The working directory for the Executable
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	// Launch arguments to be passed to the Executable
	Args []string `json:"args,omitempty"`

	// Environment variables to be set for the Executable
	Env map[string]string `json:"env,omitempty"`

	// Environment variable files to be used for initializing Executable environment
	EnvFiles []string `json:"envFiles,omitempty"`

	//+kubebuilder:default=1
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=100
	// Number of replicas to launch for the Executable
	Replicas int `json:"replicas,omitempty"`

	// Ports specifies ports to bind to the executable.
	Ports []ExecutablePort `json:"ports,omitempty"`
}

// ExecutablePort defines the desired state of a port for an executable.
type ExecutablePort struct {
	Name    string   `json:"name"`
	Port    *int     `json:"port,omitempty"`
	Dynamic bool     `json:"dynamic"`
	Env     []string `json:"env,omitempty"`
}

type ReplicaStatus struct {
	// The process ID
	PID int `json:"pid"`

	// Exit code of a process
	ExitCode int `json:"exitCode,omitempty"`

	LogFile string `json:"logfile,omitempty"`

	Ports []ReplicaPort `json:"ports,omitempty"`
}

type ReplicaPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type ExecutableStatus struct {
	FinishTimestamp *metav1.Time    `json:"finishTimestamp,omitempty"`
	Replicas        []ReplicaStatus `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Executable",type="string",JSONPath=".spec.executable"
//+kubebuilder:printcolumn:name="Args",type="string",JSONPath=".spec.args"

type Executable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExecutableSpec   `json:"spec,omitempty"`
	Status ExecutableStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type ExecutableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Executable `json:"items"`
}

func (es *ExecutableStatus) SetProcessExitCode(pid int, exitCode int) {
	for i, rs := range es.Replicas {
		if rs.PID == pid {
			rs.ExitCode = exitCode
			es.Replicas[i] = rs
			break
		}
	}
}

func (es *ExecutableStatus) RemoveReplicas(pidsToRemove []int) {
	newReplicas := make([]ReplicaStatus, 0)

	for _, rs := range es.Replicas {
		if !contains(pidsToRemove, rs.PID) {
			newReplicas = append(newReplicas, rs)
		}
	}

	es.Replicas = newReplicas
}

func (es *ExecutableStatus) AddReplica(rs ReplicaStatus) {
	es.Replicas = append(es.Replicas, rs)
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func init() {
	SchemeBuilder.Register(&Executable{}, &ExecutableList{})
}
