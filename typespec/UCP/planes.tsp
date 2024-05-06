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
import "../radius/v1/trackedresource.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The Plane Name parameter.")
model PlaneNameParameter {
  @doc("The name of the plane")
  @path
  @extension("x-ms-skip-url-encoding", true)
  @extension("x-ms-parameter-location", "method")
  planeName: ResourceNameString;
}

@doc("The UCP HTTP request base parameters.")
model PlaneBaseParameters<TResource> {
  ...ApiVersionParameter;
  ...KeysOf<TResource>;
}

@doc("The generic representation of a plane resource")
model GenericPlaneResource
  is TrackedResourceRequired<GenericPlaneResourceProperties, "System.Resources/planes", "planes"> {
  @key("planeType")
  @doc("The plane type.")
  @segment("planes")
  @path
  name: string;
}

#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("The properties of the generic representation of a plane resource.")
model GenericPlaneResourceProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@armResourceOperations
interface Planes {
  @doc("List all planes")
  listPlanes is UcpResourceList<GenericPlaneResource, ApiVersionParameter>;
}
