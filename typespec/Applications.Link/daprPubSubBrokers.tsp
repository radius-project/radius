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
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "./common.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using Azure.Core;
using OpenAPI;

@armProviderNamespace
@service({
  title: "Applications.Link Management APIs",
  version: "2022-03-15-privatepreview",
})
@doc("REST APIs for Applications.Link DaprPubSubBroker Resource")
@useDependency(Azure.ResourceManager.Versions.v1_0_Preview_1)
namespace Applications.Link;

@doc("DaprPubSubBroker link properties")
model DaprPubSubBrokerProperties extends BasicDaprResourceProperties {
  @doc("Provisioning state of the daprPubSubBroker resource at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;

  @doc("Metadata for the daprPubSubBroker resource. This should match the values specified in Dapr component spec")
  metadata?: {};

  @doc("The recipe used to automatically deploy underlying infrastructure for the daprPubSubBroker link")
  recipe?: Recipe;

  @doc("A collection of references to resources associated with the daprPubSubBroker")
  resources?: ResourceReference[];

  #suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-duplicate-property"
  @doc("DaprPubSubBroker type. These strings match the format used by Dapr Kubernetes configuration format.")
  type?: string;

  @doc("Dapr component version")
  version?: string;
}

@doc("DaprPubSubBroker link")
model DaprPubSubBrokerResource is TrackedResource<DaprPubSubBrokerProperties> {
  @doc("DaprPubSubBroker name")
  @key("daprPubSubBrokerName")
  @path
  @segment("daprPubSubBrokers")
  name: ResourceNameString;
}

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@armResourceOperations
interface DaprPubSubBroker {
  get is ArmResourceRead<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>
  >;
  create is ArmResourceCreateOrUpdateAsync<
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
  listByRootScope is ArmResourceListByParent<
    DaprPubSubBrokerResource,
    UCPBaseParameters<DaprPubSubBrokerResource>,
    "RootScope",
    "RootScope"
  >;
}
