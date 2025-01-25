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

// DeploymentTemplateSpec defines the desired state of a DeploymentTemplate resource.
type DeploymentTemplateSpec struct {
	// Template is the ARM JSON manifest that defines the resources to deploy.
	Template string `json:"template,omitempty"`

	// Parameters is the ARM JSON parameters for the template.
	Parameters map[string]string `json:"parameters,omitempty"`

	// ProviderConfig specifies the scopes for resources.
	ProviderConfig string `json:"providerConfig,omitempty"`
}

// DeploymentTemplateStatus defines the observed state of a DeploymentTemplate resource.
type DeploymentTemplateStatus struct {
	// ObservedGeneration is the most recent generation observed for this DeploymentTemplate.
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// StatusHash is a hash of the DeploymentTemplate's state (template, parameters, and provider config).
	StatusHash string `json:"statusHash,omitempty"`

	// OutputResources is a list of the resourceIDs that were created by the template on the last deployment.
	OutputResources []string `json:"outputResources,omitempty"`

	// Operation tracks the status of an in-progress provisioning operation.
	Operation *ResourceOperation `json:"operation,omitempty"`

	// Phrase indicates the current status of the Deployment Template.
	Phrase DeploymentTemplatePhrase `json:"phrase,omitempty"`
}

// DeploymentTemplatePhrase is a string representation of the current status of a Deployment Template.
type DeploymentTemplatePhrase string

const (
	// DeploymentTemplatePhraseUpdating indicates that the Deployment Template is being updated.
	DeploymentTemplatePhraseUpdating DeploymentTemplatePhrase = "Updating"

	// DeploymentTemplatePhraseReady indicates that the Deployment Template is ready.
	DeploymentTemplatePhraseReady DeploymentTemplatePhrase = "Ready"

	// DeploymentTemplatePhraseFailed indicates that the Deployment Template has failed.
	DeploymentTemplatePhraseFailed DeploymentTemplatePhrase = "Failed"

	// DeploymentTemplatePhraseDeleting indicates that the Deployment Template is being deleted.
	DeploymentTemplatePhraseDeleting DeploymentTemplatePhrase = "Deleting"

	// DeploymentTemplatePhraseDeleted indicates that the Deployment Template has been deleted.
	DeploymentTemplatePhraseDeleted DeploymentTemplatePhrase = "Deleted"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase",description="Status of the resource"
// +kubebuilder:resource:categories={"all","radius"}

// DeploymentTemplate is the Schema for the deploymenttemplates API
type DeploymentTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentTemplateSpec   `json:"spec,omitempty"`
	Status DeploymentTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeploymentTemplateList contains a list of DeploymentTemplate
type DeploymentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentTemplate{}, &DeploymentTemplateList{})
}
