// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"strings"

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
			{
				Heading:  "HEALTH_STATE",
				JSONPath: "{ .properties.status.healthState }",
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
					// For Radius resource types only show last part of the resource type. Example: mongo.com.MongoDatabase instead of Microsoft.CustomProviders/mongo.com.MongoDatabase
					// For non-Radius resources types, show full resource type, Microsoft.ServiceBus/namespaces for example.
					// TODO: "Microsoft.CustomProviders" should be updated to reflect Radius RP name once we move out of custom RP mode:
					// https://github.com/project-radius/radius/issues/1534
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

func GetAzureCloudEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .Kind }",
			},
			{
				Heading:  "SUBSCRIPTION ID",
				JSONPath: "{ .SubscriptionID }",
			},
			{
				Heading:  "RESOURCE GROUP",
				JSONPath: "{ .ResourceGroup }",
			},
			{
				Heading:  "CONTROL PLANE RESOURCE GROUP",
				JSONPath: "{ .ControlPlaneResourceGroup }",
			},
			{
				Heading:  "CLUSTER NAME",
				JSONPath: "{ .ClusterName }",
			},
			{
				Heading:  "DEFAULT APPLICATION",
				JSONPath: "{ .DefaultApplication }",
			},
		},
	}
}

func GetLocalEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .Kind }",
			},
			{
				Heading:  "DEFAULT APPLICATION",
				JSONPath: "{ .DefaultApplication }",
			},
			{
				Heading:  "CONTEXT",
				JSONPath: "{ .Context }",
			},
			{
				Heading:  "NAMESPACE",
				JSONPath: "{ .Namespace }",
			},
			{
				Heading:  "CLUSTER NAME",
				JSONPath: "{ .ClusterName }",
			},
			{
				Heading:  "API SERVER BASE URL",
				JSONPath: "{ .APIServerBaseURL }",
			},
			{
				Heading:  "API DEPLOYMENT ENGINER BASE URL",
				JSONPath: "{ .APIDeploymentEngineBaseURL }",
			},
		},
	}
}

func GetKubernetesEnvironmentTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .Kind }",
			},
			{
				Heading:  "CONTEXT",
				JSONPath: "{ .Context }",
			},
			{
				Heading:  "NAMESPACE",
				JSONPath: "{ .Namespace }",
			},
			{
				Heading:  "DEFAULT APPLICATION",
				JSONPath: "{ .DefaultApplication }",
			},
			{
				Heading:  "API SERVER BASE URL",
				JSONPath: "{ .APIServerBaseURL }",
			},
			{
				Heading:  "API DEPLOYMENT ENGINER BASE URL",
				JSONPath: "{ .APIDeploymentEngineBaseURL }",
			},
		},
	}
}

func GetLocalRpTableEnvironmentFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "KIND",
				JSONPath: "{ .Kind }",
			},
			{
				Heading:  "SUBSCRIPTION ID",
				JSONPath: "{ .SubscriptionID }",
			},
			{
				Heading:  "RESOURCE GROUP",
				JSONPath: "{ .ResourceGroup }",
			},
			{
				Heading:  "CONTROL PLANE RESOURCE GROUP",
				JSONPath: "{ .ControlPlaneResourceGroup }",
			},
			{
				Heading:  "CLUSTER NAME",
				JSONPath: "{ .ClusterName }",
			},
			{
				Heading:  "DEFAULT APPLICATION",
				JSONPath: "{ .DefaultApplication }",
			},
			{
				Heading:  "URL",
				JSONPath: "{ .URL }",
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
				JSONPath: "{ .Kind }",
			},
			{
				Heading:  "DEFAULT APPLICATION",
				JSONPath: "{ .DefaultApplication }",
			},
		},
	}
}


