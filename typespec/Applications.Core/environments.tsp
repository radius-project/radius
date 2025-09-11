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
import "./common.tsp";
import "../radius/v1/trackedresource.tsp";
import "./extensions.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Core;

@doc("The environment resource")
model EnvironmentResource
  is TrackedResourceRequired<EnvironmentProperties, "environments"> {
  @doc("environment name")
  @key("environmentName")
  @path
  @segment("environments")
  name: ResourceNameString;
}

@doc("Environment properties")
model EnvironmentProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The compute resource used by application environment.")
  compute: EnvironmentCompute;

  @doc("Cloud providers configuration for the environment.")
  providers?: Providers;

  @doc("Simulated environment.")
  simulated?: boolean;

  @doc("Specifies Recipes linked to the Environment.")
  recipes?: Record<Record<RecipeProperties>>;

  @doc("Configuration for Recipes. Defines how each type of Recipe should be configured and run.")
  recipeConfig?: RecipeConfigProperties;

  @doc("The environment extension.")
  @extension("x-ms-identifiers", [])
  extensions?: Array<Extension>;

  @doc("The recipePacks property is an array of Recipe Pack IDs")
  recipePacks?: Array<string>;
}

@doc("Configuration for Recipes. Defines how each type of Recipe should be configured and run.")
model RecipeConfigProperties {
  @doc("Configuration for Terraform Recipes. Controls how Terraform plans and applies templates as part of Recipe deployment.")
  terraform?: TerraformConfigProperties;

  @doc("Configuration for Bicep Recipes. Controls how Bicep plans and applies templates as part of Recipe deployment.")
  bicep?: BicepConfigProperties;

  @doc("Environment variables injected during recipe execution for the recipes in the environment, currently supported for Terraform recipes.")
  env?: EnvironmentVariables;

  @doc("Environment variables containing sensitive information can be stored as secrets. The secrets are stored in Applications.Core/SecretStores resource.")
  envSecrets?: Record<SecretReference>;
}

@doc("Configuration for Bicep Recipes. Controls how Bicep plans and applies templates as part of Recipe deployment.")
model BicepConfigProperties {
  @doc("Authentication information used to access private bicep registries, which is a map of registry hostname to secret config that contains credential information.")
  authentication?: Record<RegistrySecretConfig>;
}

@doc("Registry Secret Configuration used to authenticate to private bicep registries.")
model RegistrySecretConfig {
  @doc("The ID of an Applications.Core/SecretStore resource containing credential information used to authenticate private container registry.The keys in the secretstore depends on the type.")
  secret?: string;
}

@doc("Configuration for Terraform Recipes. Controls how Terraform plans and applies templates as part of Recipe deployment.")
model TerraformConfigProperties {
  @doc("Authentication information used to access private Terraform module sources. Supported module sources: Git.")
  authentication?: AuthConfig;

  @doc("Configuration for Terraform Recipe Providers. Controls how Terraform interacts with cloud providers, SaaS providers, and other APIs. For more information, please see: https://developer.hashicorp.com/terraform/language/providers/configuration.")
  providers?: Record<Array<ProviderConfigProperties>>;
}

@doc("Authentication information used to access private Terraform module sources. Supported module sources: Git.")
model AuthConfig {
  @doc("Authentication information used to access private Terraform modules from Git repository sources.")
  git?: GitAuthConfig;
}

@doc("Authentication information used to access private Terraform modules from Git repository sources.")
model GitAuthConfig {
  @doc("Personal Access Token (PAT) configuration used to authenticate to Git platforms.")
  pat?: Record<SecretConfig>;
}

@doc("Personal Access Token (PAT) configuration used to authenticate to Git platforms.")
model SecretConfig {
  @doc("The ID of an Applications.Core/SecretStore resource containing the Git platform personal access token (PAT). The secret store must have a secret named 'pat', containing the PAT value. A secret named 'username' is optional, containing the username associated with the pat. By default no username is specified.")
  secret?: string;
}

