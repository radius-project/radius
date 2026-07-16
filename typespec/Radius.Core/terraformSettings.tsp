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
  The `Radius.Core/terraformSettings` Resource Type holds reusable Terraform CLI settings that Environments apply when running Terraform Recipes. Its primary use is authenticating to private Terraform registries that host the modules referenced by a Recipe Pack, along with configuring provider installation and injecting environment variables during Recipe execution.
  
  Platform engineers define a `Radius.Core/terraformSettings` resource once and reference it from any Environment whose Recipes pull modules from a private registry.
  
  ## Defining Terraform settings
  
  Configure `terraformrc.credentials`, keyed by registry hostname, to authenticate to a private Terraform registry. Each entry points to a secret whose `token` key holds the registry token:
  
  ```bicep
  extension radius
  
  @description('The Radius Environment ID. Injected automatically by the rad CLI.')
  param environment string
  
  resource registrySecret 'Radius.Security/secrets@2025-08-01-preview' = {
    name: 'terraform-registry-token'
    properties: {
      environment: environment
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
  @doc("Terraform settings name")
  @key("terraformSettingsName")
  @path
  @segment("terraformSettings")
  name: ResourceNameString;
}

@doc("Terraform settings properties.")
model TerraformSettingsProperties {
  @doc("(Read Only) The status of the Terraform settings resource within the Radius control plane.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("(Read Only) Resource IDs of the Environments that reference this Terraform settings resource.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("(Optional) Settings for the Terraform CLI configuration file. Radius renders these into a `.terraformrc` file used when running Terraform Recipes.")
  terraformrc?: TerraformrcConfig;

  @doc("(Optional) Environment variables injected into the Terraform process during Recipe execution.")
  env?: Record<string>;
}

@doc("Terraform CLI configuration file (.terraformrc) settings. See https://developer.hashicorp.com/terraform/cli/config for details.")
model TerraformrcConfig {
  @doc("(Optional) Controls where Terraform installs providers from, such as a network mirror instead of the public registry.")
  providerInstallation?: TerraformProviderInstallation;

  @doc("(Optional) Credentials for authenticating to private Terraform registries such as `app.terraform.io`. Maps a registry hostname to its credential configuration. This authenticates to Terraform CLI registries over HTTP and does not authenticate Git-based module sources, which use a separate mechanism.")
  credentials?: Record<TerraformCredentialConfig>;
}

@doc("Provider installation configuration for Terraform CLI.")
model TerraformProviderInstallation {
  @doc("(Optional) A network mirror to install providers from instead of the public registry.")
  networkMirror?: TerraformProviderMirror;

  @doc("(Optional) Providers to install directly from the public registry rather than a mirror.")
  direct?: TerraformProviderDirect;
}

@doc("Network mirror configuration for Terraform providers.")
model TerraformProviderMirror {
  @doc("(Optional) The URL of the provider mirror.")
  url?: string;

  @doc("(Optional) Provider address patterns to include from this mirror.")
  include?: string[];

  @doc("(Optional) Provider address patterns to exclude from this mirror.")
  exclude?: string[];
}

@doc("Direct provider installation configuration.")
model TerraformProviderDirect {
  @doc("(Optional) Provider address patterns to include for direct installation.")
  include?: string[];

  @doc("(Optional) Provider address patterns to exclude from direct installation.")
  exclude?: string[];
}

@doc("Credential configuration for a Terraform registry or module source host.")
model TerraformCredentialConfig {
  @doc("(Optional) The ID of a `Radius.Security/secrets` resource containing the authentication token. The secret must have a key named `token`.")
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
