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

package v1

import (
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// UnknownComputeKind represents kubernetes compute resource type.
	UnknownComputeKind EnvironmentComputeKind = "unknown"
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
	// ACIComputeKind represents ACI compute resource type.
	ACIComputeKind EnvironmentComputeKind = "aci"
)

// BasicDaprResourceProperties is the basic resource properties for dapr resources.
type BasicDaprResourceProperties struct {
	// ComponentName represents the name of the component.
	ComponentName string `json:"componentName,omitempty"`
}

// BasicResourceProperties is the basic resource model for Radius resources.
type BasicResourceProperties struct {
	// Environment represents the id of environment resource.
	Environment string `json:"environment,omitempty"`
	// Application represents the id of application resource.
	Application string `json:"application,omitempty"`

	// Status represents the resource status.
	Status ResourceStatus `json:"status,omitempty"`
}

var _ BasicResourcePropertiesAdapter = (*BasicResourceProperties)(nil)

// ApplicationID implements BasicResourcePropertiesAdapter.
func (b *BasicResourceProperties) ApplicationID() string {
	return b.Application
}

// EnvironmentID implements BasicResourcePropertiesAdapter.
func (b *BasicResourceProperties) EnvironmentID() string {
	return b.Environment
}

// GetResourceStatus implements BasicResourcePropertiesAdapter.
func (b *BasicResourceProperties) GetResourceStatus() ResourceStatus {
	return b.Status
}

// SetResourceStatus implements BasicResourcePropertiesAdapter.
func (b *BasicResourceProperties) SetResourceStatus(status ResourceStatus) {
	b.Status = status
}

// DaprComponentMetadataValue is the value of a Dapr Metadata
type DaprComponentMetadataValue struct {
	// Plain text value
	Value string `json:"value,omitempty"`
	// SecretKeyRef is a reference to a secret in a Dapr secret store
	SecretKeyRef *DaprComponentSecretRef `json:"secretKeyRef,omitempty"`
}

// DaprComponentSecretRef is a reference to a secret in a Dapr secret store
type DaprComponentSecretRef struct {
	// Name of the secret in the secret store
	Name string `json:"name,omitempty"`
	// Key of the secret in the secret store
	Key string `json:"key,omitempty"`
}

// DaprComponentAuth represents the auth configuration for a Dapr component
type DaprComponentAuth struct {
	// SecretStore is the name of the secret store to fetch secrets from
	SecretStore string `json:"secretStore,omitempty"`
}

// ResourceStatus represents the output status of Radius resource.
type ResourceStatus struct {
	// Compute represents a resource presented in the underlying platform.
	Compute *EnvironmentCompute `json:"compute,omitempty"`

	// OutputResources represents the output resources associated with the radius resource.
	OutputResources []OutputResource `json:"outputResources,omitempty"`
	Recipe          *RecipeStatus    `json:"recipe,omitempty"`
}

// DeepCopyRecipeStatus creates a copy of ResourceStatus.
// It only deep-copies the Recipe field (if not nil).
// Other fields (Compute and OutputResources) will reference
// the same underlying data as the original.
func (original ResourceStatus) DeepCopyRecipeStatus() ResourceStatus {
	copy := original

	if original.Recipe != nil {
		copy.Recipe = &RecipeStatus{
			TemplateKind:    original.Recipe.TemplateKind,
			TemplatePath:    original.Recipe.TemplatePath,
			TemplateVersion: original.Recipe.TemplateVersion,
		}
	}

	return copy
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind              EnvironmentComputeKind      `json:"kind"`
	KubernetesCompute KubernetesComputeProperties `json:"kubernetes,omitempty"`
	ACICompute        ACIComputeProperties        `json:"aci,omitempty"`

	// Environment-level identity that can be used by any resource in the environment.
	// Resources can specify its own identities and they will override the environment-level identity.
	Identity *IdentitySettings `json:"identity,omitempty"`
}

// KubernetesComputeProperties represents the kubernetes compute of the environment.
type KubernetesComputeProperties struct {
	// ResourceID represents the resource ID for kubernetes compute resource.
	ResourceID string `json:"resourceId,omitempty"`

	// Namespace represents Kubernetes namespace.
	Namespace string `json:"namespace"`
}

