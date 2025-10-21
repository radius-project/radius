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
			name:   "Kubernetes deployment",
			gitURL: "https://github.com/company/terraform-k8s-app",
			module: &TerraformModule{
				Name: "terraform-k8s-app",
				Variables: []TerraformVariable{
					{Name: "app_name", Type: "string"},
					{Name: "namespace", Type: "string"},
				},
			},
			expected: "Kubernetes.Orchestration", // k8s is detected as Orchestration, not just Resources
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
		{"k8s-deployment", "Kubernetes"},
		{"kubernetes-ingress", "Kubernetes"},
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
		{"s3-storage", "Storage"},
		{"blob-storage", "Storage"},
		{"ec2-instance", "Compute"},
		{"vm-compute", "Compute"},
		{"eks-cluster", "Orchestration"},
		{"aks-kubernetes", "Orchestration"},
		{"iam-security", "Security"},
		{"monitoring-logs", "Observability"},
		{"generic-module", ""},
	}

	for _, tt := range tests {
		t.Run(tt.moduleName, func(t *testing.T) {
			result := extractCategory(tt.moduleName)
			require.Equal(t, tt.expected, result)
		})
	}
}