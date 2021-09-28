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
//+kubebuilder:printcolumn:name="Application",type="string",JSONPath=".spec.application"
//+kubebuilder:printcolumn:name="Resource",type="string",JSONPath=".spec.resource"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase"

type DaprIOPubSubTopicComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceSpec   `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type DaprIOPubSubTopicComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DaprIOPubSubTopicComponent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DaprIOPubSubTopicComponent{}, &DaprIOPubSubTopicComponentList{})
}
