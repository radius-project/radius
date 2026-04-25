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

  @doc("Authentication information used to access private Bicep registries, which is a map of registry hostname to secret config that contains credential information.")
  authentication?: Record<BicepRegistrySecretConfig>;
}

@doc("Registry Secret Configuration used to authenticate to private Bicep registries.")
model BicepRegistrySecretConfig {
  @doc("The ID of an Applications.Core/SecretStore resource containing credential information used to authenticate private container registry. The keys in the secret store depend on the type.")
  secret?: string;
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
