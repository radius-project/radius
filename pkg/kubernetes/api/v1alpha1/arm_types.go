// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArmSpec defines the desired state of Arm
type ArmSpec struct {
}

// ArmStatus defines the observed state of Arm
type ArmStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status

// Arm is the Schema for the Arms API
type Arm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ArmSpec   `json:"spec,omitempty"`
	Status ArmStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ArmList contains a list of Arm
type ArmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Arm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Arm{}, &ArmList{})
}
