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
import "./extensions.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Core;

model ContainerResource is TrackedResource<ContainerProperties> {
  @doc("Container name")
  @path
  @key("containerName")
  @segment("containers")
  name: ResourceNameString;
}

@doc("Container properties")
model ContainerProperties is RadiusResource<ContainerProperties> {
  @doc("The status of the last operation.")
  provisioningState?: ProvisioningState;

  @doc("Definition of a container.")
  container: Container;

  @doc("Specifies a connection to another resource.")
  connections?: Record<ConnectionProperties>;

  @doc("Configuration for supported external identity providers")
  identity?: IdentitySettings;

  @doc("Extensions spec of the resource")
  @extension("x-ms-identifiers", [])
  extensions?: Extension[];
}

@doc("Specifies a listening port for the container")
model ContainerPortProperties {
  @doc("The listening port number")
  containerPort: int32;

  @doc("Protocol in use by the port")
  protocol?: PortProtocol;

  @doc("Specifies a route provided by this port")
  provides?: string;

  @doc("Specifies the URL scheme of the communication protocol. Consumers can use the scheme to construct a URL. The value defaults to 'http' or 'https' depending on the port value")
  scheme?: string;

  @doc("Specifies the port that will be exposed by this container. Must be set when value different from containerPort is desired")
  port?: int32;
}

@doc("Properties for readiness/liveness probe")
@discriminator("kind")
model HealthProbeProperties {
  @doc("Initial delay in seconds before probing for readiness/liveness")
  initialDelaySeconds?: float32;

  @doc("Threshold number of times the probe fails after which a failure would be reported")
  failureThreshold?: float32;

  @doc("Interval for the readiness/liveness probe in seconds")
  periodSeconds?: float32;

  @doc("Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds")
  timeoutSeconds?: float32 = 5.0;
}

@doc("Specifies the properties for readiness/liveness probe using HTTP Get")
model HttpGetHealthProbeProperties extends HealthProbeProperties {
  @doc("The HealthProbeProperties kind")
  kind: "httpGet";

  @doc("The listening port number")
  containerPort: int32;

  @doc("The route to make the HTTP request on")
  path: string;

  @doc("Custom HTTP headers to add to the get request")
  headers?: Record<string>;
}

@doc("Specifies the properties for readiness/liveness probe using TCP")
model TcpHealthProbeProperties extends HealthProbeProperties {
  @doc("The HealthProbeProperties kind")
  kind: "tcp";

  @doc("The listening port number")
  containerPort: int32;
}

@doc("Specifies the properties for readiness/liveness probe using an executable")
model ExecHealthProbeProperties extends HealthProbeProperties {
  @doc("The HealthProbeProperties kind")
  kind: "exec";

  @doc("Command to execute to probe readiness/liveness")
  command: string;
}

@discriminator("kind")
@doc("Specifies a volume for a container")
model Volume {
  @doc("The path where the volume is mounted")
  mountPath: string;
}

@doc("Specifies an ephemeral volume for a container")
model EphemeralVolume extends Volume {
  @doc("The Volume kind")
  kind: "ephemeral";

  @doc("Backing store for the ephemeral volume")
  managedStore: ManagedStore;
}

@doc("Specifies a persistent volume for a container")
model PersistentVolume extends Volume {
  @doc("The Volume kind")
  kind: "persistent";

  @doc("Container read/write access to the volume")
  permission?: VolumePermission;

  @doc("The source of the volume")
  source: string;
}

@doc("IAM properties")
model IamProperties {
  @doc("The kind of IAM provider to configure")
  kind: IAMKind;

  @doc("RBAC permissions to be assigned on the source resource")
  roles?: string[];
}

@doc("Connection Properties")
model ConnectionProperties {
  @doc("The source of the connection")
  source: string;

  @doc("default environment variable override")
  disableDefaultEnvVars?: boolean = false;

  @doc("iam properties")
  iam?: IamProperties;
}

@doc("Definition of a container")
model Container {
  @doc("The registry and image to download and run in your container")
  image: string;

  @doc("environment")
  env?: Record<string>;

  @doc("container ports")
  ports?: Record<ContainerPortProperties>;

  @doc("readiness probe properties")
  readinessProbe?: HealthProbeProperties;

  @doc("liveness probe properties")
  livenessProbe?: HealthProbeProperties;

  @doc("container volumes")
  volumes?: Record<Volume>;

  @doc("Entrypoint array. Overrides the container image's ENTRYPOINT")
  command?: string[];

  @doc("Arguments to the entrypoint. Overrides the container image's CMD")
  args?: string[];

  @doc("Working directory for the container")
  workingDir: string;
}

@doc("The protocol in use by the port")
enum PortProtocol {
  @doc("TCP protocol")
  TCP,

  @doc("UDP protocol")
  UDP,
}

@doc("The managed store for the ephemeral volume")
enum ManagedStore {
  @doc("Memory store")
  memory,

  @doc("Disk store")
  disk,
}

@doc("The persistent volume permission")
enum VolumePermission {
  @doc("Read only")
  read,

  @doc("Read and write")
  write,
}

@doc("The kind of IAM provider to configure")
enum IAMKind {
  @doc("Azure Active Directory")
  azure,
}

@armResourceOperations
interface Containers {
  @example("./examples/ContainersGet.json", "ContainersGet")
  get is ArmResourceRead<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  @example("./examples/ContainersPut.json", "ContainersPut")
  create is ArmResourceCreateOrReplaceAsync<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  @example("./examples/ContainersPatch.json", "ContainersPatch")
  update is ArmResourcePatchAsync<
    ContainerResource,
    ContainerProperties,
    UCPBaseParameters<ContainerResource>
  >;

  @example("./examples/ContainersDelete.json", "ContainersDelete")
  delete is ArmResourceDeleteAsync<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  @example("./examples/ContainersListByScope.json", "ContainersListByScope")
  listByScope is ArmResourceListByParent<
    ContainerResource,
    UCPBaseParameters<ContainerResource>,
    "Scope",
    "Scope"
  >;
}
