// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
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

// RecipeDefinition represents the recipe configuration details.
type RecipeDefinition struct {
	// Driver represents the kind of infrastructure language used to define recipe.
	Driver string
	// ResourceType represents the type of the link this recipe can be consumed by.
	ResourceType string
	// Parameters represents key/value parameters to pass to the recipe template at deployment.
	Parameters map[string]any
	// TemplatePath represents path to the template provided by the recipe.
	TemplatePath string
}

type ConfigurationLoader interface {
	// LoadConfiguration fetches environment/application information and return runtime and provider configuration.
	LoadConfiguration(ctx context.Context, recipe recipes.RecipeMetadata) (*Configuration, error)
	//	LoadRecipe fetches the recipe information from the environment.
	LoadRecipe(ctx context.Context, recipe recipes.RecipeMetadata) (*RecipeDefinition, error)
}
