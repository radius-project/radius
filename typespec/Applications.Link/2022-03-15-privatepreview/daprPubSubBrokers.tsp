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
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Link;

@doc("DaprPubSubBroker link")
model DaprPubSubBrokerResource is TrackedResource<DaprPubSubBrokerProperties> {
  @doc("DaprPubSubBroker name")
  @key("daprPubSubBrokerName")
  @path
  @segment("daprPubSubBrokers")
  name: ResourceNameString;
}

@doc("DaprPubSubBroker link properties")
model DaprPubSubBrokerProperties
  is DaprResourceProperties<DaprPubSubBrokerProperties> {
  @doc("Provisioning state of the daprPubSubBroker resource at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("A collection of references to resources associated with the daprPubSubBroker")
  resources?: ResourceReference[];

  ...RecipeBaseProperties;
}

@armResourceOperations
interface DaprPubSubBroker {
  @example("./examples/DaprPubSubBrokersGet.json", "DaprPubSubBrokersGet")
  get is ArmResourceRead<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  @extension(
    "x-ms-examples",
    {
      DaprPubSubBrokerPut: {
        $ref: "./examples/DaprPubSubBrokersPut.json",
      },
      DaprPubSubBrokerPutWithRecipe: {
        $ref: "./examples/DaprPubSubBrokersPutWithRecipe.json",
      },
    }
  )
  create is ArmResourceCreateOrReplaceAsync<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  @example("./examples/DaprPubSubBrokersPatch.json", "DaprPubSubBrokersPatch")
  update is ArmResourcePatchAsync<
    DaprPubSubBrokerResource,
    DaprPubSubBrokerProperties,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  @example("./examples/DaprPubSubBrokersDelete.json", "DaprPubSubBrokersDelete")
  delete is ArmResourceDeleteAsync<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  @example(
    "./examples/DaprPubSubBrokersListByRootScope.json",
    "DaprPubSubBrokersListByRootScope"
  )
  listByScope is ArmResourceListByParent<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>,
    "Scope",
    "Scope"
  >;
}
