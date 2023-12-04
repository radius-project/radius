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

package list

import "github.com/radius-project/radius/pkg/cli/output"

// credentialFormat configures the output format of a table to display the Name and Status of a cloud provider.
func credentialFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "PROVIDER",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "REGISTERED",
				JSONPath: "{ .Enabled }",
			},
		},
	}
}
