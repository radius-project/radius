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

import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@typespec/openapi";

using OpenAPI;
using Azure.Core;
using Azure.ResourceManager;

@doc("Basic properties of a Radius resource.")
model BasicResourceProperties {
  @doc("Status of a resource.")
  @visibility("read")
  status?: ResourceStatus;

  @doc("Fully qualified resource ID for the environment that the portable resource is linked to")
  environment: string;

  @doc("Fully qualified resource ID for the application that the portable resource is consumed by")
  application?: Application;
}

@doc("Basic properties of a Dapr component object.")
model BasicDaprResourceProperties extends BasicResourceProperties {
  @doc("The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.")
  @visibility("read")
  componentName?: string;
}


@doc("IdentitySettings is the external identity setting.")
model IdentitySettings {
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
@doc("Represents backing compute resource")
model EnvironmentCompute {
  @doc("The resource id of the compute resource for application environment.")
  resourceId?: string;

  @doc("Configuration for supported external identity providers")
  identity?: IdentitySettings;
}

@doc("The Kubernetes compute configuration")
model KubernetesCompute extends EnvironmentCompute {
  @doc("The Kubernetes compute kind")
  kind: "kubernetes";

  @doc("The namespace to use for the environment.")
  `namespace`: string;
}

@doc("Status of a resource.")
model ResourceStatus {
  @doc("The compute resource associated with the resource.")
  compute?: EnvironmentCompute;

  @doc("Properties of an output resource")
  @extension("x-ms-identifiers", [])
  outputResources?: OutputResource[];
}

@doc("Properties of an output resource.")
model OutputResource {}

@doc("Fully qualified resource ID for the application that the portable resource is consumed by")
scalar Application extends string;

@doc("Describes a reference to an existing resource")
model ResourceReference {
  @doc("Resource id of an existing resource")
  id: string;
}

@doc("Provisioning state of the portable resource at the time the operation was called")
@lroStatus
enum ProvisioningState {
  ...ResourceProvisioningState,

  @doc("The resource is being provisioned")
  Provisioning,

  @doc("The resource is updating")
  Updating,

  @doc("The resource is being deleted")
  Deleting,

  @doc("The resource create request has been accepted")
  Accepted,
}

@doc("The application name")
@maxLength(63)
@pattern("^[A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9])?$")
scalar ResourceNameString extends string;
