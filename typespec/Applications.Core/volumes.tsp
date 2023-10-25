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

@doc("Radius Volume resource.")
model VolumeResource is TrackedResource<VolumeProperties> {
  @doc("Volume name")
  @key("volumeName")
  @path
  @segment("volumes")
  name: ResourceNameString;
}

@doc("Volume properties")
@discriminator("kind")
model VolumeProperties {
  ...ApplicationScopedResource;
}

@doc("Represents Azure Key Vault Volume properties")
model AzureKeyVaultVolumeProperties extends VolumeProperties {
  @doc("The kind of persistent volume. Should be 'azure.com.keyvault' for Azure Key Vault persistent volumes")
  kind: "azure.com.keyvault";

  @doc("Map specify certificate object name and [certificate properties]. See [certificate properties](#certificate)")
  certificates?: Record<CertificateObjectProperties>;

  @doc("Map specify key object name and key properties. See [key properties](#keys)")
  keys?: Record<KeyObjectProperties>;

  @doc("Resource ID for the Azure KeyVault resource.")
  resource: string;

  @doc("Map specify secret object name and secret properties. See [secret properties](#secrets)")
  secrets?: Record<SecretObjectProperties>;
}

@doc("Represents certificate object properties")
model CertificateObjectProperties {
  @doc("File name when written to disk. Same as object name if not specified")
  `alias`?: string;

  @doc("Encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8' and this field can be specificed only when value is 'privatekey'")
  encoding?: VolumeSecretEncodings = VolumeSecretEncodings.utf8;

  @doc("Certificate format 'pfx', 'pem'. Default is 'pfx'")
  format?: CertificateFormats = CertificateFormats.pem;

  @doc("Certificate name in Azure Key Vault")
  name: string;

  @doc("Certificate object type to be downloaded - the certificate itself, private key or public key of the certificate")
  certType?: CertificateTypes;

  @doc("Specific certificate version. Default is latest")
  version?: string;
}

@doc("Represents certificate formats")
enum CertificateFormats {
  @doc("PEM Certificate format")
  pem: "pem",

  @doc("PFX Certificate format")
  pfx: "pfx",
}

@doc("Represents secret encodings")
enum VolumeSecretEncodings {
  @doc("UTF-8 encoding")
  utf8: "utf-8",

  @doc("Hex encoding")
  hex: "hex",

  @doc("Base64 encoding")
  base64: "base64",
}

@doc("Represents certificate types")
enum CertificateTypes {
  @doc("Certificate type")
  certificate: "certificate",

  @doc("Private Key type")
  privatekey: "privatekey",

  @doc("Public Key type")
  publickey: "publickey",
}

@doc("Represents secret object properties")
model SecretObjectProperties {
  @doc("File name when written to disk. Same as object name if not specified")
  `alias`?: string;

  @doc("Encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8'")
  encoding?: VolumeSecretEncodings = VolumeSecretEncodings.utf8;

  @doc("Secret name in Azure Key Vault")
  name: string;

  @doc("Specific secret version. Default is latest")
  version?: string;
}

@doc("Represents key object properties")
model KeyObjectProperties {
  @doc("File name when written to disk. Same as object name if not specified")
  `alias`?: string;

  @doc("Key name in Azure Key Vault")
  name: string;

  @doc("Specific key version. Default is latest")
  version?: string;
}

@armResourceOperations
interface Volumes {
  get is ArmResourceRead<VolumeResource, UCPBaseParameters<VolumeResource>>;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    VolumeResource,
    UCPBaseParameters<VolumeResource>
  >;

  update is ArmResourcePatchAsync<
    VolumeResource,
    VolumeProperties,
    UCPBaseParameters<VolumeResource>
  >;

  delete is ArmResourceDeleteAsync<
    VolumeResource,
    UCPBaseParameters<VolumeResource>
  >;

  listByScope is ArmResourceListByParent<
    VolumeResource,
    UCPBaseParameters<VolumeResource>,
    "Scope",
    "Scope"
  >;
}
