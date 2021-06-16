// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeploymentSpec defines the desired state of Deployment
type DeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Deployment. Edit deployment_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// DeploymentStatus defines the observed state of Deployment
type DeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Deployment is the Schema for the deployments API
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentSpec   `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeploymentList contains a list of Deployment
type DeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Deployment{}, &DeploymentList{})
}
