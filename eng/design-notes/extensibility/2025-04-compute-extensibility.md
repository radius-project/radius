# Compute Platform Extensibility

* **Author**: Brooke Hamilton (@Brooke-Hamilton)

## Summary

Radius will provide extensible support for multiple compute platforms through recipes rather than hard-coded support for each platform.

Core resource types (`containers`, `gateways`, and `secretStores`) will allow recipes to be registered for them, with default recipes for Kubernetes and Azure Container Instances (ACI) provided by Radius. Customers can use, modify, or remove the default recipes.

This design enables:

* Architectural separation of Radius core logic from platform provisioning code
* Community-provided extensions to support new compute platforms without Radius code changes
* Consistent platform engineering experience across all resource types

We have two options for implementation:

1. (Recommended) Create UDT versions of the core resource types (`containers`, `gateways`, and `secretStores`) paired with recipe-based provisioning for ACI and Kubernetes. Later, remove the built-in core types and existing Kubernetes provisioning code. Only `environments` and `applications` will remain as built-in core types.
2. Add recipe support to the built-in core types and create provisioning recipes for ACI and Kubernetes. Later, remove the hard-coded provisioning code.

The primary criterion for deciding between option 1 and 2 is whether we want the application model to be represented as Radius-provided UDTs, or if we want to continue to have the core resource types that represent the application model to be built-in.

Both options have technical risk, primarily in the implementation of Kubernetes provisioning with recipes and the associated separation of provisioning code from the Radius application. Option 2 has lower initial risk and faster time to initial release because the scope of the initial release is smaller.

## Terms and definitions

| Term | Definition |
|------|------------|
| **Compute platform** | An environment where applications can be deployed and run, such as Kubernetes, Azure Container Instances (ACI), etc. |
| **Core types** | Built-in resource types provided by Radius, including `containers`, `gateways`, `secretStores`, `environments`, and `applications`. |
| **User-defined type (UDT)** | A custom resource type defined separately from Radius core types and loaded into Radius without requiring a new Radius release. |
| **Recipe** | A set of instructions that provisions a Radius resource to a Radius environment. Recipes are implemented in Bicep or Terraform. |
| **Resource Provider (RP)** | A component responsible for handling create, read, update, delete, and list (CRUDL) operations for a specific resource type. |

### Goals

* Provide platform engineers with the ability to deploy to specific platforms other than Radius, like ACI.
* Provide a recipe-based platform engineering experience that is consistent for user-defined types and core types.
* Expand the ability to provision a single application definition to multiple clouds by adding the capability to provision to multiple compute platforms like ACI and other serverless technologies.

### Non-goals

* Running the Radius control plane on a non-Kubernetes platform
* Changes to portable types

## Principles

Radius extensibility should enable creating extensions that are:

| Principle | Description |
|-----------|-------------|
| **Independently upgradeable in a runtime environment** | Recipes are independently upgradeable. |
| **Isolated and over a network protocol** | Recipes are currently implemented through Bicep and Terraform. Both run locally and communicate to the target platform over secure network protocols. |
| **Strongly typed and support versioning** | Recipes support versioning through the use of OCI-compliant registries. They are not strongly typed in the sense of having compile-time validation, but they can be validated upon registration in an environment as having the correct input parameters and output properties. |

### User scenarios

#### Configure a non-Kubernetes Radius environment

As a platform engineer I can initialize a new Radius environment for a non-Kubernetes compute platform so that Radius can authenticate and deploy to non-Kubernetes platforms.

##### Changes to `environments` Core Resource Type

A new version of `environments` will be added that does not have a hard coded list of compute kinds.

```diff
+resource environment 'Applications.Core/environments@2025-05-01-preview' = {
  name: 'myenv'
  properties: {
-    compute: {
-      kind: 'kubernetes'
-      namespace: 'default'
-      }
-    }
    recipeConfig: {
      env {
        foo: 'bar'
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches':{
        default: {
          templateKind: 'bicep'
          plainHttp: true
          templatePath: 'ghcr.io/radius-project/recipes/azure/rediscaches:latest'
        }
      }
    }
-   extensions: [
-      {
-        kind: 'kubernetesMetadata'
-        labels: {
-          'team.contact.name': 'frontend'
-        }
-      }
-    ]
  }
}
```

> NOTE: The resource types shown here are new versions, e.g., `environments@2025-05-01-preview`, not changes to the existing versions of the resource types. This allows us to avoid breaking existing deployments.

The `compute` platform is removed so that we do not have hard-coded support for specific platforms. The functionality enabled by the `compute.identity` property would be implemented via recipes so that we do not need hard-coded support for specific platforms. We could consider adding a recipe to the `environment` type if there are features enabled by `compute` that we could not achieve using recipes on the other core types.

