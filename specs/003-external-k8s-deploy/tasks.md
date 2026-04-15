# Tasks: External Kubernetes Cluster Deployment

**Input**: Design documents from `/specs/003-external-k8s-deploy/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/environments-api.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add new Go dependencies and create the new kubeconfig package skeleton

- [ ] T001 Add `github.com/aws/aws-sdk-go-v2/service/eks` dependency via `go get github.com/aws/aws-sdk-go-v2/service/eks` in go.mod
- [ ] T002 [P] Add `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6` dependency via `go get` in go.mod
- [ ] T003 [P] Create `pkg/kubernetes/kubeconfig/` package with `KubeCredentials` type and `Resolver` interface in pkg/kubernetes/kubeconfig/kubeconfig.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: TypeSpec schema, Go data models, API conversion, and validation — MUST complete before user story work

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T004 Extend `ProvidersKubernetes` model in typespec/Radius.Core/environments.tsp with `target`, `clusterType`, `clusterName` fields and `KubernetesTarget`, `KubernetesClusterType` union types per data-model.md
- [ ] T005 Run TypeSpec code generation (`make generate`) to update generated models in pkg/corerp/api/v20250801preview/
- [ ] T006 Extend internal `ProvidersKubernetes` struct in pkg/corerp/datamodel/environment.go with `Target`, `ClusterType`, `ClusterName` fields
- [ ] T007 Extend versioned `ProvidersKubernetes_v20250801preview` struct in pkg/corerp/datamodel/environment_v20250801preview.go with `Target`, `ClusterType`, `ClusterName` fields
- [ ] T008 Update API version conversion functions between versioned and internal models to map the new fields in pkg/corerp/api/v20250801preview/ converter files
- [ ] T009 Add `Kubernetes ProvidersKubernetes` field to the `Providers` struct in pkg/corerp/datamodel/environment.go
- [ ] T010 Add environment validation logic in pkg/corerp/frontend/controller/environments/createorupdateenvironment.go for property combination rules per data-model.md validation rules (FR-010, FR-012)
- [ ] T011 Add unit tests for environment validation logic covering all 5 validation rules and all error responses per contracts/environments-api.md
- [ ] T012 Update `ConfigurationLoader.LoadConfiguration()` in pkg/recipes/configloader/environment.go to populate `Providers.Kubernetes` from the Environment resource

### CLI Changes

- [ ] T013 Add flag constants `KubernetesTargetFlag`, `KubernetesClusterTypeFlag`, `KubernetesClusterNameFlag` to pkg/cli/cmd/commonflags/flags.go and register them in `AddKubernetesScopeFlags()`
- [ ] T014 Update `rad env create` preview command in pkg/cli/cmd/env/create/preview/create.go to accept all provider flags (`--kubernetes-namespace`, `--aws-account-id`, `--aws-region`, `--azure-subscription-id`, `--azure-resource-group`, `--kubernetes-target`, `--kubernetes-cluster-type`, `--kubernetes-cluster-name`) and map them to `EnvironmentProperties.Providers` fields, following the pattern in `rad env update`
- [ ] T015 [P] Update `rad env update` preview command in pkg/cli/cmd/env/update/preview/update.go to accept and extract the new kubernetes flags, following the existing Azure/AWS flag extraction pattern
- [ ] T016 [P] Update `formatKubernetesProperties()` in pkg/cli/cmd/env/show/preview/envproviders.go to display `target`, `clusterType`, `clusterName` fields when present
- [ ] T017 Add unit tests for `rad env create` with all provider flags (AWS, Azure, Kubernetes) in pkg/cli/cmd/env/create/preview/create_test.go
- [ ] T018 [P] Add unit tests for `rad env update` with external cluster flags in pkg/cli/cmd/env/update/preview/update_test.go
- [ ] T019 [P] Add unit tests for `rad env show` displaying external cluster properties in pkg/cli/cmd/env/show/preview/envproviders_test.go

### Bicep Deployment Engine Investigation

- [ ] T020 Spike: Verify the Bicep deployment engine accepts kubeconfig via `extension kubernetes { kubeConfig: ... }` for external clusters. Test by passing a non-empty kubeconfig string to the DE and confirming it uses it instead of in-cluster config. Document whether DE or recipe template changes are required. If DE changes are needed, file a cross-repo issue on radius-project/deployment-engine

**Checkpoint**: Schema, data model, validation, config loading, CLI, and DE investigation complete. User story implementation can begin.

---

## Phase 3: User Story 1 - Deploy to an External EKS Cluster (Priority: P1) 🎯 MVP

**Goal**: Terraform and Bicep recipes deploy Kubernetes resources to an external EKS cluster using AWS credentials

**Independent Test**: Create an environment with `target=external, clusterType=eks, clusterName=<name>`, execute a recipe that creates a ConfigMap, verify it exists on the external EKS cluster

### Implementation for User Story 1

- [ ] T021 [US1] Implement EKS kubeconfig acquisition in pkg/kubernetes/kubeconfig/eks.go: `GetEKSClusterInfo()` (calls `eks.DescribeCluster` for endpoint + CA cert) and `GetEKSToken()` (STS `PresignGetCallerIdentity` → `k8s-aws-v1.` + base64 token) per research.md
- [ ] T022 [US1] Add unit tests for EKS kubeconfig acquisition in pkg/kubernetes/kubeconfig/eks_test.go: mock AWS SDK calls, verify token format (`k8s-aws-v1.` prefix, base64 RawURL encoding), verify error handling for missing cluster and permission errors, verify error messages are clear and actionable (FR-011)
- [ ] T023 [US1] Update `kubernetesProvider.BuildConfig()` in pkg/recipes/terraform/config/providers/kubernetes.go to check `envConfig.Providers.Kubernetes.Target` — when `external` with `clusterType=eks`, call EKS kubeconfig acquisition and return `map[string]any{"host": ..., "token": ..., "cluster_ca_certificate": ...}` per research.md
- [ ] T024 [US1] Add unit tests for `kubernetesProvider.BuildConfig()` in pkg/recipes/terraform/config/providers/kubernetes_test.go covering: external EKS returns inline credentials, current/omitted target returns existing behavior unchanged, AND verify the Kubernetes backend config (state storage) is NOT altered when target=external (FR-008)

### Shared (cloud-agnostic, used by both US1 and US2)

- [ ] T025 Update Bicep driver in pkg/recipes/driver/bicep/bicep.go to construct a kubeconfig YAML string from `KubeCredentials` and pass it via the deployment engine's `extension kubernetes { kubeConfig: ... }` parameter when `target=external` (cloud-agnostic — works for both EKS and AKS)
- [ ] T026 Update `ResourceClient` in pkg/portableresources/processors/resourceclient.go to create a per-environment Kubernetes client from `KubeCredentials` for resource deletion on external clusters (cloud-agnostic)

**Checkpoint**: User Story 1 fully functional + shared Bicep driver and ResourceClient support complete

---

## Phase 4: User Story 2 - Deploy to an External AKS Cluster (Priority: P1)

**Goal**: Terraform and Bicep recipes deploy Kubernetes resources to an external AKS cluster using Azure credentials with Entra ID authentication

**Independent Test**: Create an environment with `target=external, clusterType=aks, clusterName=<name>`, execute a recipe that creates a ConfigMap, verify it exists on the external AKS cluster

### Implementation for User Story 2

- [ ] T027 [US2] Implement AKS kubeconfig acquisition in pkg/kubernetes/kubeconfig/aks.go: `GetAKSRestConfig()` calls `ListClusterUserCredentials` (exec format) to extract server + CA cert, then `cred.GetToken()` with scope `6dae42f8-4368-4678-94ff-3960e28e3630/.default` for Entra ID bearer token, per research.md
- [ ] T028 [US2] Add unit tests for AKS kubeconfig acquisition in pkg/kubernetes/kubeconfig/aks_test.go: mock Azure SDK calls, verify kubeconfig YAML parsing extracts correct server + CA, verify Entra ID token acquisition with AKS AAD scope, verify error handling for permission and cluster-not-found errors, verify error messages are clear and actionable (FR-011)
- [ ] T029 [US2] Update `kubernetesProvider.BuildConfig()` in pkg/recipes/terraform/config/providers/kubernetes.go to handle `clusterType=aks` — call AKS kubeconfig acquisition and return inline credentials map
- [ ] T030 [US2] Add unit tests for `kubernetesProvider.BuildConfig()` in pkg/recipes/terraform/config/providers/kubernetes_test.go covering: external AKS returns inline credentials with Entra ID token

**Checkpoint**: User Stories 1 AND 2 both work — EKS and AKS external clusters supported

---

## Phase 5: User Story 3 - Default Behavior Unchanged for Current Cluster (Priority: P1)

**Goal**: Existing environments without external cluster properties continue to function identically

**Independent Test**: Deploy a recipe with an environment that has no `target` property and verify resources land on the local cluster with no behavioral change

### Implementation for User Story 3

- [ ] T031 [US3] Add unit tests in pkg/recipes/terraform/config/providers/kubernetes_test.go verifying: (a) `BuildConfig()` with no `target` field returns existing behavior, (b) `BuildConfig()` with `target=current` returns existing behavior, (c) no regression in in-cluster and out-of-cluster paths
- [ ] T032 [US3] Add unit tests in pkg/corerp/frontend/controller/environments/ verifying: (a) creating environment with only `namespace` succeeds, (b) creating environment with `target=current` succeeds, (c) `target=current` with `clusterType` set is rejected
- [ ] T033 [US3] Verify existing environment functional tests still pass with no modifications (run `make test` for relevant packages)

**Checkpoint**: All 3 user stories complete — backward compatibility verified, external EKS and AKS both work

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Code quality, documentation, and final validation

- [ ] T034 [P] Run `make lint` and `make format-check` and fix any issues across all modified files
- [ ] T035 [P] Run `make generate` to ensure all generated code is up-to-date after TypeSpec and model changes
- [ ] T036 [P] Add TypeSpec examples for external cluster scenarios in typespec/Radius.Core/examples/2025-08-01-preview/ (EKS and AKS environment create examples)
- [ ] T037 Run full unit test suite for affected packages: `go test ./pkg/kubernetes/kubeconfig/... ./pkg/recipes/terraform/config/providers/... ./pkg/corerp/... ./pkg/portableresources/...`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phases 3-5)**: All depend on Foundational (Phase 2)
  - US1 and US2 can proceed in parallel (different cloud providers, different files)
  - Shared tasks (T025-T026) run after US1 EKS-specific tasks complete
  - US3 depends on US1 and US2 being complete (regression validation)
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1 — EKS)**: Can start after Phase 2 — No dependencies on other stories
- **User Story 2 (P1 — AKS)**: Can start after Phase 2 — No dependencies on US1. Uses shared Bicep driver and ResourceClient from T025-T026
- **User Story 3 (P1 — Backward compat)**: Should run after US1 and US2 to validate no regressions

### Within Each User Story

- Kubeconfig acquisition module before provider config integration
- Provider config integration before Bicep driver integration
- Unit tests alongside implementation
- Story checkpoint before moving to next

### Parallel Opportunities

```
Phase 2 complete (T004-T020)
  ├── US1 (EKS): T021→T022→T023→T024
  │   └── Shared: T025→T026 (Bicep driver + ResourceClient)
  │
  └── US2 (AKS): T027→T028→T029→T030
       (parallel with US1 — different files, reuses T025-T026)

Both complete → US3: T031→T032→T033 → Phase 6: T034-T037
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T020)
3. Complete Phase 3: User Story 1 — EKS (T021-T026)
4. **STOP and VALIDATE**: Test EKS external cluster deployment end-to-end
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → Schema, validation, CLI, and DE investigation ready
2. Add User Story 1 (EKS) + Shared → Test independently → MVP
3. Add User Story 2 (AKS) → Test independently → Full external cluster support
4. Add User Story 3 (backward compat) → Verify no regressions → Production ready
5. Polish → Code quality and documentation complete
