// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
)

type ConfigurationLoader interface {
	Load(ctx context.Context, recipe recipes.Recipe) (*Configuration, error)
}

type Configuration struct {
	Runtime   RuntimeConfiguration
	Providers map[string]map[string]any
}

type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

type KubernetesRuntime struct {
	Namespace string `json:"namespace,omitempty"`
}
