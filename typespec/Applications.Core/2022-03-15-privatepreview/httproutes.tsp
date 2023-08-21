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

namespace Applications.Core;

@doc("Radius HTTPRoute Resource.")
model HttpRouteResource is TrackedResource<HttpRouteProperties> {
  @doc("HTTPRoute name")
  @key("httpRouteName")
  @path
  @segment("httpRoutes")
  name: ResourceNameString;
}

@doc("HTTPRoute properties")
model HttpRouteProperties is RadiusResource<HttpRouteProperties> {
  @doc("Provisioning state of the HTTPRoute at the time the operation was called.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The internal hostname accepting traffic for the HTTP Route. Readonly.")
  hostname?: string;

  @doc("The port number for the HTTP Route. Defaults to 80. Readonly.")
  port?: int32;

  @doc("The scheme used for traffic. Readonly.")
  @visibility("read")
  scheme?: string;

  @doc("A stable URL that that can be used to route traffic to a resource. Readonly.")
  @visibility("read")
  url?: string;
}

@armResourceOperations
interface HttpRoutes {
  @example("./examples/HttpRoutesGet.json", "RadiusHttpRoutesGet")
  get is ArmResourceRead<
    HttpRouteResource,
    UCPBaseParameters<HttpRouteResource>
  >;

  @example("./examples/HttpRoutesPut.json", "RadiusHttpRoutesPut")
  create is ArmResourceCreateOrReplaceAsync<
    HttpRouteResource,
    UCPBaseParameters<HttpRouteResource>
  >;

  @example("./examples/HttpRoutesPatch.json", "RadiusHttpRoutesPatch")
  update is ArmResourcePatchAsync<
    HttpRouteResource,
    HttpRouteProperties,
    UCPBaseParameters<HttpRouteResource>
  >;

  @example("./examples/HttpRoutesDelete.json", "RadiusHttpRoutesDelete")
  delete is ArmResourceDeleteAsync<
    HttpRouteResource,
    UCPBaseParameters<HttpRouteResource>
  >;

  @example(
    "./examples/HttpRoutesListByScope.json",
    "RadiusHttpRoutesListByScope"
  )
  listByScope is ArmResourceListByParent<
    HttpRouteResource,
    UCPBaseParameters<HttpRouteResource>,
    "Scope",
    "Scope"
  >;
}
