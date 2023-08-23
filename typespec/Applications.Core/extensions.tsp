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
  @doc("The kind of the resource.")
  kind: "kubernetesNamespace";

  @doc("The namespace of the application environment.")
  `namespace`: string;
}

@doc("Kubernetes metadata extension of a environment/application resource.")
model KubernetesMetadataExtension extends Extension {
  @doc("The kind of the resource.")
  kind: "kubernetesMetadata";

  @doc("Annotations to be applied to the Kubernetes resources output by the resource")
  annotations: Record<string>;

  @doc("Labels to be applied to the Kubernetes resources output by the resource")
  labels: Record<string>;
}

@doc("ManualScaling Extension")
model ManualScalingExtension extends Extension {
  @doc("Specifies the extension of the resource")
  kind: "manualScaling";

  @doc("Replica count.")
  replicas: int32;
}

@doc("Specifies the resource should have a Dapr sidecar injected")
model DaprSidecarExtension extends Extension {
  @doc("Specifies the extension of the resource")
  kind: "daprSidecar";

  @doc("The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. ")
  appPort?: int32;

  @doc("The Dapr appId. Specifies the identifier used by Dapr for service invocation.")
  appId: string;

  @doc("Specifies the Dapr configuration to use for the resource.")
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
