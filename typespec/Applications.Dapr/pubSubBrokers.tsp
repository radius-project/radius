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
using OpenAPI;

namespace Applications.Dapr;

@doc("Dapr PubSubBroker portable resource")
model DaprPubSubBrokerResource is TrackedResource<DaprPubSubBrokerProperties> {
  @doc("PubSubBroker name")
  @key("pubSubBrokerName")
  @path
  @segment("pubSubBrokers")
  name: ResourceNameString;
}

@doc("Dapr PubSubBroker portable resource properties")
model DaprPubSubBrokerProperties {
  ...EnvironmentScopedResource;
  ...DaprResourceProperties;

  @doc("A collection of references to resources associated with the pubSubBroker")
  resources?: ResourceReference[];

  ...RecipeBaseProperties;
}

@armResourceOperations
interface PubSubBrokers {
  get is ArmResourceRead<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  update is ArmResourcePatchAsync<
    DaprPubSubBrokerResource,
    DaprPubSubBrokerProperties,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  delete is ArmResourceDeleteAsync<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;

  listByScope is ArmResourceListByParent<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>,
    "Scope",
    "Scope"
  >;
}
