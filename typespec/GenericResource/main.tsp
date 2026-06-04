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

import "@typespec/http";
import "@typespec/rest";
import "@typespec/versioning";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-client-generator-core";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ClientGenerator.Core;

@armProviderNamespace
@service(#{ title: "Generic Resource Management APIs" })
@versioned(Versions)
@doc("Generic resource management client")
namespace GenericResource;

@doc("The available API versions.")
enum Versions {
  @doc("2023-10-01-preview")
  v2023_10_01_preview: "2023-10-01-preview",
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

@doc("The type of identity that created/modified the resource.")
enum CreatedByType {
  User,
  Application,
  ManagedIdentity,
  Key,
}

@doc("Metadata pertaining to creation and last modification of the resource.")
model SystemData {
  @doc("The identity that created the resource.")
  createdBy?: string;

  @doc("The type of identity that created the resource.")
  createdByType?: CreatedByType;

  @doc("The timestamp of resource creation (UTC).")
  createdAt?: utcDateTime;

  @doc("The identity that last modified the resource.")
  lastModifiedBy?: string;

  @doc("The type of identity that last modified the resource.")
  lastModifiedByType?: CreatedByType;

  @doc("The timestamp of resource last modification (UTC)")
  lastModifiedAt?: utcDateTime;
}

@doc("Common fields that are returned in the response for all Azure Resource Manager resources")
model Resource {
  @visibility(Lifecycle.Read)
  @doc("Fully qualified resource ID for the resource. Ex - /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/{resourceProviderNamespace}/{resourceType}/{resourceName}")
  id?: string;

  @visibility(Lifecycle.Read)
  @doc("The name of the resource")
  name?: string;

  @visibility(Lifecycle.Read)
  @doc("The type of the resource. E.g. \"Microsoft.Compute/virtualMachines\" or \"Microsoft.Storage/storageAccounts\"")
  type?: string;
}

@doc("The resource model definition for an Azure Resource Manager tracked top level resource which has 'tags' and a 'location'")
model TrackedResource extends Resource {
  @doc("Resource tags.")
  tags?: Record<string>;

  @doc("The geo-location where the resource lives")
  location: string;
}

@doc("Generic resource")
model GenericResource extends TrackedResource {
  @doc("Any object")
  properties: Record<unknown>;

  @visibility(Lifecycle.Read)
  @doc("Metadata pertaining to creation and last modification of the resource.")
  systemData?: SystemData;
}

@doc("Object that includes an array of GenericResources and a possible link for next set")
model GenericResourcesList {
  @doc("List of GenericResources resources")
  @pageItems
  value?: GenericResource[];

  @doc("The link used to fetch the next page of resource list.")
  @nextLink
  nextLink?: string;
}

@doc("The resource management error additional info.")
model ErrorAdditionalInfo {
  @visibility(Lifecycle.Read)
  @doc("The additional info type.")
  type?: string;

  @visibility(Lifecycle.Read)
  @doc("The additional info.")
  info?: {};
}

@doc("The error detail.")
model ErrorDetail {
  @visibility(Lifecycle.Read)
  @doc("The error code.")
  code?: string;

  @visibility(Lifecycle.Read)
  @doc("The error message.")
  message?: string;

  @visibility(Lifecycle.Read)
  @doc("The error target.")
  target?: string;

  @visibility(Lifecycle.Read)
  @doc("The error details.")
  details?: ErrorDetail[];

  @visibility(Lifecycle.Read)
  @doc("The error additional info.")
  additionalInfo?: ErrorAdditionalInfo[];
}

@error
@doc("Common error response for all Azure Resource Manager APIs to return error details for failed operations. (This also follows the OData error response format.).")
model ErrorResponse {
  @doc("The error object.")
  error?: ErrorDetail;
}

// ---------------------------------------------------------------------------
// Client-init params (hoisted onto the client constructor via TCGC)
// ---------------------------------------------------------------------------

@doc("Client initialization parameters for GenericResources.")
model GenericResourceClientParameters {
  @doc("The scope in which the resource is present.")
  rootScope: string;

  @doc("The azure resource type. For example RedisCache, RabbitMQ and other")
  resourceType: string;
}

// Shared path params declared on operations; hoisted to the client via @clientInitialization.
model RootScopePathParams {
  @path
  @doc("The scope in which the resource is present.")
  rootScope: string;

  @path
  @doc("The azure resource type. For example RedisCache, RabbitMQ and other")
  resourceType: string;
}

model ResourceNamePathParam {
  @path
  @doc("The name of the generic resource")
  resourceName: string;
}

model ApiVersionParam {
  @query("api-version")
  @apiVersion
  @doc("The API version to use for this operation.")
  apiVersion: string;
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

@route("/{+rootScope}/providers/{+resourceType}")
interface GenericResources {
  @doc("Lists information about all resources of the given resource type in the given root scope")
  @list
  @get
  listByRootScope(...RootScopePathParams, ...ApiVersionParam):
    | GenericResourcesList
    | ErrorResponse;

  @doc("Retrieves information about a generic resource with the name given of the type given within the root scope")
  @get
  @route("/{resourceName}")
  get(...RootScopePathParams, ...ResourceNamePathParam, ...ApiVersionParam):
    | GenericResource
    | ErrorResponse;

  @doc("Creates or updates a Generic resource")
  @put
  @route("/{resourceName}")
  createOrUpdate(
    ...RootScopePathParams,
    ...ResourceNamePathParam,
    ...ApiVersionParam,
    @body genericResourceParameters: GenericResource,
  ):
    | {
        @doc("Resource 'GenericResource' update operation succeeded")
        @statusCode
        statusCode: 200;

        @body body: GenericResource;
      }
    | {
        @doc("Resource 'GenericResource' create operation succeeded")
        @statusCode
        statusCode: 201;

        @body body: GenericResource;
      }
    | ErrorResponse;

  @doc("Deletes an existing Generic resource")
  @delete
  @route("/{resourceName}")
  delete(...RootScopePathParams, ...ResourceNamePathParam, ...ApiVersionParam):
    | ArmDeletedResponse
    | ArmDeleteAcceptedLroResponse
    | ArmDeletedNoContentResponse
    | ErrorResponse;

  @doc("Lists secrets for a resource")
  @post
  @route("/{resourceName}/listSecrets")
  listSecrets(
    ...RootScopePathParams,
    ...ResourceNamePathParam,
    ...ApiVersionParam,
  ): Record<string> | ErrorResponse;
}

// Hoist rootScope + resourceType onto the client constructor.
@@clientInitialization(GenericResources, GenericResourceClientParameters);

// Mark createOrUpdate as LRO so the emitter produces a BeginCreateOrUpdate poller.
// delete is detected as an LRO natively via the ARM accepted-LRO response template.
@@Azure.ClientGenerator.Core.Legacy.markAsLro(GenericResources.createOrUpdate);