##### Changes to `applications` Core Resource Type

Extensions are removed from the `applications` core type because the type of data being set in extensions would be set as recipe parameters. This is a new version of `applications`.

```diff
+resource app 'Applications.Core/applications@2025-05-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
-   extensions: [
-     {
-       kind: 'kubernetesNamespace'
-       namespace: 'myapp'
-     }
-     {
-       kind: 'kubernetesMetadata'
-       labels: {
-         'team.contact.name': 'frontend'
-       }
-     }
-   ]
  }
}
```

#### Initialize a workspace

As a platform engineer I can use `rad init` (or the equivalent rad `workspace`, `group` and `environment` commands), plus `rad recipe register`, to set up a non-Kubernetes compute platform.

`rad init` would be unchanged (in terms of the user experience). Platform engineers would have to add recipes for each core type and UDT they plan to use. By default, the command will register resource types and recipes for Kubernetes provisioning.

We could consider adding a flag to `rad init` that would identify a specific set of recipes. For example, `rad init aci` would install a set of default recipes for the ACI platform. However, this work is not in scope for this design.

#### Extend Radius to a new platform by creating new recipes

The `rad recipe register` CLI command would be unchanged, as it already has the ability to associate a recipe to an environment and a resource type, and it supports setting default values for specified recipe parameters for that environment.

```shell
rad recipe register <recipe name> \
  --environment <environment name> \
  --resource-type <core or UDT resource type name> \
  --parameters throughput=400
```

#### Register recipes for types

As a platform engineer I can set recipes on the resource types of container, gateway, and secret store so that I can configure my deployments.

#### `containers` Resource Type

Extensions and runtimes are removed from `containers` because the data in those sections would be set as parameters to a recipe.

This is a new version of `containers`. `Applications.Core` remains as the namespace because the purpose of the `containers` type remains the same whether the type is hard-coded or created as a UDT.

```diff
+resource frontend 'Applications.Core/containers@2025-05-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: {
      image: 'registry/container:tag'
      env:{
        DEPLOYMENT_ENV: {
          value: 'prod'
        }
        DB_CONNECTION: {
          value: db.listSecrets().connectionString
        }
      }
      ports: {
        http: {
          containerPort: 80
          protocol: 'TCP'
        }
      }
    }
-   extensions: [
-     {
-       kind: 'daprSidecar'
-       appId: 'frontend'
-     }
-     {
-       kind:  'manualScaling'
-       replicas: 5
-     }
-     {
-       kind: 'kubernetesMetadata'
-       labels: {
-         'team.contact.name': 'frontend'
-       }
-     }
-   ]
-   runtimes: {
-     kubernetes: {
-       base: loadTextContent('base-container.yaml')
-       pod: {
-         containers: [
-           {
-             name: 'log-collector'
-             image: 'ghcr.io/radius-project/fluent-bit:2.1.8'
-           }
-         ]
-         hostNetwork: true
-       }
-     }
-   }
  }
}
```

#### `gateways` and `secretStores` Resource Types

The `gateways` resource type will be created as a UDT with a new version, as in `Applications.Core/gateways@2025-05-01-preview`, with no schema changes.

The `secretStores` resource type will be created as a UDT with a new version, as in `Applications.Core/secretStores@2025-05-01-preview`, with no schema changes.

#### Extenders

The `extenders` type (`Applications.Core/extenders@2023-10-01-preview`) will be removed when all development phases are complete because the capability provided by them will be replaced by UDTs.

<!--
## Design

### High Level Design

This design covers the removal of imperative provisioning code, Bicep-based provisioning for Kubernetes and other platforms, and the opportunity to create reusable Bicep modules for customers to use in their recipes.

> NOTE: This design builds upon the existing designs for Radius UDTs in order to provide extensible provisioning for multiple platforms. 

The recommended development option builds upon existing designs for Radius UDTs in order to provide extensible provisioning to multiple platforms. We could provide ACI and other platform support, as well as an alternate recipe-based Kubernetes provisining path without making any changes to Radius. However, the plan includes removing the core resource types and the associated provisioning logic is a meaningful change, as is the provisioning logic that must be implemented in Bicep. We also believe we can provide reusable components in Bicep.

### Bicep Language Features for Recipe Extensibility

Bicep provides several powerful language features that can be leveraged to create a modular, reusable recipe system for compute platform extensibility:

#### Modules and Module Libraries

Base type declarations that can be extended in customer recipes
Standard types used in parameter lists and outputs
Module registry

#### Extension Points and Hooks

Can we provide extension points in the Bicep recipes?

