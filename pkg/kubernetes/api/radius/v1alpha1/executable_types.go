// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	// Number of replicas to launch for the Executable
	Replicas int `json:"replicas,omitempty"`
}

type ReplicaStatus struct {
	// The process ID
	PID int `json:"pid"`

	// Exit code of a process
	ExitCode int `json:"exitCode,omitempty"`
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

func (in *Executable) DeepCopy() *Executable {
	if in == nil {
		return nil
	}
	out := new(Executable)
	in.DeepCopyInto(out)
	return out
}

func (in *Executable) DeepCopyInto(out *Executable) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *Executable) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ExecutableList) DeepCopy() *ExecutableList {
	if in == nil {
		return nil
	}
	out := new(ExecutableList)
	in.DeepCopyInto(out)
	return out
}

func (in *ExecutableList) DeepCopyInto(out *ExecutableList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Executable, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ExecutableList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ExecutableSpec) DeepCopyInto(out *ExecutableSpec) {
	*out = *in

	if in.Args != nil {
		out.Args = make([]string, len(in.Args))
		copy(out.Args, in.Args)
	}

	if in.Env != nil {
		out.Env = make(map[string]string, len(in.Env))
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}

	if in.EnvFiles != nil {
		out.EnvFiles = make([]string, len(in.EnvFiles))
		copy(out.EnvFiles, in.EnvFiles)
	}
}

func (in *ExecutableStatus) DeepCopyInto(out *ExecutableStatus) {
	*out = *in

	if in.FinishTimestamp != nil {
		ft := in.FinishTimestamp.DeepCopy()
		out.FinishTimestamp = ft
	}

	if in.Replicas != nil {
		out.Replicas = make([]ReplicaStatus, len(in.Replicas))
		for i, ps := range in.Replicas {
			newPs := new(ReplicaStatus)
			*newPs = ps
			out.Replicas[i] = *newPs
		}
	}
}

func (es *ExecutableStatus) SetProcessExitCode(pid int, exitCode int) error {
	for i, rs := range es.Replicas {
		if rs.PID == pid {
			rs.ExitCode = exitCode
			es.Replicas[i] = rs
			return nil
		}
	}

	return fmt.Errorf("replica with PID %d not found", pid)
}

func (es *ExecutableStatus) AddReplica(rs ReplicaStatus) {
	es.Replicas = append(es.Replicas, rs)
}

func init() {
	SchemeBuilder.Register(&Executable{}, &ExecutableList{})
}
