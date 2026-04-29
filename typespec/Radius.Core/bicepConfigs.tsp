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

import "@typespec/rest";
import "@typespec/versioning";
import "@typespec/openapi";
import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "../radius/v1/trackedresource.tsp";

using Azure.ResourceManager;
using TypeSpec.Versioning;
using TypeSpec.Http;
using TypeSpec.Rest;
using Autorest;
using Azure.Core;
using OpenAPI;

namespace Radius.Core;

@doc("The Bicep configuration resource, providing reusable Bicep recipe settings for environments.")
model BicepConfigResource
  is TrackedResourceRequired<BicepConfigProperties, "bicepConfigs"> {
  @doc("Bicep configuration name")
  @key("bicepConfigName")
  @path
  @segment("bicepConfigs")
  name: ResourceNameString;
}

@doc("Bicep configuration properties.")
model BicepConfigProperties {
  @doc("The status of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Environments that reference this Bicep configuration.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("Registry authentication configuration for accessing private Bicep registries.")
  registryAuthentication?: BicepRegistryAuthentication;
}

@doc("Authentication configuration for private Bicep registries.")
model BicepRegistryAuthentication {
  @doc("The authentication method to use. Supported values: BasicAuth, AzureWI, AwsIrsa.")
  authenticationMethod?: BicepAuthenticationMethod;

  @doc("The ID of an Applications.Core/SecretStore resource containing username and password for BasicAuth. Required when authenticationMethod is 'BasicAuth'.")
  basicAuthSecretId?: string;

  @doc("Azure Workload Identity client ID. Required when authenticationMethod is 'AzureWI'.")
  azureWiClientId?: string;

  @doc("Azure Workload Identity tenant ID. Required when authenticationMethod is 'AzureWI'.")
  azureWiTenantId?: string;

  @doc("AWS IAM Role ARN for IRSA authentication. Required when authenticationMethod is 'AwsIrsa'.")
  awsIamRoleArn?: string;
}

@doc("Supported authentication methods for private Bicep registries.")
union BicepAuthenticationMethod {
  string,

  @doc("Username/password authentication.")
  BasicAuth: "BasicAuth",

  @doc("Azure Workload Identity authentication.")
  AzureWI: "AzureWI",

  @doc("AWS IAM Roles for Service Accounts authentication.")
  AwsIrsa: "AwsIrsa",
}

@armResourceOperations
interface BicepConfigs {
  get is ArmResourceRead<
    BicepConfigResource,
    UCPBaseParameters<BicepConfigResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    BicepConfigResource,
    UCPBaseParameters<BicepConfigResource>
  >;

  update is ArmResourcePatchSync<
    BicepConfigResource,
    BicepConfigProperties,
    UCPBaseParameters<BicepConfigResource>
  >;

  delete is ArmResourceDeleteSync<
    BicepConfigResource,
    UCPBaseParameters<BicepConfigResource>
  >;

  listByScope is ArmResourceListByParent<
    BicepConfigResource,
    UCPBaseParameters<BicepConfigResource>,
    "Scope",
    "Scope"
  >;
}
