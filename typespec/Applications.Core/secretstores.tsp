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

model SecretStoreResource is TrackedResource<SecretStoreProperties> {
  @doc("The name of your Secret Store.")
  @key("secretStoreName")
  @path
  @segment("secretStores")
  name: ResourceNameString;
}

@doc("The properties of SecretStore")
model SecretStoreProperties {
  ...ApplicationScopedResource;

  #suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-duplicate-property"
  @doc("The type of secret in your resource.")
  type?: SecretStoreDataType = SecretStoreDataType.generic;

  @doc("An object to represent key-value type secrets.")
  data: Record<SecretValueProperties>;

  @doc("Reference to the backing secret store resource, required only if valueFrom specifies referenced secret name.")
  resource?: string;
}

@doc("The type of SecretStore data")
enum SecretStoreDataType {
  @doc("Generic secret data type")
  generic,

  @doc("Certificate secret data type")
  certificate,
}

@doc("The type of SecretValue Encoding")
enum SecretValueEncoding {
  @doc("The raw secret value")
  raw,

  @doc("The base64-encoded secret value")
  base64,
}

@doc("The Secret value source properties")
model ValueFromProperties {
  @doc("The name of the secret or key of `properties.resource`.")
  name: string;

  @doc("The version of the secret.")
  version?: string;
}

@doc("The properties of SecretValue")
model SecretValueProperties {
  @doc("The encoding type of the data value (default is `'raw'`).")
  encoding?: SecretValueEncoding = SecretValueEncoding.raw;

  @doc("The value of the secret key.")
  @secret
  value?: string;

  @doc("A reference to an external secret. This field is currently not in use, as it is meant for supporting more types of external secrets in the future.")
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

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
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

  listByScope is ArmResourceListByParent<
    SecretStoreResource,
    UCPBaseParameters<SecretStoreResource>,
    "Scope",
    "Scope"
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
