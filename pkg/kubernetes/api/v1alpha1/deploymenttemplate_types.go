// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeploymentTemplateSpec defines the desired state of Arm
type DeploymentTemplateSpec struct {
	Content *runtime.RawExtension `json:"content,omitempty"`
}

// DeploymentTemplateStatus defines the observed state of Arm
type DeploymentTemplateStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status

// DeploymentTemplate is the Schema for the DeploymentTemplate API
type DeploymentTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTemplateSpec   `json:"spec,omitempty"`
	Status DeploymentTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeploymentTemplateList contains a list of Arm
type DeploymentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTemplate{}, &DeploymentTemplateList{})
}
