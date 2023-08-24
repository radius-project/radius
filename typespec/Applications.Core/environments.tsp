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
model EnvironmentResource is TrackedResource<EnvironmentProperties> {
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

  @doc("Specifies Recipes linked to the Environment.")
  recipes?: Record<Record<RecipeProperties>>;

  @doc("The environment extension.")
  @extension("x-ms-identifiers", [])
  extensions?: Array<Extension>;
}

@doc("The Cloud providers configuration")
model Providers {
  @doc("The Azure cloud provider configuration")
  azure?: ProvidersAzure;

  @doc("The AWS cloud provider configuration")
  aws?: ProvidersAws;
}

@doc("The Azure cloud provider definition")
model ProvidersAzure {
  @doc("Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'")
  scope: string;
}

@doc("The AWS cloud provider definition")
model ProvidersAws {
  @doc("Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2'")
  scope: string;
}

@doc("Format of the template provided by the recipe. Allowed values: bicep, terraform.")
@discriminator("templateKind")
model RecipeProperties {
  @doc("Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.")
  templatePath: string;

  @doc("Key/value parameters to pass to the recipe template at deployment")
  parameters?: {};
}

@doc("Represents Bicep recipe properties.")
model BicepRecipeProperties extends RecipeProperties {
  @doc("The Bicep template kind.")
  templateKind: "bicep";
}

@doc("Represents Terraform recipe properties.")
model TerraformRecipeProperties extends RecipeProperties {
  @doc("The Terraform template kind.")
  templateKind: "terraform";

  @doc("Version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.")
  templateVersion: string;
}

@doc("Represents the request body of the getmetadata action.")
model RecipeGetMetadata {
  @doc("Type of the link this recipe can be consumed by. For example: 'Applications.Link/mongoDatabases'")
  linkType: string;

  @doc("The name of the recipe registered to the environment")
  name: string;
}

@doc("The properties of a Recipe linked to an Environment.")
model RecipeGetMetadataResponse {
  @doc("The format of the template provided by the recipe. Allowed values: bicep, terraform.")
  templateKind: string;

  @doc("The path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.")
  templatePath: string;

  @doc("The version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.")
  templateVersion: string;

  @doc("The key/value parameters to pass to the recipe template at deployment.")
  parameters: {};
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
