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

  @doc("Authentication settings keyed by registry hostname.")
  registryAuthentication?: Record<BicepRegistrySecretConfig>;
}

@doc("Registry secret configuration used to authenticate to private Bicep registries.")
model BicepRegistrySecretConfig {
  @doc("Resource ID of the Radius.Security/secrets entry containing credential data (for example username/password, Azure workload identity, AWS IRSA).")
  secret?: string;
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