type ACIComputeProperties struct {
	ResourceGroup string `json:"resourceGroup"`
}

// RadiusResourceModel represents the interface of radius resource type.
// TODO: Replace DeploymentDataModel with RadiusResourceModel later when link rp leverages generic.
type RadiusResourceModel interface {
	v1.ResourceDataModel

	ApplyDeploymentOutput(deploymentOutput DeploymentOutput) error
	OutputResources() []OutputResource

	ResourceMetadata() BasicResourcePropertiesAdapter
}

// ResourceScopePropertiesAdapter is the interface that wraps the resource scope properties (application and environment ids).
type ResourceScopePropertiesAdapter interface {
	// ApplicationID returns the resource id of the application.
	ApplicationID() string

	// EnvironmentID returns the resource id of the environment.
	EnvironmentID() string
}

// BasicResourcePropertiesAdapter is the interface that wraps the basic resource properties.
type BasicResourcePropertiesAdapter interface {
	ResourceScopePropertiesAdapter

	// GetResourceStatus returns the resource status.
	GetResourceStatus() ResourceStatus

	// SetResourceStatus sets the resource status.
	SetResourceStatus(status ResourceStatus)
}

// ScopesEqual compares two resources and returns true if their Application and
// Environment fields are equal (i.e. resource belongs to the same env and app).
func ScopesEqual(a ResourceScopePropertiesAdapter, b ResourceScopePropertiesAdapter) bool {
	return strings.EqualFold(a.ApplicationID(), b.ApplicationID()) && strings.EqualFold(a.EnvironmentID(), b.EnvironmentID())
}

// IsGlobalScopedResource checks if resource is global scoped.
func IsGlobalScopedResource(resource ResourceScopePropertiesAdapter) bool {
	return resource.ApplicationID() == "" && resource.EnvironmentID() == ""
}

// DeploymentOutput is the output details of a deployment.
type DeploymentOutput struct {
	DeployedOutputResources []OutputResource
	ComputedValues          map[string]any
	SecretValues            map[string]SecretValueReference
}

// DeploymentDataModel is the interface that wraps existing data models
// and enables us to use in generic deployment backend controllers.
type DeploymentDataModel interface {
	v1.ResourceDataModel

	ApplyDeploymentOutput(deploymentOutput DeploymentOutput) error

	OutputResources() []OutputResource
}

// BuildExternalOutputResources builds a slice of maps containing the LocalID, Provider and Identity of each
// OutputResource.
func BuildExternalOutputResources(outputResources []OutputResource) []map[string]any {
	var externalOutputResources []map[string]any
	for _, or := range outputResources {
		externalOutput := map[string]any{
			"id": or.ID.String(),
		}

		if or.LocalID != "" {
			externalOutput["LocalID"] = or.LocalID
		}

		externalOutputResources = append(externalOutputResources, externalOutput)
	}

	return externalOutputResources
}

// ComputedValueReference represents a non-secret value that can accessed once the output resources
// have been deployed.
type ComputedValueReference struct {
	// ComputedValueReference might hold a static value in `.Value` or might be a reference
	// that needs to be looked up.
	//
	// If `.Value` is set then treat this as a static value.
	//
	// If `.Value == nil` then use the `.PropertyReference` or to look up a property in the property
	// bag returned from deploying the resource via `handler.Put`.
	//
	// If `.Value == nil` && `.PropertyReference` is unset, then use JSONPointer to evaluate a JSON path
	// into the 'resource'.

	// LocalID specifies the output resource to be used for lookup. Does not apply with `.Value`
	LocalID string

	// Value specifies a static value to copy to computed values.
	Value any

	// PropertyReference specifies a property key to look up in the resource's *persisted properties*.
	PropertyReference string

	// JSONPointer specifies a JSON Pointer that cn be used to look up the value in the resource's body.
	JSONPointer string

	// Transformer transforms datamodel resource with the computed values.
	Transformer func(v1.DataModelInterface, map[string]any) error
}

// SecretValueReference represents a secret value that can accessed on the output resources
// have been deployed.
type SecretValueReference struct {
	// Value is the secret value itself
	Value string
}
