// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Application",type="string",JSONPath=".spec.application"
//+kubebuilder:printcolumn:name="Resource",type="string",JSONPath=".spec.resource"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase"

type MongoDBComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceSpec   `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type MongoDBComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MongoDBComponent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MongoDBComponent{}, &MongoDBComponentList{})
}

var _ webhook.Validator = &MongoDBComponent{}

func (r *MongoDBComponent) ValidateCreate() error {
	resourcelog.Info("validate create", "name", r.Name)

	return nil
}

func (r *MongoDBComponent) ValidateUpdate(old runtime.Object) error {
	resourcelog.Info("validate update", "name", r.Name)

	return nil
}

func (r *MongoDBComponent) ValidateDelete() error {
	resourcelog.Info("validate delete", "name", r.Name)

	return nil
}
