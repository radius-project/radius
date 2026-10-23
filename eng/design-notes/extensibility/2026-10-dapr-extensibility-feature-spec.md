<!--
Source template: https://github.com/radius-project/radius/blob/main/eng/design-notes/templates/feature-spec-template.md
Licensed under the Apache License 2.0.
-->

# Topic: Recipe-backed Dapr extensibility for Radius

* **Author**: Will Tsai (@willtsai)

## Topic Summary

Radius will ship `Radius.Dapr` as default extensible resource types, just like `Radius.Compute/containers`, to replace the legacy `Applications.Dapr` types. Developers will declare the Dapr capabilities their applications need. Platform engineers will use the corresponding platform Recipe Pack to provision those capabilities. Resource type definitions, Recipes, and platform Packs will live in [`radius-project/resource-types-contrib`](https://github.com/radius-project/resource-types-contrib), following the compute-extensibility model. Shipping the types by default does not require hard-coded Dapr provisioning logic.

The MVP covers state stores, pub/sub brokers, secret stores, and configuration stores. Each resource type has four Recipes: local Kubernetes, Amazon Elastic Kubernetes Service (EKS), Azure Kubernetes Service (AKS), and Azure Container Apps (ACA). These 16 Recipes become four Dapr mappings in each corresponding platform Pack: the existing Kubernetes and AKS Packs and the planned EKS and ACA Packs. There are no separate Dapr-only Packs. Each Recipe uses the backing service specified in the requirements matrix below.

### Top level goals

- Let developers use supported Dapr capabilities without writing component types, component versions, provider metadata, or credentials in their application definitions.
- Make all four `Radius.Dapr` types available by default, with no separate type registration. Deliver their Recipes through the platform Packs users already select for their Environments.
- Let platform engineers change component provisioning or add a supported backend through Recipes and Packs without adding Dapr-specific Go provisioning code to Radius.
- Provide all four capabilities on local Kubernetes, EKS, AKS, and ACA. A representative application must perform the documented operation through Dapr for all 16 type-and-target combinations, not merely create component resources.
- Keep the same logical Dapr declarations and consumer connections across all four targets. Environment configuration and backing services may differ; portability does not imply identical backend semantics.

### Non-goals (out of scope)

- Supporting every Dapr component or feature. New resource types for bindings, subscriptions, actors, workflows, resiliency, and service invocation are outside this MVP.
- Installing or upgrading the Dapr runtime, managing its release lifecycle, or moving the Radius control plane off Kubernetes.
- Creating Kubernetes clusters, EKS or AKS clusters, or ACA managed environments. The platform engineer supplies the compute target.
- Guaranteeing portability for provider-specific settings, data formats, consistency guarantees, or unsupported Dapr runtime features.
- Offering multiple default backends for each type-and-target combination. Platform engineers can supply custom Recipes for other backends.
- Automatically moving application data, translating every legacy metadata field, or deleting the legacy resource provider as part of the initial release.
- Introducing a separate Dapr CLI, a new Recipe execution model, or a new Pack registration mechanism.

## User profile and challenges

### User persona(s)

The primary user is an application developer who uses Dapr APIs and wants to declare a dependency without choosing the service that implements it.

The platform engineer selects compute platforms and backing services, controls credentials and access, and publishes approved Recipes and Packs. Resource contributors extend these choices without maintaining a fork of Radius.

### Challenge(s) faced by the user

- **Legacy types cannot be extended independently of Radius.** The legacy `Applications.Dapr` schemas and provisioning behavior are implemented in Radius code. Platform engineers can customize their Recipes, but changing the resource schemas or built-in provisioning logic requires changes to Radius itself. They cannot extend these types through separately defined resource types and Recipes as the new extensibility architecture allows.
- **No ready-to-use extensibility path.** Radius does not yet ship `Radius.Dapr` as default extensible types, and the platform Recipe Packs do not include their Recipes. Dapr users who want to adopt the same extensibility model as compute must build and assemble that support themselves.
- **Infrastructure details in application definitions.** In manual provisioning mode, developers must specify the Dapr component's type, version, and backend metadata. This ties their application definitions to infrastructure choices that the platform team should own.
- **Kubernetes-specific built-in provisioning.** The built-in path creates Kubernetes Dapr Components. ACA requires a different component resource within its managed environment, so platform engineers cannot reuse the built-in Kubernetes provisioning path to deploy those dependencies on ACA.

### Positive user outcome

Developers can deploy an application that reads and writes state, publishes and receives a message, retrieves a permitted secret, or reads configuration through Dapr without authoring the component's provider metadata. Platform engineers can choose the backend and target platform in a Pack, see what it will create, and reuse it across compatible Environments.

Across local Kubernetes, EKS, AKS, and ACA, switching the target Environment does not require rewriting the application's logical Dapr dependency or Dapr API calls. Differences that affect application behavior remain explicit in the Pack's documented capabilities.

## Key scenarios

### Scenario 1: Prepare an Environment for Dapr applications

A platform engineer selects the Kubernetes, EKS, AKS, or ACA platform Pack for an Environment. Alongside its other Recipes, that Pack supplies Recipes for all four default `Radius.Dapr` types with the backing services below. No additional Dapr Pack is needed. The engineer can inspect billable resources and ownership before deploying an application; overlapping mappings or an incompatible target produce an actionable error.

### Scenario 2: Use Dapr without component metadata

A developer declares one of the default `Radius.Dapr` resources, connects a Dapr-enabled workload to it, and deploys to a prepared Environment. The workload receives the component name and successfully performs the documented Dapr operation without the developer installing extra types, supplying provider-specific metadata, or embedding credentials.

### Scenario 3: Move from local Kubernetes to a cloud target

A developer deploys the same logical dependency and consumer connection first to local Kubernetes, then to EKS, AKS, or ACA. The first three targets use Kubernetes Dapr Components; ACA uses components in the workload's managed environment. Each target provisions or connects to the service in the matrix, performs the declared operation, and rejects unsupported settings rather than silently dropping them.

### Scenario 4: Customize a backend without changing Radius

A platform engineer publishes a customized Recipe and Pack for an approved backend. The Pack states whether it creates that service or requires an existing one, which billable services it creates, and which resources Radius will manage. Developers continue to use the supported `Radius.Dapr` contract.

### Scenario 5: Diagnose and recover from a deployment failure

A developer encounters a missing Recipe mapping, an unsupported capability, or a component that cannot access its backend. Diagnostics distinguish these failures and identify the affected resource and corrective action without exposing credentials; after the configuration is corrected, redeployment does not create duplicate components or take ownership of unrelated services.

## Key dependencies and risks

- **Platform Pack and runtime readiness.** Each target needs a platform Pack with compatible compute and Dapr Recipes, plus a Dapr runtime. The inspected [`resource-types-contrib` catalog](https://github.com/radius-project/resource-types-contrib/tree/99db37b326bfcd529ae2b98b11eec2fd289a71c0/recipe-packs) contains Kubernetes, AKS, and ACI Packs, but no EKS or ACA Pack. The new Dapr mappings must be included in the planned EKS and ACA platform Packs. ACA compute support is proposed in [#13233](https://github.com/radius-project/radius/pull/13233).
- **Platform capability differences.** [ACA supports a subset of Dapr components](https://learn.microsoft.com/en-us/azure/container-apps/dapr-overview#supported-dapr-apis-components-and-tooling). Each Pack must document supported capabilities and differences that affect application behavior.
- **Access and secrets.** Components must be available only to authorized workloads. Developers must not need to embed credentials in application definitions.
- **Ownership and data loss.** A Recipe can create a backing service as well as a component. Users must know which resources are Recipe-owned and which are external. Referencing an existing service must not make it eligible for deletion. Changes that replace a data-bearing service require a documented retention and cutover procedure, not an ordinary Pack switch.
- **Distribution and compatibility.** The Radius release must include the four default types and compatible Dapr mappings in its supported platform Packs. Follow the existing [Recipe Pack distribution workflow](../../../docs/contributing/contributing-recipe-packs.md); users must not have to register the types manually or assemble unpublished artifacts.
- **Migration compatibility.** Changing the namespace creates a distinct Radius resource; it does not rename the existing one. Users need a migration path that accounts for component identity, consumer bindings, secret references, and ownership before Radius retires legacy resources.

## Key assumptions to test and questions to answer

- **Existing Dapr runtime:** Users already have their own Dapr installation available on local Kubernetes, EKS, or AKS; ACA uses its managed Dapr runtime. The `Radius.Dapr` types and Recipes do not install, upgrade, or manage Dapr on the user's behalf.
- **Selected service compatibility:** All 16 combinations in the matrix must support the stated Dapr operations. Verify each with a working application; backend selection is not an open question.
- **Developer abstraction:** Can the required scenarios use a small typed contract rather than an unstructured Dapr metadata bag? Walk representative manual-mode and Recipe-mode applications through a prototype, including an unsupported setting. Keep provider-specific configuration with the platform engineer; identify any genuinely application-owned settings.
- **EKS and ACA Pack readiness:** Confirm that the planned platform Packs can deploy complete Dapr applications. Users must not need to deploy workloads manually to make the Dapr Recipes usable.

## Current state

At Radius revision [`9b1c68e`](https://github.com/radius-project/radius/tree/9b1c68e7cf5842175a0d8a48911b8fcd8373668f), the four built-in types are `Applications.Dapr/stateStores`, `pubSubBrokers`, `secretStores`, and `configurationStores`. Their [type definitions](https://github.com/radius-project/radius/blob/9b1c68e7cf5842175a0d8a48911b8fcd8373668f/pkg/daprrp/frontend/controller/types.go#L23-L50) and resource-specific behavior live in Radius, including `pkg/daprrp`.

These types already support Recipe provisioning. [Recipe mode is the default](https://github.com/radius-project/radius/blob/9b1c68e7cf5842175a0d8a48911b8fcd8373668f/pkg/daprrp/api/v20231001preview/datamodel_util.go#L75-L85); manual mode requires component type, version, and metadata, while [Recipe mode rejects those manual fields](https://github.com/radius-project/radius/blob/9b1c68e7cf5842175a0d8a48911b8fcd8373668f/pkg/daprrp/api/v20231001preview/statestore_conversion.go#L47-L85). The [manual processor creates a Kubernetes component](https://github.com/radius-project/radius/blob/9b1c68e7cf5842175a0d8a48911b8fcd8373668f/pkg/daprrp/processors/statestores/processor.go#L42-L116), while Recipe mode delegates provisioning. The limitation is not that Recipes are absent or that every developer must supply metadata.

The `radius-project/recipes` repository already provides local Kubernetes Recipes backed by Redis for [state stores](https://github.com/radius-project/recipes/blob/main/local-dev/statestores.bicep), [pub/sub brokers](https://github.com/radius-project/recipes/blob/main/local-dev/pubsubbrokers.bicep), and [configuration stores](https://github.com/radius-project/recipes/blob/main/local-dev/configurationstores.bicep), and by Kubernetes Secrets for [secret stores](https://github.com/radius-project/recipes/blob/main/local-dev/secretstores.bicep). These provide a starting point for the new local Kubernetes Recipes, but do not supply default `Radius.Dapr` types or their mappings in the Kubernetes platform Pack.

Existing workloads connect by resource ID and use the resulting component name. The [state-store Recipe example](https://github.com/radius-project/radius/blob/9b1c68e7cf5842175a0d8a48911b8fcd8373668f/test/functional-portable/daprrp/noncloud/resources/testdata/daprrp-resources-statestore-recipe.bicep#L39-L72) already separates the application's dependency from its backend configuration.

[Compute extensibility](2025-06-compute-extensibility-feature-spec.md) establishes the direction for moving platform behavior into Recipes. The current [contributed Pack contract](https://github.com/radius-project/resource-types-contrib/blob/99db37b326bfcd529ae2b98b11eec2fd289a71c0/recipe-packs/README.md) maps resource types to Recipes through `Radius.Core/recipePacks`; Environments reference those Packs. ACA exposes its component as [`Microsoft.App/managedEnvironments/daprComponents`](https://learn.microsoft.com/en-us/azure/templates/microsoft.app/managedenvironments/daprcomponents), rather than a Kubernetes resource.

Radius already [registers contributed types such as `Radius.Compute/containers` by default](../../../deploy/manifest/defaults.yaml). The existing [Kubernetes](https://github.com/radius-project/resource-types-contrib/tree/main/recipe-packs/kubernetes) and [AKS](https://github.com/radius-project/resource-types-contrib/tree/main/recipe-packs/azure-aks) platform Packs include compute and other service Recipes. This feature follows the same distribution model for `Radius.Dapr`.

The [pending ACA and ECS specification (#13233)](https://github.com/radius-project/radius/pull/13233) proposes Recipe Packs that map containers, routes, secrets, and persistent volumes to ACA and Amazon ECS on Fargate. The ACA Dapr Recipes will be included in that planned ACA platform Pack.

## Details of user problem

As a developer, I want to ask for a state store or message broker and use it through Dapr. When I have to describe a component's backend metadata, my application definition becomes tied to infrastructure choices that my platform team owns. I cannot assume that the same component configuration will work when that team chooses ACA instead of Kubernetes.

As a platform engineer, I want to choose how those components are created and secured. I need to know whether a Recipe will create a service, reuse an existing one, or delete something that holds data. I want to distribute those choices through the same Recipe Pack model I use for compute, without adding another resource-specific provisioning path to Radius.

## Desired user experience outcome

As a developer, I can use the default `Radius.Dapr` types to declare a dependency, connect my workload, and use the returned component name through Dapr. I can deploy the same application contract to local Kubernetes, EKS, AKS, or ACA without installing extra types or writing the platform's component resource.

As a platform engineer, I can select a platform Pack that includes the Dapr Recipes, customize those Recipes, and make the backend's access and ownership rules visible to my teams. I can introduce the new model without silently changing existing applications.

### Detailed user experience

The matrix defines the required backing service for each default resource type and target. Each cell represents one Recipe, for a total of 16 Recipes added to the four corresponding platform Packs.

| Resource type                     | Local Kubernetes   | AWS EKS                          | Azure AKS                | Azure ACA                |
|-----------------------------------|--------------------|----------------------------------|--------------------------|--------------------------|
| `Radius.Dapr/stateStores`         | Redis              | Amazon ElastiCache for Redis OSS | Azure Managed Redis      | Azure Managed Redis      |
| `Radius.Dapr/pubSubBrokers`       | Redis Streams      | Amazon SNS + SQS                 | Azure Service Bus Topics | Azure Service Bus Topics |
| `Radius.Dapr/secretStores`        | Kubernetes Secrets | AWS Secrets Manager              | Azure Key Vault          | Azure Key Vault          |
| `Radius.Dapr/configurationStores` | Redis              | Amazon ElastiCache for Redis OSS | Azure Managed Redis      | Azure Managed Redis      |

The technical design will define authentication, networking, service sizing, and provisioning details.

1. **Choose the platform and Pack.** I select the local Kubernetes, EKS, AKS, or ACA platform Pack for my Environment. It includes mappings for all four `Radius.Dapr` types alongside its other resources. I inspect its prerequisites, billable resources, and deletion behavior.

2. **Prepare the Environment.** I confirm that Dapr is already available on my target; selecting the Pack does not install it. The four `Radius.Dapr` types are already available in a Radius release that supports this feature. I use the existing Environment and Recipe Pack experience to select a compatible platform Pack; I do not register Dapr types or attach a Dapr-only Pack. The Kubernetes default Pack includes the local Dapr Recipes. A missing type or mapping produces an actionable version or Pack compatibility error.

3. **Configure the Pack.** As a platform engineer, I supply its documented prerequisites. The Pack uses the backing services in the matrix. I can publish a custom Recipe to change those defaults; the standard application path needs no backend credentials or provider metadata.

4. **Declare the dependency.** As a developer, I declare the capability and its application association. The proposed resource types are:

   | Resource type                     | Developer intent                            | Observable MVP behavior                               |
   |-----------------------------------|---------------------------------------------|-------------------------------------------------------|
   | `Radius.Dapr/stateStores`         | Persist application state through Dapr      | Write and retrieve a value                            |
   | `Radius.Dapr/pubSubBrokers`       | Exchange messages through Dapr              | Publish a message and receive it in a subscriber      |
   | `Radius.Dapr/secretStores`        | Retrieve an authorized secret through Dapr  | Retrieve a permitted secret; deny unauthorized access |
   | `Radius.Dapr/configurationStores` | Read application configuration through Dapr | Retrieve a configured value                           |

   This illustrative Bicep is not a released schema or runnable sample. `<version>` and the final application-facing properties remain to be defined. The developer declares a dependency without describing its component internals.

   ```bicep
   param environmentId string
   param applicationId string

   resource state 'Radius.Dapr/stateStores@<version>' = {
     name: 'cart-state'
     properties: {
       environment: environmentId
       application: applicationId
     }
   }
   ```

5. **Connect and enable Dapr for the workload.** I connect the workload to the resource by ID using the compute resource's connection model, and configure its Dapr app ID through the compute Recipe's supported Dapr settings. Each `Radius.Dapr` resource exposes a read-only `componentName` for its consumers. The workload receives that name through the documented connection binding on all four targets; component credentials are not exposed as ordinary connection outputs. I do not have to infer the name from a Kubernetes object or Azure resource ID. Declaring a Dapr dependency does not enable the workload's sidecar.

6. **Deploy and exercise the capability.** The selected Recipe creates the platform-native component and provisions or connects to the service in the matrix. The component and workload use the same target namespace or ACA managed environment, with access limited to the intended consumers. Each Pack's sample performs all four operations in the table through the workload's Dapr runtime, covering all 16 Recipes. Successful provisioning is distinct from a successful runtime call; setup guidance covers both.

7. **Inspect failures and retry.** Deployment errors identify the resource, selected Recipe or missing mapping, and the rejected property or failed operation. Runtime guidance distinguishes missing Dapr enablement, component initialization, network access, and backend authentication. Sensitive values stay out of logs and normal outputs. Reapplying unchanged input preserves component identity and does not create duplicate backing services. A conflicting component owned by another deployment produces an ownership error rather than an overwrite.

8. **Update, migrate, or remove deliberately.** A Pack update does not itself move an application to a different backend or transfer its data. Documentation explains when existing resources use an updated Recipe and which changes require redeployment or a new resource. For legacy adoption, I follow a compatibility example and verify consumer bindings before cutover. Deleting a component preserves external services and follows the documented policy for Recipe-owned resources; failed cleanup remains visible with recovery guidance. Removing a Pack is not a shortcut for deleting its applications or data.

## Key investments

### Feature 1

Ship the four `Radius.Dapr` resource types by default, using the same extensible resource-type model as `Radius.Compute/containers`. Define typed application-facing properties and a consistent component-name connection contract. Users do not need to register the types separately.

### Feature 2

Provide 16 Recipes: one per resource type for local Kubernetes, EKS, AKS, and ACA. Each Recipe creates the target's Dapr component and provisions or connects to the service in the matrix.

### Feature 3

Add the four corresponding Dapr Recipe mappings to each platform Pack in `resource-types-contrib`: the existing [Kubernetes](https://github.com/radius-project/resource-types-contrib/tree/main/recipe-packs/kubernetes) and [AKS](https://github.com/radius-project/resource-types-contrib/tree/main/recipe-packs/azure-aks) Packs and the planned EKS and ACA Packs. Preserve their other resource mappings and document the supported versions, service defaults, and application examples. Do not create separate Dapr-only Packs.
