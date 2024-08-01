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

package show

import (
	"github.com/radius-project/radius/pkg/cli/output"
)

func credentialFormatAzureServicePrincipal() output.FormatterOptions {
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
				Heading:  "KIND",
				JSONPath: "{ .AzureCredentials.Kind }",
			},
			{
				Heading:  "CLIENTID",
				JSONPath: "{ .AzureCredentials.ServicePrincipal.ClientID }",
			},
			{
				Heading:  "TENANTID",
				JSONPath: "{ .AzureCredentials.ServicePrincipal.TenantID }",
			},
		},
	}
}

func credentialFormatAzureWorkloadIdentity() output.FormatterOptions {
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
				Heading:  "KIND",
				JSONPath: "{ .AzureCredentials.Kind }",
			},
			{
				Heading:  "CLIENTID",
				JSONPath: "{ .AzureCredentials.WorkloadIdentity.ClientID }",
			},
			{
				Heading:  "TENANTID",
				JSONPath: "{ .AzureCredentials.WorkloadIdentity.TenantID }",
			},
		},
	}
}

func credentialFormatAWSAccessKey() output.FormatterOptions {
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
				Heading:  "KIND",
				JSONPath: "{ .AWSCredentials.Kind }",
			},
			{
				Heading:  "ACCESSKEYID",
				JSONPath: "{ .AWSCredentials.AccessKey.AccessKeyID }",
			},
		},
	}
}

func credentialFormatAWSIRSA() output.FormatterOptions {
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
				Heading:  "KIND",
				JSONPath: "{ .AWSCredentials.Kind }",
			},
			{
				Heading:  "ROLEARN",
				JSONPath: "{ .AWSCredentials.IRSA.RoleARN }",
			},
		},
	}
}
