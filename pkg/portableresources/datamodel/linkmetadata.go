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

package datamodel

import (
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// LinkMetadata represents internal DataModel properties common to all portable resource types.
type LinkMetadata struct {
	// ComputedValues map is any resource values that will be needed for more operations.
	// For example; database name to generate secrets for cosmos DB.
	ComputedValues map[string]any `json:"computedValues,omitempty"`

	// Stores action to retrieve secret values. For Azure, connectionstring is accessed through cosmos listConnectionString operation, if secrets are not provided as input
	SecretValues map[string]rpv1.SecretValueReference `json:"secretValues,omitempty"`

	RecipeData portableresources.RecipeData `json:"recipeData,omitempty"`
}

// LinkMode specifies how to build a portable resource. Options are to build automatically via ‘recipe’ or ‘resource’, or build manually via ‘values’. Selection determines which set of fields to additionally require.
type LinkMode string

const (
	// LinkModeRecipe is the recipe mode for portable resource deployment
	LinkModeRecipe LinkMode = "recipe"
	// LinkModeResource is the resource mode for portable resource deployment
	LinkModeResource LinkMode = "resource"
	// LinkModeResource is the values mode for portable resource deployment
	LinkModeValues LinkMode = "values"
	// RecipeContextParameter is the parameter context for recipe deployment
	RecipeContextParameter string = "context"
)
