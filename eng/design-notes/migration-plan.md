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

## Proposed Directory Structure

The migrated documents should be organized by topic area within `eng/design-notes/` using a flat-but-grouped structure.

### Naming Convention

Migrated files use a `YYYY-MM-description.md` naming convention (year and month only). Source files that include a day component (e.g., `2025-08-29-container-resource-type.md`) are renamed to drop the day (e.g., `2025-08-container-resource-type.md`). Living documents such as `api-design-guidelines.md` omit the date prefix entirely.

```text
.specify/                                      # Spec Kit configuration, scripts, and templates
├── init-options.json
├── memory/
│   └── constitution.md
├── scripts/
│   ├── bash/
│   │   ├── check-prerequisites.sh
│   │   ├── common.sh
│   │   ├── create-new-feature.sh
│   │   ├── setup-plan.sh
│   │   └── update-agent-context.sh
│   └── powershell/
│       ├── check-prerequisites.ps1
│       ├── common.ps1
│       ├── create-new-feature.ps1
│       ├── setup-plan.ps1
│       └── update-agent-context.ps1
└── templates/
    ├── agent-file-template.md
    ├── checklist-template.md
    ├── constitution-template.md
    ├── plan-template.md
    ├── spec-template.md
    └── tasks-template.md

eng/
├── design-notes/
│   ├── README.md                          # Instructions for adding documents
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
│   │   ├── 2025-09-routes-resource-type.md
│   │   └── 2025-07-resource-types-contribution.md
│   ├── gitops/                            # GitOps integration designs
│   │   ├── 2024-06-gitops-feature-spec.md
│   │   ├── 2024-10-deploymenttemplate-controller.md
│   │   └── 2025-01-gitops-technical-design.md
│   ├── guides/                            # Living design guidelines
│   │   └── api-design-guidelines.md
│   ├── recipes/                           # Recipe engine and providers
│   │   ├── 2023-07-terraform-template-version.md
│   │   ├── 2023-08-garbage-collection.md
│   │   ├── 2023-09-populate-terraform-resourcs-ids.md
│   │   ├── 2023-11-support-insecure-registries.md
│   │   ├── 2023-11-validate-template-path.md
│   │   ├── 2024-01-support-private-terraform-repository.md
│   │   ├── 2024-02-terraform-providers.md
│   │   ├── 2024-04-terraform-provider-secrets.md
│   │   ├── 2024-06-private-bicep-registries.md
│   │   ├── 2025-08-recipe-packs.md
│   │   └── 2025-09-container-recipe.md
│   ├── security/                          # Threat models and security designs
│   │   ├── 2024-08-applications-rp-component-threat-model.md
│   │   ├── 2024-08-controller-component-threat-model.md
│   │   ├── 2024-08-dashboard-component-threat-model.md
│   │   ├── 2024-11-ucp-component-threat-model.md
│   │   └── 2025-11-secrets-redactdata.md
│   ├── templates/                         # Design document templates
│   │   ├── design-template.md
│   │   ├── feature-spec-template.md
│   │   └── threat-model-template.md
│   ├── tools/                             # Engineering tools and workflows
│   │   ├── 2023-12-test-organization.md
│   │   └── 2025-03-workflow-changes.md
│   └── ucp/                               # Universal Control Plane designs
│       └── 2024-03-planes-apis.md

specs/                                         # Spec Kit specifications
├── 001-lrt-current-release/
└── 001-remove-bicep-types-submodule/
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
| [`architecture/2025-03-upgrade-design-doc.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2025-03-upgrade-design-doc.md) | Designs in-place upgrades for the Radius control plane via `rad upgrade kubernetes` (migrated as `eng/design-notes/architecture/2025-03-upgrade-design.md`). | `cmd/pre-upgrade/` exists; Helm-based upgrade path is implemented (`pkg/cli/helm/`). |

