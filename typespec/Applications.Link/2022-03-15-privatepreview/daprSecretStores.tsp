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

import "../../radius/v1/ucprootscope.tsp";
import "../../radius/v1/resources.tsp";
import "./common.tsp";

using OpenAPI;
using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;

namespace Applications.Link;

@doc("DaprSecretStore link properties")
model DaprSecretStoreProperties extends BasicDaprResourceProperties {
  @doc("Provisioning state of the dapr secret store link at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Dapr Secret Store type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/")
  @friendlyName("type")
  secretStoreType?: string;

  @doc("Dapr component version")
  version?: string;

  @doc("Metadata for the Secret Store resource. This should match the values specified in Dapr component spec")
  metadata?: {};

  @doc("The recipe used to automatically deploy underlying infrastructure for the daprSecretStore link")
  recipe?: Recipe;

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;
}

@doc("DaprSecretStore link")
model DaprSecretStoreResource is TrackedResource<DaprSecretStoreProperties> {
  @doc("DaprSecretStore name")
  @key("daprSecretStoreName")
  @path
  @segment("daprSecretStores")
  name: ResourceNameString;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface DaprSecretStore {
  get is ArmResourceRead<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;
  create is ArmResourceCreateOrReplaceAsync<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;
  update is ArmResourcePatchAsync<
    DaprSecretStoreResource,
    DaprSecretStoreProperties,
    UCPBaseParameters<DaprSecretStoreResource>
  >;
  delete is ArmResourceDeleteAsync<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>
  >;

  listByRootScope is ArmResourceListByParent<
    DaprSecretStoreResource,
    UCPBaseParameters<DaprSecretStoreResource>,
    "RootScope",
    "RootScope"
  >;
}
