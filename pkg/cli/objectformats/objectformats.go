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

package objectformats

import (
	"github.com/radius-project/radius/pkg/cli/output"
)

// GetResourceTableFormat returns the fields to output from a resource object.
// This function should be used with the generated CoreRP and other portable resource types.
func GetResourceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:     "GROUP",
				JSONPath:    "{ .ID }",
				Transformer: &ResourceIDToResourceGroupNameTransformer{},
			},
			{
				Heading:  "STATE",
				JSONPath: "{ .Properties.ProvisioningState }",
			},
		},
	}
}

func GetRecipePackTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RECIPE PACK",
				JSONPath: "{ .Name }",
			},
			{
				Heading:     "GROUP",
				JSONPath:    "{ .ID }",
				Transformer: &ResourceIDToResourceGroupNameTransformer{},
			},
		},
	}
}

func GetRecipeFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE TYPE",
				JSONPath: "{ .ResourceType }",
			},
			{
				Heading:  "RECIPE KIND",
				JSONPath: "{ .RecipeKind }",
			},
			{
				Heading:  "RECIPE LOCATION",
				JSONPath: "{ .RecipeLocation }",
			},
		},
	}
}

func GetRecipeFormatWithoutHeadings() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "",
				JSONPath: "{ .ResourceType }",
			},
			{
				Heading:  "",
				JSONPath: "{ .RecipeKind }",
			},
			{
				Heading:  "",
				JSONPath: "{ .RecipeLocation }",
			},
		},
	}
}

// GetGenericResourceTableFormat returns the fields to output from a generic resource object.
// This function should be used with the Go type GenericResource.
// The difference between this function and the GetResourceTableFormat function above is that
// GenericResource properties is a map and the way to get the ProvisioningState is different.
func GetGenericResourceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:     "GROUP",
				JSONPath:    "{ .ID }",
				Transformer: &ResourceIDToResourceGroupNameTransformer{},
			},
			{
				Heading:  "STATE",
				JSONPath: "{ .Properties.provisioningState }",
			},
		},
	}
}

func GetRecipesForEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RECIPE PACK",
				JSONPath: "{ .RecipePack }",
			},
			{
				Heading:  "RESOURCE TYPE",
				JSONPath: "{ .ResourceType }",
			},
			{
				Heading:  "RECIPE KIND",
				JSONPath: "{ .RecipeKind }",
			},
			{
				Heading:  "RECIPE LOCATION",
				JSONPath: "{ .RecipeLocation }",
			},
		},
	}
}
