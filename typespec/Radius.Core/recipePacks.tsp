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
  
  An Environment references a Recipe Pack through its `recipePacks` property. When the Recipe Pack and the Environment are deployed to the same resource group, declare it as an `existing` resource and reference its `.id`:
  
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
  
  Radius is installed with a `default` Recipe Pack in the `default` resource group. When you create an Environment with the Radius CLI and do not set `recipePacks`, the Environment uses this `default` Recipe Pack.
  
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
  @doc("(Read Only) The status of the Recipe Pack resource within the Radius control plane.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("(Read Only) Resource IDs of the Environments that reference this Recipe Pack.")
  @visibility(Lifecycle.Read)
  referencedBy?: string[];

  @doc("(Required) The Recipes in this pack, keyed by the resource type each Recipe provisions. Each key is a resource type such as `Radius.Data/redisCaches`.")
  recipes: Record<RecipeDefinition>;
}

@doc("Recipe definition for a specific resource type")
model RecipeDefinition {
  @doc("(Required) The kind of Recipe, which determines how Radius runs it.")
  kind: RecipeKind;

  @doc("(Optional) Connect to the source using HTTP instead of HTTPS. Use this only when the source does not support HTTPS such as a locally hosted registry for Bicep recipes. Defaults to `false` if not specified.")
  plainHttp?: boolean;

  @doc("(Required) Location of the Recipe. For Bicep Recipes this is an OCI registry reference. For Terraform Recipes this is the module source such as a Git URL or a Terraform registry module.")
  source: string;

  @doc("(Optional) Default parameter values passed to the Recipe when it runs. An Environment can override these per resource type through its `recipeParameters` property.")
  parameters?: Record<unknown>;

  @doc("(Optional) Maps the module outputs onto the resource type properties for recipes that point directly at a Bicep or Terraform module. Each value is the module output name for a non-secret property. Under the reserved `secrets` key a nested object maps secret property names to module output names and always routes those outputs to the resource secret outputs.")
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
