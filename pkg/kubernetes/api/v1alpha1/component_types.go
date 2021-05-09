// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComponentSpec defines the desired state of Component
type ComponentSpec struct {
	Application string `json:"application"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Config *runtime.RawExtension `json:"config,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Run *runtime.RawExtension `json:"run,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Uses *[]runtime.RawExtension `json:"dependsOn,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Bindings map[string]runtime.RawExtension `json:"provides,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Traits *[]runtime.RawExtension `json:"traits,omitempty"`
}

// ComponentStatus defines the observed state of Component
type ComponentStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Component is the Schema for the components API
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec,omitempty"`
	Status ComponentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentList contains a list of Component
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Component{}, &ComponentList{})
}
