/*
Copyright 2024 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeploymentResourceSpec defines the desired state of DeploymentResource
type DeploymentResourceSpec struct {
	// ID is the resource ID.
	ID string `json:"id,omitempty"`

	// ProviderConfig specifies the scope for resources
	ProviderConfig string `json:"providerConfig,omitempty"`
}

// DeploymentResourceStatus defines the observed state of DeploymentResource
type DeploymentResourceStatus struct {
	// ID is the resource ID.
	ID string `json:"id,omitempty"`

	// ProviderConfig specifies the scope for resources
	ProviderConfig string `json:"providerConfig,omitempty"`

	// ObservedGeneration is the most recent generation observed for this DeploymentResource.
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Operation tracks the status of an in-progress provisioning operation.
	Operation *ResourceOperation `json:"operation,omitempty"`

	// Phrase indicates the current status of the Deployment Resource.
	Phrase DeploymentResourcePhrase `json:"phrase,omitempty"`

	// Message is a human-readable description of the status of the Deployment Resource.
	Message string `json:"message,omitempty"`
}

// DeploymentResourcePhrase is a string representation of the current status of a Deployment Resource.
type DeploymentResourcePhrase string

const (
	// DeploymentResourcePhraseReady indicates that the Deployment Resource is ready.
	DeploymentResourcePhraseReady DeploymentResourcePhrase = "Ready"

	// DeploymentResourcePhraseFailed indicates that the Deployment Resource has failed.
	DeploymentResourcePhraseFailed DeploymentResourcePhrase = "Failed"

	// DeploymentResourcePhraseDeleting indicates that the Deployment Resource is being deleted.
	DeploymentResourcePhraseDeleting DeploymentResourcePhrase = "Deleting"

	// DeploymentResourcePhraseDeleted indicates that the Deployment Resource has been deleted.
	DeploymentResourcePhraseDeleted DeploymentResourcePhrase = "Deleted"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DeploymentResource is the Schema for the DeploymentResources API
type DeploymentResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentResourceSpec   `json:"spec,omitempty"`
	Status DeploymentResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeploymentResourceList contains a list of DeploymentResource
type DeploymentResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentResource{}, &DeploymentResourceList{})
}
