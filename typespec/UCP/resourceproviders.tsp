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

@doc("The resource provider namespace. Example: 'Applications.Datastores'.")
@maxLength(63)
@pattern("^([A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9]))\\.([A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9]))?$")
scalar ResourceProviderNamespaceString extends string;

@doc("The resource type name. Example: 'redisCaches'.")
@maxLength(63)
@pattern("^([A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9]))$")
scalar ResourceTypeNameString extends string;

@doc("The resource type API version. Example: '2023-10-01-preview'.")
@maxLength(63)
@pattern("^\\d{4}-\\d{2}-\\d{2}(-preview)?$")
scalar ApiVersionNameString extends string;

@doc("The resource provider location name. Example: 'eastus'.")
@maxLength(63)
@pattern("^[A-Za-z][A-Za-z0-9]*$")
scalar LocationNameString extends string;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-path-segment-invalid-chars"
@doc("The resource type for defining a resource provider.")
model ResourceProviderResource
  is Azure.ResourceManager.TrackedResource<ResourceProviderProperties> {
  @key("resourceProviderName")
  @doc("The resource provider name. This is also the resource provider namespace. Example: 'Applications.Datastores'.")
  @path
  @segment("providers/System.Resources/resourceproviders")
  name: ResourceProviderNamespaceString;
}

@doc("The properties of a resource provider.")
model ResourceProviderProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("The resource type for defining a resource type supported by the containing resource provider.")
model ResourceTypeResource
  is Azure.ResourceManager.ProxyResource<ResourceTypeProperties> {
  @key("resourceTypeName")
  @doc("The resource type name.")
  @path
  @segment("resourcetypes")
  name: ResourceTypeNameString;
}

@doc("The properties of a resource type.")
model ResourceTypeProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The default api version for the resource type.")
  defaultApiVersion?: ApiVersionNameString;
}

@doc("The resource type for defining an API version of a resource type supported by the containing resource provider.")
model ApiVersionResource
  is Azure.ResourceManager.ProxyResource<ApiVersionProperties> {
  @key("apiVersionName")
  @doc("The API version name.")
  @path
  @segment("apiversions")
  name: ApiVersionNameString;
}

@doc("The properties of an API version.")
model ApiVersionProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("The resource type for defining a location of the containing resource provider. The location resource represents a logical location where the resource provider operates.")
model LocationResource
  is Azure.ResourceManager.ProxyResource<LocationProperties> {
  @key("locationName")
  @doc("The location name.")
  @path
  @segment("locations")
  name: LocationNameString;
}

@doc("The properties of a location.")
model LocationProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Address of a resource provider implementation.")
  address?: string;

  @doc("Configuration for resource types supported by the location.")
  resourceTypes?: Record<LocationResourceType>;
}

@doc("The configuration for a resource type in a specific location.")
model LocationResourceType {
  @doc("The configuration for API versions of a resource type supported by the location.")
  apiVersions?: Record<LocationResourceTypeApiVersion>;
}

@doc("The configuration for an API version of an resource type.")
model LocationResourceTypeApiVersion {
  // Empty for now.
}

@doc("The summary of a resource provider configuration. This type is optimized for querying resource providers and supported types.")
model ResourceProviderSummary {
  @doc("The resource provider name.")
  name: ResourceProviderNamespaceString;

  @doc("The resource provider locations.")
  locations: Record<ResourceProviderSummaryLocation>;

  @doc("The resource types supported by the resource provider.")
  resourceTypes: Record<ResourceProviderSummaryResourceType>;
}

@doc("The configuration of a resource provider in a specific location.")
model ResourceProviderSummaryLocation {}

@doc("A resource type and its versions.")
model ResourceProviderSummaryResourceType {
  @doc("API versions supported by the resource type.")
  apiVersions: Record<ResourceTypeSummaryResultApiVersion>;

  @doc("The default api version for the resource type.")
  defaultApiVersion?: string;
}

@doc("The configuration of a resource type API version.")
model ResourceTypeSummaryResultApiVersion {}

model ResourceProviderBaseParameters<Resource> {
  ...PlaneBaseParameters<RadiusPlaneResource>;
  ...KeysOf<Resource>;
}

model ResourceTypeBaseParameters<Resource> {
  ...PlaneBaseParameters<RadiusPlaneResource>;
  ...KeysOf<ResourceProviderResource>;
  ...KeysOf<Resource>;
}

model ApiVersionBaseParameters<Resource> {
  ...PlaneBaseParameters<RadiusPlaneResource>;
  ...KeysOf<ResourceProviderResource>;
  ...KeysOf<ResourceTypeResource>;
  ...KeysOf<Resource>;
}

