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

  @doc("Terraform CLI configuration file settings. Maps directly to the Terraform CLI configuration file (.terraformrc).")
  terraformrc?: TerraformrcConfig;

  @doc("Environment variables injected during Terraform recipe execution.")
  env?: Record<string>;
}

@doc("Terraform CLI configuration file (.terraformrc) settings. See https://developer.hashicorp.com/terraform/cli/config for details.")
model TerraformrcConfig {
  @doc("Provider installation configuration. Specifies the location of providers via network mirrors or direct downloads.")
  providerInstallation?: TerraformProviderInstallation;

  @doc("Credentials for authenticating to private Terraform registries and module sources. Map of hostname to credential configuration.")
  credentials?: Record<TerraformCredentialConfig>;
}

@doc("Provider installation configuration for Terraform CLI.")
model TerraformProviderInstallation {
  @doc("Network mirror configuration for downloading providers.")
  networkMirror?: TerraformProviderMirror;

  @doc("Direct provider installation configuration.")
  direct?: TerraformProviderDirect;
}

@doc("Network mirror configuration for Terraform providers.")
model TerraformProviderMirror {
  @doc("The URL of the provider mirror.")
  url?: string;

  @doc("Provider address patterns to include from this mirror.")
  include?: string[];

  @doc("Provider address patterns to exclude from this mirror.")
  exclude?: string[];
}

@doc("Direct provider installation configuration.")
model TerraformProviderDirect {
  @doc("Provider address patterns to include for direct installation.")
  include?: string[];

  @doc("Provider address patterns to exclude from direct installation.")
  exclude?: string[];
}

@doc("Credential configuration for a Terraform registry or module source host.")
model TerraformCredentialConfig {
  @doc("The ID of an Applications.Core/SecretStore resource containing the authentication token. The secret store must have a secret named 'token'.")
  secret?: string;
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
