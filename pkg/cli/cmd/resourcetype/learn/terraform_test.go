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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTerraformModule(t *testing.T) {
	tests := []struct {
		name          string
		terraformCode string
		expected      []TerraformVariable
	}{
		{
			name: "simple string variable",
			terraformCode: `variable "vpc_name" {
  type        = string
  description = "Name of the VPC"
  default     = "my-vpc"
}`,
			expected: []TerraformVariable{
				{
					Name:        "vpc_name",
					Type:        "string",
					Description: "Name of the VPC",
					Default:     "my-vpc",
					Required:    false,
				},
			},
		},
		{
			name: "required variable without default",
			terraformCode: `variable "availability_zones" {
  type        = list(string)
  description = "List of availability zones"
}`,
			expected: []TerraformVariable{
				{
					Name:        "availability_zones",
					Type:        "list(string)",
					Description: "List of availability zones",
					Default:     nil,
					Required:    true,
				},
			},
		},
		{
			name: "multiple variables",
			terraformCode: `variable "vpc_cidr" {
  type        = string
  description = "CIDR block for VPC"
  default     = "10.0.0.0/16"
}

variable "enable_dns_hostnames" {
  type        = bool
  description = "Enable DNS hostnames in VPC"
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to resources"
}`,
			expected: []TerraformVariable{
				{
					Name:        "vpc_cidr",
					Type:        "string",
					Description: "CIDR block for VPC",
					Default:     "10.0.0.0/16",
					Required:    false,
				},
				{
					Name:        "enable_dns_hostnames",
					Type:        "bool",
					Description: "Enable DNS hostnames in VPC",
					Default:     true,
					Required:    false,
				},
				{
					Name:        "tags",
					Type:        "map(string)",
					Description: "Tags to apply to resources",
					Default:     nil,
					Required:    true,
				},
			},
		},
		{
			name: "variable with object default",
			terraformCode: `variable "vpc_config" {
  type = object({
    cidr_block           = string
    enable_dns_hostnames = bool
    enable_dns_support   = bool
  })
  description = "VPC configuration object"
  default = {
    cidr_block           = "10.0.0.0/16"
    enable_dns_hostnames = true
    enable_dns_support   = true
  }
}`,
			expected: []TerraformVariable{
				{
					Name:        "vpc_config",
					Type:        "object({\n    cidr_block           = string\n    enable_dns_hostnames = bool\n    enable_dns_support   = bool\n  })",
					Description: "VPC configuration object",
					Default: map[string]interface{}{
						"cidr_block":           "10.0.0.0/16",
						"enable_dns_hostnames": true,
						"enable_dns_support":   true,
					},
					Required: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tempDir, err := os.MkdirTemp("", "terraform-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Write the Terraform code to a variables.tf file
			err = os.WriteFile(filepath.Join(tempDir, "variables.tf"), []byte(tt.terraformCode), 0644)
			require.NoError(t, err)

			// Parse the module
			module, err := ParseTerraformModule(tempDir)
			require.NoError(t, err)

			// Check that we got the expected variables
			require.Equal(t, len(tt.expected), len(module.Variables))

			for i, expectedVar := range tt.expected {
				actualVar := module.Variables[i]
				require.Equal(t, expectedVar.Name, actualVar.Name)
				require.Equal(t, expectedVar.Type, actualVar.Type)
				require.Equal(t, expectedVar.Description, actualVar.Description)
				require.Equal(t, expectedVar.Default, actualVar.Default)
				require.Equal(t, expectedVar.Required, actualVar.Required)
			}
		})
	}
}

func TestConvertTerraformTypeToJSONSchema(t *testing.T) {
	tests := []struct {
		terraformType string
		expected      string
	}{
		{"string", "string"},
		{"number", "number"},
		{"bool", "boolean"},
		{"boolean", "boolean"},
		{"list(string)", "array"},
		{"set(string)", "array"},
		{"map(string)", "object"},
		{"object({name = string})", "object"},
		{"unknown_type", "string"},
		{"", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.terraformType, func(t *testing.T) {
			result := ConvertTerraformTypeToJSONSchema(tt.terraformType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateResourceTypeName(t *testing.T) {
	tests := []struct {
		moduleName string
		expected   string
	}{
		{"terraform-aws-vpc", "awsVpc"},
		{"tf-azure-storage", "azureStorage"},
		{"my-custom-module", "myCustom"},
		{"simple", "simple"},
		{"terraform-", "customResource"},
		{"", "customResource"},
		{"Test.Module", "testModule"},
	}

	for _, tt := range tests {
		t.Run(tt.moduleName, func(t *testing.T) {
			result := generateResourceTypeName(tt.moduleName)
			require.Equal(t, tt.expected, result)
		})
	}
}
