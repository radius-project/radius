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

// These operation helpers are created based on the following TypeSpec operations:
// https://github.com/Azure/typespec-azure/blob/main/packages/typespec-azure-resource-manager/lib/operations.tsp

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@get
@autoRoute
@doc("List {name}", Azure.ResourceManager.CommonTypes.Resource)
@segmentOf(Azure.ResourceManager.CommonTypes.Resource)
@armResourceList(Azure.ResourceManager.CommonTypes.Resource)
op UcpResourceList<
  ResourceType extends Azure.ResourceManager.CommonTypes.Resource, 
  TBaseParameters>(
  ...TBaseParameters,
  ...ParentKeysOf<ResourceType>,
): ArmResponse<ResourceListResult<ResourceType>> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Get a {name}", Resource)
@get
@armResourceRead(Resource)
op UcpResourceRead<
  Resource extends Foundations.Resource, TBaseParameters>(
  ...TBaseParameters,
): ArmResponse<Resource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Create a {name}", Resource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "azure-async-operation",
  }
)
@armResourceCreateOrUpdate(Resource)
@put
op UcpResourceCreateOrUpdateAsync<
  Resource extends Foundations.Resource,
  TBaseParameters
>(
  ...TBaseParameters,

  @doc("Resource create parameters.")
  @body
  resource: Resource,
): ArmResourceUpdatedResponse<Resource> | ArmResourceCreatedResponse<Resource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Update a {name}", Resource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "location",
  }
)
@armResourceUpdate(Resource)
@patch
op UcpCustomPatchAsync<
  Resource extends Foundations.Resource,
  TBaseParameters,
  TPatchModel extends TypeSpec.Reflection.Model = TagsUpdateModel<Resource>
>(
  ...TBaseParameters,

  @doc("The resource properties to be updated.")
  @body
  properties: TPatchModel,
): ArmResponse<Resource> | ArmAcceptedLroResponse<"Resource update request accepted."> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Delete a {name}", Resource)
@extension("x-ms-long-running-operation", true)
@extension(
  "x-ms-long-running-operation-options",
  {
    `final-state-via`: "location",
  }
)
@armResourceDelete(Resource)
@delete
op UcpResourceDeleteAsync<
  Resource extends Foundations.Resource,
  TBaseParameters
>(...TBaseParameters):
  | ArmDeletedResponse
  | ArmDeleteAcceptedLroResponse
  | ArmDeletedNoContentResponse
  | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Create a {name}", Resource)
@armResourceCreateOrUpdate(Resource)
@put
op UcpResourceCreateOrUpdateSync<
  Resource extends Foundations.Resource,
  TBaseParameters
>(
  ...TBaseParameters,

  @doc("Resource create parameters.")
  @body
  resource: Resource,
): ArmResourceUpdatedResponse<Resource> | ArmResourceCreatedSyncResponse<Resource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Update a {name}", Resource)
@armResourceUpdate(Resource)
@patch
op UcpCustomPatchSync<
  Resource extends Foundations.Resource,
  TBaseParameters,
  TPatchModel extends TypeSpec.Reflection.Model = TagsUpdateModel<Resource>
>(
  ...TBaseParameters,

  @doc("The resource properties to be updated.")
  @body
  properties: TPatchModel,
): ArmResponse<Resource> | ErrorResponse;

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-operation-outside-interface"
@autoRoute
@doc("Delete a {name}", Resource)
@armResourceDelete(Resource)
@delete
op UcpResourceDeleteSync<
  Resource extends Foundations.Resource,
  TBaseParameters
>(
  ...TBaseParameters,
): ArmDeletedResponse | ArmDeletedNoContentResponse | ErrorResponse;
