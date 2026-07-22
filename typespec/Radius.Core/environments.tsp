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
  
  An Environment defines three things for the Applications deployed to it:
  
  - **Where resources are deployed**: the target compute platform and cloud provider accounts, set through the `providers` property.
  - **Which Recipes are used**: the Recipe Packs whose Recipes provision the infrastructure backing application resources, set through the `recipePacks` property.
  - **Advanced Terraform and Bicep settings**: environment-wide Recipe parameters and Terraform or Bicep engine configuration applied when Recipes run.
  
  ## Defining an Environment
  
  The simplest Environment can be created directly with the `rad environment create` command, without a Bicep file:
  
  ```bash
  rad environment create my-environment
  ```
  
  For more advanced configurations, define an Environment as a `Radius.Core/environments` resource in a Bicep file. For example:
  
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
  
  resource myRecipePack 'Radius.Core/recipePacks@2025-08-01-preview' existing = {
    name: 'my-recipe-pack'
  }
  ```
  
  Both properties have defaults. If `providers` is omitted, resources are deployed to Kubernetes in the `default` namespace. If `recipePacks` is omitted, the Environment uses the `default` Recipe Pack in the `default` resource group.
  
  ## Deploying an Environment
  
  Deploy a defined Environment with the `rad deploy` command:
  
  ```bash
  rad deploy ./environment.bicep
  ```
  
  ## Cloud providers
  
  To use Recipes that provision resources in a cloud provider, you must configure that provider's account details on the Environment by setting the `providers` property. For AWS, set the account ID and the region resources are deployed to:
  
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
  
  For Azure, set the subscription ID and the name of the resource group that resources are deployed to:
  
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
  
  For advanced Terraform and Bicep settings, such as private module sources and registry authentication, reference a `Radius.Core/terraformSettings` or `Radius.Core/bicepSettings` resource from the `terraformSettings` and `bicepSettings` properties.
  
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
  @doc("(Read Only) The status of the Environment resource within the Radius control plane. Does not include the resources deployed to the Environment.")
  @visibility(Lifecycle.Read)
  provisioningState?: ProvisioningState;

  @doc("(Optional) Resource IDs of the Recipe Packs this Environment uses to provision infrastructure for application resources. Defaults to the `default` Recipe Pack in the `default` resource group if not specified.")
  recipePacks?: string[];

  @doc("(Optional) Parameters passed to Recipes when they run, keyed by resource type. Values here override the default parameters defined in the Recipe Pack for every resource of that type deployed to this Environment.")
  recipeParameters?: Record<RecipeParameterValue>;

  @doc("(Optional) Target compute platform and cloud provider accounts that resources are deployed into. Defaults to Kubernetes in the `default` namespace if not specified.")
  providers?: Providers;

  @doc("(Optional) Resource ID of a `Radius.Core/terraformSettings` resource that supplies Terraform CLI settings, such as private registry credentials, used when running Terraform Recipes in this Environment.")
  terraformSettings?: string;

  @doc("(Optional) Resource ID of a `Radius.Core/bicepSettings` resource that supplies Bicep engine settings, such as private registry authentication, used when running Bicep Recipes in this Environment.")
  bicepSettings?: string;

  @doc("(Optional) When true, the Environment is simulated and does not deploy real infrastructure. Recipes are evaluated but no resources are provisioned, which is useful for validating application definitions. Defaults to `false` if not specified.")
  simulated?: boolean;
}

@doc("Recipe parameter configuration for a specific resource type.")
model RecipeParameterValue {
  ...Record<unknown>;
}

@doc("The Azure cloud provider definition.")
model ProvidersAzure {
  @doc("(Required) ID of the Azure subscription that resources are deployed into.")
  subscriptionId: string;

  @doc("(Optional) Name of the Azure resource group that resources are deployed into. Most Bicep and Terraform Recipes expect a resource group in the deployment context, so set this whenever deploying Azure resources.")
  resourceGroupName?: string;

  @doc("(Optional) Managed or workload identity Radius uses to authenticate to Azure when deploying resources.")
  identity?: IdentitySettings;
}

model ProvidersKubernetes {
  @doc("(Required) Kubernetes namespace that workloads are deployed into.")
  `namespace`: string;
}

@doc("The AWS cloud provider definition.")
model ProvidersAws {
  @doc("(Required) ID of the AWS account that resources are deployed into.")
  accountId: string;

  @doc("(Required) AWS region that resources are deployed into.")
  region: string;
}

model Providers {
  @doc("(Optional) Configuration for deploying resources to Azure.")
  azure?: ProvidersAzure;

  @doc("(Optional) Configuration for deploying resources to Kubernetes.")
  kubernetes?: ProvidersKubernetes;

  @doc("(Optional) Configuration for deploying resources to AWS.")
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
