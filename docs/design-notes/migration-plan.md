# Design Notes Migration Plan

This document reviews all design documents in the [radius-project/design-notes](https://github.com/radius-project/design-notes) repository and recommends which should be migrated to the main Radius repository. It also proposes an improved directory structure for the migrated documents.

## Migration Criteria

Documents are recommended for migration if they meet **all** of the following:

1. **Implemented** — The design has been implemented in the current codebase
2. **Relevant** — The design describes functionality that exists and is actively maintained
3. **Not Applications.Core** — The design is not about `Applications.Core/*` resource types or the Applications RP

Documents are **excluded** from migration if any of the following apply:

- **Aspirational / Not Yet Implemented** — The design describes future plans with no corresponding code
- **Out of Date** — The design was never implemented or has been superseded
- **Applications.Core Related** — The design is about `Applications.Core/*`, `Applications.Dapr/*`, `Applications.Datastores/*`, or `Applications.Messaging/*` resource types managed by the Applications RP (`pkg/corerp/`, `pkg/daprrp/`, `pkg/datastoresrp/`, `pkg/messagingrp/`)
- **Operational / Templates** — The design is a template, test plan, or project management artifact

## Proposed Directory Structure

The migrated documents should be organized by topic area within `docs/design-notes/` using a flat-but-grouped structure:

```text
docs/
├── architecture/                          # Existing — keep as-is
│   ├── deployment-engine.md
│   └── state-persistence.md
├── design-notes/
│   ├── README.md                          # Index linking to all design notes
│   ├── architecture/                      # System-level architecture decisions
│   │   ├── 2023-06-arch-vnext.md
│   │   ├── 2023-10-kubernetes-integration.md
│   │   ├── 2024-05-radius-on-dapr.md
│   │   └── 2025-03-upgrade-design.md
│   ├── cli/                               # CLI-specific designs
│   │   ├── 2024-04-azure-workload-identity.md
│   │   └── 2024-06-aws-irsa-support.md
│   ├── extensibility/                     # Resource extensibility, UDTs, and compute extensibility
│   │   ├── 2024-06-resource-extensibility-feature-spec.md
│   │   ├── 2024-07-user-defined-types.md
│   │   ├── 2024-07-user-defined-types-schema-design.md
│   │   ├── 2024-08-resource-types-registration.md
│   │   ├── 2025-02-user-defined-resource-type-feature-spec.md
│   │   ├── 2025-04-compute-extensibility.md
│   │   ├── 2025-06-compute-extensibility-feature-spec.md
│   │   ├── 2025-08-container-resource-type.md
│   │   └── 2025-09-routes-resource-type.md
│   ├── gitops/                            # GitOps integration designs
│   │   ├── 2024-06-gitops-feature-spec.md
│   │   ├── 2024-10-deploymenttemplate-controller.md
│   │   └── 2025-01-gitops-technical-design.md
│   ├── guides/                            # Living design guidelines
│   │   └── api-design-guidelines.md
│   ├── recipes/                           # Recipe engine and providers
│   │   ├── 2023-07-terraform-template-version.md
│   │   ├── 2023-08-garbage-collection.md
│   │   ├── 2023-09-populate-terraform-resource-ids.md
│   │   ├── 2023-11-support-insecure-registries.md
│   │   ├── 2023-11-validate-template-path.md
│   │   ├── 2024-01-support-private-terraform-repository.md
│   │   ├── 2024-02-terraform-providers.md
│   │   ├── 2024-04-terraform-provider-secrets.md
│   │   ├── 2024-06-private-bicep-registries.md
│   │   ├── 2025-08-recipe-packs.md
│   │   └── 2025-09-container-recipe.md
│   ├── security/                          # Threat models and security designs
│   │   ├── 2024-08-controller-component-threat-model.md
│   │   ├── 2024-11-ucp-component-threat-model.md
│   │   └── 2025-11-secrets-redactdata.md
│   ├── tools/                             # Engineering tools and workflows
│   │   ├── 2023-12-test-organization.md
│   │   └── 2025-03-workflow-changes.md
│   └── ucp/                               # Universal Control Plane designs
│       └── 2024-03-planes-apis.md
├── contributing/                          # Existing — keep as-is
├── release-notes/                         # Existing — keep as-is
└── ucp/                                   # Existing — keep as-is
```

### Rationale for Structure Changes

| Change | Rationale |
|--------|-----------|
| Group by topic, not by original repo directory | The original repo mixed concerns (e.g., `resources/` contained both Applications.Core and recipe designs). Topic-based grouping makes documents easier to find. |
| Separate `security/` directory | Threat models and security designs (including sensitive data redaction) are cross-cutting concerns that benefit from being co-located rather than scattered across architecture/ or component-specific directories. |
| Separate `extensibility/` directory | User-defined types, resource extensibility, and compute extensibility (recipe-backed core resource types) are a cohesive feature area covering how Radius supports custom and platform-specific resource types. |
| Separate `gitops/` directory | GitOps spans the DeploymentTemplate controller and the overall feature spec; grouping them clarifies the feature area. |
| `guides/` for living documents | API design guidelines and similar living documents are distinct from point-in-time design notes. |
| Keep existing `docs/architecture/` intact | The existing architecture docs (`deployment-engine.md`, `state-persistence.md`) are current reference documentation, not historical design notes. |

---

## Documents Recommended for Migration

### Architecture (4 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`architecture/2023-06-arch-vnext.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2023-06-arch-vnext.md) | Proposes vNext service architecture to address in-memory data store issues in the Deployment Engine, simplify building new resource providers, and add resource change notification. | Radius uses Dapr for data/queue (`pkg/components/`), the `armrpc` package provides an RP programming model (`pkg/armrpc/`). |
| [`architecture/2023-10-kubernetes-integration.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2023-10-kubernetes-integration.md) | Proposes a Kubernetes-first adoption strategy, focusing on ease-of-adoption by integrating with existing K8s tooling rather than requiring full application model rewrite. | Radius has deep K8s integration via the controller (`pkg/controller/`), CRDs, and K8s-native workflows. |
| [`architecture/2024-05-radius-on-dapr.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-05-radius-on-dapr.md) | Proposes replacing Radius plumbing with Dapr for state store, pub/sub, and workflows, taking an install-time dependency on Dapr. | Radius requires Dapr; state store and queue use Dapr building blocks (`pkg/components/database/`, `pkg/components/queue/`). |
| [`architecture/2025-03-upgrade-design-doc.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2025-03-upgrade-design-doc.md) | Designs in-place upgrades for the Radius control plane via `rad upgrade kubernetes`. | `cmd/pre-upgrade/` exists; Helm-based upgrade path is implemented (`pkg/cli/helm/`). |

### CLI (2 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`cli/2024-04-azure-workload-identity.md`](https://github.com/radius-project/design-notes/blob/main/cli/2024-04-azure-workload-identity.md) | Enables Azure workload identity (federated identity) for Radius to deploy and manage Azure resources without client secrets. Status: **Approved**. | Azure credential management exists in `pkg/ucp/credentials/`, `pkg/cli/azure/`. |
| [`cli/2024-06-04-aws-irsa-support.md`](https://github.com/radius-project/design-notes/blob/main/cli/2024-06-04-aws-irsa-support.md) | Enables AWS IRSA (IAM Roles for Service Accounts) for Radius to deploy and manage AWS resources without static access keys. | AWS credential management exists in `pkg/ucp/credentials/`, `pkg/cli/aws/`. |

### Extensibility (9 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`features/2024-06-resource-extensibility-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2024-06-resource-extensibility-feature-spec.md) | Feature spec for user-defined resource types (UDTs), enabling users to define custom resource types beyond built-in types. | Dynamic RP (`pkg/dynamicrp/`, `cmd/dynamic-rp/`) handles user-defined types. |
| [`architecture/2024-07-user-defined-types.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-07-user-defined-types.md) | High-level architecture for user-defined types, including dynamic and declarative implementation approach. | Dynamic RP implements declarative UDT processing. |
| [`architecture/2024-07-user-defned-types-schema-design.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-07-user-defned-types-schema-design.md) | Defines the OpenAPI subset supported for UDT schemas and validation rules. | Schema validation for resource type manifests exists in the codebase. |
| [`architecture/2024-08-resource-types-registration.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-resource-types-registration.md) | Detailed API design for resource type registration (resource providers, types, locations, API versions). | Registration APIs exist in UCP (`pkg/ucp/`); `rad resource-type create` CLI command exists. |
| [`features/2025-02-user-defined-resource-type-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-02-user-defined-resource-type-feature-spec.md) | Comprehensive feature spec for user-defined resource types, covering the full user experience. | UDT feature is implemented and actively used via `pkg/dynamicrp/` and `pkg/cli/manifest/`. |
| [`architecture/2025-04-compute-extensibility.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2025-04-compute-extensibility.md) | Designs extensible support for multiple compute platforms through recipes rather than hard-coded support; core resource types (`containers`, `gateways`, `secretStores`) allow recipe registration. | `Radius.Compute/containers` and routes are defined in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`; recipe-backed provisioning is active. |
| [`features/2025-06-compute-extensibility-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-06-compute-extensibility-feature-spec.md) | Feature spec for recipe-backed core resource types, decoupling Radius core logic from platform-specific provisioning code. | Implemented via `Radius.Compute/containers`, `Radius.Compute/routes`, and `Radius.Compute/persistentVolumes` in the compute provider manifest. |
| [`features/2025-08-29-container-resource-type.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-08-29-container-resource-type.md) | Defines version two of the Containers Resource Type (`Radius.Compute/containers`) with multi-container support, Kubernetes-first design, and recipe-backed provisioning. | Full schema in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`; environment recipe parameters reference `Radius.Compute/containers`. |
| [`features/2025-09-02-routes-resource-type.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-09-02-routes-resource-type.md) | Proposes the Routes resource type replacing Gateways, removing the Contour dependency and enabling recipe-backed L7 ingress. | Routes defined in `deploy/manifest/built-in-providers/dev/radius_compute.yaml` as a recipe-backed resource type. |

### GitOps (3 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`features/2024-06-gitops-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2024-06-gitops-feature-spec.md) | Feature spec for integrating Radius with GitOps tools (Flux, ArgoCD) via the DeploymentTemplate controller. | DeploymentTemplate CRD and controller exist in `pkg/controller/reconciler/`. |
| [`architecture/2024-10-deploymenttemplate-controller.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-10-deploymenttemplate-controller.md) | Design for the DeploymentTemplate Kubernetes controller that deploys Bicep manifests using K8s tooling. | Controller is implemented in `pkg/controller/reconciler/`; CRDs are defined. |
| [`tools/2025-01-gitops-technical-design.md`](https://github.com/radius-project/design-notes/blob/main/tools/2025-01-gitops-technical-design.md) | Technical design for the Radius Flux Controller that watches Flux GitRepository sources and reconciles Bicep deployments. | Flux controller implemented in `pkg/controller/reconciler/flux_controller.go` with GitRepository predicate in `pkg/controller/reconciler/flux_gitrepository_predicate.go`; functional tests in `test/functional-portable/kubernetes/noncloud/flux_test.go`. |

### Recipes (11 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`recipe/2023-07-terraform-template-version.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-07-terraform-template-version.md) | Makes Terraform recipe version optional for non-registry module sources (HTTP URLs, etc.). | Terraform recipe driver handles various module sources in `pkg/recipes/driver/terraform/`. |
| [`recipe/2023-08-garbage-collection.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-08-garbage-collection.md) | Moves recipe resource garbage collection from portable resource controllers to the per-recipe driver abstraction. | Recipe engine and drivers implement GC in `pkg/recipes/engine/`, `pkg/recipes/driver/`. |
| [`recipe/2023-09-populate-terraform-resourcs-ids.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-09-populate-terraform-resourcs-ids.md) | Populates Terraform recipe output with resource IDs by parsing the Terraform state file. | Terraform state parsing exists in `pkg/recipes/driver/terraform/`. |
| [`recipe/2023-11-support-insecure-registries.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-11-support-insecure-registries.md) | Adds `plainHttp` support for publishing and pulling Bicep recipes from insecure (non-TLS) registries. | `plainHttp` property exists in Bicep recipe configuration. |
| [`recipe/2023-11-validate-template-path.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-11-validate-template-path.md) | Adds validation for Terraform recipe template paths, rejecting unsupported module sources with clear error messages. | Template path validation exists in `pkg/recipes/`. |
| [`recipe/2024-01-support-private-terraform-repository.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-01-support-private-terraform-repository.md) | Adds authentication support for Terraform modules from private Git repositories. | `recipeConfig.terraform.authentication.git` support exists in `pkg/recipes/`. |
| [`recipe/2024-02-terraform-providers.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-02-terraform-providers.md) | Enables multiple Terraform provider configurations (including non-default providers and aliases) available to all recipes in an environment. | Provider configuration handling exists in `pkg/recipes/driver/terraform/`. |
| [`recipe/2024-04-terraform-provider-secrets.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-04-terraform-provider-secrets.md) | Details the handling of secrets data between the recipe engine and Terraform driver for provider configurations. | Secrets handling for Terraform providers is implemented in `pkg/recipes/`. |
| [`recipe/2024-06-private-bicep-registries.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-06-private-bicep-registries.md) | Adds authentication support for Bicep recipes stored in private OCI-compliant registries. Status: **Approved**. | Private registry authentication exists in `pkg/recipes/driver/bicep/`. |
| [`recipe/2025-08-recipe-packs.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2025-08-recipe-packs.md) | Designs Recipe Packs as a first-class resource type, enabling bundling of multiple recipe selections into a reusable unit referenced by environments. | RecipePack TypeSpec in `typespec/Radius.Core/recipePacks.tsp`; controller in `pkg/corerp/frontend/controller/recipepacks/`. |
| [`recipe/2025-09-container.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2025-09-container.md) | Replaces the imperative Go renderer chain for containers with a Bicep recipe for the `Radius.Compute/containers` resource type. | `Radius.Compute/containers` is recipe-backed in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`. |

### Security (3 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`architecture/2024-08-controller-component-threat-model.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-controller-component-threat-model.md) | Threat model for the Radius Controller component (Recipe and Deployment controllers, validating webhook). | Controller exists at `pkg/controller/`, `cmd/controller/`. |
| [`architecture/2024-11-ucp-component-threat-model.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-11-ucp-component-threat-model.md) | Threat model for the UCP component (proxy, credential storage, resource routing). | UCP exists at `pkg/ucp/`, `cmd/ucpd/`. |
| [`resources/2025-11-11-secrets-redactdata.md`](https://github.com/radius-project/design-notes/blob/main/resources/2025-11-11-secrets-redactdata.md) | Designs sensitive data redaction for `Radius.Security/secrets`, ensuring secret values are not exposed through API responses or logs. | `Radius.Security/secrets` resource type defined in `deploy/manifest/built-in-providers/dev/radius_security.yaml` with full schema for secret data storage and referencing. |

### Tools (2 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`tools/2023-12-test-organization.md`](https://github.com/radius-project/design-notes/blob/main/tools/2023-12-test-organization.md) | Proposes functional test directory structure separating cloud vs. non-cloud tests, portable vs. platform-specific. | Test directory structure at `test/functional-portable/` matches this proposal. |
| [`tools/2025-03-workflow-changes.md`](https://github.com/radius-project/design-notes/blob/main/tools/2025-03-workflow-changes.md) | Design principles for GitHub workflows: testable from forks, testable on developer machines, no logic duplication. | Workflow practices described here are reflected in `.github/workflows/` and the project's Makefile-based approach. |

### UCP (1 document)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`ucp/2024-03-planes-apis.md`](https://github.com/radius-project/design-notes/blob/main/ucp/2024-03-planes-apis.md) | Redesigns the UCP planes API to use separate resource types per plane type instead of a single shared resource type. | Planes API implementation exists in `pkg/ucp/`. |

### Guides (1 document)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`guide/api-design-guidelines.md`](https://github.com/radius-project/design-notes/blob/main/guide/api-design-guidelines.md) | Prescriptive API design guidelines for Radius contributors, currently focused on secrets handling. Living document. | Guidelines are actively referenced in design reviews and API development. |

**Total: 36 documents recommended for migration.**

---

## Documents NOT Recommended for Migration

### Aspirational / Not Yet Implemented (8 documents)

These documents describe future plans with no corresponding implementation in the current codebase:

| Document | Reason for Exclusion |
|----------|---------------------|
| `architecture/2025-04-aci-support.md` | ACI integration is not implemented; no ACI-specific code exists. |
| `architecture/2025-10-terraform-bicep-settings.md` | `Radius.Core/terraformSettings` and `Radius.Core/bicepSettings` resources do not exist yet. |
| `features/2024-11-authz-feature-spec.md` | Authorization/RBAC is not implemented. The UCP threat model confirms no RBAC exists. |
| `features/2025-01-serverless-feature-spec.md` | Serverless platform support (ACI, ECS, ACA) is not implemented. |
| `features/2025-07-10-offline-install-feature-spec.md` | Offline/air-gapped installation is not implemented. |
| `features/2025-07-23-radius-configuration-ux.md` | Configuration UX modeling configs as resources is not implemented. |
| `features/2025-07-radius-resource-types-contribution.md` | Community contribution model for resource types is not implemented. |
| `features/2025-08-14-terraform-bicep-settings.md` | Terraform/Bicep settings refactoring feature spec is not implemented. |

### Applications.Core Related (12 documents)

These documents are about `Applications.Core/*` or other `Applications.*` resource types managed by the Applications RP:

| Document | Reason for Exclusion |
|----------|---------------------|
| `architecture/2024-08-applications-rp-component-threat-model.md` | Threat model for the Applications RP covering `Applications.Core`, `Applications.Dapr`, `Applications.Datastores`, and `Applications.Messaging`. |
| `architecture/2024-08-dashboard-component-threat-model.md` | Threat model for the Dashboard component (separate repository, tied to Applications RP). |
| `features/2024-07-secretstore-feature-spec.md` | Feature spec for extending `Applications.Core/secretStores` use cases. |
| `recipe/2024-01-global-scope-secret-store.md` | Extends `Applications.Core/secretStores` to global scope. |
| `resources/2023-04-tls-termination.md` | TLS termination for `Applications.Core/gateways`. |
| `resources/2023-07-fail-deployments.md` | Container deployment failure classification for `Applications.Core/containers`. |
| `resources/2023-10-app-graph.md` | Application graph API for `Applications.Core` resource provider. |
| `resources/2023-10-recipe-details.md` | Adding recipe information to `Applications.Core/*` resource data models. |
| `resources/2023-10-simulated-environment.md` | Simulated environment flag on `Applications.Core/environments`. |
| `resources/2024-06-support-secretstores-env.md` | Secret store references in `Applications.Core/containers` environment variables. |
| `resources/2024-10-dapr-bindings.md` | Dapr Bindings implementation for `Applications.Dapr/bindings`. |
| `resources/2025-01-gateway-timeouts.md` | Configurable timeouts for `Applications.Core/gateways`. |

### Operational / Templates (4 items)

| Document | Reason for Exclusion |
|----------|---------------------|
| `template/YYYY-MM-design-template.md` | Design document template — not a design note itself. |
| `template/YYYY-MM-feature-spec-template.md` | Feature spec template. |
| `template/YYYY-MM-threat-model-template.md` | Threat model template. |
| `recipe/2023-08-recipes-test-plan.md` | Test plan from early Terraform recipe development; the tests themselves exist in the codebase. |

### Spec Kit Specifications (2 items)

| Document | Reason for Exclusion |
|----------|---------------------|
| `specs/001-lrt-current-release/` | Spec Kit project management artifacts (plans, tasks, checklists) — not architectural design notes. |
| `specs/001-remove-bicep-types-submodule/` | Spec Kit project management artifacts for a specific engineering task. |

### Empty Sections (1 item)

| Document | Reason for Exclusion |
|----------|---------------------|
| `bicep/README.md` | Directory contains only a README with no design documents. |

**Total: 27 documents/items not recommended for migration.**
