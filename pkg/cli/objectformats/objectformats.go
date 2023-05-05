// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
)

// # Function Explanation
// 
//	GetApplicationStatusTableFormat() returns a FormatterOptions object containing two columns, "APPLICATION" and 
//	"RESOURCES", which can be used to format the output of an application status table. If an error occurs, the function 
//	will return an empty FormatterOptions object.
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
//	GetApplicationGatewaysTableFormat() returns a FormatterOptions object containing the columns to be used when displaying 
//	application gateways in a table format. It includes the gateway name and endpoint. If an error occurs, the function will
//	 return an empty FormatterOptions object.
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
//	GetResourceTableFormat() returns a FormatterOptions object containing two columns, "RESOURCE" and "TYPE", which are 
//	populated with the Name and Type fields of the input object. If the input object does not contain the Name or Type 
//	fields, an error is returned.
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
//	GetResourceGroupTableFormat() returns a FormatterOptions object containing two columns, "ID" and "Name", which are 
//	populated with the values from the corresponding JSONPaths. If the JSONPaths are invalid, an error will be returned.
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
//	GetGenericEnvironmentTableFormat() returns a FormatterOptions object containing a list of columns to be used for 
//	formatting a table of environment variables. The columns contain the environment variable name. If an error occurs, the 
//	function will return an empty FormatterOptions object.
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
//	GetGenericEnvErrorTableFormat() returns a FormatterOptions object containing a single column with the heading "errors:" 
//	and a JSONPath to access the errors from the request object. This allows callers to easily format and display errors in 
//	a table format.
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
//	GetWorkspaceTableFormat() returns a FormatterOptions object containing column headings and JSONPaths for displaying 
//	workspace information in a table format. It is up to the caller to handle any errors that may occur when using this 
//	function.
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
//	CloudProviderTableFormat() returns a FormatterOptions object containing two columns, "NAME" and "Status", which are 
//	populated with the values of the Name and Enabled fields of the input object respectively. If the input object does not 
//	contain the specified fields, an error is returned.
func CloudProviderTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "Status",
				JSONPath: "{ .Enabled }",
			},
		},
	}
}

// # Function Explanation
// 
//	GetCloudProviderTableFormat takes in a credential type and returns a FormatterOptions object with the appropriate 
//	columns for the given credential type. If the credential type is not recognized, an empty FormatterOptions object is 
//	returned.
func GetCloudProviderTableFormat(credentialType string) output.FormatterOptions {
	if strings.EqualFold(credentialType, "azure") {
		return output.FormatterOptions{
			Columns: []output.Column{
				{
					Heading:  "NAME",
					JSONPath: "{ .Name }",
				},
				{
					Heading:  "Status",
					JSONPath: "{ .Enabled }",
				},
				{
					Heading:  "ClientID",
					JSONPath: "{ .AzureCredentials.ClientID }",
				},
				{
					Heading:  "TenantID",
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
					Heading:  "Status",
					JSONPath: "{ .Enabled }",
				},
				{
					Heading:  "AccessKeyID",
					JSONPath: "{ .AWSCredentials.AccessKeyID }",
				},
			},
		}
	}
	return output.FormatterOptions{}
}

// # Function Explanation
// 
//	GetEnvironmentRecipesTableFormat() returns a FormatterOptions object containing the columns to be used when displaying a
//	 table of environment recipes. It includes the name, type, and template path of each recipe. If an error occurs, the 
//	function will return an empty FormatterOptions object.
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
//	GetUpdateEnvironmentTableFormat() returns a FormatterOptions object containing the columns to be used when displaying 
//	the output of an update environment request. It includes the name, compute kind, recipes and providers of the 
//	environment. If any of the columns are missing, an error will be returned.
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
//	GetRecipeTableFormat() returns a FormatterOptions object containing the column headings and JSONPaths for a recipe 
//	table. It provides a way to format the output of a recipe table in a consistent way, allowing for easy error handling.
func GetRecipeTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RECIPE NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .LinkType }",
			},
			{
				Heading:  "TEMPLATE",
				JSONPath: "{ .TemplatePath }",
			},
		},
	}
}

// # Function Explanation
// 
//	GetRecipeParamsTableFormat() returns a FormatterOptions object containing a list of Columns that can be used to format a
//	 table of recipe parameters. The Columns contain the parameter name, type, default value, min and max values. If any of 
//	the values are not present, an empty string is returned.
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
