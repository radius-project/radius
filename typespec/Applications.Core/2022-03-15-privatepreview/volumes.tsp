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
model VolumeProperties is RadiusResource<VolumeProperties> {
  @doc("Provisioning state of the Volume at the time the operation was called.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("Represents Azure Key Vault Volume properties")
model AzureKeyVaultVolumeProperties extends VolumeProperties {
  @doc("The Azure Key Vault Volume kind")
  kind: "azure.com.keyvault";

  @doc("The KeyVault certificates that this volume exposes")
  certificates?: Record<CertificateObjectProperties>;

  @doc("The KeyVault keys that this volume exposes")
  keys?: Record<KeyObjectProperties>;

  @doc("The ID of the keyvault to use for this volume resource")
  resource: string;

  @doc("The KeyVault secrets that this volume exposes")
  secrets?: Record<SecretObjectProperties>;
}

@doc("Represents certificate object properties")
model CertificateObjectProperties {
  @doc("File name when written to disk")
  `alias`?: string;

  @doc("Encoding format. Default utf-8")
  encoding?: VolumeSecretEncodings = VolumeSecretEncodings.utf8;

  @doc("Certificate format. Default pem")
  format?: CertificateFormats = CertificateFormats.pem;

  @doc("The name of the certificate")
  name: string;

  @doc("Certificate object type to be downloaded - the certificate itself, private key or public key of the certificate")
  certType?: CertificateTypes;

  @doc("Certificate version")
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
  @doc("File name when written to disk")
  `alias`?: string;

  @doc("Encoding format. Default utf-8")
  encoding?: VolumeSecretEncodings = VolumeSecretEncodings.utf8;

  @doc("The name of the certificate")
  name: string;

  @doc("Certificate version")
  version?: string;
}

@doc("Represents key object properties")
model KeyObjectProperties {
  @doc("File name when written to disk")
  `alias`?: string;

  @doc("The name of the certificate")
  name: string;

  @doc("Certificate version")
  version?: string;
}

@armResourceOperations
interface Volumes {
  get is ArmResourceRead<VolumeResource, UCPBaseParameters<VolumeResource>>;
  create is ArmResourceCreateOrReplaceAsync<
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
