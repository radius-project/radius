// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"context"
	"fmt"
)

type Recipe struct {
	Name          string
	ApplicationID string
	EnvironmentID string
	ResourceID    string
	Parameters    map[string]interface{}
}

type ConfigurationLoader interface {
	Load(ctx context.Context, recipe Recipe) (*Configuration, error)
}

type Repository interface {
	Lookup(ctx context.Context, recipe Recipe) (*Definition, error)
}

type Configuration struct {
	Runtime   RuntimeConfiguration
	Providers map[string]map[string]interface{}
}

type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

type KubernetesRuntime struct {
	Namespace string `json:"namespace,omitempty"`
}

type Definition struct {
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
