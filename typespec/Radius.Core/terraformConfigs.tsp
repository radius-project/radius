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
using TypeSpec.Versioning;
using TypeSpec.Http;
using TypeSpec.Rest;
using Autorest;
using Azure.Core;
using OpenAPI;

namespace Radius.Core;

@doc("The Terraform configuration resource, providing reusable Terraform recipe settings for environments.")
model TerraformConfigResource
  is TrackedResourceRequired<TerraformConfigProperties, "terraformConfigs"> {
  @doc("Terraform configuration name")
  @key("terraformConfigName")
  @path
  @segment("terraformConfigs")
  name: ResourceNameString;
}

@doc("Terraform configuration properties.")
model TerraformConfigProperties {
  @doc("The status of the asynchronous operation.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("Environments that reference this Terraform configuration.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("Authentication information used to access private Terraform module sources. Supported module sources: Git.")
  authentication?: TerraformAuthConfig;

  @doc("Configuration for Terraform Recipe Providers. Controls how Terraform interacts with cloud providers, SaaS providers, and other APIs. For more information, please see: https://developer.hashicorp.com/terraform/language/providers/configuration.")
  providers?: Record<Array<TerraformProviderConfigProperties>>;

  @doc("Environment variables injected during Terraform recipe execution.")
  env?: Record<string>;

  @doc("Secret-backed environment variables injected during Terraform recipe execution. The secrets are stored in Applications.Core/SecretStores resource.")
  envSecrets?: Record<TerraformSecretReference>;
}

@doc("Authentication information used to access private Terraform module sources. Supported module sources: Git.")
model TerraformAuthConfig {
  @doc("Authentication information used to access private Terraform modules from Git repository sources.")
  git?: TerraformGitAuthConfig;
}

@doc("Authentication information used to access private Terraform modules from Git repository sources.")
model TerraformGitAuthConfig {
  @doc("Personal Access Token (PAT) configuration used to authenticate to Git platforms.")
  pat?: Record<TerraformSecretConfig>;
}

@doc("Personal Access Token (PAT) configuration used to authenticate to Git platforms.")
model TerraformSecretConfig {
  @doc("The ID of an Applications.Core/SecretStore resource containing the Git platform personal access token (PAT). The secret store must have a secret named 'pat', containing the PAT value. A secret named 'username' is optional, containing the username associated with the pat. By default no username is specified.")
  secret?: string;
}

// ProviderConfigProperties allows to get the additional properties and secrets. To ensure that `additionalProperties` is true, we need to extend `Record<unknown>`.
#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("Configuration for a Terraform provider, including credentials and other settings needed for recipe execution.")
model TerraformProviderConfigProperties extends Record<unknown> {
  @doc("Sensitive data in provider configuration can be stored as secrets. The secrets are stored in Applications.Core/SecretStores resource.")
  secrets?: Record<TerraformSecretReference>;
}

@doc("A reference to a secret stored in an Applications.Core/SecretStore resource.")
model TerraformSecretReference {
  @doc("The ID of an Applications.Core/SecretStore resource containing sensitive data required for recipe execution.")
  source: string;

  @doc("The key for the secret in the secret store.")
  key: string;
}

@armResourceOperations
interface TerraformConfigs {
  get is ArmResourceRead<
    TerraformConfigResource,
    UCPBaseParameters<TerraformConfigResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    TerraformConfigResource,
    UCPBaseParameters<TerraformConfigResource>
  >;

  update is ArmResourcePatchSync<
    TerraformConfigResource,
    TerraformConfigProperties,
    UCPBaseParameters<TerraformConfigResource>
  >;

  delete is ArmResourceDeleteSync<
    TerraformConfigResource,
    UCPBaseParameters<TerraformConfigResource>
  >;

  listByScope is ArmResourceListByParent<
    TerraformConfigResource,
    UCPBaseParameters<TerraformConfigResource>,
    "Scope",
    "Scope"
  >;
}
