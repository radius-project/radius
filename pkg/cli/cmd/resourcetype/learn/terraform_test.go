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

func TestParseVariablesFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []TerraformVariable
	}{
		{
			name: "simple string variable",
			content: `variable "vpc_name" {
  type        = string
  description = "Name of the VPC"
  default     = "my-vpc"
}`,
			expected: []TerraformVariable{
				{
					Name:        "vpc_name",
					Type:        "string",
					Description: "Name of the VPC",
					Default:     `"my-vpc"`,
					Required:    false,
				},
			},
		},
		{
			name: "required variable without default",
			content: `variable "availability_zones" {
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
			content: `variable "vpc_cidr" {
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
					Default:     `"10.0.0.0/16"`,
					Required:    false,
				},
				{
					Name:        "enable_dns_hostnames",
					Type:        "bool",
					Description: "Enable DNS hostnames in VPC",
					Default:     "true",
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
			name: "variable with object type",
			content: `variable "vpc_config" {
  type = object({
    cidr_block           = string
    enable_dns_hostnames = bool
    enable_dns_support   = bool
  })
  description = "VPC configuration object"
}`,
			expected: []TerraformVariable{
				{
					Name: "vpc_config",
					Type: `object({
    cidr_block           = string
    enable_dns_hostnames = bool
    enable_dns_support   = bool
  })`,
					Description: "VPC configuration object",
					Default:     nil,
					Required:    true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variables, err := parseVariablesFromContent(tt.content)
			require.NoError(t, err)
			require.Equal(t, tt.expected, variables)
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