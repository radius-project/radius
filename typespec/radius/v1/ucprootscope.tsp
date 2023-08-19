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

import "@typespec/openapi";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

using OpenAPI;
using TypeSpec.Http;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;

namespace Radius.V1;

@doc("The default rootScope parameter type.")
model RootScopeParameter {
  @path
  @minLength(1)
  @maxLength(90)
  @pattern("^[-\\w\\._\\(\\)]+$")
  @extension("x-ms-skip-url-encoding", true)
  @doc("The scope in which the resource is present. UCP Scope is /planes/{planeType}/{planeName}/resourceGroup/{resourcegroupID} and Azure resource scope is /subscriptions/{subscriptionID}/resourceGroup/{resourcegroupID}")
  rootScope: string;
}

@doc("The UCP HTTP request base parameters.")
model UCPBaseParameters<TResource extends ArmResource> {
  ...ApiVersionParameter;
  ...RootScopeParameter;
}