Provide a high-level description, using diagrams as appropriate, and top-level
explanations to convey the architectural/design overview. Don’t go into a lot
of details yet but provide enough information about the relationship between
these components and other components. Call out or highlight new components
that are not part of this feature (dependencies). This diagram generally
treats the components as black boxes. Provide a pointer to a more detailed
design document, if one exists. 
-->

<!--
### Architecture Diagram
Provide a diagram of the system architecture, illustrating how different
components interact with each other in the context of this proposal.

Include separate high level architecture diagram and component specific diagrams, wherever appropriate.
-->

<!--
### Detailed Design

This section should be detailed and thorough enough that another developer
could implement your design and provide enough detail to get a high confidence
estimate of the cost to implement the feature but isn’t as detailed as the 
code. Be sure to also consider testability in your design.

For each change, give each "change" in the proposal its own section and
describe it in enough detail that someone else could implement it. Cover
ALL of the important decisions like names. Your goal is to get an agreement
to proceed with coding and PRs.

If there are alternatives you are considering please include that in the open
questions section. If the product has a layered architecture, it's good to
align these sections with the product's layers. This will help readers use
their current understanding to understand your ideas.

Discuss the rationale behind architectural choices and alternative options 
considered during the design process.
-->

#### Risks and Mitigations

The primary risk mitigation is to begin with provisioning ACI using UDTs and recipes, as that activity will not break existing logic and will expose any unknown issues like additional architectural or design changes that are needed.

| Risk | Description | Mitigation |
|------|-------------|------------|
| Bicep capabilities and limitations | Does the imperative Go provisioning code for Kubernetes contain logic that would be difficult or impossible to implement in Bicep using the Kubernetes extension for Bicep? | Early POC, plus implementing ACI provisioning first will provide an early indicator of limitations. Terraform could also be used for Kubernetes provisioning. |
| Radius graph | Updates to the Radius graph may prove difficult and time consuming | Phase 1 will provide early detection of this risk if it becomes an issue. |
| `containers.connections` | Recipes will have to create connections, which may uncover complexity. | This risk is related to the graph risk, and we will use Phase 1 to provide early detection. |
| `containers` type complexity | The `containers` type has a large surface area, which may affect effort and schedule.  | Maintain versioned support for older types during transition, provide clear migration paths. |

<!--
Describe what's not ideal about this plan. Does it lock us into a 
particular design for future changes or is it flexible if we were to 
pivot in the future. This is a good place to cover risks.
-->

<!--
#### Proposed Option
Describe the recommended option and provide reasoning behind it.
-->

<!--
### API design (if applicable)

Include if applicable – any design that changes our public REST API, CLI
arguments/commands, or Go APIs for shared components should provide this
section. Write N/A here if not applicable.
- Describe the REST APIs in detail for new resource types or updates to
  existing resource types. E.g. API Path and Sample request and response.
- Describe new commands in the CLI or changes to existing CLI commands.
- Describe the new or modified Go APIs for any shared components.
-->

<!--
### CLI Design (if applicable)
Include if applicable – any design that changes Radius CLI
arguments/commands. Write N/A here if not applicable.
- Describe new commands in the CLI or changes to existing CLI commands.
-->

<!--
### Implementation Details
High level description of updates to each component. Provide information on 
the specific sub-components that will be updated, for example, controller, processor, renderer,
recipe engine, driver, to name a few.

#### UCP (if applicable)
#### Bicep (if applicable)
#### Deployment Engine (if applicable)
#### Core RP (if applicable)
#### Portable Resources / Recipes RP (if applicable)
-->

<!--
### Error Handling
Describe the error scenarios that may occur and the corresponding recovery/error handling and user experience.
-->

<!-- 
## Test plan

* How much of our functional testing is directly loading RPs vs using application Bicep files and measuring the result of a deployment?

### Recipe Testing

* Interface implementation of recipes: does each recipe implement the right parameters and return the right outputs? This should be available for customers, too, and run upon recipe registration. (What tests are currently done upon recipe registration?)
* Reusable modules - what capabilities of Bicep would make a reusable module testable?  
-->

<!--

Include the test plan to validate the features including the areas that
need functional tests.

Describe any functionality that will create new testing challenges:
- New dependencies
- External assets that tests need to access
- Features that do I/O or change OS state and are thus hard to unit test
-->

<!--
## Security

Describe any changes to the existing security model of Radius or security 
challenges of the features. For each challenge describe the security threat 
and its mitigation with this design. 

Examples include:
- Authentication 
- Storing secrets and credentials
- Using cryptography

If this feature has no new challenges or changes to the security model
then describe how the feature will use existing security features of Radius.
-->

<!--
## Compatibility (optional)

Describe potential compatibility issues with other components, such as
incompatibility with older CLIs, and include any breaking changes to
behaviors or APIs.
-->

<!--
## Monitoring and Logging

