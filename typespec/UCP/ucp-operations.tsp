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
import "@typespec/openapi";
import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";

using TypeSpec.Http;
using TypeSpec.Rest;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@get
@autoRoute
@doc("List {name}", TResource)
@segmentOf(TResource)
@armResourceList(TResource)
op UcpResourceListByRootScope<TResource extends ArmResource, TBaseParameters>(
  ...TBaseParameters,
  ...ParentKeysOf<TResource>,
): ArmResponse<ResourceListResult<TResource>> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Get a {name}", TResource)
@get
@armResourceRead(TResource)
op UcpResourceRead<TResource extends ArmResource, TBaseParameters>(
  ...TBaseParameters,
): ArmResponse<TResource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Create a {name}", TResource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "azure-async-operation",
  }
)
@armResourceCreateOrUpdate(TResource)
@put
op UcpResourceCreateOrUpdateAsync<
  TResource extends ArmResource,
  TBaseParameters
>(
  ...TBaseParameters,

  @doc("Resource create parameters.")
  @body
  resource: TResource,
): ArmResourceUpdatedResponse<TResource> | ArmResourceCreatedResponse<TResource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Update a {name}", TResource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "location",
  }
)
@armResourceUpdate(TResource)
@patch
op UcpCustomPatchAsync<
  TResource extends ArmResource,
  TBaseParameters,
  TPatchModel extends TypeSpec.Reflection.Model = TagsUpdateModel<TResource>
>(
  ...TBaseParameters,

  @doc("The resource properties to be updated.")
  @body
  properties: TPatchModel,
): ArmResponse<TResource> | ArmAcceptedLroResponse<"Resource update request accepted."> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Delete a {name}", TResource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "location",
  }
)
@armResourceDelete(TResource)
@delete
op UcpResourceDeleteAsync<TResource extends ArmResource, TBaseParameters>(
  ...TBaseParameters,
):
  | ArmDeletedResponse
  | ArmDeleteAcceptedLroResponse
  | ArmDeletedNoContentResponse
  | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Create a {name}", TResource)
@armResourceCreateOrUpdate(TResource)
@put
op UcpResourceCreateOrUpdateSync<
  TResource extends ArmResource,
  TBaseParameters
>(
  ...TBaseParameters,

  @doc("Resource create parameters.")
  @body
  resource: TResource,
): ArmResourceUpdatedResponse<TResource> | ArmResourceCreatedSyncResponse<TResource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Update a {name}", TResource)
@armResourceUpdate(TResource)
@patch
op UcpCustomPatchSync<
  TResource extends ArmResource,
  TBaseParameters,
  TPatchModel extends TypeSpec.Reflection.Model = TagsUpdateModel<TResource>
>(
  ...TBaseParameters,

  @doc("The resource properties to be updated.")
  @body
  properties: TPatchModel,
): ArmResponse<TResource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Delete a {name}", TResource)
@armResourceDelete(TResource)
@delete
op UcpResourceDeleteSync<TResource extends ArmResource, TBaseParameters>(
  ...TBaseParameters,
): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
