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

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "./common.tsp";
import "./ucp-operations.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Ucp;

@doc("The resource group resource")
model ResourceGroupResource is TrackedResource<ResourceGroupProperties> {
  @doc("The name of resource group")
  @path
  @key("resourceGroupName")
  @segment("resourcegroups")
  name: ResourceNameString;
}

@doc("The resource group resource properties")
model ResourceGroupProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("The UCP HTTP request base parameters.")
model ResourceGroupBaseParameters<TResource, TPlaneBase = PlaneResource> {
  ...PlaneBaseParameters<TPlaneBase>;
  ...KeysOf<TResource>;
}

@armResourceOperations
interface ResourceGroups {
  @doc("List resource groups")
  listByRootScope is UcpResourceListByRootScope<
    ResourceGroupResource,
    PlaneBaseParameters<PlaneResource>
  >;

  @doc("Get a resource group")
  get is UcpResourceRead<
    ResourceGroupResource,
    ResourceGroupBaseParameters<ResourceGroupResource>
  >;

  @doc("Create or update a resource group")
  createOrUpdate is UcpResourceCreateOrUpdateSync<
    ResourceGroupResource,
    ResourceGroupBaseParameters<ResourceGroupResource>
  >;
  /*
  update is UcpCustomPatchAsync<
    ResourceGroupResource,
    ResourceGroupBaseParameters<ResourceGroupResource>
  >;*/

  @doc("Delete a resource group")
  delete is UcpResourceDeleteSync<
    ResourceGroupResource,
    ResourceGroupBaseParameters<ResourceGroupResource>
  >;
}
