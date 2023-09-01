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

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The plane resource")
model PlaneResource is TrackedResource<PlaneResourceProperties> {
  @key("planeType")
  @doc("The plane type.")
  @segment("planes")
  @path
  name: string;
}

@doc("Plane kinds supported.")
enum PlaneKind {
  @doc("UCP Native Plane")
  UCPNative,

  @doc("Azure Plane")
  Azure,

  @doc("AWS Plane")
  AWS,
}

@doc("The Plane Name parameter.")
model PlaneNameParameter {
  @doc("The name of the plane")
  @path
  @extension("x-ms-skip-url-encoding", true)
  @extension("x-ms-parameter-location", "method")
  planeName: ResourceNameString;
}

@doc("The UCP HTTP request base parameters.")
model PlaneBaseParameters<
  TResource,
  TPlaneName extends {} = PlaneNameParameter
> {
  ...ApiVersionParameter;
  ...KeysOf<TResource>;
  ...TPlaneName;
}

@doc("The Plane properties.")
model PlaneResourceProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The kind of plane")
  kind: PlaneKind;

  @doc("URL to forward requests to for non UCP Native Plane")
  url?: string;

  @doc("Resource Providers for UCP Native Plane")
  resourceProviders?: Record<string>;
}

@armResourceOperations
interface Planes {
  @doc("List all planes")
  listPlanes is UcpResourceList<PlaneResource, ApiVersionParameter>;

  @doc("List planes by type")
  @get
  @autoRoute
  @armResourceRead(PlaneResource)
  @armResourceList(PlaneResource)
  listByType(
    ...PlaneBaseParameters<PlaneResource, {}>,
  ): ArmResponse<ResourceListResult<PlaneResource>> | ErrorResponse;

  @doc("Get a plane by name")
  get is UcpResourceRead<PlaneResource, PlaneBaseParameters<PlaneResource>>;

  @doc("Create or update a plane")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    PlaneResource,
    PlaneBaseParameters<PlaneResource>
  >;

  @doc("Update a plane")
  update is UcpCustomPatchAsync<
    PlaneResource,
    PlaneBaseParameters<PlaneResource>
  >;

  @doc("Delete a plane")
  delete is UcpResourceDeleteAsync<
    PlaneResource,
    PlaneBaseParameters<PlaneResource>
  >;
}
