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
}

type DockerContainerStatus struct {
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
