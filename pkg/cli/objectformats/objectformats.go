// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
)

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
