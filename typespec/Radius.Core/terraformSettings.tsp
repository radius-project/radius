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

@doc("""
  The `Radius.Core/terraformSettings` Resource Type holds reusable Terraform CLI configuration that Environments apply when running Terraform Recipes. Its primary use is authenticating to private Terraform registries that host the modules referenced by a Recipe Pack, along with configuring provider installation and injecting environment variables during Recipe execution.
  
  Platform engineers define a `Radius.Core/terraformSettings` resource once and reference it from any Environment whose Recipes pull modules from a private registry.
  
  ## Defining Terraform settings
  
  Configure `terraformrc.credentials`, keyed by registry hostname, to authenticate to a private Terraform registry. Each entry points to a secret whose `token` key holds the registry token:
  
  ```bicep
  extension radius
  
  resource registrySecret 'Radius.Security/secrets@2025-08-01-preview' = {
    name: 'terraform-registry-token'
    properties: {
      data: {
        token: { value: 'my-registry-token' }
      }
    }
  }
  
  resource terraformRegistry 'Radius.Core/terraformSettings@2025-08-01-preview' = {
    name: 'private-registry'
    properties: {
      terraformrc: {
        credentials: {
          'app.terraform.io': {
            secret: registrySecret.id
          }
        }
      }
    }
  }
  ```
  
  Two other settings are available: provider installation and environment variables.
  
  ### Provider installation
  
  In air-gapped or internal environments, use `terraformrc.providerInstallation` to install providers from a network mirror instead of the public registry. Set the mirror `url`, optionally narrow it with `include` or `exclude` provider address patterns, and use `direct` to control which providers are still downloaded directly:
  
  ```bicep
  resource providerMirror 'Radius.Core/terraformSettings@2025-08-01-preview' = {
    name: 'internal-mirror'
    properties: {
      terraformrc: {
        providerInstallation: {
          networkMirror: {
            url: 'https://terraform.corp.example.com/providers/'
            include: ['*']
          }
          direct: {
            exclude: ['*']
          }
        }
      }
    }
  }
  ```
  
  ### Environment variables
  
  Use `env` to inject environment variables into every Terraform Recipe run, for example to raise the Terraform log level for troubleshooting or to tune CLI behavior:
  
  ```bicep
  resource terraformLogging 'Radius.Core/terraformSettings@2025-08-01-preview' = {
    name: 'terraform-logging'
    properties: {
      env: {
        TF_LOG: 'TRACE'
        TF_REGISTRY_CLIENT_TIMEOUT: '15'
      }
    }
  }
  ```
  
  ## Deploying Terraform settings
  
  Deploy the settings resource with the `rad deploy` command:
  
  ```bash
  rad deploy ./terraform-settings.bicep
  ```
  
  ## Referencing Terraform settings from an Environment
  
  Reference the settings from an Environment by setting its `terraformSettings` property to the resource ID. Every Terraform Recipe run in that Environment then uses the configured registry credentials:
  
  ```bicep
  extension radius
  
  resource terraformRegistry 'Radius.Core/terraformSettings@2025-08-01-preview' existing = {
    name: 'private-registry'
  }
  
  resource myEnvironment 'Radius.Core/environments@2025-08-01-preview' = {
    name: 'my-environment'
    properties: {
      terraformSettings: terraformRegistry.id
    }
  }
  ```
  
  For more information, see the Radius documentation at https://docs.radapp.io.
  """)
model TerraformSettingsResource
  is TrackedResourceRequired<TerraformSettingsProperties, "terraformSettings"> {
  @doc("Terraform configuration name")
  @key("terraformSettingsName")
  @path
  @segment("terraformSettings")
  name: ResourceNameString;
}

@doc("Terraform configuration properties.")
model TerraformSettingsProperties {
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

  @doc("Credentials for authenticating to private Terraform registries (HTTP-based, e.g. app.terraform.io). Map of registry hostname to credential configuration. Rendered as native `credentials \"hostname\" {}` blocks in the generated .terraformrc. Note: this is for Terraform CLI registry auth (HTTP), not for Git-based module sources; Git auth is a separate mechanism.")
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
  @doc("The ID of a secret resource containing the authentication token. Supported types: Radius.Security/secrets (recommended) or Applications.Core/secretStores. The secret must have a key named 'token'.")
  secret?: string;
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
