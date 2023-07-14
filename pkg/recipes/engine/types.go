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

package engine

import (
	"context"

	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

//go:generate mockgen -destination=./mock_engine.go -package=engine -self_package github.com/project-radius/radius/pkg/recipes/engine github.com/project-radius/radius/pkg/recipes/engine Engine

type Engine interface {
	// Execute gathers environment configuration and recipe definition and calls the driver to deploy the recipe.
	Execute(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.RecipeOutput, error)
	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, outputResources []rpv1.OutputResource, client processors.ResourceClient, recipe recipes.ResourceMetadata) error
}
