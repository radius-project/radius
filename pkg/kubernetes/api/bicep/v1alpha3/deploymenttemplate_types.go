// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeploymentTemplateSpec defines the desired state of Arm
type DeploymentTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Content *runtime.RawExtension `json:"content,omitempty"`
}

// DeploymentTemplateStatus defines the observed state of Arm
type DeploymentTemplateStatus struct {
	Operations []DeploymentTemplateOperation `json:"resources,omitempty"`
}

type DeploymentTemplateOperation struct {
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Provisioned bool   `json:"provisioned,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","bicep"}
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
