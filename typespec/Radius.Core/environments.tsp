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
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("List of Recipe Pack resource IDs linked to this environment.")
  recipePacks?: string[];

  @doc("Recipe specific parameters that apply to all resources of a given type in this environment.")
  recipeParameters?: Record<RecipeParameterValue>;

  @doc("Cloud provider configuration for the environment.")
  providers?: Providers;

  @doc("Simulated environment.")
  simulated?: boolean;
}

@doc("Recipe parameter configuration for a specific resource type.")
model RecipeParameterValue {
  ...Record<unknown>;
}

@doc("The Azure cloud provider definition.")
model ProvidersAzure {
  @doc("Azure subscription ID hosting deployed resources.")
  subscriptionId: string;

  @doc("Optional resource group name.")
  resourceGroupName?: string;

  @doc("External identity settings (moved from compute).")
  identity?: IdentitySettings;
}

model ProvidersKubernetes {
  @doc("Kubernetes namespace to deploy workloads into.")
  `namespace`: string;
}

@doc("The AWS cloud provider definition.")
model ProvidersAws {
  @doc("Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2'.")
  scope: string;
}

model Providers {
  @doc("The Azure cloud provider configuration.")
  azure?: ProvidersAzure;

  @doc("The Kubernetes provider configuration.")
  kubernetes?: ProvidersKubernetes;

  @doc("The AWS cloud provider configuration.")
  aws?: ProvidersAws;
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
}
