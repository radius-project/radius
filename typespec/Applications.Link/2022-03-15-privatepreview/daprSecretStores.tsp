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
import "@azure-tools/typespec-providerhub";

import "../../radius/v1/ucprootscope.tsp";
import "../../radius/v1/resources.tsp";
import "./common.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Link;

@doc("DaprSecretStore link properties")
model DaprSecretStoreProperties
  is DaprResourceProperties<DaprSecretStoreProperties> {
  @doc("Provisioning state of the dapr secret store link at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  ...RecipeBaseProperties;
}

@doc("DaprSecretStore link")
model DaprSecretStoreResource is TrackedResource<DaprSecretStoreProperties> {
  @doc("DaprSecretStore name")
  @key("daprSecretStoreName")
  @path
  @segment("daprSecretStores")
  name: ResourceNameString;
}

@armResourceOperations
interface DaprSecretStores {
  @example("./examples/DaprSecretStoresGet.json", "DaprSecretStoresGet")
  get is ArmResourceRead<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;

  @example("./examples/DaprSecretStoresPut.json", "DaprSecretStoresPut")
  create is ArmResourceCreateOrReplaceAsync<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;

  @example("./examples/DaprSecretStoresPatch.json", "DaprSecretStoresPatch")
  update is ArmResourcePatchAsync<
    DaprSecretStoreResource,
    DaprSecretStoreProperties,
    UCPBaseParameters<DaprSecretStoreResource>
  >;

  @example("./examples/DaprSecretStoresDelete.json", "DaprSecretStoresDelete")
  delete is ArmResourceDeleteAsync<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;

  @example(
    "./examples/DaprSecretStoresListByRootScope.json",
    "DaprSecretStoresListByRootScope"
  )
  listByScope is ArmResourceListByParent<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>,
    "Scope",
    "Scope"
  >;
}