model LocationBaseParameters<Resource> {
  ...PlaneBaseParameters<RadiusPlaneResource>;
  ...KeysOf<ResourceProviderResource>;
  ...KeysOf<Resource>;
}

@armResourceOperations
interface ResourceProviders {
  @doc("List resource providers.")
  @route("GET /planes/radius/{planeName}/providers/System.Resources/resourceproviders")
  list is UcpResourceList<
    ResourceProviderResource,
    PlaneBaseParameters<RadiusPlaneResource>
  >;

  @doc("Get the specified resource provider.")
  @route("GET /planes/{resourceProviderName}")
  get is UcpResourceRead<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Create or update a resource provider")
  @route("PUT /planes/{resourceProviderName}")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Delete a resource provider")
  @route("DELETE /planes/{resourceProviderName}")
  delete is UcpResourceDeleteAsync<
    ResourceProviderResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("List resource provider summaries. The resource provider summary aggregates the most commonly used information including locations, api versions and resource types.")
  @segment("providers")
  listProviderSummaries(
    ...PlaneBaseParameters<RadiusPlaneResource>,
  ): Azure.Core.Page<ResourceProviderSummary> | ErrorResponse;

  @doc("Get the specified resource provider summary. The resource provider summary aggregates the most commonly used information including locations, api versions and resource types.")
  getProviderSummary(
    ...PlaneBaseParameters<RadiusPlaneResource>,

    @doc("The resource provider name. This is also the resource provider namespace. Example: 'Applications.Datastores'.")
    @path
    @segment("providers")
    resourceProviderName: ResourceProviderNamespaceString,
  ): ArmResponse<ResourceProviderSummary> | ErrorResponse;
}

@armResourceOperations
interface ResourceTypes {
  @doc("List resource types.")
  @route("GET /planes/radius/{planeName}/providers/System.Resources/resourceproviders/{resourceProviderName}/resourcetypes")
  list is UcpResourceList<
    ResourceTypeResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Get the specified resource type.")
  @route("GET /planes/radius/{planeName}/providers/System.Resources/resourceproviders/{resourceProviderName}/resourcetypes/{resourceTypeName}")
  get is UcpResourceRead<
    ResourceTypeResource,
    ResourceTypeBaseParameters<ResourceTypeResource>
  >;

  @doc("Create or update a resource type")
  @route("PUT /planes/radius/{planeName}/providers/System.Resources/resourceproviders/{resourceProviderName}/resourcetypes/{resourceTypeName}")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    ResourceTypeResource,
    ResourceTypeBaseParameters<ResourceTypeResource>
  >;

  @doc("Delete a resource type")
  @route("DELETE /planes/radius/{planeName}/providers/System.Resources/resourceproviders/{resourceProviderName}/resourcetypes/{resourceTypeName}")
  delete is UcpResourceDeleteAsync<
    ResourceTypeResource,
    ResourceTypeBaseParameters<ResourceTypeResource>
  >;
}

@route("/planes")
@armResourceOperations
interface ApiVersions {
  @doc("List API versions.")
  list is UcpResourceList<
    ApiVersionResource,
    ResourceTypeBaseParameters<ResourceTypeResource>
  >;

  @doc("Get the specified API version.")
  get is UcpResourceRead<
    ApiVersionResource,
    ApiVersionBaseParameters<ApiVersionResource>
  >;

  @doc("Create or update an API version.")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    ApiVersionResource,
    ApiVersionBaseParameters<ApiVersionResource>
  >;

  @doc("Delete an API version.")
  delete is UcpResourceDeleteAsync<
    ApiVersionResource,
    ApiVersionBaseParameters<ApiVersionResource>
  >;
}

@route("/planes")
@armResourceOperations
interface Locations {
  @doc("List available locations for the specified resource provider.")
  list is UcpResourceList<
    LocationResource,
    ResourceProviderBaseParameters<ResourceProviderResource>
  >;

  @doc("Get the specified location. The location resource represents a logical location where the resource provider operates.")
  get is UcpResourceRead<
    LocationResource,
    LocationBaseParameters<LocationResource>
  >;

  @doc("Create or update a location. The location resource represents a logical location where the resource provider operates.")
  createOrUpdate is UcpResourceCreateOrUpdateAsync<
    LocationResource,
    LocationBaseParameters<LocationResource>
  >;

  @doc("Delete a location. The location resource represents a logical location where the resource provider operates.")
  delete is UcpResourceDeleteAsync<
    LocationResource,
    LocationBaseParameters<LocationResource>
  >;
}