### CLI (2 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`cli/2024-04-azure-workload-identity.md`](https://github.com/radius-project/design-notes/blob/main/cli/2024-04-azure-workload-identity.md) | Enables Azure workload identity (federated identity) for Radius to deploy and manage Azure resources without client secrets. Status: **Approved**. | Azure credential management exists in `pkg/ucp/credentials/`, `pkg/cli/azure/`. |
| [`cli/2024-06-04-aws-irsa-support.md`](https://github.com/radius-project/design-notes/blob/main/cli/2024-06-04-aws-irsa-support.md) | Enables AWS IRSA (IAM Roles for Service Accounts) for Radius to deploy and manage AWS resources without static access keys. | AWS credential management exists in `pkg/ucp/credentials/`, `pkg/cli/aws/`. |

### Extensibility (10 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`features/2024-06-resource-extensibility-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2024-06-resource-extensibility-feature-spec.md) | Feature spec for user-defined resource types (UDTs), enabling users to define custom resource types beyond built-in types. | Dynamic RP (`pkg/dynamicrp/`, `cmd/dynamic-rp/`) handles user-defined types. |
| [`architecture/2024-07-user-defined-types.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-07-user-defined-types.md) | High-level architecture for user-defined types, including dynamic and declarative implementation approach. | Dynamic RP implements declarative UDT processing. |
| [`architecture/2024-07-user-defined-types-schema-design.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-07-user-defined-types-schema-design.md) | Defines the OpenAPI subset supported for UDT schemas and validation rules. | Schema validation for resource type manifests exists in the codebase. |
| [`architecture/2024-08-resource-types-registration.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-resource-types-registration.md) | Detailed API design for resource type registration (resource providers, types, locations, API versions). | Registration APIs exist in UCP (`pkg/ucp/`); `rad resource-type create` CLI command exists. |
| [`features/2025-02-user-defined-resource-type-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-02-user-defined-resource-type-feature-spec.md) | Comprehensive feature spec for user-defined resource types, covering the full user experience. | UDT feature is implemented and actively used via `pkg/dynamicrp/` and `pkg/cli/manifest/`. |
| [`architecture/2025-04-compute-extensibility.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2025-04-compute-extensibility.md) | Designs extensible support for multiple compute platforms through recipes rather than hard-coded support; core resource types (`containers`, `gateways`, `secretStores`) allow recipe registration. | `Radius.Compute/containers` and routes are defined in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`; recipe-backed provisioning is active. |
| [`features/2025-06-compute-extensibility-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-06-compute-extensibility-feature-spec.md) | Feature spec for recipe-backed core resource types, decoupling Radius core logic from platform-specific provisioning code. | Implemented via `Radius.Compute/containers`, `Radius.Compute/routes`, and `Radius.Compute/persistentVolumes` in the compute provider manifest. |
| [`features/2025-08-29-container-resource-type.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-08-29-container-resource-type.md) | Defines version two of the Containers Resource Type (`Radius.Compute/containers`) with multi-container support, Kubernetes-first design, and recipe-backed provisioning. | Full schema in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`; environment recipe parameters reference `Radius.Compute/containers`. |
| [`features/2025-09-02-routes-resource-type.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-09-02-routes-resource-type.md) | Proposes the Routes resource type replacing Gateways, removing the Contour dependency and enabling recipe-backed L7 ingress. | Routes defined in `deploy/manifest/built-in-providers/dev/radius_compute.yaml` as a recipe-backed resource type. |
| [`features/2025-07-radius-resource-types-contribution.md`](https://github.com/radius-project/design-notes/blob/main/features/2025-07-radius-resource-types-contribution.md) | Feature spec defining the experience and pathways for community members to contribute new resource types and Recipes to the Radius ecosystem, including maturity levels (Alpha, Beta, Stable). | Migrated as `extensibility/2025-07-resource-types-contribution.md`. |

### GitOps (3 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`features/2024-06-gitops-feature-spec.md`](https://github.com/radius-project/design-notes/blob/main/features/2024-06-gitops-feature-spec.md) | Feature spec for integrating Radius with GitOps tools (Flux, ArgoCD) via the DeploymentTemplate controller. | DeploymentTemplate CRD and controller exist in `pkg/controller/reconciler/`. |
| [`architecture/2024-10-deploymenttemplate-controller.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-10-deploymenttemplate-controller.md) | Design for the DeploymentTemplate Kubernetes controller that deploys Bicep manifests using K8s tooling. | Controller is implemented in `pkg/controller/reconciler/`; CRDs are defined. |
| [`tools/2025-01-gitops-technical-design.md`](https://github.com/radius-project/design-notes/blob/main/tools/2025-01-gitops-technical-design.md) | Technical design for the Radius Flux Controller that watches Flux GitRepository sources and reconciles Bicep deployments. | Flux controller implemented in `pkg/controller/reconciler/flux_controller.go` with GitRepository predicate in `pkg/controller/reconciler/flux_gitrepository_predicate.go`; functional tests in `test/functional-portable/kubernetes/noncloud/flux_test.go`. |

### Security (5 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`architecture/2024-08-controller-component-threat-model.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-controller-component-threat-model.md) | Threat model for the Radius Controller component (Recipe and Deployment controllers, validating webhook). | Controller exists at `pkg/controller/`, `cmd/controller/`. |
| [`architecture/2024-08-applications-rp-component-threat-model.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-applications-rp-component-threat-model.md) | Threat model for the Applications RP component, covering resource lifecycle management, cloud credential access, and recipe execution security. | Applications RP exists at `pkg/corerp/`, `cmd/applications-rp/`. |
| [`architecture/2024-08-dashboard-component-threat-model.md`](https://github.com/radius-project/design-notes/blob/main/architecture/2024-08-dashboard-component-threat-model.md) | Threat model for the Radius Dashboard component (Backstage-based), covering the frontend SPA, backend plugin, and Radius API client security. | Dashboard exists in the `dashboard` repository; Radius plugin integrates via the Radius API. |
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
| [`guides/api-design-guidelines.md`](https://github.com/radius-project/design-notes/blob/main/guides/api-design-guidelines.md) | Prescriptive API design guidelines for Radius contributors, currently focused on secrets handling. Living document. | Guidelines are actively referenced in design reviews and API development. |

### Recipes (11 documents)

| Document | Summary | Evidence of Implementation |
|----------|---------|---------------------------|
| [`recipe/2023-07-terraform-template-version.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-07-terraform-template-version.md) | Makes Terraform recipe `templateVersion` optional for non-registry sources while preserving versioned registry modules. | Terraform config generation handles omitted versions in `pkg/recipes/terraform/config/config.go`; tests cover the empty-version case in `pkg/recipes/terraform/config/config_test.go`. |
| [`recipe/2023-08-garbage-collection.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-08-garbage-collection.md) | Moves Bicep recipe garbage collection into the recipe execution path by comparing prior output resources with the current deployment output. | `pkg/recipes/engine/types.go` carries previous state into execution; `pkg/recipes/driver/bicep/bicep.go` deletes obsolete output resources; `pkg/recipes/errorcodes.go` defines `RecipeGarbageCollectionFailed`. |
| [`recipe/2023-09-populate-terraform-resourcs-ids.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-09-populate-terraform-resourcs-ids.md) | Populates Terraform recipe output resources from Terraform state and synthesizes UCP-qualified IDs for Azure, AWS, and Kubernetes resources. | `pkg/recipes/driver/terraform/terraform.go` parses Terraform state in `getDeployedOutputResources`; coverage for AWS and Kubernetes ID conversion exists in `pkg/recipes/driver/terraform/terraform_test.go`. |
| [`recipe/2023-11-support-insecure-registries.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-11-support-insecure-registries.md) | Adds opt-in `plainHttp` support for publishing and consuming Bicep recipes from insecure OCI registries. | `pkg/cli/cmd/bicep/publish/publish.go` and `pkg/cli/cmd/recipe/register/register.go` expose the `plain-http` flag; `pkg/rp/util/registry.go` sets `repo.PlainHTTP`; `pkg/recipes/types.go` carries the setting in recipe definitions. |
| [`recipe/2023-11-validate-template-path.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2023-11-validate-template-path.md) | Validates Terraform recipe template paths during environment conversion so invalid module sources fail early with user-facing errors. | `pkg/corerp/api/v20231001preview/environment_conversion.go` validates Terraform template paths and rejects unsupported local module paths; `pkg/corerp/api/v20231001preview/environment_conversion_test.go` covers accepted registry and HTTP inputs. |
| [`recipe/2024-01-support-private-terraform-repository.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-01-support-private-terraform-repository.md) | Adds authentication configuration for private Terraform module repositories using secret-backed git credentials in environment recipe config. | `pkg/recipes/engine/engine.go` resolves secret references before execution; `pkg/recipes/driver/terraform/terraform.go` exposes `FindSecretIDs`; `pkg/recipes/configloader/secrets.go` loads the referenced secret data. |
| [`recipe/2024-02-terraform-providers.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-02-terraform-providers.md) | Supports multiple Terraform provider configurations, aliases, and environment variables at the environment recipe-config level. | Terraform provider configuration generation is implemented in `pkg/recipes/terraform/config/providers/`; runtime environment variables and `envSecrets` are applied in `pkg/recipes/terraform/execute.go`. |
| [`recipe/2024-04-terraform-provider-secrets.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-04-terraform-provider-secrets.md) | Defines how provider and environment-variable secrets are discovered in the engine and passed into the Terraform driver without persisting secret values. | The engine-to-driver secret flow is implemented through `pkg/recipes/driver/types.go`, `pkg/recipes/engine/engine.go`, and `pkg/recipes/configloader/secrets.go`; Terraform-specific secret extraction is covered in `pkg/recipes/terraform/types.go`. |
| [`recipe/2024-06-private-bicep-registries.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2024-06-private-bicep-registries.md) | Adds secret-backed authentication for private Bicep registries, including basic auth, Azure workload identity, and AWS IRSA. | Registry auth resolution is implemented in `pkg/rp/util/registry.go`; auth client types live in `pkg/rp/util/authclient/`; tests cover auth modes in `pkg/rp/util/registry_test.go` and `pkg/rp/util/authclient/types_test.go`. |
| [`recipe/2025-08-recipe-packs.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2025-08-recipe-packs.md) | Designs Recipe Packs as a first-class resource type, enabling bundling of multiple recipe selections into a reusable unit referenced by environments. | RecipePack TypeSpec in `typespec/Radius.Core/recipePacks.tsp`; controller in `pkg/corerp/frontend/controller/recipepacks/`. |
| [`recipe/2025-09-container.md`](https://github.com/radius-project/design-notes/blob/main/recipe/2025-09-container.md) | Replaces the imperative Go renderer chain for containers with a Bicep recipe for the `Radius.Compute/containers` resource type. Migrated as `recipes/2025-09-container-recipe.md`. | `Radius.Compute/containers` is recipe-backed in `deploy/manifest/built-in-providers/dev/radius_compute.yaml`. |

### Templates (3 documents)

| Document | Summary |
|----------|--------|
| [`template/YYYY-MM-design-template.md`](https://github.com/radius-project/design-notes/blob/main/template/YYYY-MM-design-template.md) | Template for writing design documents. Migrated as `design-template.md`. |
| [`template/YYYY-MM-feature-spec-template.md`](https://github.com/radius-project/design-notes/blob/main/template/YYYY-MM-feature-spec-template.md) | Template for writing feature specifications. Migrated as `feature-spec-template.md`. |
| [`template/YYYY-MM-threat-model-template.md`](https://github.com/radius-project/design-notes/blob/main/template/YYYY-MM-threat-model-template.md) | Template for writing threat models. Migrated as `threat-model-template.md`. |

### Spec Kit Specifications (2 directories)

Spec Kit specifications are structured project artifacts (plans, research, tasks, checklists) that differ from point-in-time design notes. They will be migrated to `specs/` at the repository root.

| Source | Destination | Summary |
|--------|-------------|-------|
| [`specs/001-lrt-current-release/`](https://github.com/radius-project/design-notes/tree/main/specs/001-lrt-current-release) | `specs/001-lrt-current-release/` | Design spec for long-running tests using the current release, including plan, research, tasks, checklists, and quickstart. |
| [`specs/001-remove-bicep-types-submodule/`](https://github.com/radius-project/design-notes/tree/main/specs/001-remove-bicep-types-submodule) | `specs/001-remove-bicep-types-submodule/` | Specification for removing the bicep-types submodule and migrating to pnpm, including plans, research, and tasks. |

### Spec Kit Configuration (1 directory)

The [`.specify/`](https://github.com/radius-project/design-notes/tree/main/.specify) directory contains Spec Kit configuration, scripts, templates, and the project constitution. It is migrated to `.specify/` at the repository root (alongside `.github/`) to keep the Spec Kit workflow functional across the repository.

| Source | Destination | Summary |
|--------|-------------|--------|
| [`.specify/init-options.json`](https://github.com/radius-project/design-notes/blob/main/.specify/init-options.json) | `.specify/init-options.json` | Spec Kit initialization options (AI provider, script type, version). |
| [`.specify/memory/constitution.md`](https://github.com/radius-project/design-notes/blob/main/.specify/memory/constitution.md) | `.specify/memory/constitution.md` | Project constitution defining core principles, technology stack, development workflow, and governance for the Radius design notes repository. |
| [`.specify/scripts/bash/`](https://github.com/radius-project/design-notes/tree/main/.specify/scripts/bash) | `.specify/scripts/bash/` | Bash scripts for Spec Kit workflows: `check-prerequisites.sh`, `common.sh`, `create-new-feature.sh`, `setup-plan.sh`, `update-agent-context.sh`. |
| [`.specify/scripts/powershell/`](https://github.com/radius-project/design-notes/tree/main/.specify/scripts/powershell) | `.specify/scripts/powershell/` | PowerShell scripts mirroring the Bash scripts: `check-prerequisites.ps1`, `common.ps1`, `create-new-feature.ps1`, `setup-plan.ps1`, `update-agent-context.ps1`. |
| [`.specify/templates/`](https://github.com/radius-project/design-notes/tree/main/.specify/templates) | `.specify/templates/` | Spec Kit templates: `agent-file-template.md`, `checklist-template.md`, `constitution-template.md`, `plan-template.md`, `spec-template.md`, `tasks-template.md`. |

**Total: 42 documents/directories recommended for migration (41 + `.specify/`).**

### Agents and Prompts (20 files)

Custom agents and Copilot prompts from the design-notes repository are migrated to the `.github/agents` and `.github/prompts` directories to support Spec Kit workflows and development in the Radius repository.

**Agents (10 files)**

| Source | Destination | Summary |
|--------|-------------|---------|
| [`.github/agents/radius.speckit.prompt.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/radius.speckit.prompt.agent.md) | `.github/agents/radius.speckit.prompt.agent.md` | Agent for creating Radius-specific Spec Kit prompts. |
| [`.github/agents/speckit.analyze.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.analyze.agent.md) | `.github/agents/speckit.analyze.agent.md` | Spec Kit agent for analyzing and understanding design documents. |
| [`.github/agents/speckit.checklist.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.checklist.agent.md) | `.github/agents/speckit.checklist.agent.md` | Spec Kit agent for creating and managing project checklists. |
| [`.github/agents/speckit.clarify.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.clarify.agent.md) | `.github/agents/speckit.clarify.agent.md` | Spec Kit agent for clarifying and refining project requirements. |
| [`.github/agents/speckit.constitution.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.constitution.agent.md) | `.github/agents/speckit.constitution.agent.md` | Spec Kit agent for defining and maintaining project constitution. |
| [`.github/agents/speckit.implement.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.implement.agent.md) | `.github/agents/speckit.implement.agent.md` | Spec Kit agent for implementing features and tracking implementation progress. |
| [`.github/agents/speckit.plan.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.plan.agent.md) | `.github/agents/speckit.plan.agent.md` | Spec Kit agent for creating and managing project plans. |
| [`.github/agents/speckit.specify.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.specify.agent.md) | `.github/agents/speckit.specify.agent.md` | Spec Kit agent for specifying features and design decisions. |
| [`.github/agents/speckit.tasks.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.tasks.agent.md) | `.github/agents/speckit.tasks.agent.md` | Spec Kit agent for creating and managing project tasks. |
| [`.github/agents/speckit.taskstoissues.agent.md`](https://github.com/radius-project/design-notes/blob/main/.github/agents/speckit.taskstoissues.agent.md) | `.github/agents/speckit.taskstoissues.agent.md` | Spec Kit agent for converting tasks to GitHub issues. |

**Prompts (10 files)**

| Source | Destination | Summary |
|--------|-------------|---------|
| [`.github/prompts/radius.speckit.prompt.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/radius.speckit.prompt.prompt.md) | `.github/prompts/radius.speckit.prompt.prompt.md` | Prompt for creating Radius-specific Spec Kit prompts and agents. |
| [`.github/prompts/speckit.analyze.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.analyze.prompt.md) | `.github/prompts/speckit.analyze.prompt.md` | Spec Kit prompt for analyzing design documents and project documents. |
| [`.github/prompts/speckit.checklist.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.checklist.prompt.md) | `.github/prompts/speckit.checklist.prompt.md` | Spec Kit prompt for creating and managing project checklists. |
| [`.github/prompts/speckit.clarify.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.clarify.prompt.md) | `.github/prompts/speckit.clarify.prompt.md` | Spec Kit prompt for clarifying and refining project requirements and specifications. |
| [`.github/prompts/speckit.constitution.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.constitution.prompt.md) | `.github/prompts/speckit.constitution.prompt.md` | Spec Kit prompt for defining and maintaining project constitution and governance. |
| [`.github/prompts/speckit.implement.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.implement.prompt.md) | `.github/prompts/speckit.implement.prompt.md` | Spec Kit prompt for implementing features and tracking implementation progress. |
| [`.github/prompts/speckit.plan.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.plan.prompt.md) | `.github/prompts/speckit.plan.prompt.md` | Spec Kit prompt for creating and managing project plans and roadmaps. |
| [`.github/prompts/speckit.specify.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.specify.prompt.md) | `.github/prompts/speckit.specify.prompt.md` | Spec Kit prompt for specifying features, design decisions, and requirements. |
| [`.github/prompts/speckit.tasks.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.tasks.prompt.md) | `.github/prompts/speckit.tasks.prompt.md` | Spec Kit prompt for creating, organizing, and managing project tasks. |
| [`.github/prompts/speckit.taskstoissues.prompt.md`](https://github.com/radius-project/design-notes/blob/main/.github/prompts/speckit.taskstoissues.prompt.md) | `.github/prompts/speckit.taskstoissues.prompt.md` | Spec Kit prompt for converting tasks to GitHub issues for tracking and collaboration. |

**Total: 65 documents/directories recommended for migration (45 + 20 agents/prompts).**

## Documents NOT Recommended for Migration

### Aspirational / Not Yet Implemented (7 documents)

These documents describe future plans with no corresponding implementation in the current codebase:

| Document | Reason for Exclusion |
|----------|---------------------|
| `architecture/2025-04-aci-support.md` | ACI integration is not implemented; no ACI-specific code exists. |
| `architecture/2025-10-terraform-bicep-settings.md` | `Radius.Core/terraformSettings` and `Radius.Core/bicepSettings` resources do not exist yet. |
| `features/2024-11-authz-feature-spec.md` | Authorization/RBAC is not implemented. The UCP threat model confirms no RBAC exists. |
| `features/2025-01-serverless-feature-spec.md` | Serverless platform support (ACI, ECS, ACA) is not implemented. |
| `features/2025-07-10-offline-install-feature-spec.md` | Offline/air-gapped installation is not implemented. |
| `features/2025-07-23-radius-configuration-ux.md` | Configuration UX modeling configs as resources is not implemented. |
| `features/2025-07-radius-resource-types-contribution.md` | Migrated to `extensibility/` — see Documents Recommended for Migration. |
| `features/2025-08-14-terraform-bicep-settings.md` | Terraform/Bicep settings refactoring feature spec is not implemented. |

### Applications.Core Related (10 documents)

These documents are about `Applications.Core/*` or other `Applications.*` resource types managed by the Applications RP:

| Document | Reason for Exclusion |
|----------|---------------------|
| `architecture/2024-08-applications-rp-component-threat-model.md` | Migrated to `security/` — see Documents Recommended for Migration. |
| `architecture/2024-08-dashboard-component-threat-model.md` | Migrated to `security/` — see Documents Recommended for Migration. |
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

### Operational (1 item)

| Document | Reason for Exclusion |
|----------|---------------------|
| `recipe/2023-08-recipes-test-plan.md` | Test plan from early Terraform recipe development; the tests themselves exist in the codebase. |

### Empty Sections (1 item)

| Document | Reason for Exclusion |
|----------|---------------------|
| `bicep/README.md` | Directory contains only a README with no design documents. |

**Total: 22 documents/items not recommended for migration.**
