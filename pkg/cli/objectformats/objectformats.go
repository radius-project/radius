// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"github.com/project-radius/radius/pkg/cli/output"
)

func GetApplicationTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "APPLICATION",
				JSONPath: "{ .name }",
			},
			{
				Heading:  "PROVISIONING_STATE",
				JSONPath: "{ .properties.status.provisioningState }",
			},
		},
	}
}

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
				Heading:  "ENVIRONMENT",
				JSONPath: "{ .Environment }",
			},
		},
	}
}
