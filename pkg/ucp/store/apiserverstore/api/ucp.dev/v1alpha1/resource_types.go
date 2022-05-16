// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

type ResourceEntry struct {
	ID          string `json:"id"`
	APIVersion  string `json:"apiVersion"`
	ETag        string `json:"etag"`
	ContentType string `json:"contentType"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Data *runtime.RawExtension `json:"data,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}

//+kubebuilder:object:root=true

// Resource is the Schema for the resources API
// +k8s:openapi-gen=true
type Resource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Entries []ResourceEntry `json:"entries,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceList contains a list of Resource
type ResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Resource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Resource{}, &ResourceList{})
}
