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

@doc("The base properties of a Dapr component object.")
model DaprResourceProperties {
  @doc("The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.")
  @visibility("read")
  componentName?: string;

  @doc("The metadata for Dapr resource which must match the values specified in Dapr component spec")
  metadata?: Record<MetadataValue>;

  #suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-duplicate-property"
  @doc("Dapr component type which must matches the format used by Dapr Kubernetes configuration format")
  type?: string;

  @doc("Dapr component version")
  version?: string;

  @doc("The name of the Dapr component to be used as a secret store")
  auth?: DaprResourceAuth;
}

@doc("Authentication properties for a Dapr component object")
model DaprResourceAuth {
  @doc("Secret store to fetch secrets from")
  secretStore?: string;
}

@doc("A single metadata for a Dapr component object")
model MetadataValue {
  @doc("The plain text value of the metadata")
  value?: string;

  @doc("A reference of a value in a secret store component")
  secretKeyRef?: MetadataValueFromSecret;
}

@doc("A reference of a value in a secret store component.")
model MetadataValueFromSecret {
  @doc("Secret name in the secret store component")
  name: string;

  @doc("The field to select in the secret value. If the secret value is a string, it should be equal to the secret name")
  key: string;
}
