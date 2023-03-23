// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

type RecipeMetadata struct {
	//The name of the recipe within the environment
	Name string
	//Fully qualified resource ID for the application that the link is linked to
	ApplicationID string
	//Fully qualified resource ID for the application that the link is consumed by
	EnvironmentID string
	//Fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	//Key/value parameters to pass into the recipe at deployment
	Parameters map[string]any
}

type RecipeResult struct {
	Resources []string
	Secrets   map[string]any
	Values    map[string]any
}

type ConfigurationLoader interface {
	Load(ctx context.Context, recipe RecipeMetadata) (*Configuration, error)
	Lookup(ctx context.Context, recipe RecipeMetadata) (*RecipeDefinition, error)
}

type Configuration struct {
	// Kubernetes Runtime configuration for the environment.
	Runtime RuntimeConfiguration
	//Cloud providers configuration for the environment
	Providers datamodel.Providers
}

type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

type KubernetesRuntime struct {
	Namespace string `json:"namespace,omitempty"`
}

type Engine interface {
	Execute(ctx context.Context, recipe RecipeMetadata) (*RecipeResult, error)
}

type Driver interface {
	Execute(ctx context.Context, configuration Configuration, recipe RecipeMetadata, definition RecipeDefinition) (*RecipeResult, error)
}

type RecipeDefinition struct {
	Driver       string
	ResourceType string
	Parameters   map[string]interface{}
	TemplatePath string
}
type ErrRecipeNotFound struct {
	Name        string
	Environment string
}

func (e *ErrRecipeNotFound) Error() string {
	return fmt.Sprintf("could not find recipe %q in environment %q", e.Name, e.Environment)
}

func (e *ErrRecipeNotFound) Is(other error) bool {
	_, ok := other.(*ErrRecipeNotFound)
	return ok
}
