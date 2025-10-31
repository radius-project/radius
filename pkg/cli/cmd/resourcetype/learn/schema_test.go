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

package learn

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInferNamespaceFromModule(t *testing.T) {
	tests := []struct {
		name     string
		gitURL   string
		module   *TerraformModule
		expected string
	}{
		{
			name:   "AWS VPC module",
			gitURL: "https://github.com/company/terraform-aws-vpc",
			module: &TerraformModule{
				Name: "terraform-aws-vpc",
				Variables: []TerraformVariable{
					{Name: "vpc_cidr", Type: "string"},
					{Name: "availability_zones", Type: "list(string)"},
				},
			},
			expected: "AWS.Network",
		},
		{
			name:   "Azure storage module",
			gitURL: "https://github.com/company/terraform-azure-storage",
			module: &TerraformModule{
				Name: "terraform-azure-storage",
				Variables: []TerraformVariable{
					{Name: "storage_account_name", Type: "string"},
					{Name: "resource_group_name", Type: "string"},
				},
			},
			expected: "Azure.Storage",
		},
		{
			name:   "GCP database module",
			gitURL: "https://github.com/company/terraform-gcp-database",
			module: &TerraformModule{
				Name: "terraform-gcp-database",
				Variables: []TerraformVariable{
					{Name: "database_name", Type: "string"},
					{Name: "project_id", Type: "string"},
				},
			},
			expected: "GCP.Data",
		},
		{
			name:   "Kubernetes Redis module",
			gitURL: "https://github.com/squareops/terraform-kubernetes-redis",
			module: &TerraformModule{
				Name: "terraform-kubernetes-redis",
				Variables: []TerraformVariable{
					{Name: "redis_name", Type: "string"},
					{Name: "namespace", Type: "string"},
				},
			},
			expected: "Custom.Data", // redis is detected as Data category
		},
		{
			name:   "Kubernetes deployment",
			gitURL: "https://github.com/company/terraform-k8s-app",
			module: &TerraformModule{
				Name: "terraform-k8s-app",
				Variables: []TerraformVariable{
					{Name: "app_name", Type: "string"},
					{Name: "namespace", Type: "string"},
				},
			},
			expected: "Custom.Resources", // k8s is not a provider, no specific category detected
		},
		{
			name:   "Generic module with variable hints",
			gitURL: "https://github.com/company/infrastructure-module",
			module: &TerraformModule{
				Name: "infrastructure-module",
				Variables: []TerraformVariable{
					{Name: "vpc_id", Type: "string"},
					{Name: "subnet_ids", Type: "list(string)"},
					{Name: "aws_region", Type: "string"},
				},
			},
			expected: "AWS.Network",
		},
		{
			name:   "Fallback to default",
			gitURL: "https://github.com/company/unknown-module",
			module: &TerraformModule{
				Name: "unknown-module",
				Variables: []TerraformVariable{
					{Name: "some_config", Type: "string"},
				},
			},
			expected: "Custom.Resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferNamespaceFromModule(tt.module, tt.gitURL)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractProvider(t *testing.T) {
	tests := []struct {
		moduleName string
		expected   string
	}{
		{"terraform-aws-vpc", "AWS"},
		{"azure-storage-module", "Azure"},
		{"gcp-compute-instance", "GCP"},
		{"google-cloud-sql", "GCP"},
		{"k8s-deployment", ""},     // k8s is not a cloud provider
		{"kubernetes-ingress", ""}, // kubernetes is not a cloud provider
		{"docker-container", "Docker"},
		{"helm-chart", "Helm"},
		{"generic-module", ""},
	}

	for _, tt := range tests {
		t.Run(tt.moduleName, func(t *testing.T) {
			result := extractProvider(tt.moduleName)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCategory(t *testing.T) {
	tests := []struct {
		moduleName string
		expected   string
	}{
		{"aws-vpc-module", "Network"},
		{"azure-network-security", "Network"}, // network appears first in the name
		{"rds-database", "Data"},
		{"postgres-db", "Data"},
		{"kubernetes-redis", "Data"}, // redis is detected as Data, not Orchestration
		{"s3-storage", "Storage"},
		{"blob-storage", "Storage"},
		{"ec2-instance", "Compute"},
		{"vm-compute", "Compute"},
		{"eks-cluster", "Orchestration"},    // actual k8s cluster is orchestration
		{"aks-kubernetes", "Orchestration"}, // actual k8s cluster is orchestration
		{"iam-security", "Security"},
		{"monitoring-logs", "Observability"},
		{"k8s-app", ""}, // just k8s without specific resource type
		{"generic-module", ""},
	}

	for _, tt := range tests {
		t.Run(tt.moduleName, func(t *testing.T) {
			result := extractCategory(tt.moduleName)
			require.Equal(t, tt.expected, result)
		})
	}
}
func TestShouldSkipVariable(t *testing.T) {
	tests := []struct {
		variableName string
		shouldSkip   bool
	}{
		// Only context should be skipped (recipe context)
		{"context", true},
		{"Context", true},
		{"CONTEXT", true},

		// Everything else should NOT be skipped
		{"vpc_name", false},
		{"vpc_cidr", false},
		{"aws_region", false},
		{"resource_id", false},
		{"id", false},
		{"metadata", false},
		{"status", false},
		{"radius_context", false},
		{"provision_state", false},
		{"provisioning_state", false},
		{"created_time", false},
		{"updated_time", false},
		{"last_modified", false},
		{"etag", false},
		{"system_data", false},
		{"custom_context", false}, // Contains "context" but not exact match
		{"my_context", false},     // Contains "context" but not exact match
	}

	for _, tt := range tests {
		t.Run(tt.variableName, func(t *testing.T) {
			result := shouldSkipVariable(tt.variableName)
			require.Equal(t, tt.shouldSkip, result)
		})
	}
}

func TestFormatDefaultValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "null",
		},
		{
			name:     "string value",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "string with quotes",
			input:    `"quoted string"`,
			expected: "quoted string",
		},
		{
			name:     "boolean true",
			input:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			input:    false,
			expected: "false",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "float",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: "[]",
		},
		{
			name:     "string array",
			input:    []interface{}{"item1", "item2", "item3"},
			expected: "[item1, item2, item3]",
		},
		{
			name:     "mixed array",
			input:    []interface{}{"string", 42, true},
			expected: "[string, 42, true]",
		},
		{
			name:     "empty object",
			input:    map[string]interface{}{},
			expected: "{}",
		},
		{
			name: "simple object",
			input: map[string]interface{}{
				"name":    "test",
				"enabled": true,
				"count":   5,
			},
			expected: "{ count = 5, enabled = true, name = test }",
		},
		{
			name: "nested object",
			input: map[string]interface{}{
				"config": map[string]interface{}{
					"timeout": 30,
					"retry":   true,
				},
				"name": "service",
			},
			expected: "{ config = { retry = true, timeout = 30 }, name = service }",
		},
		{
			name: "object with array",
			input: map[string]interface{}{
				"tags":  []interface{}{"prod", "web"},
				"count": 3,
			},
			expected: "{ count = 3, tags = [prod, web] }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDefaultValue(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
