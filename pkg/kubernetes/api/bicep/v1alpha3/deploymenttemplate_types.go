// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const DeploymentTemplateKind = "DeploymentTemplate"

// DeploymentTemplateSpec defines the desired state of Arm
type DeploymentTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Content *runtime.RawExtension `json:"content,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// DeploymentTemplateStatus defines the observed state of Arm
type DeploymentTemplateStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// ObservedGeneration captures the last generation
	// that was captured and completed by the reconciler
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ResourceStatuses holds the status of each deployed resource in the template.
	// +optional
	ResourceStatuses []ResourceStatus `json:"resourceStatuses,omitempty"`

	// The readable status "phrase" of the deployment template.
	Phrase string `json:"phrase,omitempty"`

	Outputs map[string]DeploymentOutput `json:"outputs,omitempty"`
}

type DeploymentOutput struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

type ResourceStatus struct {
	// status of the condition, one of True, False, Unknown.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`

	// The name of the resource.
	// +required
	Name string `json:"name"`

	// The kind of the resource.
	// +required
	Kind string `json:"kind"`

	// ResourceID
	// +required
	ResourceID string `json:"resourceId"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","bicep"}
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase"

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
