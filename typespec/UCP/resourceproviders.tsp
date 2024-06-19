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
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The resource provider namespace")
@maxLength(63)
@pattern("^([A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9]))\\.([A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9]))?$")
scalar ResourceProviderNamespaceString extends string;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-path-segment-invalid-chars"
model ResourceProviderResource is TrackedResource<ResourceProviderProperties> {
  @key("resourceProviderName")
  @doc("The resource provider name.")
  @path
  @segment("providers")
  name: ResourceProviderNamespaceString;
}


@doc("Resource provider properties")
model ResourceProviderProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  // NOTE: we don't implement regional routing yet, this is here to avoid a breaking change
  // in the future.
  @doc("The configuration of the resource provider in each supported location.")
  locations: Record<ResourceProviderLocation>;

  @extension("x-ms-identifiers", ["resourceType"])
  @doc("The resource types supported by the provider.")
  @visibility("read")
  resourceTypes: ResourceType[];
}

@doc("The configuration of a resource provider in a specific location.")
model ResourceProviderLocation {
  @doc("The address of the resource provider implementation.")
  address: string;
}

@doc("A resource type supported by the resource provider.")
model ResourceType {
  #suppress "@azure-tools/typespec-azure-core/property-name-conflict"
  @doc("The resource type name.")
  resourceType: string;

  @doc("The routing behavior for the resource type.")
  routingType: ResourceTypeRoutingBehavior;

  @doc("The supported resource type api versions.")
  apiVersions: Record<ResourceTypeApiVersion>;

  @doc("The additional capabilities offered by this resource type.")
  capabilities: string[];

  @doc("The default api version for the resource type.")
  defaultApiVersion: string;

  @doc("The locations that are supported by this resource type.")
  locations: string[];
}

// Note: we don't implement other behaviors yet, this is here to avoid a breaking change
// in the future.
@doc("The routing behavior for a resource type.")
enum ResourceTypeRoutingBehavior {
  @doc("The resource type is routed to a separate resource provider implementation.")
  Provider,

  @doc("The resource type is implemented inside UCP.")
  Internal, 
}

@doc("The supported api versions for a resource type.")
model ResourceTypeApiVersion {
  @doc("The OpenAPI v3 schema for the resource types.")
  schema: Record<unknown>;
}

@doc("The UCP HTTP request base parameters.")
model ResourceProviderBaseParameters<TResource> {
  ...PlaneBaseParameters<RadiusPlaneResource>;
  ...KeysOf<TResource>;
}

@route("/planes")
@armResourceOperations
interface ResourceProviders {
  @doc("List resource providers.")
  list is UcpResourceList<
    ResourceProviderResource,
    PlaneBaseParameters<RadiusPlaneResource>
  >;

  @doc("Get the specified resource provider.")
  get is UcpResourceRead<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Create or update a resource provider")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Delete a resource provider")
  delete is UcpResourceDeleteAsync<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;
}
