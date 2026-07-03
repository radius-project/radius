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
model BicepSettingsResource
  is TrackedResourceRequired<BicepSettingsProperties, "bicepSettings"> {
  @doc("Bicep configuration name")
  @key("bicepSettingsName")
  @path
  @segment("bicepSettings")
  name: ResourceNameString;
}

@doc("Bicep configuration properties.")
model BicepSettingsProperties {
  @doc("The status of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Environments that reference this Bicep configuration.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("Authentication configuration for private Bicep registries, keyed by registry hostname (e.g. 'corp.acr.io'). The Bicep driver looks up credentials by the host parsed from the recipe template path.")
  registryAuthentications?: Record<BicepRegistryAuthentication>;
}

@doc("Authentication configuration for a single private Bicep registry. When `authenticationMethod` is `BasicAuth`, `basicAuthSecretId` is required; the controller rejects configs that omit it.")
model BicepRegistryAuthentication {
  @doc("The authentication method to use. Supported values: BasicAuth, AzureWI, AwsIrsa.")
  authenticationMethod?: BicepAuthenticationMethod;

  @doc("The ID of a secret resource containing username and password for BasicAuth. Supported types: Radius.Security/secrets (recommended) or Applications.Core/secretStores. Required when authenticationMethod is 'BasicAuth'.")
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
interface BicepSettings {
  get is ArmResourceRead<
    BicepSettingsResource,
    UCPBaseParameters<BicepSettingsResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    BicepSettingsResource,
    UCPBaseParameters<BicepSettingsResource>
  >;

  update is ArmResourcePatchSync<
    BicepSettingsResource,
    BicepSettingsProperties,
    UCPBaseParameters<BicepSettingsResource>
  >;

  delete is ArmResourceDeleteSync<
    BicepSettingsResource,
    UCPBaseParameters<BicepSettingsResource>
  >;

  listByScope is ArmResourceListByParent<
    BicepSettingsResource,
    UCPBaseParameters<BicepSettingsResource>,
    "Scope",
    "Scope"
  >;
}
