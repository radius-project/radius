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

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "../radius/v1/trackedresource.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Radius.Core;

@doc("""
  The Radius.Core/applications Resource Type represents a Radius Application: a logical grouping of the resources that make up a single application, such as containers, databases, and message queues, along with the connections between them. Radius uses the Application to build the application graph, apply shared configuration, and manage its resources together throughout their lifecycle.
  
  An Application is always deployed to a Radius Environment, which is supplied through the `environment` property. To define an Application, add a `Radius.Core/applications` resource to your application definition Bicep file.
  
  ```
  extension radius
  
  @description('The Radius Environment ID. Injected automatically by the rad CLI.')
  param environment string
  
  resource myApp 'Radius.Core/applications@2025-08-01-preview' = {
    name: 'my-app'
    properties: {
      environment: environment
    }
  }
  ```
  
  Resources are composed into an application by setting their `application` property to the application ID. For example, in order to add a container to this application, add the following to the application definition Bicep file and set `application: myApp.id`:
  
  ```
  resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
    name: 'frontend'
    properties: {
      environment: environment
      application: myApp.id
      containers: {
        ...
      }
    }
  }
  
  For more information, see the Radius documentation at https://docs.radapp.io.
  ```
  """)
model ApplicationResource
  is TrackedResourceRequired<ApplicationProperties, "applications"> {
  @doc("The application name")
  @path
  @key("applicationName")
  @segment("applications")
  @visibility(Lifecycle.Read, Lifecycle.Create)
  name: ResourceNameString;
}

@doc("Application properties")
model ApplicationProperties {
  @doc("The status of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Fully qualified resource ID for the environment that the application is linked to")
  environment: string;

  @doc("Status of a resource.")
  @visibility(Lifecycle.Read)
  status?: ResourceStatus;
}

@doc("Describes the application architecture and its dependencies.")
model ApplicationGraphResponse {
  @doc("The resources in the application graph.")
  @extension("x-ms-identifiers", #["id"])
  resources: Array<ApplicationGraphResource>;

  @doc("Map from iconHash to the verbatim SVG UTF-8 bytes for every icon referenced by any resource in this response. Populated only when the request specifies `includeIcons: true`.")
  icons?: Record<string>;
}

@doc("Request body for the getGraph action.")
model GetGraphRequest {
  @doc("When true, `ApplicationGraphResponse.icons` is populated with the SVG bytes for every distinct `iconHash` referenced by the response's resources. When false or omitted, only per-resource `iconHash` values are returned.")
  includeIcons?: boolean;

  @doc("Optional caller-supplied dependency edges, grouped by source resource ID. Each entry maps a source Radius resource ID to the list of outbound Kind: Dependency edges leaving that source. Every entry MUST have direction: Outbound and kind: Dependency; entries that don't are ignored. The server merges these onto the connection-only graph it computes from stored resources: excluded types are dropped, unknown endpoints are dropped, and any pair already present as Kind: Connection wins over the caller-supplied Dependency. Typically populated by `rad app graph -a <name> <app.bicep>` after compiling the Bicep template locally.")
  dependsOnEdges?: Record<Array<ApplicationGraphConnection>>;
}

@doc("Describes the connection between two resources.")
model ApplicationGraphConnection {
  @doc("The resource ID ")
  id: string;

  @doc("The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and 'Inbound' indicates indicates this connection specifies the ID of the source.")
  direction: Direction;

  @doc("Discriminator identifying the origin of this edge. 'Connection' edges are author-declared entries under properties.connections. 'Dependency' edges are implicit dependencies derived from Bicep's dependsOn list (static graph only in Phase 1). Every emitted edge carries a kind.")
  kind: ConnectionKind;
}

@doc("The direction of a connection.")
enum Direction {
  @doc("The resource defining this connection makes an outbound connection resource specified by this id.")
  Outbound,

  @doc("The resource defining this connection accepts inbound connections from the resource specified by this id.")
  Inbound,
}

@doc("The origin of a connection: 'Connection' for author-declared entries in properties.connections, 'Dependency' for implicit entries derived from Bicep's dependsOn list.")
enum ConnectionKind {
  @doc("The edge was declared by the author in properties.connections.")
  Connection,

  @doc("The edge was inferred from a Bicep dependsOn entry (implicit dependency). Not emitted by the runtime graph in Phase 1 — runtime dependency extraction is Phase 2.")
  Dependency,
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
  @extension("x-ms-identifiers", #["id"])
  outputResources: Array<ApplicationGraphOutputResource>;

  @doc("The connections between resources in the application graph.")
  @extension("x-ms-identifiers", #[])
  connections: Array<ApplicationGraphConnection>;

  @doc("provisioningState of this resource.")
  provisioningState: string;

  @doc("Stable hash over the authorable properties of this resource and its sorted dependsOn list. Used by tooling to classify resources as added, removed, modified, or unchanged across graphs. Format: 'sha256:{hex}'.")
  diffHash?: string;

  @doc("SHA-256 hex of the resource type's icon SVG, matching `iconHash` on the resource-type registry. Null when the type has no icon registered. The bytes themselves are delivered inline via `ApplicationGraphResponse.icons` when the request specifies `includeIcons: true`; otherwise clients fetch bytes by hash from the resource-type icon endpoint.")
  iconHash?: string;

  @doc("Resource-type-specific properties of the resource as returned by its resource provider. The shape of this map varies by `type` (e.g. a `Radius.Compute/containers` resource exposes different keys than `Radius.Datastores/redisCaches` or any user-defined resource type) and matches the `properties` returned by that resource's GET response. Top-level keys already surfaced as first-class fields on this model (`provisioningState`, `connections`) are omitted. The whole `status` object is also omitted because it may contain computed values that include sensitive data (for example, connection strings); the redundant `status.outputResources` list is surfaced via the dedicated `outputResources` field instead. Use `diffHash` rather than a byte-wise compare of this map to detect change.")
  properties?: Record<unknown>;
}

@doc("Describes an output resource that comprises an application graph resource.")
model ApplicationGraphOutputResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;

  @doc("A deep link to the underlying resource in its cloud provider's portal (for example, the Azure portal). Empty when no link is available for the resource.")
  portalUrl?: string;
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
    ApplicationProperties,
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
    GetGraphRequest,
    ApplicationGraphResponse,
    UCPBaseParameters<ApplicationResource>
  >;
}
