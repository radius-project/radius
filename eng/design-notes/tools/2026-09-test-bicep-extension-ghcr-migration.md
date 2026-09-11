# Migrate functional-test Bicep extensions from ACR to GHCR

- **Author**: Will Smith (@willdavsmith)
- **Related design**: [Migrate Radius Bicep extensions from ACR to GHCR](./2026-09-bicep-extension-ghcr-migration.md)

## Overview

Radius cloud and long-running tests publish test-only Bicep extensions to the ACR selected by `TEST_BICEP_TYPES_REGISTRY`. This design records how that workload could move to GHCR and how to remove the Test-tenant ACR. It is separate from the production migration because Test-tenant resources do not need to move with the public release path.

## Objectives

### Goals

- Publish test Radius and user-defined type extensions as public GHCR packages.
- Use run-unique tags and remove completed or expired test versions.
- Remove `TEST_BICEP_TYPES_REGISTRY` and Bicep-specific Azure authentication.
- Remove the Terraform-managed test ACR without changing other shared test resources.

### Non-goals

- Migrating production Radius or AWS extensions.
- Migrating Bicep recipe registries.
- Changing the shared functional-test Entra application, AKS cluster, storage account, or Cosmos DB account.
- Migrating functional-test container images, which already use `ghcr.io/radius-project/dev`.

## Artifact contract

| Extension              | Current reference                                       | Target reference                                                     |
|------------------------|---------------------------------------------------------|----------------------------------------------------------------------|
| Functional-test Radius | `br:${TEST_BICEP_TYPES_REGISTRY}/test/radius:<run-tag>` | `br:ghcr.io/radius-project/test/bicep-types-radius:<run-tag>`        |
| Functional-test UDT    | `br:${TEST_BICEP_TYPES_REGISTRY}/testresources:latest`  | `br:ghcr.io/radius-project/test/bicep-types-testresources:<run-tag>` |

Both GHCR packages are public. Test workflows require package credentials only when publishing or deleting versions; restores are anonymous.

## Design

`functional-test-cloud.yaml` publishes run-specific Radius types and `long-running-azure.yaml` publishes the `testresources` extension. Each publishing job logs in to GHCR with its repository `GITHUB_TOKEN` and receives `packages: write` only after its existing contributor trust or environment approval gate passes.

All test versions use a run-unique tag. Replacing `testresources:latest` prevents concurrent runs from overwriting each other. An `if: always()` step removes versions created by a completed run, and scheduled cleanup removes versions left by cancelled jobs.

Generated test `bicepconfig.json` files include `ociEnabled` and reference the GHCR packages. Azure login remains where tests provision Azure resources, but Bicep publishing and restore no longer depend on Azure credentials.

## Phased rollout

| Phase                   | Change                                                                                 | Exit criteria                                                          |
|-------------------------|----------------------------------------------------------------------------------------|------------------------------------------------------------------------|
| 1. Create packages      | Create and link the two public GHCR packages; define tag and retention policy.         | A trusted job can publish and anonymously restore a temporary version. |
| 2. Cut over workflows   | Update cloud and long-running workflows, enable `ociEnabled`, and add version cleanup. | Concurrent runs use isolated tags and no workflow references the ACR.  |
| 3. Remove configuration | Remove `TEST_BICEP_TYPES_REGISTRY` from Radius and `wellknown`.                        | GitHub configuration no longer exposes the test ACR.                   |
| 4. Remove ACR           | Apply the `wellknown` Terraform change that deletes only the ACR and its direct roles. | The ACR is absent and the remaining test infrastructure is unchanged.  |

The package setup, workflow changes, and Terraform removal can be prepared in parallel. They merge in that order because Terraform removal is destructive.

## Test plan

- Publish both packages with only the trusted job's `GITHUB_TOKEN`.
- Restore both packages anonymously from a clean Bicep cache.
- Verify an untrusted pull-request job cannot publish or delete package versions.
- Verify concurrent runs use distinct tags.
- Verify normal and scheduled cleanup do not delete an active run.
- Run cloud functional and long-running tests without `TEST_BICEP_TYPES_REGISTRY`.

## Security

The extensions contain test schemas derived from public source and are intentionally public. Untrusted pull-request jobs can restore them but cannot publish or delete package versions. The workflow grants `packages: write` only to the gated job that owns the test run.

## Current ACR inventory

The ACR `crradfunctest1b2s.azurecr.io` was inventoried on September 9, 2026.

| Repository        | Current tags                        | Manifests | Cleanup                                                                                   |
|-------------------|-------------------------------------|-----------|-------------------------------------------------------------------------------------------|
| `test/radius`     | 870 `pr-func*` tags                 | 870       | Historical versions do not require migration.                                             |
| `testresources`   | `latest`                            | 32        | Historical manifests do not require migration.                                            |
| `applications-rp` | `lockfix`, `lockfix2`, `lockfix3`   | 3         | Confirm no manual consumers; current tests use `ghcr.io/radius-project/dev`, then delete. |
| `dynamic-rp`      | `lockfix2`, `lockfix3`, `stockmain` | 3         | Confirm no manual consumers; current tests use `ghcr.io/radius-project/dev`, then delete. |

This is 908 manifests and approximately 482 MB of aggregate manifest-reported image size. The registry has no ACR tasks, webhooks, or registry tokens.

## Azure and repository cleanup

The ACR is the Terraform resource `azurerm_container_registry.this` in `radius-project/wellknown`. It is in the Test subscription and the shared `rg-radfunctestsrd-1b2s` resource group.

| Area                       | Cleanup                                                                                                                                                                                                                  |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`    | Replace `TEST_BICEP_TYPES_REGISTRY` in `functional-test-cloud.yaml` and `long-running-azure.yaml`; remove the repository variable after cutover.                                                                         |
| `radius-project/wellknown` | Remove `azurerm_container_registry.this`, `azurecaf_name.test_acr`, `azurerm_role_assignment.test_principal_acr_push`, the `test_acr_*` outputs, and the `TEST_BICEP_TYPES_REGISTRY` GitHub variable output and example. |
| Test service principal     | Remove the ACR-specific `AcrPush` assignment from `app-radfunctest-1b2s`; retain the principal and its subscription roles.                                                                                               |
| AKS identity               | Remove the direct `AcrPull` assignment from `shruku-agentpool`; retain the managed identity and cluster.                                                                                                                 |
| Shared resource group      | Retain the storage account, Cosmos DB account, OIDC material, Entra application, and unrelated role assignments.                                                                                                         |

Apply the Terraform removal only after both workflows no longer reference or write to the ACR.

## Open question

- How long should completed functional-test package versions be retained in GHCR?
