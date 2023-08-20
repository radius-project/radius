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
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

import "../../radius/v1/ucprootscope.tsp";
import "../../radius/v1/resources.tsp";
import "./common.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using Azure.Core;
using OpenAPI;

namespace Applications.Core;

model SecretStoreResource is TrackedResource<SecretStoreProperties> {
  @doc("SecretStore name")
  @key("secretStoreName")
  @path
  @segment("secretStores")
  name: ResourceNameString;
}

@doc("SecretStore properties")
model SecretStoreProperties extends BasicResourceProperties {
  @doc("Provisioning state of the Gateway at the time the operation was called.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The type of secret store data")
  @friendlyName("type")
  secretType?: SecretStoreDataType = SecretStoreDataType.generic;

  @doc("An object to represent key-value type secrets")
  data: Record<SecretValueProperties>;

  @doc("The resource id of external secret store.")
  resource?: string;
}

@doc("SecretStore data type")
enum SecretStoreDataType {
  @doc("Generic secret data type")
  generic,

  @doc("Certificate secret data type")
  certificate,
}

@doc("SecretValue Encoding type")
enum SecretValueEncoding {
  @doc("The raw secret value")
  raw,

  @doc("The base64-encoded secret value")
  base64,
}

@doc("The Secret value source properties")
model ValueFromProperties {
  @doc("The name of the referenced secret.")
  name: string;

  @doc("The version of the referenced secret.")
  version?: string;
}

@doc("SecretValue properties")
model SecretValueProperties {
  @doc("The encoding of value")
  encoding?: SecretValueEncoding = SecretValueEncoding.raw;

  @doc("The value of secret.")
  @secret
  value: string;

  @doc("The referenced secret in properties.resource")
  valueFrom?: ValueFromProperties;
}

@doc("The list of secrets")
model SecretStoreListSecretsResult {
  @doc("The type of secret store data")
  type: SecretStoreDataType;

  @doc("An object to represent key-value type secrets")
  data: Record<SecretValueProperties>;
}

@armResourceOperations
interface SecretStores {
  get is ArmResourceRead<
    SecretStoreResource,
    UCPBaseParameters<SecretStoreResource>
  >;
  create is ArmResourceCreateOrUpdateAsync<
    SecretStoreResource,
    UCPBaseParameters<SecretStoreResource>
  >;
  update is ArmResourcePatchAsync<
    SecretStoreResource,
    SecretStoreProperties,
    UCPBaseParameters<SecretStoreResource>
  >;
  delete is ArmResourceDeleteAsync<
    SecretStoreResource,
    UCPBaseParameters<SecretStoreResource>
  >;

  listByRootScope is ArmResourceListByParent<
    SecretStoreResource,
    UCPBaseParameters<SecretStoreResource>,
    "RootScope",
    "RootScope"
  >;

  @doc("List the secrets of a secret stores.")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    SecretStoreResource,
    {},
    SecretStoreListSecretsResult,
    UCPBaseParameters<SecretStoreResource>
  >;
}
