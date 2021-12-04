// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"strings"

	"github.com/Azure/radius/pkg/cli/output"
)

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

func GetResourceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE",
				JSONPath: "{ .name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .type }",
				Transformer: func(t string) string {
					tokens := strings.Split(t, "/")
					if tokens[0] == "Microsoft.CustomProviders" {
						return tokens[len(tokens)-1]
					}
					return t
				},
			},
			{
				Heading:  "PROVISIONING_STATE",
				JSONPath: "{ .properties.status.provisioningState }",
			},
			{
				Heading:  "HEALTH_STATE",
				JSONPath: "{ .properties.status.healthState }",
			},
		},
	}
}
