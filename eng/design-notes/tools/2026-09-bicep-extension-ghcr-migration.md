# Migrate Radius Bicep extensions from ACR to GHCR

- **Author**: Will Smith (@willdavsmith)

## Overview

Radius publishes the Radius and AWS Bicep extensions to Azure Container Registry (ACR) and generates `bicepconfig.json` files that reference those artifacts. [Bicep v0.45.6 added support for publishing and restoring extensions from generic OCI registries](https://github.com/Azure/bicep/pull/18956), including GitHub Container Registry (GHCR).

This design moves public extension distribution from ACR to GHCR.

## Objectives

### Goals

- Publish the Radius and AWS Bicep extensions as public GHCR packages.
- Preserve the current extension tag semantics.
- Upgrade the Bicep CLI shipped by Radius to an OCI-capable version.
- Move active consumers, documentation, and samples to GHCR.
- Remove Azure credentials and cross-organization dispatch used only for production extension publishing.

### Non-goals

- Changing extension tag or release-channel semantics.
- Updating historical release notes, design notes, or blog posts.
- `radiusdeploymentengine.azurecr.io` ACR migration.
- Migrating the Test-tenant ACR selected by `TEST_BICEP_TYPES_REGISTRY`. See [Functional-test Bicep extension migration from ACR to GHCR](./2026-09-test-bicep-extension-ghcr-migration.md).

## Artifact contract

| Extension | Source repository                | Current reference                       | Target reference                                     |
|-----------|----------------------------------|-----------------------------------------|------------------------------------------------------|
| Radius    | `radius-project/radius`          | `br:biceptypes.azurecr.io/radius:<tag>` | `br:ghcr.io/radius-project/bicep-types-radius:<tag>` |
| AWS       | `radius-project/bicep-types-aws` | `br:biceptypes.azurecr.io/aws:<tag>`    | `br:ghcr.io/radius-project/bicep-types-aws:<tag>`    |

The migration preserves the tags produced by the current release workflows. The packages are public so Bicep can restore them without credentials.

GHCR-backed configurations require an OCI-capable Bicep CLI and the experimental feature while it remains gated:

```json
{
  "experimentalFeaturesEnabled": {
    "ociEnabled": true
  },
  "extensions": {
    "radius": "br:ghcr.io/radius-project/bicep-types-radius:latest",
    "aws": "br:ghcr.io/radius-project/bicep-types-aws:latest"
  }
}
```

## Design

### Publishing and consumption

Each source repository publishes its extension to GHCR with its repository `GITHUB_TOKEN` and `packages: write`. Docker login supplies the credentials consumed by Bicep's OCI transport. This removes GHCR publishing from the private `azure-octo/radius-publisher` repository and links each package to its source repository.

The existing publisher dispatch remains in place for ACR during the compatibility period. Both destinations receive extensions generated from the same source commit and release channel.

### Bicep toolchain

Radius currently pins Bicep v0.42.1 in `build/tools.yaml` and `pkg/cli/bicep/tools/download_tools.go`. The pin moves to a tested stable version at or above v0.45.6, with checksums updated through the existing tool update path.

The newer Bicep trust model rejects unlisted local registries. Workflows that publish to `localhost` set `BICEP_TRUSTED_REGISTRIES` explicitly rather than disabling registry validation.

`rad bicep publish-extension` must enable `ociEnabled` without changing a user's configuration. The command runs Bicep from its temporary generation directory with a temporary `bicepconfig.json`; GHCR credentials continue to come from the Docker credential store.

### Consumer configuration

The configuration generators change first:

- `pkg/cli/setup/application.go`
- `build/scripts/generate-bicepconfig.sh`
- Radius build and functional-test workflow configuration generation

Checked-in configs, tests, agent assets, documentation, and samples then consume the canonical GHCR references. Obsolete aliases such as `radiusCompute`, `radiusData`, and `radiusSecurity` are replaced by the unified `radius` extension rather than receiving new GHCR packages.

## Phased rollout

| Phase                   | Change                                                                                                                       | Exit criteria                                                                   |
|-------------------------|------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------|
| 1. Define contract      | Confirm package names, tag mapping, backfill set, and ACR retention policy.                                                  | Package and compatibility contracts are approved.                               |
| 2. Enable OCI toolchain | Upgrade Bicep, enable `ociEnabled`, support trusted local registries, and update `rad bicep publish-extension`.              | Radius can publish and restore from GHCR and local OCI registries.              |
| 3. Establish GHCR       | Add direct GHCR publishing for Radius and AWS, make packages public, and backfill supported tags while retaining ACR writes. | Anonymous GHCR restore works and both destinations pass parity tests.           |
| 4. Cut over Radius      | Change config generators, core CI, tests, and active Radius defaults.                                                        | A clean Radius install builds Radius and AWS templates using GHCR.              |
| 5. Migrate consumers    | Update active satellite repositories, docs, samples, and release scripts.                                                    | No active default-branch consumer depends on the production ACR references.     |
| 6. Retire ACR writes    | Stop ACR publishing, remove publisher-only Azure credentials, and add a legacy-reference check.                              | GHCR is the only production publisher; ACR remains read-only for compatibility. |
| 7. Remove ACR           | Remove ACR, including prior-version packages, when the compatibility policy permits it.                                      | `biceptypes.azurecr.io` no longer exists.                                       |

## Parallel execution and queueing

Work should be split by repository and merged through explicit gates rather than one cross-repository change.

| Workstream                                    | Can start                                                             | Merge gate                                                 |
|-----------------------------------------------|-----------------------------------------------------------------------|------------------------------------------------------------|
| Radius Bicep upgrade and local-registry fixes | Immediately                                                           | Existing local and cloud Bicep tests pass.                 |
| Radius GHCR publisher                         | After the artifact contract is fixed                                  | Package is public and anonymous restore succeeds.          |
| AWS GHCR publisher                            | After the artifact contract is fixed; parallel with Radius publishing | Package is public and anonymous restore succeeds.          |
| GHCR tag backfill and visibility setup        | After each package is first published                                 | Required tags restore successfully.                        |
| Radius canonical consumer changes             | Draft in parallel with publishing                                     | Toolchain upgrade and both public packages are ready.      |
| Satellite repository changes                  | Draft in parallel by repository                                       | Canonical Radius cutover is validated.                     |
| Documentation and samples                     | Draft in parallel                                                     | A released Radius version ships the OCI-capable Bicep CLI. |
| ACR publisher retirement                      | Queue after all cutover work                                          | At least one stable release completes on GHCR.             |

## Security

Extension packages contain public type metadata and are intentionally public. Publishing uses repository-scoped `GITHUB_TOKEN` credentials with `contents: read` and `packages: write`; no personal access token is required. Consumers restore anonymously.

## Compatibility and risks

- Released Radius versions continue using ACR and require those artifacts to remain readable.
- The GHCR hostname must not be backported without the OCI-capable Bicep CLI and `ociEnabled`.
- Local OCI tests fail after the Bicep upgrade unless their hosts are explicitly trusted.
- A missing `ociEnabled` setting causes Bicep to select its legacy registry transport and fail against GHCR.
- A private GHCR package breaks anonymous restore; package visibility is a release gate.

## Open questions

- What should the exact GHCR path be for the types? The current proposal uses `ghcr.io/radius-project/bicep-types-radius` and `ghcr.io/radius-project/bicep-types-aws`.
- How long should the dual-publish phase last?
- Are we safe to delete old ACR bicep extensions? If so, when?

## Cleanup after ACR retirement

Deleting `biceptypes.azurecr.io` breaks immutable references in released CLIs, release branches, and user repositories. Cleanup begins only after the compatibility policy explicitly allows that break, all supported releases use GHCR, and the ACR publishers have been disabled.

### Cleanup gates

- All supported Radius releases ship an OCI-capable Bicep CLI and generate GHCR references.
- Required production tags have been copied to GHCR and verified from a clean, anonymous Bicep restore.
- Active default branches contain no production references to `biceptypes.azurecr.io`.
- Production ACR writes have been disabled for the agreed observation period.
- The final ACR catalog, tags, and digests have been recorded.
- Maintainers have approved ending compatibility for released artifacts that still reference ACR.

Anonymous pulls are not attributable without registry diagnostics, so absence of observed traffic is not a sufficient deletion signal. The supported-release policy is the deletion authority.

### Production ACR contents

The following inventory was read from `biceptypes.azurecr.io`.

| Repository       | Current tags                                                                       | Manifests | Cleanup                                                                           |
|------------------|------------------------------------------------------------------------------------|-----------|-----------------------------------------------------------------------------------|
| `radius`         | 72 tags: release and RC tags from `0.37` through `0.60`, plus `edge` and `latest`  | 1,303     | Mirror required tags to GHCR; delete last because released Radius clients use it. |
| `aws`            | 61 tags: release and RC tags from `0.37` through `0.60`, plus `latest` and `pr-45` | 223       | Mirror required tags to GHCR; delete after the Radius consumer cutover.           |
| `radiuscompute`  | `0.56`, `latest`                                                                   | 6         | Do not migrate; delete after consumers use the unified `radius` extension.        |
| `radiusdata`     | `0.56`, `latest`                                                                   | 8         | Do not migrate; delete after consumers use the unified `radius` extension.        |
| `radiussecurity` | `0.56`, `latest`                                                                   | 2         | Do not migrate; delete after consumers use the unified `radius` extension.        |
| `aci-confcon`    | `latest`                                                                           | 3         | No active organization reference was found; confirm ownership, then delete.       |
| `radius-aci`     | `latest`                                                                           | 7         | No active organization reference was found; confirm ownership, then delete.       |
| `test/radius`    | 223 `pr-func*` tags                                                                | 223       | Delete with the production ACR and remove its purge automation.                   |
| `testresources`  | `latest`                                                                           | 70        | No active reference to this repository was found; delete with the production ACR. |

### Repository and automation cleanup

| Repository                       | Cleanup                                                                                                                                                                                                                                                                                                                                                                                  |
|----------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`          | Replace the Bicep publisher dispatch in `.github/workflows/build.yaml`; remove its `azure-octo/radius-publisher` dispatch and monitoring steps after dual publishing; remove the `biceptypes` cleanup job from `.github/workflows/purge-azure-test-resources.yaml`; remove active ACR references from CLI scaffolding, generated configs, CI, tests, contributor docs, and agent assets. |
| `radius-project/bicep-types-aws` | Replace `.github/workflows/publish-bicep.yaml` with direct GHCR publishing, then remove its publisher-app dispatch path.                                                                                                                                                                                                                                                                 |
| `azure-octo/radius-publisher`    | Delete `publish-bicep-types.yml` and `publish-bicep-types-aws.yml`. Retain the repository and unrelated Deployment Engine and Static Web App workflows.                                                                                                                                                                                                                                  |
| `radius-project/wellknown`       | Remove the `BICEPTYPES_*` GitHub secret outputs and examples after the ACR purge job is removed. Retain the separate functional-test registry configuration.                                                                                                                                                                                                                             |
| Consumer repositories            | Remove active ACR references from `radius-project/docs`, `samples`, `recipes`, `resource-types-contrib`, `ai-extensions`, `ai-extensions-fixture`, `app-assembly-eval`, `github-extension`, `skills`, `lab`, and `azure-octo/radius-demo`.                                                                                                                                               |
| Historical content               | Leave historical release notes, design notes, and `radius-project/blog` content unchanged. Exclude those paths from the legacy-reference check.                                                                                                                                                                                                                                          |

Release branches and existing user repositories cannot be rewritten centrally. They remain the reason to retain ACR until the compatibility window ends.

### GitHub configuration cleanup

Remove settings only after their last workflow consumer is gone.

| Repository                       | Remove                                                                                                                                                                                                                                                                                                            | Retain or repurpose                                                                                                                                                                               |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`          | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, `BICEPTYPES_SUBSCRIPTION_ID`, `BICEP_TYPES_AZURE_CREDENTIALS`, `BICEP_TYPES_REGISTRY_LOGIN_SERVER`, `BICEP_TYPES_REGISTRY_USERNAME`, `BICEP_TYPES_REGISTRY_PASSWORD`, and `BICEP_TYPES_RESOURCE_GROUP`; remove or rename variable `BICEP_TYPES_REGISTRY`. | Retain `RADIUS_PUBLISHER_BOT_*` while Deployment Engine publishing uses it. The `publish-bicep` environment may gate GHCR publishing; delete it only if the replacement workflow does not use it. |
| `radius-project/bicep-types-aws` | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, `BICEPTYPES_SUBSCRIPTION_ID`, `PUBLISH_BICEP_APP_PRIVATE_KEY`, `RADIUS_PUBLISHER_BOT_APP_ID`, and `RADIUS_PUBLISHER_BOT_PRIVATE_KEY` after direct GHCR publishing replaces the dispatch workflow.                                                         | Retain the `publish-bicep` environment if it remains the approval boundary for GHCR publishing.                                                                                                   |
| `azure-octo/radius-publisher`    | Secrets `BICEP_TYPES_AZURE_CLIENT_ID`, `BICEP_TYPES_AZURE_TENANT_ID`, and `BICEP_TYPES_AZURE_SUBSCRIPTION_ID` after both Bicep workflows are deleted.                                                                                                                                                             | Retain settings used by Deployment Engine or other publisher workflows.                                                                                                                           |

The Publisher GitHub App remains installed on `azure-octo/radius-publisher` while the Deployment Engine path uses it. Removing the Bicep workflows does not by itself justify deleting the App or the shared Radius repository credentials.

### Azure resource cleanup

#### Production ACR

The production resources are in the `Radius OSS Assets` subscription:

| Resource                                                    | Cleanup                                                                                                                                                                                              |
|-------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Resource group `bicep-types`                                | Delete after the final compatibility gate. It contains only the production ACR.                                                                                                                      |
| ACR `biceptypes`                                            | Deleted with the resource group, including all repositories, manifests, and registry-scoped RBAC assignments.                                                                                        |
| App registration and service principal `Radius-bicep-types` | Delete after publishing stops. It has eight GitHub Actions federated credentials for Radius, AWS Bicep types, and `azure-octo/radius-publisher`, and its only Azure role is Contributor on this ACR. |
| Resource-group and registry RBAC                            | Remove four resource-group assignments and seven direct registry assignments, or allow resource-group deletion to remove the scoped assignments. Do not delete shared user identities.               |
| `radlrtest00-aks` managed identity                          | Remove its direct `AcrPush` assignment only. Retain the managed identity because it belongs to long-running test infrastructure.                                                                     |
| Unresolved service principals                               | Remove their ACR/resource-group assignments. Delete the principals only after confirming they have no other assignments or owners.                                                                   |

The registry currently has no ACR tasks, webhooks, tokens, custom scope maps, private endpoints, resource locks, or geo-replications requiring separate cleanup. It has no managed identity and admin credentials are disabled.

### Resources retained

- The private functional-test ACR identified by `TEST_BICEP_TYPES_REGISTRY` (`crradfunctest1b2s.azurecr.io`) and its identities remain out of scope.
- Bicep recipe registries and their credentials remain out of scope.
- `radiusdeploymentengine.azurecr.io` remains out of scope.
- GHCR packages unrelated to Bicep extension distribution remain unchanged.

### Cleanup order

1. Disable ACR publication and record the final inventory.
2. Remove ACR consumers, publisher dispatches, purge automation, and obsolete GitHub settings.
3. Wait for the approved compatibility period and confirm GHCR remains the only production write path.
4. Remove the dedicated `Radius-bicep-types` application identity and its federated credentials.
5. Delete the `bicep-types` resource group.
6. Verify the Azure resources and obsolete GitHub settings are absent and the code-search allowlist contains only historical references.
