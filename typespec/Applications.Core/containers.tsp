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

model ContainerResource is TrackedResource<ContainerProperties> {
  @doc("The name of your resource. See [common values]({{< ref "resource-schema.md#common-values" >}}) for more information.")
  @path
  @key("containerName")
  @segment("containers")
  name: ResourceNameString;
}

@doc("Properties of the resource.")
model ContainerProperties {
  ...ApplicationScopedResource;

  @doc("Container configuration.")
  container: Container;

  @doc("List of connections to other resources.")
  connections?: Record<ConnectionProperties>;

  @doc("Configuration for supported external identity providers")
  identity?: IdentitySettings;

  @doc("List of extensions on the container.")
  @extension("x-ms-identifiers", [])
  extensions?: Extension[];

  @doc("Specifies how the underlying container resource is provisioned and managed.")
  resourceProvisioning?: ContainerResourceProvisioning;

  @doc("A collection of references to resources associated with the container")
  resources?: ResourceReference[];

  @doc("The restart policy for the underlying container")
  restartPolicy?: RestartPolicy;

  @doc("Runtime specific configurations for the container.")
  runtimes?: RuntimesProperties;
}

@doc("Specifies how the underlying service/resource is provisioned and managed. Available values are 'internal', where Radius manages the lifecycle of the resource internally, and 'manual', where a user manages the resource.")
enum ContainerResourceProvisioning {
  @doc("The resource lifecycle will be managed internally by Radius")
  internal,

  @doc("The resource lifecycle will be managed by the user")
  manual,
}

@doc("Restart policy for the container")
enum RestartPolicy {
  @doc("Always")
  Always,

  @doc("OnFailure")
  OnFailure,

  @doc("Never")
  Never,
}

@doc("Runtime specific configurations for the container.")
model RuntimesProperties {
  @doc("Kubernetes specific configuration for the container.")
  kubernetes?: KubernetesRuntimeProperties;
}

#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("A strategic merge patch that will be applied to the PodSpec object when this container is being deployed.")
model KubernetesPodSpec is Record<unknown>;

@doc("Kubernetes specific configuration for the container.")
model KubernetesRuntimeProperties {
  @doc("The base Kubernetes resource manifest on top of which Radius specified properties will be applied. Supported resource types are documented [here]({{<ref "guides/author-apps/containers/overview#base-kubernetes-yaml">}}).")
  base?: string;

  #suppress "@azure-tools/typespec-azure-core/bad-record-type"
  @doc("The pod specifications to apply to the Kubernetes resource created by Radius. Any field defined on PodSpec can be set here.")
  pod?: KubernetesPodSpec;
}

@doc("Specifies a listening port for the container")
model ContainerPortProperties {
  @doc("The port the container exposes.")
  containerPort: int32;

  @doc("The protocol the container exposes. Options are 'TCP' and 'UCP'.")
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

  @doc("Threshold number of times the probe fails after which a failure would be reported.")
  failureThreshold?: float32;

  @doc("Interval for the readiness/liveness probe in seconds")
  periodSeconds?: float32;

  @doc("Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds")
  timeoutSeconds?: float32 = 5.0;
}

@doc("Specifies the properties for readiness/liveness probe using HTTP Get")
model HttpGetHealthProbeProperties extends HealthProbeProperties {
  @doc("Type of readiness check, `httpGet` or `tcp` or `exec`.")
  kind: "httpGet";

  @doc("Used when kind is `httpGet` or `tcp`. The listening port number.")
  containerPort: int32;

  @doc("Used when kind is `httpGet`. The route to make the HTTP request on")
  path: string;

  @doc("Custom HTTP headers to add to the get request")
  headers?: Record<string>;
}

@doc("Specifies the properties for readiness/liveness probe using TCP")
model TcpHealthProbeProperties extends HealthProbeProperties {
  @doc("Type of readiness check, `httpGet` or `tcp` or `exec`.")
  kind: "tcp";

  @doc("Used when kind is `httpGet` or `tcp`. The listening port number.")
  containerPort: int32;
}

@doc("Specifies the properties for readiness/liveness probe using an executable")
model ExecHealthProbeProperties extends HealthProbeProperties {
  @doc("Type of readiness check, `httpGet` or `tcp` or `exec`.")
  kind: "exec";

  @doc("Used when kind is exec. Command to execute to probe readiness/liveness")
  command: string;
}

@discriminator("kind")
@doc("Specifies a volume for a container")
model Volume {
  @doc("The container path to mount the volume to.")
  mountPath?: string;
}

@doc("Specifies an ephemeral volume for a container")
model EphemeralVolume extends Volume {
  @doc("The type of volume, either ephemeral or persistent.")
  kind: "ephemeral";

  @doc("The backing storage medium to use when kind is 'ephemeral'. Either disk or memory.")
  managedStore: ManagedStore;
}

@doc("Specifies a persistent volume for a container")
model PersistentVolume extends Volume {
  @doc("The type of volume, either ephemeral or persistent.")
  kind: "persistent";

  @doc("Container read/write access to the volume")
  permission?: VolumePermission;

  @doc("A volume resource to mount when kind is 'persistent'.")
  source: string;
}

@doc("IAM properties")
model IamProperties {
  @doc("Type of IAM role. Only azure supported today")
  kind: IAMKind;

  @doc("The list IAM roles to set on the target resource.")
  roles?: string[];
}

@doc("Connection Properties")
model ConnectionProperties {
  @doc("The id of the resource the container is connecting to. For network connections to other services this is in the form '[scheme]://[serviceName]:[port]'")
  source: string;

  @doc("default environment variable override")
  disableDefaultEnvVars?: boolean = false;

  @doc("Identity and access management (IAM) roles to set on the target resource.")
  iam?: IamProperties;
}

@doc("Definition of a container")
model Container {
  @doc("The registry and image to download and run in your container. Follows the format <registry-hostname>:<port>/<image-name>:<tag> where registry hostname is optional and defaults to the Docker public registry, port is optional and defaults to 443, tag is optional and defaults to latest.")
  image: string;

  @doc("How to pull images. Defaults to the runtime’s default behavior. For Kubernetes behavior refer to https://kubernetes.io/docs/concepts/containers/images/#required-image-pull")
  imagePullPolicy?: ImagePullPolicy;

  @doc("A list of environment variables to be set for the container.")
  env?: Record<string>;

  @doc("Ports the container provides")
  ports?: Record<ContainerPortProperties>;

  @doc("Readiness probe configuration.")
  readinessProbe?: HealthProbeProperties;

  @doc("Liveness probe configuration.")
  livenessProbe?: HealthProbeProperties;

  @doc("Volumes to mount into the container.")
  volumes?: Record<Volume>;

  @doc("Entrypoint array. Overrides the container image’s ENTRYPOINT.")
  command?: string[];

  @doc("Arguments to the entrypoint. Overrides the container image’s CMD.")
  args?: string[];

  @doc("Working directory for the container.")
  workingDir?: string;
}

@doc("The image pull policy for the container")
enum ImagePullPolicy {
  @doc("Always")
  Always,

  @doc("IfNotPresent")
  IfNotPresent,

  @doc("Never")
  Never,
}

@doc("The protocol the container exposes. Options are ‘TCP’ and ‘UCP’.")
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
  get is ArmResourceRead<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  update is ArmResourcePatchAsync<
    ContainerResource,
    ContainerProperties,
    UCPBaseParameters<ContainerResource>
  >;

  delete is ArmResourceDeleteAsync<
    ContainerResource,
    UCPBaseParameters<ContainerResource>
  >;

  listByScope is ArmResourceListByParent<
    ContainerResource,
    UCPBaseParameters<ContainerResource>,
    "Scope",
    "Scope"
  >;
}