// ProviderConfigProperties allows to get the additional properties and secrets. To ensure that `additionalProperties` is true, we need to extend `Record<unknown>`.
// Reference: https://github.com/Azure/typespec-azure/blob/main/packages/typespec-autorest/test/additional-properties.test.ts
#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("This configuration holds the necessary information to authenticate and interact with a provider for the recipe execution.")
model ProviderConfigProperties extends Record<unknown> {
  @doc("Sensitive data in provider configuration can be stored as secrets. The secrets are stored in Applications.Core/SecretStores resource.")
  secrets?: Record<SecretReference>;
}

@doc("The environment variables injected during Terraform Recipe execution for the recipes in the environment.")
model EnvironmentVariables extends Record<string> {}

@doc("The Cloud providers configuration.")
model Providers {
  @doc("The Azure cloud provider configuration.")
  azure?: ProvidersAzure;

  @doc("The AWS cloud provider configuration.")
  aws?: ProvidersAws;
}

@doc("The Azure cloud provider definition.")
model ProvidersAzure {
  @doc("Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'.")
  scope: string;
}

@doc("The AWS cloud provider definition.")
model ProvidersAws {
  @doc("Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2'.")
  scope: string;
}

@doc("Format of the template provided by the recipe. Allowed values: bicep, terraform.")
@discriminator("templateKind")
model RecipeProperties {
  @doc("Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.")
  templatePath: string;

  @doc("Key/value parameters to pass to the recipe template at deployment.")
  parameters?: {};
}

@doc("Represents Bicep recipe properties.")
model BicepRecipeProperties extends RecipeProperties {
  @doc("The Bicep template kind.")
  templateKind: "bicep";

  @doc("Connect to the Bicep registry using HTTP (not-HTTPS). This should be used when the registry is known not to support HTTPS, for example in a locally-hosted registry. Defaults to false (use HTTPS/TLS).")
  plainHttp?: boolean;
}

@doc("Represents Terraform recipe properties.")
model TerraformRecipeProperties extends RecipeProperties {
  @doc("The Terraform template kind.")
  templateKind: "terraform";

  @doc("Version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.")
  templateVersion?: string;
}

@doc("This secret is used within a recipe. Secrets are encrypted, often have fine-grained access control, auditing and are recommended to be used to hold sensitive data.")
model SecretReference {
  @doc("The ID of an Applications.Core/SecretStore resource containing sensitive data required for recipe execution.")
  source: string;

  @doc("The key for the secret in the secret store.")
  key: string;
}

@doc("Represents the request body of the getmetadata action.")
model RecipeGetMetadata {
  @doc("Type of the resource this recipe can be consumed by. For example: 'Applications.Datastores/mongoDatabases'.")
  resourceType: string;

  @doc("The name of the recipe registered to the environment.")
  name: string;
}

@doc("The properties of a Recipe linked to an Environment.")
model RecipeGetMetadataResponse {
  @doc("The format of the template provided by the recipe. Allowed values: bicep, terraform.")
  templateKind: string;

  @doc("The path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.")
  templatePath: string;

  @doc("The version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.")
  templateVersion?: string;

  @doc("The key/value parameters to pass to the recipe template at deployment.")
  parameters: {};

  @doc("Connect to the Bicep registry using HTTP (not-HTTPS). This should be used when the registry is known not to support HTTPS, for example in a locally-hosted registry. Defaults to false (use HTTPS/TLS).")
  plainHttp?: boolean;
}

@armResourceOperations
interface Environments {
  get is ArmResourceRead<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>
  >;

  update is ArmResourcePatchSync<
    EnvironmentResource,
    EnvironmentProperties,
    UCPBaseParameters<EnvironmentResource>
  >;

  delete is ArmResourceDeleteSync<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>
  >;

  listByScope is ArmResourceListByParent<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>,
    "Scope",
    "Scope"
  >;

  @doc("Gets recipe metadata including parameters and any constraints on the parameters.")
  @action("getMetadata")
  getMetadata is ArmResourceActionSync<
    EnvironmentResource,
    RecipeGetMetadata,
    RecipeGetMetadataResponse,
    UCPBaseParameters<EnvironmentResource>
  >;
}
