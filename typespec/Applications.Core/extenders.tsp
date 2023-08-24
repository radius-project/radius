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
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Core;

@doc("Response for list secrets API call")
model ExtenderListSecretResponse {}

@doc("ExtenderResource link")
model ExtenderResource is TrackedResource<ExtenderProperties> {
  @path
  @doc("The name of the ExtenderResource link resource")
  @key("extenderName")
  @segment("extenders")
  name: ResourceNameString;
}

// ExtenderResoruce allows to get the additional properties. To ensure that `additionalProperties` is true,
// we need to extend `Record<unknown>`.
// Reference: https://github.com/Azure/typespec-azure/blob/main/packages/typespec-autorest/test/additional-properties.test.ts
#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("ExtenderResource link properties")
model ExtenderProperties extends Record<unknown> {
  ...EnvironmentScopedResource;

  @doc("The secrets for referenced resource")
  secrets?: {};

  @doc("The recipe used to automatically deploy underlying infrastructure for the extender link")
  recipe?: Recipe;

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;
}

@armResourceOperations
interface Extenders {
  get is ArmResourceRead<ExtenderResource, UCPBaseParameters<ExtenderResource>>;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    ExtenderResource,
    UCPBaseParameters<ExtenderResource>
  >;

  update is ArmResourcePatchAsync<
    ExtenderResource,
    ExtenderProperties,
    UCPBaseParameters<ExtenderResource>
  >;

  delete is ArmResourceDeleteAsync<
    ExtenderResource,
    UCPBaseParameters<ExtenderResource>
  >;

  listByScope is ArmResourceListByParent<
    ExtenderResource,
    UCPBaseParameters<ExtenderResource>,
    "Scope",
    "Scope"
  >;

  @doc("Lists secrets values for the specified Extender resource")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    ExtenderResource,
    {},
    ExtenderListSecretResponse,
    UCPBaseParameters<ExtenderResource>
  >;
}
