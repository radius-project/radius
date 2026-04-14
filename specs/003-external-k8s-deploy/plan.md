# Implementation Plan: External Kubernetes Cluster Deployment

**Branch**: `003-external-k8s-deploy` | **Date**: 2026-04-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/003-external-k8s-deploy/spec.md`

## Summary

Enhance Radius to deploy recipe workloads (Terraform and Bicep) to external AKS and EKS clusters. The environment's `ProvidersKubernetes` model gains `target`, `clusterType`, and `clusterName` properties. When `target=external`, Radius dynamically obtains a kubeconfig using registered cloud credentials before recipe execution. For EKS: STS presigned URL token + `eks.DescribeCluster`. For AKS: `ListClusterUserCredentials` + Entra ID (AAD) token. Credentials are passed inline to Terraform's Kubernetes provider and through the deployment engine's kubeconfig parameter for Bicep.

## Technical Context

**Language/Version**: Go (version per go.mod) + TypeSpec for API definitions
**Primary Dependencies**:
- AWS SDK v2 (`aws-sdk-go-v2/service/eks` — new, `aws-sdk-go-v2/service/sts` — existing)
- Azure SDK (`azidentity` — existing, `armcontainerservice/v6` — new)
- `k8s.io/client-go` — existing
**Storage**: N/A (kubeconfig is ephemeral, not persisted)
**Testing**: `go test` (unit), functional tests in `test/`
**Target Platform**: Kubernetes (in-cluster), Linux containers
**Project Type**: Control plane service (Go), API definitions (TypeSpec)
**Performance Goals**: Kubeconfig acquisition adds <5s overhead per recipe execution (single API call per cloud provider)
**Constraints**: Token lifetime ~15 min (EKS), variable for AKS Entra ID tokens. Fresh token per execution, no caching.
**Scale/Scope**: Affects 2 recipe engines (Terraform, Bicep), 1 API type (environments), 1 API version

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. API-First Design | PASS | TypeSpec changes designed first. ProvidersKubernetes extended with new fields. API contract documented in contracts/environments-api.md |
| II. Idiomatic Code Standards | PASS | Go code follows existing patterns in pkg/recipes/terraform/config/providers/ and pkg/azure/credential/ |
| III. Multi-Cloud Neutrality | PASS | Core feature — enables deployment to EKS (AWS) and AKS (Azure) external clusters. Cloud-specific implementations behind kubeconfig acquisition abstraction |
| IV. Testing Pyramid | PASS | Unit tests for kubeconfig generators, validation logic. Integration tests for end-to-end recipe execution against external clusters |
| V. Collaboration-Centric | PASS | Platform engineers configure external clusters; developers deploy recipes without knowing cluster details |
| VI. Open Source | PASS | Spec stored in specs/ directory. No proprietary dependencies |
| VII. Simplicity Over Cleverness | PASS | Direct SDK calls (~15 LOC for EKS token). No caching, no temp files, no abstraction layers beyond what's needed |
| VIII. Separation of Concerns | PASS | Kubeconfig acquisition is a separate module from provider config generation. Environment validation is separate from execution |
| IX. Incremental Adoption | PASS | `target` defaults to `current`. Existing environments unaffected. New fields are optional |
| XII. Resource Type Schema Quality | PASS | New enum types documented with descriptions. Validation rules enforce required field combinations |
| XVII. Polyglot Coherence | PASS | TypeSpec → Go data model → Terraform JSON config. Consistent patterns across layers |

**Post-Phase 1 Re-check**: All gates still PASS. No violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/003-external-k8s-deploy/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: Research findings
├── data-model.md        # Phase 1: Entity model changes
├── quickstart.md        # Phase 1: Usage guide
├── contracts/
│   └── environments-api.md  # Phase 1: API contract changes
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2: Task list (created by /speckit.tasks)
```

### Source Code (repository root)

```text
typespec/
└── Radius.Core/
    └── environments.tsp           # TypeSpec: ProvidersKubernetes model changes

pkg/
├── cli/
│   └── cmd/
│       ├── commonflags/
│       │   └── flags.go                           # New flag constants: KubernetesTarget, ClusterType, ClusterName
│       └── env/
│           ├── create/
│           │   └── preview/
│           │       └── create.go                  # rad env create: accept all provider flags
│           ├── update/
│           │   └── preview/
│           │       └── update.go                  # rad env update: accept new kubernetes flags
│           └── show/
│               └── preview/
│                   └── envproviders.go            # rad env show: display target, clusterType, clusterName
│
├── corerp/
│   ├── datamodel/
│   │   ├── environment.go                         # Internal data model: Providers + ProvidersKubernetes
│   │   └── environment_v20250801preview.go        # Versioned data model
│   ├── api/
│   │   └── v20250801preview/
│   │       └── zz_generated_models.go             # Generated from TypeSpec (auto)
│   └── frontend/
│       └── controller/
│           └── environments/
│               └── createorupdateenvironment.go   # Environment validation logic
│
├── kubernetes/
│   └── kubeconfig/                                # NEW package
│       ├── kubeconfig.go                          # KubeCredentials type + resolver interface
│       ├── eks.go                                 # EKS kubeconfig acquisition (STS presigned URL)
│       ├── eks_test.go
│       ├── aks.go                                 # AKS kubeconfig acquisition (Entra ID token)
│       └── aks_test.go
│
├── recipes/
│   ├── types.go                                   # Configuration.Providers gets Kubernetes field
│   ├── configloader/
│   │   └── environment.go                         # LoadConfiguration: populate Kubernetes provider
│   ├── terraform/
│   │   └── config/
│   │       └── providers/
│   │           └── kubernetes.go                  # BuildConfig: external inline credentials
│   └── driver/
│       └── bicep/
│           └── bicep.go                           # Bicep driver: inject external kubeconfig
│
└── portableresources/
    └── processors/
        └── resourceclient.go                      # ResourceClient: per-environment K8s client for deletion

test/
├── functional-portable/                           # Functional tests for external cluster recipes
└── rp/                                            # RP tests
```

**Note**: The Bicep deployment engine (`ghcr.io/radius-project/deployment-engine`) is an external C# service. A spike task (T020) investigates whether DE or recipe template changes are needed for external kubeconfig injection. If so, a cross-repo issue will be filed on `radius-project/deployment-engine`.

**Structure Decision**: Changes touch the existing Radius source tree. The only new package is `pkg/kubernetes/kubeconfig/` which encapsulates cloud-specific kubeconfig acquisition logic. All other changes extend existing files and packages.

## Complexity Tracking

No constitution violations. No complexity justification needed.