Include the list of instrumentation such as metric, log, and trace to 
diagnose this new feature. It also describes how to troubleshoot this feature
with the instrumentation. 
-->

## Recommended Development Plan: Move core types to UDTs

The recommended option is to implement core types as UDTs, and later remove the hard-coded core types.

| Phase | Name | Size | Activities | Customer Capabilities |
| ----- | ---- | ---- | ---------- | --------------------- |
| 1 | Create UDTs for core types, provision ACI with recipes | M | - Implement core types as UDTs for `containers`, `gateways`, and `secretStores`.<br>- Implement ACI provisioning from recipes<br>| - ACI is provisioned from default recipes<br> - ACI recipes can be modified/replaced by customers |
| 2 | Provision Kubernetes with recipes| L | Convert Kubernetes deployments to recipes for the UDT core types<br> | Kubernetes recipes can be modified/replaced by customers |
| 3 | Remove Existing Core Types | S | - Remove core types that are hard coded into Radius<br> - Remove Kubernetes and ACI provisioning from Radius | Original core types are no longer available in Radius. |
| | Release | | | |

### Advantages vs Alternate Plans

* **Consistent application model**: All resource types are UDTs.
* **Builds upon existing UDT feature**: Adding core types as UDTs builds upon the UDT capability in Radius.
* **Clear architectural separation**: This plan implements a clear architectural separation between core functionality (environments and applications) and platform-specific provisioning (UDTs and recipes).

### Disadvantages vs Alternate Plans

* **Changed application model**: This plan results in a more consistent application model, but it is a change from the existing application model, which may cause confusion and upgrade work for users.
* **Higher initial complexity**: Implementing core types as UDTs requires solving more problems before having a releasable feature.
* **Delayed delivery**: Initial capabilities will take longer to deliver a release.

## Alternate Plan: Add Recipe Support to Existing Core Types

The alternate plan is to keep the core types (`containers`, `gateways`, and `secretStores`) as built-in types in the Radius application model, and add recipe support to them.

| Phase | Name | Size | Activities | Customer Capabilities |
| ----- | ---- | ---- | ---------- | --------------------- |
| 1 | Support Recipes on Core Types | M | Enable recipes on new versions of core types | Recipes can be registered for core resource types. |
| | Release | | | |
| 2 | ACI recipes | M | Implement ACI provisioning from recipes |- ACI is deployed via default recipes<br> - ACI recipes can be modified/replaced by customers |
| | Release | | | |
| 3 | Kubernetes recipes | L | - Implement Kubernetes provisioning from recipes<br>- Remove existing Kubernetes provisioning code |- Kubernetes is deployed via default recipes<br> - Kubernetes recipes can be modified/replaced by customers |
| | Release | | | |

### Advantages vs Recommended Plan

* **Stable application model**: Maintains the familiar Radius application model that customers are already using.
* **Incremental change**: Adding recipe support to core types is an additive change, and adding ACI provisioning with recipes is not disruptive to the existing Kubernetes provisioning code.
* **Earlier delivery**: ACI recipe provisioning can be added and released before Kubernetes recipe is released.

### Disadvantages vs Recommended Plan

* **Less architectural flexibility**: Core types remain hard-coded in Radius, limiting some extensibility options, e.g., the ability of a customer to copy and modify a core type.
* **Inconsistent resource type model**: Core types and UDTs would have different implementation approaches. However, customers can choose to ignore the core types and implement their own.

## Open Questions

* What is the impact on the Radius graph, and how would we continue to support it? We will could implement the graph as the relationships defined in Bicep, or implement connections in a Bicep/Terraform reusable module.
* Is the Radius group concept affected by this design?
* Can everything currently deployed by Go code be deployed using recipes? We think so, but need to prove it through prototyping.

## Architecture Alternatives Considered for Extensibility

An alternative architecture for extensibility that we considered was to enable the registration of custom resource providers. This architecture could be added to Radius later; it does not conflict with or replace recipe-based provisioning.

* Customers can register their own RPs for UDTs.
* Customer RPs implement an OpenAPI specification generated from UDT type definitions that defines a set of REST endpoints.
* Radius routes CRUDL operations to the custom RPs.
* RPs can be written in any language, hosted on any platform, and must be managed and deployed by the customer.
* Operations in addition to CRUDL could be supported.

We did not select this option because:

* Recipes are simpler and lower effort for users to implement than custom RPs.
* Having recipes on the core types will enable most customization scenarios.
* Implementing recipes does not exclude adding custom RPs later as an additional extensibility point.

## Design Review Notes
* Migration and upgrades must be considered in detail designs.
* We may need more robust validation of recipes during registration.
* A separate feature spec and design is planned to cover the user experience for extensibility.
<!-- 
* More detail on what an environment means as it relates to a target platform.
-->
