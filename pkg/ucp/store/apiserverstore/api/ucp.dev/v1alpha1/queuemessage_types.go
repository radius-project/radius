/*
Copyright 2023 The Radius Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// controller-gen object paths=./api/ucp.dev/v1alpha1/... object:headerFile=../../../boilerplate.go.txt
// controller-gen rbac:roleName=manager-role crd paths=./api/ucp.dev/v1alpha1/... output:crd:dir=../../../deploy/Chart/crds/ucpd

type QueueMessageSpec struct {
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

// QueueMessage is the Schema for QueueMessage API.
// +k8s:openapi-gen=true
type QueueMessage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec QueueMessageSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// QueueMessageList contains a list of OperationQueue
type QueueMessageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []QueueMessage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&QueueMessage{}, &QueueMessageList{})
}
