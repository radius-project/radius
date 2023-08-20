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
import "./extensions.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using Azure.Core;
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
model EnvironmentProperties extends BasicResourceProperties {
  @visibility("read")
  @doc("The status of the last operation.")
  provisioningState?: ProvisioningState;

  @doc("The compute resource used by application environment.")
  compute: EnvironmentCompute;

  @doc("Cloud providers configuration for the environment.")
  provider?: Providers;

  @doc("Specifies Recipes linked to the Environment.")
  recipes?: RecipeProperties;

  @doc("The environment extension.")
  @extension("x-ms-identifiers", [])
  extenions?: Array<Extension>;
}

@doc("The Cloud providers configuration")
model Providers {
  @doc("The Azure cloud provider configuration")
  azure?: ProviderAzure;

  @doc("The AWS cloud provider configuration")
  aws?: ProviderAws;
}

@doc("The Azure cloud provider definition")
model ProviderAzure {
  @doc("Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'")
  scope: string;
}

@doc("The AWS cloud provider definition")
model ProviderAws {
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

@doc("The properties of a Recipe linked to an Environment.")
model RecipeMetadataResult {
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
interface Environment {
  get is ArmResourceRead<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>
  >;
  create is ArmResourceCreateOrUpdateSync<
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

  listByRootScope is ArmResourceListByParent<
    EnvironmentResource,
    UCPBaseParameters<EnvironmentResource>,
    "RootScope",
    "RootScope"
  >;

  @doc("Gets recipe metadata including parameters and any constraints on the parameters.")
  @action("getmetadata")
  getmetadata is ArmResourceActionSync<
    EnvironmentResource,
    {},
    RecipeMetadataResult,
    UCPBaseParameters<EnvironmentResource>
  >;
}
