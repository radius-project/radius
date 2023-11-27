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

using OpenAPI;

@doc("Extension of a environment/application resource.")
@discriminator("kind")
model Extension {}

@doc("Kubernetes namespace extension of a environment/application resource.")
model KubernetesNamespaceExtension extends Extension {
  @doc("The kind of extension being used. Must be ‘kubernetesNamespace’.")
  kind: "kubernetesNamespace";

  @doc("The namespace where all application-scoped resources generate Kubernetes objects.")
  `namespace`: string;
}

@doc("Kubernetes metadata extension of a environment/application resource.")
model KubernetesMetadataExtension extends Extension {
  @doc("The kind of extension being used. Must be 'kubernetesMetadata'.")
  kind: "kubernetesMetadata";

  @doc("The Kubernetes annotations to set on your application and its resources.")
  annotations?: Record<string>;

  @doc("The Kubernetes labels to be set on the application and its resources.")
  labels?: Record<string>;
}

@doc("ManualScaling Extension")
model ManualScalingExtension extends Extension {
  @doc("The kind of extension.")
  kind: "manualScaling";

  @doc("The number of replicas to run.")
  replicas: int32;
}

@doc("Specifies the resource should have a Dapr sidecar injected")
model DaprSidecarExtension extends Extension {
  @doc("The kind of extension.")
  kind: "daprSidecar";

  @doc("The port your service exposes to Dapr.")
  appPort?: int32;

  @doc("The appId of the Dapr sidecar.")
  appId: string;

  @doc("The configuration to use for the Dapr sidecar.")
  config?: string;

  @doc("Specifies the Dapr app-protocol to use for the resource.")
  protocol?: DaprSidecarExtensionProtocol;
}

@doc("The Dapr sidecar extension protocol")
enum DaprSidecarExtensionProtocol {
  @doc("HTTP protocol")
  http,

  @doc("gRPC protocol")
  grpc,
}
