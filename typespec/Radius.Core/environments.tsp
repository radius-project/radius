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
  The `Radius.Core/environments` Resource Type represents a Radius Environment: the deployment target that platform engineers configure for their developers. Every Radius Application is deployed to an Environment through its `environment` property.
  
  An Environment defines three things for the applications deployed to it:
  
  - **Where resources are deployed**: the target compute platform and cloud provider accounts, set through the `providers` property.
  - **Which Recipes are used**: the Recipe Packs whose Recipes provision the infrastructure backing application resources, set through the `recipePacks` property.
  - **Advanced Terraform and Bicep settings**: environment-wide recipe parameters and Terraform or Bicep engine configuration applied when Recipes run.
  
  Simple Environments can be created with the `rad environment create` command. More advanced configurations require defining a `Radius.Core/environments` resource in a Bicep file and deploying it with `rad deploy`. For example:
  
  ```bicep
  extension radius
  
  resource myEnvironment 'Radius.Core/environments@2025-08-01-preview' = {
    name: 'my-environment'
    properties: {
      recipePacks: [
        myRecipePack.id
      ]
      providers: {
        kubernetes: {
          namespace: 'my-namespace'
        }
      }
    }
  }
  ```
  
  Both properties have defaults. If `providers` is omitted, resources are deployed to Kubernetes in the `default` namespace. If `recipePacks` is omitted, the Environment uses the `default` Recipe Pack in the `default` resource group.
  
  ## Cloud providers
  
  By default an Environment deploys to Kubernetes. To deploy cloud resources, configure the `providers` property with your cloud account. For AWS, set the account ID and region:
  
  ```bicep
  extension radius
  
  resource myEnvironment 'Radius.Core/environments@2025-08-01-preview' = {
    name: 'my-environment'
    properties: {
      providers: {
        aws: {
          accountId: '123456789012'
          region: 'us-west-2'
        }
      }
    }
  }
  ```
  
  For Azure, set the subscription and resource group that resources are deployed into:
  
  ```bicep
  extension radius
  
  resource myEnvironment 'Radius.Core/environments@2025-08-01-preview' = {
    name: 'my-environment'
    properties: {
      providers: {
        azure: {
          subscriptionId: '00000000-0000-0000-0000-000000000000'
          resourceGroupName: 'my-resource-group'
        }
      }
    }
  }
  ```
  
  The `providers` property selects the target account, but the credentials Radius uses to authenticate to AWS and Azure are configured separately with the `rad credential register` command.
  
  ## Recipe Packs
  
  A Recipe Pack is a collection of Recipes, defined as a separate `Radius.Core/recipePacks` resource. A Recipe is an infrastructure-as-code module, a Terraform module or Bicep template, that provisions the infrastructure backing an application resource. Reference one or more Recipe Packs by their resource IDs in the `recipePacks` property to control which Recipes are used. See the `Radius.Core/recipePacks` documentation for details.
  
  Use `recipeParameters` to pass environment-specific parameters to the Recipes defined in the referenced Recipe Packs, for example to standardize configuration such as SKUs or instance sizes across an Environment.
  
  ## Advanced Terraform and Bicep settings
  
  For advanced Terraform and Bicep configuration, such as private module sources and registry authentication, reference a `Radius.Core/terraformSettings` or `Radius.Core/bicepSettings` resource from the `terraformSettings` and `bicepSettings` properties.
  
  For more information, see the Radius documentation at https://docs.radapp.io.
  """)
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

  @doc("Resource ID of a Radius.Core/terraformSettings resource providing Terraform recipe settings.")
  terraformSettings?: string;

  @doc("Resource ID of a Radius.Core/bicepSettings resource providing Bicep recipe settings.")
  bicepSettings?: string;

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
  @doc("AWS account ID for AWS resources to be deployed into.")
  accountId: string;

  @doc("AWS region for AWS resources to be deployed into.")
  region: string;
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
