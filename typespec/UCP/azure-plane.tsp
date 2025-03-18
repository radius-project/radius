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
import "./common.tsp";
import "./planes.tsp";
import "./ucp-operations.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The Azure plane resource.")
model AzurePlaneResource
  is TrackedResourceRequired<
    AzurePlaneResourceProperties,
    "System.Azure/planes",
    "planes"
  > {
  @doc("The plane name.")
  @segment("azure")
  @path
  @key("planeName")
  name: ResourceNameString;
}

@doc("The Plane properties.")
model AzurePlaneResourceProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The URL used to proxy requests.")
  url: string;
}

@route("/planes")
@armResourceOperations
interface AzurePlanes {
  @doc("List azure planes")
  @get
  @route("/azure")
  @armResourceList(AzurePlaneResource)
  list(
    ...ApiVersionParameter,
  ): ArmResponse<ResourceListResult<AzurePlaneResource>> | ErrorResponse;

  @doc("Get a plane by name")
  get is UcpResourceRead<
    AzurePlaneResource,
    PlaneBaseParameters<AzurePlaneResource>
  >;

  @doc("Create or update a plane")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    AzurePlaneResource,
    PlaneBaseParameters<AzurePlaneResource>
  >;

  @doc("Update a plane")
  update is UcpCustomPatchAsync<
    AzurePlaneResource,
    PlaneBaseParameters<AzurePlaneResource>
  >;

  @doc("Delete a plane")
  delete is UcpResourceDeleteAsync<
    AzurePlaneResource,
    PlaneBaseParameters<AzurePlaneResource>
  >;
}
