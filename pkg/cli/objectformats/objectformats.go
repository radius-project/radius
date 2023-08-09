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
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
)

// # Function Explanation
//
// GetApplicationStatusTableFormat() sets up the columns and headings for a table to display application names and resource counts.
func GetApplicationStatusTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "APPLICATION",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "RESOURCES",
				JSONPath: "{ .ResourceCount }",
			},
		},
	}
}

// # Function Explanation
//
// GetApplicationGatewaysTableFormat() returns a FormatterOptions object which contains a list of columns to be used for
// formatting the output of a list of application gateways.
func GetApplicationGatewaysTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "GATEWAY",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "ENDPOINT",
				JSONPath: "{ .Endpoint }",
			},
		},
	}
}

// # Function Explanation
//
// GetResourceTableFormat() returns a FormatterOptions struct containing two columns, one for the resource name and one for
// the resource type.
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
		},
	}
}

// # Function Explanation
//
// GetResourceGroupTableFormat() returns a FormatterOptions object containing a list of columns with their headings and JSONPaths.
func GetResourceGroupTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "ID",
				JSONPath: "{ .id }",
			},
			{
				Heading:  "Name",
				JSONPath: "{ .name }",
			},
		},
	}
}

// # Function Explanation
//
// GetGenericEnvironmentTableFormat returns a FormatterOptions struct containing a slice of Columns, each of which
// contains a Heading and JSONPath.
func GetGenericEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
		},
	}
}

// # Function Explanation
//
// GetGenericEnvErrorTableFormat() returns a FormatterOptions struct containing a single column with the heading "errors:"
// and a JSONPath to the Errors field.
func GetGenericEnvErrorTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "errors:",
				JSONPath: "{ .Errors }",
			},
		},
	}
}

// # Function Explanation
//
// "GetWorkspaceTableFormat() returns a FormatterOptions object which contains a list of columns to be used for displaying
// workspace information such as name, kind, kubecontext and environment."
func GetWorkspaceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "WORKSPACE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .Connection.kind }",
			},
			{
				Heading:  "KUBECONTEXT",
				JSONPath: "{ .Connection.context }",
			},
			{
				Heading:  "ENVIRONMENT",
				JSONPath: "{ .Environment }",
			},
		},
	}
}

// # Function Explanation
//
// CloudProviderTableFormat() configures the output format of a table to display the Name and Status of a cloud provider.
func CloudProviderTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "REGISTERED",
				JSONPath: "{ .Enabled }",
			},
		},
	}
}

// # Function Explanation
//
// GetCloudProviderTableFormat function returns a FormatterOptions struct based on the credentialType parameter, which can
// be either "azure" or "aws".
func GetCloudProviderTableFormat(credentialType string) output.FormatterOptions {
	if strings.EqualFold(credentialType, "azure") {
		return output.FormatterOptions{
			Columns: []output.Column{
				{
					Heading:  "NAME",
					JSONPath: "{ .Name }",
				},
				{
					Heading:  "REGISTERED",
					JSONPath: "{ .Enabled }",
				},
				{
					Heading:  "CLIENTID",
					JSONPath: "{ .AzureCredentials.ClientID }",
				},
				{
					Heading:  "TENANTID",
					JSONPath: "{ .AzureCredentials.TenantID }",
				},
			},
		}
	} else if strings.EqualFold(credentialType, "aws") {
		return output.FormatterOptions{
			Columns: []output.Column{
				{
					Heading:  "NAME",
					JSONPath: "{ .Name }",
				},
				{
					Heading:  "REGISTERED",
					JSONPath: "{ .Enabled }",
				},
				{
					Heading:  "ACCESSKEYID",
					JSONPath: "{ .AWSCredentials.AccessKeyID }",
				},
			},
		}
	}
	return output.FormatterOptions{}
}

// # Function Explanation
//
// GetEnvironmentRecipesTableFormat() returns a FormatterOptions struct containing a list of Columns with their respective
// Headings and JSONPaths to be used for formatting the output of environment recipes.
func GetEnvironmentRecipesTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .LinkType }",
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

type OutputEnvObject struct {
	EnvName     string
	ComputeKind string
	Recipes     int
	Providers   int
}

// GetUpdateEnvironmentTableFormat returns the fields to output from env object after upation.
//
// # Function Explanation
//
// GetUpdateEnvironmentTableFormat() returns a FormatterOptions object containing the column headings and JSONPaths for the
// environment table.
func GetUpdateEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .EnvName }",
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

// # Function Explanation
//
// GetRecipeParamsTableFormat returns a FormatterOptions struct containing the column headings and JSONPaths for the
// recipe parameters table.
func GetRecipeParamsTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "PARAMETER NAME",
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
