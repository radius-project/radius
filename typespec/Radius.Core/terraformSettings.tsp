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

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "../radius/v1/trackedresource.tsp";

using Azure.ResourceManager;
using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using OpenAPI;

namespace Radius.Core;

@doc("Terraform settings resource.")
model TerraformSettingsResource
  is TrackedResourceRequired<TerraformSettingsProperties, "terraformSettings"> {
  @doc("Terraform settings resource name.")
  @key("terraformSettingsName")
  @path
  @segment("terraformSettings")
  name: ResourceNameString;
}

@doc("Terraform settings properties.")
model TerraformSettingsProperties {
  @doc("Provisioning state of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Terraform CLI configuration equivalent to the terraformrc file.")
  terraformrc?: TerraformCliConfiguration;

  @doc("Terraform backend configuration.")
  backend?: TerraformBackendConfiguration;

  @doc("Environment variables injected into the Terraform process.")
  env?: Record<string>;

  @doc("Logging configuration applied to Terraform executions.")
  logging?: TerraformLoggingConfiguration;

  @doc("List of environment resource IDs that reference this settings resource.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];
}

@doc("Terraform CLI configuration matching the terraformrc file.")
model TerraformCliConfiguration {
  @doc("Provider installation configuration controlling how Terraform installs providers.")
  providerInstallation?: TerraformProviderInstallationConfiguration;

  @doc("Credentials keyed by registry or module source hostname.")
  credentials?: Record<TerraformCredentialConfiguration>;
}

@doc("Provider installation options for Terraform.")
model TerraformProviderInstallationConfiguration {
  @doc("Network mirror configuration used to download providers.")
  networkMirror?: TerraformNetworkMirrorConfiguration;

  @doc("Direct installation rules controlling when Terraform reaches public registries.")
  direct?: TerraformDirectConfiguration;
}

@doc("Network mirror configuration for Terraform providers.")
model TerraformNetworkMirrorConfiguration {
  @doc("Mirror URL used to download providers.")
  url: string;

  @doc("Provider addresses included in this mirror.")
  include?: string[];

  @doc("Provider addresses excluded from this mirror.")
  exclude?: string[];
}

@doc("Direct installation configuration for Terraform providers.")
model TerraformDirectConfiguration {
  @doc("Provider addresses included when falling back to direct installation.")
  include?: string[];

  @doc("Provider addresses excluded from direct installation.")
  exclude?: string[];
}

@doc("Reference to a secret stored in Radius.Security/secrets.")
model SecretReference {
  @doc("Resource ID of the Radius.Security/secrets entry.")
  secretId: string;

  @doc("Key within the secret to retrieve.")
  key: string;
}

@doc("Credential configuration for Terraform provider or module sources.")
model TerraformCredentialConfiguration {
  @doc("Token credential for Terraform Cloud/Enterprise authentication.")
  token?: SecretReference;
}

@doc("Terraform backend configuration matching the terraform block.")
model TerraformBackendConfiguration {
  @doc("Backend type (for example 'kubernetes').")
  type: string;

  @doc("Backend-specific configuration values.")
  config?: Record<string>;
}

@doc("Logging options for Terraform executions.")
model TerraformLoggingConfiguration {
  @doc("Terraform log verbosity (maps to TF_LOG).")
  level?: TerraformLogLevel;

  @doc("Destination file path for Terraform logs (maps to TF_LOG_PATH).")
  path?: string;
}

@doc("Terraform log verbosity levels.")
enum TerraformLogLevel {
  trace: "TRACE",
  debug: "DEBUG",
  info: "INFO",
  warn: "WARN",
  error: "ERROR",
  fatal: "FATAL",
}

@armResourceOperations
interface TerraformSettings {
  get is ArmResourceRead<
    TerraformSettingsResource,
    UCPBaseParameters<TerraformSettingsResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    TerraformSettingsResource,
    UCPBaseParameters<TerraformSettingsResource>
  >;

  update is ArmResourcePatchSync<
    TerraformSettingsResource,
    TerraformSettingsProperties,
    UCPBaseParameters<TerraformSettingsResource>
  >;

  delete is ArmResourceDeleteSync<
    TerraformSettingsResource,
    UCPBaseParameters<TerraformSettingsResource>
  >;

  listByScope is ArmResourceListByParent<
    TerraformSettingsResource,
    UCPBaseParameters<TerraformSettingsResource>,
    "Scope",
    "Scope"
  >;
}
