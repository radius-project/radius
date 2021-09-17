// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Application",type="string",JSONPath=".spec.hierarchy[1]"
//+kubebuilder:printcolumn:name="Component",type="string",JSONPath=".spec.hierarchy[2]"
//+kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.kind"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase"

type ContainerComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceSpec   `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type ContainerComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContainerComponent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ContainerComponent{}, &ContainerComponentList{})
}
