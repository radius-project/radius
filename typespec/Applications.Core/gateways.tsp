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

namespace Applications.Core;

model GatewayResource is TrackedResource<GatewayProperties> {
  @doc("The name of your Gateway.")
  @key("gatewayName")
  @path
  @segment("gateways")
  name: ResourceNameString;
}

@doc("Gateway properties")
model GatewayProperties {
  ...ApplicationScopedResource;

  @doc("Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet).")
  internal?: boolean = false;

  @doc("The hostname information for this gateway.")
  hostname?: GatewayHostname;

  @doc("The routes attached to this gateway.")
  @extension("x-ms-identifiers", [])
  routes: GatewayRoute[];

  @doc("TLS/SSL configuration for this gateway.")
  tls?: GatewayTls;

  @doc("URL of the gateway resource. Readonly")
  @visibility("read")
  url?: string;
}

@doc("Tls Minimum versions for Gateway resource.")
enum TlsMinVersion {
  @doc("TLS Version 1.2")
  tls12: "1.2",

  @doc("TLS Version 1.3")
  tls13: "1.3",
}

@doc("TLS configuration definition for Gateway resource.")
model GatewayTls {
  @doc("Configures the gateway to passthrough encrypted SSL traffic to an HTTP route and container. Requires a single route to be set with no 'path' defined (just destination). With sslPassthrough set to true, the gateway can only support SNI routing. Path based routing cannot be supported. Defaults to 'false'.")
  sslPassthrough?: boolean = false;

  @doc("The minimum TLS protocol to support for TLS termination.")
  minimumProtocolVersion?: TlsMinVersion = TlsMinVersion.tls12;

  @doc("The Radius Secret Store resource ID that holds the TLS certificate data for TLS termination.")
  certificateFrom?: string;
}

@doc("Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io.")
model GatewayHostname {
  @doc("A custom DNS prefix for the generated hostname.")
  prefix?: string;

  @doc("A fully-qualified domain name to use for the gateway.")
  fullyQualifiedHostname?: string;
}

@doc("Route attached to Gateway")
model GatewayRoute {
  @doc("The path to match the incoming request path on. Not required when `tls.sslPassthrough` is set to `'true'`.")
  path?: string;

  @doc("The service to route traffic to, in the form `'[scheme]://[serviceName]:[port]'`.")
  destination?: string;

  @doc("The prefix to replace in the incoming request path that is sent to the destination route.")
  replacePrefix?: string;
}

@armResourceOperations
interface Gateways {
  get is ArmResourceRead<GatewayResource, UCPBaseParameters<GatewayResource>>;

  create is ArmResourceCreateOrReplaceAsync<
    GatewayResource,
    UCPBaseParameters<GatewayResource>
  >;

  createOrUpdate is ArmResourcePatchAsync<
    GatewayResource,
    GatewayProperties,
    UCPBaseParameters<GatewayResource>
  >;

  delete is ArmResourceDeleteAsync<
    GatewayResource,
    UCPBaseParameters<GatewayResource>
  >;

  listByScope is ArmResourceListByParent<
    GatewayResource,
    UCPBaseParameters<GatewayResource>,
    "Scope",
    "Scope"
  >;
}
