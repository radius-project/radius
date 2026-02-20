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
using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using OpenAPI;

namespace Radius.Core;

@doc("Bicep settings resource.")
model BicepSettingsResource
  is TrackedResourceRequired<BicepSettingsProperties, "bicepSettings"> {
  @doc("Bicep settings resource name.")
  @key("bicepSettingsName")
  @path
  @segment("bicepSettings")
  name: ResourceNameString;
}

@doc("Bicep settings properties.")
model BicepSettingsProperties {
  @doc("Provisioning state of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Authentication settings for private registries.")
  authentication?: BicepAuthenticationConfiguration;
}

@doc("Authentication configuration for Bicep registries.")
model BicepAuthenticationConfiguration {
  @doc("Authentication entries keyed by registry hostname.")
  registries?: Record<BicepRegistryAuthentication>;
}

@doc("Registry authentication options for a private Bicep registry.")
model BicepRegistryAuthentication {
  @doc("Basic authentication settings for a registry.")
  basic?: BicepBasicAuthentication;

  @doc("Azure Workload Identity authentication settings for a registry.")
  azureWorkloadIdentity?: BicepAzureWorkloadIdentityAuthentication;

  @doc("AWS IRSA authentication settings for a registry.")
  awsIrsa?: BicepAwsIrsaAuthentication;
}

@doc("Basic authentication configuration for a Bicep registry.")
model BicepBasicAuthentication {
  @doc("Username for basic authentication.")
  username?: string;

  @doc("Password credential for basic authentication.")
  password?: SecretReference;
}

@doc("Azure Workload Identity configuration for a Bicep registry.")
model BicepAzureWorkloadIdentityAuthentication {
  @doc("Client ID used for Azure Workload Identity.")
  clientId?: string;

  @doc("Tenant ID used for Azure Workload Identity.")
  tenantId?: string;

  @doc("Token credential for Azure Workload Identity authentication.")
  token?: SecretReference;
}

@doc("AWS IRSA configuration for a Bicep registry.")
model BicepAwsIrsaAuthentication {
  @doc("ARN of the AWS IAM role used for IRSA.")
  roleArn?: string;

  @doc("Token credential for AWS IRSA authentication.")
  token?: SecretReference;
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
