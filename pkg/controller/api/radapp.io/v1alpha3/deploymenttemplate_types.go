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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// DeploymentTemplateSpec defines the desired state of a DeploymentTemplate resource.
type DeploymentTemplateSpec struct {
	// Template is the ARM JSON manifest that defines the resources to deploy.
	Template string `json:"template,omitempty"`

	// Parameters is the ARM JSON parameters for the template.
	Parameters map[string]string `json:"parameters,omitempty"`

	// ProviderConfig specifies the scopes for resources.
	ProviderConfig string `json:"providerConfig,omitempty"`

	// Repository is the git repository that the Bicep manifests are stored in.
	Repository string `json:"repository,omitempty"`

	// SourceRef identifies the Flux source and artifact used to produce this template.
	SourceRef *DeploymentTemplateSourceReference `json:"sourceRef,omitempty"`
}

// DeploymentTemplateSourceReference records a generated template's source.
type DeploymentTemplateSourceReference struct {
	// Namespace is the GitRepository namespace.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
	// Name is the GitRepository name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// UID distinguishes a source from a replacement with the same name.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	UID types.UID `json:"uid"`
	// Generation is the source specification generation accepted for this template.
	// +kubebuilder:validation:Minimum=1
	Generation int64 `json:"generation"`
	// Revision is the accepted artifact's revision.
	Revision string `json:"revision,omitempty"`
	// Digest is the accepted artifact's content digest.
	Digest string `json:"digest,omitempty"`
}

// DeploymentTemplateStatus defines the observed state of a DeploymentTemplate resource.
type DeploymentTemplateStatus struct {
	// ObservedGeneration is the most recent generation observed for this DeploymentTemplate.
	// +kubebuilder:validation:Format=""
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// StatusHash is a hash of the DeploymentTemplate's state (template, parameters, and provider config).
	StatusHash string `json:"statusHash,omitempty"`

	// OutputResources is a list of the resourceIDs that were created by the template on the last deployment.
	OutputResources []string `json:"outputResources,omitempty"`

	// PendingCleanup retains outputs that cannot have trackers in a terminating namespace.
	PendingCleanup []OutputResourceCleanup `json:"pendingCleanup,omitempty"`

	// Operation tracks the status of an in-progress provisioning operation.
	Operation *ResourceOperation `json:"operation,omitempty"`

	// OperationSpecHash identifies the desired spec submitted by the in-progress operation.
	OperationSpecHash string `json:"operationSpecHash,omitempty"`

	// Phrase indicates the current status of the Deployment Template.
	Phrase DeploymentTemplatePhrase `json:"phrase,omitempty"`
}

// OutputResourceCleanup persists deletion progress for an output without a tracker.
type OutputResourceCleanup struct {
	// ID is the output resource ID returned by a completed deployment.
	ID string `json:"id"`
	// Operation is the accepted DELETE operation, if any.
	Operation *ResourceOperation `json:"operation,omitempty"`
}

// DeploymentTemplatePhrase is a string representation of the current status of a Deployment Template.
type DeploymentTemplatePhrase string

const (
	// DeploymentTemplatePhraseUpdating indicates that the Deployment Template is being updated.
	DeploymentTemplatePhraseUpdating DeploymentTemplatePhrase = "Updating"

	// DeploymentTemplatePhraseReady indicates that the Deployment Template is ready.
	DeploymentTemplatePhraseReady DeploymentTemplatePhrase = "Ready"

	// DeploymentTemplatePhraseReadyPendingCleanup indicates that the latest deployment
	// succeeded but residual DeploymentResources removed from the previous generation
	// are still draining. The reconciler transitions to Ready once they are gone.
	DeploymentTemplatePhraseReadyPendingCleanup DeploymentTemplatePhrase = "ReadyPendingCleanup"

	// DeploymentTemplatePhraseHeld indicates that source state must be resolved before new effects.
	DeploymentTemplatePhraseHeld DeploymentTemplatePhrase = "Held"

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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &DeploymentTemplate{}, &DeploymentTemplateList{})
		return nil
	})
}
