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

namespace Radius.Core;

@doc("""
  The `Radius.Core/recipePacks` Resource Type represents a Recipe Pack: a named collection of Recipes that platform engineers assign to an Environment. A Recipe maps a resource type (such as `Radius.Data/redisCaches`) to an infrastructure-as-code module, a Terraform module or Bicep template, that provisions the infrastructure backing that resource when a developer deploys it.
  
  A Recipe Pack is defined as its own resource and referenced from an Environment's `recipePacks` property, so one Recipe Pack can be shared across many Environments.
  
  ## Defining a Recipe Pack
  
  Each entry in the `recipes` map is keyed by resource type. Set the recipe `kind` (`bicep` or `terraform`) and its `source` (an OCI registry reference for Bicep recipes, or a module source for Terraform recipes).
  
  ```bicep
  extension radius
  
  resource dataRecipes 'Radius.Core/recipePacks@2025-08-01-preview' = {
    name: 'data-recipes'
    properties: {
      recipes: {
        'Radius.Data/redisCaches': {
          kind: 'bicep'
          source: 'ghcr.io/my-org/recipes/redis:latest'
        }
        'Radius.Data/postgreSqlDatabases': {
          kind: 'terraform'
          source: 'git::https://github.com/my-org/recipes//postgresql'
        }
      }
    }
  }
  ```
  
  ## Recipe parameters
  
  Each Recipe can declare default `parameters` that are passed to its module. Platform engineers set baseline values here, and developers get them automatically. An Environment can override these values per resource type through its `recipeParameters` property.
  
  ```bicep
  resource dataRecipes 'Radius.Core/recipePacks@2025-08-01-preview' = {
    name: 'data-recipes'
    properties: {
      recipes: {
        'Radius.Data/redisCaches': {
          kind: 'bicep'
          source: 'ghcr.io/my-org/recipes/redis:latest'
          parameters: {
            sku: 'Standard'
            capacity: 1
          }
        }
      }
    }
  }
  ```
  
  ## Deploying a Recipe Pack
  
  Deploy the Bicep file with `rad deploy` to create the Recipe Pack resource. Once deployed, list and inspect Recipe Packs with the `rad recipe-pack list` and `rad recipe-pack show` commands.
  
  ## Referencing a Recipe Pack from an Environment
  
  An Environment references a Recipe Pack through its `recipePacks` property. When the Recipe Pack and the Environment are deployed to the same resource group, reference it by its symbolic name:
  
  ```bicep
  extension radius
  
  resource dataRecipes 'Radius.Core/recipePacks@2025-08-01-preview' existing = {
    name: 'data-recipes'
  }
  
  resource myEnvironment 'Radius.Core/environments@2025-08-01-preview' = {
    name: 'my-environment'
    properties: {
      recipePacks: [
        dataRecipes.id
      ]
    }
  }
  ```
  
  To reference a Recipe Pack in a different resource group, use its full resource ID instead:
  
  ```bicep
  recipePacks: [
    '/planes/radius/local/resourceGroups/shared/providers/Radius.Core/recipePacks/data-recipes'
  ]
  ```
  
  Radius is installed with a `default` Recipe Pack in the `default` resource group, which an Environment uses when `recipePacks` is not set.
  
  Prebuilt Recipe Packs and the Recipes they reference are published in the [resource-types-contrib](https://github.com/radius-project/resource-types-contrib) repository.
  
  For more information, see the Radius documentation at https://docs.radapp.io.
  """)
model RecipePackResource
  is TrackedResourceRequired<RecipePackProperties, "recipePacks"> {
  @doc("Recipe pack name")
  @key("recipePackName")
  @path
  @segment("recipePacks")
  name: ResourceNameString;
}

@doc("Recipe Pack properties")
model RecipePackProperties {
  @doc("The status of the asynchronous operation")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("List of environment IDs that reference this recipe pack")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("Map of resource types to their recipe configurations")
  recipes: Record<RecipeDefinition>;
}

@doc("Recipe definition for a specific resource type")
model RecipeDefinition {
  @doc("The type of recipe (e.g., Terraform, Bicep)")
  kind: RecipeKind;

  @doc("Connect to the source using HTTP (not HTTPS). This should be used when the source is known not to support HTTPS, for example in a locally hosted registry for Bicep recipes. Defaults to false (use HTTPS/TLS)")
  plainHttp?: boolean;

  @doc("The source of the recipe. For Bicep recipes this is the OCI registry reference. For Terraform recipes this is the module source.")
  source: string;

  @doc("Parameters to pass to the recipe")
  parameters?: Record<unknown>;

  @doc("Maps the module's outputs onto the resource type's properties, for recipes that point directly at a Bicep or Terraform module. Each entry's value is either a string (the module output name for a non-secret property) or, under the reserved `secrets` key, a nested object mapping secret property names to module output names. A `secrets` entry always routes its module output to the resource's secret outputs regardless of how the module classified it (for example an AVM module's `primaryConnectionString`). Example: `{ host: 'name', secrets: { connectionString: 'primaryConnectionString' } }`.")
  outputs?: Record<unknown>;
}

@doc("The type of recipe")
enum RecipeKind {
  @doc("Terraform recipe")
  terraform: "terraform",

  @doc("Bicep recipe")
  bicep: "bicep",
}

@armResourceOperations
interface RecipePacks {
  get is ArmResourceRead<
    RecipePackResource,
    UCPBaseParameters<RecipePackResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    RecipePackResource,
    UCPBaseParameters<RecipePackResource>
  >;

  update is ArmResourcePatchSync<
    RecipePackResource,
    RecipePackProperties,
    UCPBaseParameters<RecipePackResource>
  >;

  delete is ArmResourceDeleteSync<
    RecipePackResource,
    UCPBaseParameters<RecipePackResource>
  >;

  listByScope is ArmResourceListByParent<
    RecipePackResource,
    UCPBaseParameters<RecipePackResource>,
    "Scope",
    "Scope"
  >;
}
