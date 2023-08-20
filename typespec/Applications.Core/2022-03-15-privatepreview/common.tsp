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

@doc("IdentitySetting is the external identity setting.")
model IdentitySetting {
  @doc("kind of identity setting")
  kind: IdentitySettingKind;

  @doc("The URI for your compute platform's OIDC issuer")
  oidcIssuer?: string;

  @doc("The resource ID of the provisioned identity")
  resource?: string;
}

@doc("IdentitySettingKind is the kind of supported external identity setting")
enum IdentitySettingKind {
  @doc("undefined identity")
  undefined: "undefined",

  @doc("azure ad workload identity")
  `azure.com.workload`: "azure.com.workload",
}

@discriminator("kind")
@doc("Compute resource used by application environment resource.")
model EnvironmentCompute {
  @doc("The resource id of the compute resource for application environment.")
  resourceId?: string;

  @doc("Configuration for supported external identity providers")
  identity?: IdentitySetting;
}

@doc("The Kubernetes compute configuration")
model KubernetesCompute extends EnvironmentCompute {
  @doc("The Kubernetes compute kind")
  kind: "kubernetes";

  @doc("The namespace to use for the environment.")
  `namespace`: string;
}
