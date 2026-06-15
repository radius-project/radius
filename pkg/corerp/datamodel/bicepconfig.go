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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const BicepConfigResourceType = "Radius.Core/bicepConfigs"

// BicepConfig represents the Radius.Core/bicepConfigs resource.
type BicepConfig struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties BicepConfigResourceProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the BicepConfig instance.
func (r *BicepConfig) ResourceTypeName() string {
	return BicepConfigResourceType
}

// BicepConfigResourceProperties represents the properties of the Bicep config resource.
type BicepConfigResourceProperties struct {
	// RegistryAuthentications maps registry hostname (e.g. "corp.acr.io") to its
	// authentication configuration. The Bicep driver looks up credentials by the
	// host parsed from the recipe template path.
	RegistryAuthentications map[string]BicepRegistryAuthentication `json:"registryAuthentications,omitempty"`

	// ReferencedBy is a list of environment IDs that reference this config.
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

// BicepRegistryAuthentication holds authentication configuration for private Bicep registries.
type BicepRegistryAuthentication struct {
	// AuthenticationMethod is the method to use (BasicAuth, AzureWI, AwsIrsa).
	AuthenticationMethod string `json:"authenticationMethod,omitempty"`

	// BasicAuthSecretId is the ID of a SecretStore with username/password for BasicAuth.
	BasicAuthSecretId string `json:"basicAuthSecretId,omitempty"`

	// AzureWiClientId is the Azure Workload Identity client ID.
	AzureWiClientId string `json:"azureWiClientId,omitempty"`

	// AzureWiTenantId is the Azure Workload Identity tenant ID.
	AzureWiTenantId string `json:"azureWiTenantId,omitempty"`

	// AwsIamRoleArn is the AWS IAM Role ARN for IRSA.
	AwsIamRoleArn string `json:"awsIamRoleArn,omitempty"`
}
