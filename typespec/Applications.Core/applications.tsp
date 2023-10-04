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
import "./extensions.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Core;

@doc("Radius Application resource")
model ApplicationResource is TrackedResource<ApplicationProperties> {
  @doc("The application name")
  @path
  @key("applicationName")
  @segment("applications")
  name: ResourceNameString;
}

@doc("Application properties")
model ApplicationProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Fully qualified resource ID for the environment that the portable resource is linked to")
  environment: string;

  @doc("The application extension.")
  @extension("x-ms-identifiers", [])
  extensions?: Array<Extension>;

  @doc("Status of a resource.")
  @visibility("read")
  status?: ResourceStatus;
}

@doc("Describes the application architecture and its dependencies.")
model ApplicationGraphResponse {
  @doc("The resources in the application graph.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphResource>;
}

@doc("Describes the connection between two resources.")
model ApplicationGraphConnection {
  @doc("The resource ID ")
  id: string;

  @doc("The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and 'Inbound' indicates indicates this connection specifies the ID of the source.")
  direction: Direction;
}

@doc("The direction of a connection.")
enum Direction {
  @doc("The resource defining this connection makes an outbound connection resource specified by this id.")
  Outbound,

  @doc("The resource defining this connection accepts inbound connections from the resource specified by this id.")
  Inbound,
}

@doc("Describes a resource in the application graph.")
model ApplicationGraphResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;

  @doc("The resources that comprise this resource.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphOutputResource>;

  @doc("The connections between resources in the application graph.")
  @extension("x-ms-identifiers",[])
  connections: Array<ApplicationGraphConnection>;
}

@doc("Describes an output resource that comprises an application graph resource.")
model ApplicationGraphOutputResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface Applications {
  get is ArmResourceRead<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;

  update is ArmResourcePatchSync<
    ApplicationResource,
    EnvironmentProperties,
    UCPBaseParameters<ApplicationResource>
  >;

  delete is ArmResourceDeleteSync<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;

  listByScope is ArmResourceListByParent<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>,
    "Scope",
    "Scope"
  >;

  @doc("Gets the application graph and resources.")
  @action("getGraph")
  getGraph is ArmResourceActionSync<
    ApplicationResource,
    {},
    ApplicationGraphResponse,
    UCPBaseParameters<ApplicationResource>
  >;
}
