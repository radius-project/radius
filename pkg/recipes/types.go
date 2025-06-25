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

package recipes

import (
	"bytes"
	"encoding/json"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// Configuration represents runtime and cloud provider configuration, which is used by the driver while deploying recipes.
type Configuration struct {
	// Kubernetes Runtime configuration for the environment.
	Runtime RuntimeConfiguration
	// Cloud providers configuration for the environment
	Providers datamodel.Providers
	// Simulated represents whether the environment is simulated or not.
	Simulated bool

	RecipeConfig datamodel.RecipeConfigProperties
}

// RuntimeConfiguration represents Runtime configuration for the environment.
type RuntimeConfiguration struct {
	Kubernetes              *KubernetesRuntime              `json:"kubernetes,omitempty"`
	AzureContainerInstances *AzureContainerInstancesRuntime `json:"azureContainerInstances,omitempty"`
}

// KubernetesRuntime represents application and environment namespaces.
type KubernetesRuntime struct {
	// Namespace is set to the application namespace when the portable resource is application-scoped, and set to the environment namespace when it is environment scoped
	Namespace string `json:"namespace,omitempty"`

	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}

// AzureContainerInstancesRuntime represents Azure Container Instances runtime configuration.
type AzureContainerInstancesRuntime struct {
	// TODO: Add runtime configuration for Azure Container Instances
}

// EnvironmentDefinition represents the recipe configuration details.
type EnvironmentDefinition struct {
	// Name represents the name of the recipe within the environment
	Name string

	// Driver represents the kind of infrastructure language used to define recipe.
	Driver string

	// ResourceType represents the type of the portable resource this recipe can be consumed by.
	ResourceType string

	// Parameters represents key/value pairs to pass into the recipe template for every resource using this recipe. Specified during recipe registration to environment. Can be overridden by the radius resource consuming this recipe.
	Parameters map[string]any

	// TemplatePath represents path to the template provided by the recipe.
	TemplatePath string

	// TemplateVersion represents the version of the terraform module provided by the recipe.
	TemplateVersion string

	// Allows insecure connections to registry without SSL check.
	PlainHTTP bool

	// TLS represents TLS configuration for recipe downloads over HTTPS.
	TLS *TLSConfig
}

// TLSConfig represents TLS configuration options for HTTPS connections.
type TLSConfig struct {
	// CACertificate represents a reference to a CA certificate secret.
	CACertificate *SecretReference `json:"caCertificate,omitempty"`

	// SkipVerify allows insecure connections (skip TLS verification).
	SkipVerify bool `json:"skipVerify,omitempty"`
}

// SecretReference represents a reference to a secret.
type SecretReference struct {
	// Source is the source of the secret.
	Source string `json:"source"`

	// Key is the key in the secret.
	Key string `json:"key"`
}

// ResourceMetadata represents recipe details provided while deploying a portable or a user-defined resource.
type ResourceMetadata struct {
	// Name represents the name of the recipe within the environment
	Name string

	// ApplicationID represents fully qualified resource ID for the application that the portable resource is consumed by
	ApplicationID string

	// EnvironmentID represents fully qualified resource ID for the environment that the portable resource is linked to
	EnvironmentID string

	// ResourceID represents fully qualified resource ID for the resource the recipe is deploying
	ResourceID string

	// Properties represents the properties of the resource that the recipe is deploying
	Properties map[string]any

	// Parameters represents key/value pairs to pass into the recipe template. Overrides any parameters set by the environment.
	Parameters map[string]any
}

const (
	TemplateKindBicep     = "bicep"
	TemplateKindTerraform = "terraform"

	// Recipe outputs are expected to be wrapped under an object named "result"
	ResultPropertyName = "result"
)

var (
	SupportedTemplateKind = []string{TemplateKindBicep, TemplateKindTerraform}
)

// RecipeOutput represents recipe deployment output.
type RecipeOutput struct {
	// Resources represents the list of output resources deployed recipe.
	Resources []string

	// Secrets represents the key/value pairs of secret values of the deployed resource.
	Secrets map[string]any

	// Values represents the key/value pairs of properties of the deployed resource.
	Values map[string]any

	// Status represents the recipe status at deployment time of resource.
	Status *rpv1.RecipeStatus
}

// SecretData represents secrets data and includes secret type and a map of secret keys to their values.
type SecretData struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

// PrepareRecipeResponse populates the recipe output from the recipe deployment output stored in the "result" object.
// resultValue is the value of the "result" output from the recipe deployment response.
func (ro *RecipeOutput) PrepareRecipeResponse(resultValue map[string]any) error {
	b, err := json.Marshal(&resultValue)
	if err != nil {
		return err
	}

	// Using a decoder to block unknown fields.
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(ro)
	if err != nil {
		return err
	}

	// Make sure maps are non-nil (it's just friendly).
	if ro.Secrets == nil {
		ro.Secrets = map[string]any{}
	}
	if ro.Values == nil {
		ro.Values = map[string]any{}
	}
	if ro.Resources == nil {
		ro.Resources = []string{}
	}

	return nil
}
