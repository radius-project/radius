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

package update

import "github.com/radius-project/radius/pkg/cli/output"

type environmentForDisplay struct {
	Name        string
	ComputeKind string
	Recipes     int
	Providers   int
}

// environmentFormat returns a FormatterOptions object containing the column headings and JSONPaths for the
// environment table.
func environmentFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "COMPUTE",
				JSONPath: "{ .ComputeKind }",
			},
			{
				Heading:  "RECIPES",
				JSONPath: "{ .Recipes }",
			},
			{
				Heading:  "PROVIDERS",
				JSONPath: "{ .Providers }",
			},
		},
	}
}
