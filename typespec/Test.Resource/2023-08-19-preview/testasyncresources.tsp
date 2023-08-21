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
using Azure.ResourceManager;
using Azure.Core;
using OpenAPI;
using Autorest;

namespace Test.Resource;

@doc("The TestAsyncResource which implements asynchornous API.")
model TestAsyncResource is TrackedResource<TestAsyncResourceProperties> {
  @doc("The name of TestSyncResource")
  @path
  @key("testAsyncResourceName")
  @segment("testAsyncResources")
  name: ResourceNameString;
}

@doc("TestAsyncResource properties")
model TestAsyncResourceProperties
  is RadiusResource<TestAsyncResourceProperties> {
  @visibility("read")
  @doc("The status of the last operation.")
  provisioningState?: ProvisioningState;

  @doc("The property1 of TestASyncResource")
  property1: string;
}

@armResourceOperations
interface TestAsyncResources {
  @example("./example/TestAsyncResourceGet.json", "Get TestAsyncResource")
  get is ArmResourceRead<
    TestAsyncResource,
    UCPBaseParameters<TestAsyncResource>
  >;

  @example("./example/TestAsyncResourcePut.json", "Put TestAsyncResource")
  create is ArmResourceCreateOrReplaceAsync<
    TestAsyncResource,
    UCPBaseParameters<TestAsyncResource>
  >;

  @example("./example/TestAsyncResourcePatch.json", "Patch TestAsyncResource")
  update is ArmResourcePatchAsync<
    TestAsyncResource,
    TestAsyncResourceProperties,
    UCPBaseParameters<TestAsyncResource>
  >;

  @example("./example/TestAsyncResourceDelete.json", "Delete TestAsyncResource")
  delete is ArmResourceDeleteAsync<
    TestAsyncResource,
    UCPBaseParameters<TestAsyncResource>
  >;

  @example(
    "./example/TestAsyncResourceListByScope.json",
    "List TestAsyncResource by scope"
  )
  listByScope is ArmResourceListByParent<
    TestAsyncResource,
    UCPBaseParameters<TestAsyncResource>,
    "Scope",
    "Scope"
  >;
}
