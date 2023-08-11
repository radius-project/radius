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
	"fmt"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// Configuration represents kubernetes runtime and cloud provider configuration, which is used by the driver while deploying recipes.
type Configuration struct {
	// Kubernetes Runtime configuration for the environment.
	Runtime RuntimeConfiguration
	// Cloud providers configuration for the environment
	Providers datamodel.Providers
}

// RuntimeConfiguration represents Kubernetes Runtime configuration for the environment.
type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

// KubernetesRuntime represents application and environment namespaces.
type KubernetesRuntime struct {
	// Namespace is set to the application namespace when the Link is application-scoped, and set to the environment namespace when the Link is environment scoped
	Namespace string `json:"namespace,omitempty"`
	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}

// EnvironmentDefinition represents the recipe configuration details.
type EnvironmentDefinition struct {
	// Name represents the name of the recipe within the environment
	Name string
	// Driver represents the kind of infrastructure language used to define recipe.
	Driver string
	// ResourceType represents the type of the link this recipe can be consumed by.
	ResourceType string
	// Parameters represents key/value pairs to pass into the recipe template for every resource using this recipe. Specified during recipe registration to environment. Can be overridden by the radius resource consuming this recipe.
	Parameters map[string]any
	// TemplatePath represents path to the template provided by the recipe.
	TemplatePath string
	// TemplateVersion represents the version of the terraform module provided by the recipe.
	TemplateVersion string
}

// ResourceMetadata represents recipe details provided while creating a Link resource.
type ResourceMetadata struct {
	// Name represents the name of the recipe within the environment
	Name string
	// ApplicationID represents fully qualified resource ID for the application that the link is consumed by
	ApplicationID string
	// EnvironmentID represents fully qualified resource ID for the environment that the link is linked to
	EnvironmentID string
	// ResourceID represents fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	// Parameters represents key/value pairs to pass into the recipe template. Overrides any parameters set by the environment.
	Parameters map[string]any
}

const (
	TemplateKindBicep     = "bicep"
	TemplateKindTerraform = "terraform"
)

var (
	SupportedTemplateKind = []string{TemplateKindBicep, TemplateKindTerraform}
)

type ErrRecipeNotFound struct {
	Name        string
	Environment string
}

// ErrRecipeNotFoundError returns an error message with the recipe name and environment when a recipe is not found.
func (e *ErrRecipeNotFound) Error() string {
	return fmt.Sprintf("could not find recipe %q in environment %q", e.Name, e.Environment)
}

// RecipeOutput represents recipe deployment output.
type RecipeOutput struct {
	// Resources represents the list of output resources deployed recipe.
	Resources []string
	// Secrets represents the key/value pairs of secret values of the deployed resource.
	Secrets map[string]any
	// Values represents the key/value pairs of properties of the deployed resource.
	Values map[string]any
}

// PrepareRecipeOutput populates the recipe output from the recipe deployment output stored in the "result" object.
// outputs map is the value of "result" output from the recipe deployment response.
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

	return nil
}
