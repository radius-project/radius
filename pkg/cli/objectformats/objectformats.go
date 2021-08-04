// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import "github.com/Azure/radius/pkg/cli/output"

func GetApplicationTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "APPLICATION",
				JSONPath: "{ .name }",
			},
		},
	}
}

func GetComponentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "COMPONENT",
				JSONPath: "{ .name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .kind }",
			},
		},
	}
}

func GetDeploymentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "DEPLOYMENT",
				JSONPath: "{ .name }",
			},
			{
				Heading:  "COMPONENTS",
				JSONPath: "{ .properties.components[*].componentName }",
			},
		},
	}
}
