// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
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
			{
				Heading:  "KIND",
				JSONPath: "{ .Properties.Compute.Kind }",
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

func GetCloudProviderTableFormat() output.FormatterOptions {
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
