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

package common

import (
	"github.com/radius-project/radius/pkg/cli/output"
)

// RecipeFormat returns a FormatterOptions struct containing a list of Columns with their respective
// Headings and JSONPaths to be used for formatting the output of environment recipes.
func RecipeFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RECIPE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .ResourceType }",
			},
			{
				Heading:  "TEMPLATE KIND",
				JSONPath: "{ .TemplateKind }",
			},
			{
				Heading:  "TEMPLATE VERSION",
				JSONPath: "{ .TemplateVersion }",
			},
			{
				Heading:  "TEMPLATE",
				JSONPath: "{ .TemplatePath }",
			},
		},
	}
}

// RecipeParametersFormat returns a FormatterOptions struct containing the column headings and JSONPaths for the
// recipe parameters table.
func RecipeParametersFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "PARAMETER",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:  "DEFAULT VALUE",
				JSONPath: "{ .DefaultValue }",
			},
			{
				Heading:  "MIN",
				JSONPath: "{ .MinValue }",
			},
			{
				Heading:  "MAX",
				JSONPath: "{ .MaxValue }",
			},
		},
	}
}
