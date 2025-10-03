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

@doc("The recipe pack resource")
model RecipePackResource
  is TrackedResourceRequired<RecipePackProperties, "recipePacks"> {
  @doc("recipe pack name")
  @key("recipePackName")
  @path
  @segment("recipePacks")
  name: ResourceNameString;
}

@doc("Recipe Pack properties")
model RecipePackProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Description of what this recipe pack provides")
  description?: string;

  @doc("List of environment IDs that reference this recipe pack")
  @visibility("read")
  referencedBy?: string[];

  @doc("Map of resource types to their recipe configurations")
  recipes: Record<RecipeDefinition>;
}

@doc("Recipe definition for a specific resource type")
model RecipeDefinition {
  @doc("The type of recipe (e.g., terraform, bicep)")
  recipeKind: RecipeKind;

  @doc("Connect to the location using HTTP (not-HTTPS). This should be used when the location is known not to support HTTPS, for example in a locally-hosted registry for Bicep recipes. Defaults to false (use HTTPS/TLS).")
  plainHttp?: boolean;

  @doc("URL or path to the recipe source")
  recipeLocation: string;

  @doc("Parameters to pass to the recipe")
  parameters?: {};
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
