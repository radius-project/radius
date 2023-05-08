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

	"github.com/project-radius/radius/pkg/recipes"
)

//go:generate mockgen -destination=./mock_engine.go -package=engine -self_package github.com/project-radius/radius/pkg/recipes/engine github.com/project-radius/radius/pkg/recipes/engine Engine

type Engine interface {
	// Execute gathers environment configuration and recipe definition and calls the driver to deploy the recipe.
	Execute(ctx context.Context, recipe recipes.Metadata) (*recipes.RecipeOutput, error)
}
