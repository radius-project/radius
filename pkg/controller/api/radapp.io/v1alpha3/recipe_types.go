/*
Copyright 2023.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecipeSpec defines the desired state of Recipe
type RecipeSpec struct {
	// Type is the type of resource to create. eg: 'Applications.Datastores/redisCaches'.
	// +kubebuilder:validation:Required
	Type string `json:"type,omitempty"`

	// SecretName is the name of a Kubernetes secret to create once the resource is created.
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName,omitempty"`
}

// RecipePhrase is a string representation of the current status of a Recipe.
type RecipePhrase string

const (
	// PhraseUpdating indicates that the Recipe is being updated.
	PhraseUpdating RecipePhrase = "Updating"

	// PhraseReady indicates that the Recipe is ready.
	PhraseReady RecipePhrase = "Ready"

	// PhraseFailed indicates that the Recipe has failed.
	PhraseFailed RecipePhrase = "Failed"

	// PhraseDeleting indicates that the Recipe is being deleted.
	PhraseDeleting RecipePhrase = "Deleting"

	// PhraseDeleted indicates that the Recipe has been deleted.
	PhraseDeleted RecipePhrase = "Deleted"
)

// RecipeStatus defines the observed state of Recipe
type RecipeStatus struct {
	// ObservedGeneration is the most recent generation observed for this Recipe. It corresponds to the
	// Recipe's generation, which is updated on mutation by the API Server.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Application is the resource ID of the application.
	// +kubebuilder:validation:Optional
	Application string `json:"application,omitempty"`

	// Environment is the resource ID of the environment.
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`

	// Scope is the resource ID of the scope.
	// +kubebuilder:validation:Optional
	Scope string `json:"scope,omitempty"`

	// Resource is the resource ID of the resource.
	// +kubebuilder:validation:Optional
	Resource string `json:"resource,omitempty"`

	// Operation is the operation URL of an operation in progress.
	// +kubebuilder:validation:Optional
	Operation string `json:"operation,omitempty"`

	// Phrase indicates the current status of the Recipe.
	// +kubebuilder:validation:Optional
	Phrase RecipePhrase `json:"phrase,omitempty"`

	// Secret specifies a reference to the secret being managed by this Recipe.
	// +kubebuilder:validation:Optional
	Secret corev1.ObjectReference `json:"secret,omitempty"`
}

func (r *Recipe) SetDefaults() {
	if r.Status.Scope == "" {
		r.Status.Scope = "/planes/radius/local/resourceGroups/default"
	}
	if r.Status.Environment == "" {
		r.Status.Environment = r.Status.Scope + "/providers/Applications.Core/environments/" + "default"
	}
	if r.Status.Application == "" {
		r.Status.Application = r.Status.Scope + "/providers/Applications.Core/applications/" + r.Namespace
	}
	if r.Status.Resource == "" {
		r.Status.Resource = r.Status.Scope + "/providers/" + r.Spec.Type + "/" + r.Name
	}
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Type of resource the recipe should create"
//+kubebuilder:printcolumn:name="Secret",type="string",JSONPath=".spec.secretName",description="Name of the secret to create"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase",description="Status of the resource"
//+kubebuilder:subresource:status

// Recipe is the Schema for the recipes API
type Recipe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecipeSpec   `json:"spec,omitempty"`
	Status RecipeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RecipeList contains a list of Recipe
type RecipeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recipe `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Recipe{}, &RecipeList{})
}
