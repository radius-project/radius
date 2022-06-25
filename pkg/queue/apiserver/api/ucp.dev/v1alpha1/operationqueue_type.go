// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// controller-gen object paths=./api/ucp.dev/v1alpha1/... object:headerFile=../../boilerplate.go.txt
// controller-gen rbac:roleName=manager-role crd paths=./api/ucp.dev/v1alpha1/... output:crd:dir=../../deploy/Chart/crds/ucpd

type OperationQueueSpec struct {
	// DequeueCount represents the number of dequeue.
	DequeueCount int `json:"dequeueCount"`
	// EnqueueAt represents the time when enqueuing the message
	EnqueueAt metav1.Time `json:"enqueueAt"`
	// ExpireAt represents the expiry of the message.
	ExpireAt metav1.Time `json:"expireAt"`

	// ContentType represents the content-type of Data.
	ContentType string `json:"contentType"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:PreserveUnknownFields
	Data *runtime.RawExtension `json:"data"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}

//+kubebuilder:object:root=true

// OperationQueue is the Schema for OperationQueue API.
// +k8s:openapi-gen=true
type OperationQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OperationQueueSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OperationQueueList contains a list of OperationQueue
type OperationQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperationQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OperationQueue{}, &OperationQueueList{})
}
