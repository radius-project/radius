# Migrate Radius Bicep extensions from ACR to GHCR

- **Author**: Will Smith (@willdavsmith)

## Overview

Radius publishes the Radius and AWS Bicep extensions to Azure Container Registry (ACR) and generates `bicepconfig.json` files that reference those artifacts. [Bicep v0.45.6 added support for publishing and restoring extensions from generic OCI registries](https://github.com/Azure/bicep/pull/18956), including GitHub Container Registry (GHCR).

This design moves public extension distribution from ACR to GHCR.

## Objectives

### Goals

- Publish the Radius and AWS Bicep extensions as public GHCR packages.
- Use the Radius release tag policy while preserving ACR compatibility.
- Upgrade the Bicep CLI shipped by Radius to an OCI-capable version.
- Move active consumers, documentation, and samples to GHCR.
- Remove Azure credentials and cross-organization dispatch used only for production extension publishing.

### Non-goals

- Changing Radius release version selection or channel policy.
- Updating historical release notes, design notes, or blog posts.
- `radiusdeploymentengine.azurecr.io` ACR migration.
- Migrating the Test-tenant ACR selected by `TEST_BICEP_TYPES_REGISTRY`. See [Functional-test Bicep extension migration from ACR to GHCR](./2026-09-test-bicep-extension-ghcr-migration.md).

## Artifact contract

| Extension | Source repository                | Current reference                       | Target reference                                     |
|-----------|----------------------------------|-----------------------------------------|------------------------------------------------------|
| Radius    | `radius-project/radius`          | `br:biceptypes.azurecr.io/radius:<tag>` | `br:ghcr.io/radius-project/bicep-types-radius:<tag>` |
| AWS       | `radius-project/bicep-types-aws` | `br:biceptypes.azurecr.io/aws:<tag>`    | `br:ghcr.io/radius-project/bicep-types-aws:<tag>`    |

This design selects `ghcr.io/radius-project/bicep-types-radius` and `ghcr.io/radius-project/bicep-types-aws` as the canonical package paths. Package creation, permissions, tag backfill, configuration generation, and consumer updates use these paths.

Tag selection follows `release-version.sh`. The packages are public so Bicep can restore them without credentials.

| Tag                 | Example       | GHCR publication                                                                                                                                                       |
|---------------------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Development         | `edge`        | Mutable alias updated from the current protected `main` commit's snapshot.                                                                                             |
| Latest stable       | `latest`      | Mutable alias promoted to the newest stable release's locked digest after verification and approval.                                                                   |
| Release channel     | `0.60`        | Mutable `X.Y` alias promoted to the newest approved final or patch release in that line.                                                                               |
| Full stable version | `0.60.2`      | Publish-once `X.Y.Z` tag for every approved final or patch release, staged before alias promotion. For example, Git tag `v0.60.2` publishes `0.60.2` in both packages. |
| Release candidate   | `0.60.0-rc.1` | Publish-once `X.Y.Z-rc.N` tag for the approved RC; no channel or `latest` promotion.                                                                                   |

Full-version and RC tags are **publish-once by CI policy**. GHCR does not enforce immutable tags; [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases) protect Git tags and release assets, not GHCR tags. Serialized publishing jobs confirm absence before publishing, reuse matching source/digest locks on reruns, and fail on conflicts or lookup errors without overwriting. Another authorized writer can bypass this policy.

GHCR-backed configurations require an OCI-capable Bicep CLI and the experimental feature while it remains gated:

```json
{
  "experimentalFeaturesEnabled": {
    "ociEnabled": true
  },
  "extensions": {
    "radius": "br:ghcr.io/radius-project/bicep-types-radius:0.60.2",
    "aws": "br:ghcr.io/radius-project/bicep-types-aws:0.60.2"
  }
}
```

Consumers can pin exact content with `br:ghcr.io/radius-project/bicep-types-radius@sha256:<manifest-digest>`, using the 64-character lowercase hexadecimal manifest digest; the same syntax applies to AWS and ACR references. This is a content digest, not a Git commit SHA, and no additional SHA tags are published. Record both digests in public release metadata and the release lock. Retain supported manifests and blobs after aliases move: digest pinning does not prevent deletion.

## Design

### Publishing and consumption

Production GHCR writes run in separate publishing jobs in `radius-project/radius` and `radius-project/bicep-types-aws`. Generation jobs have read-only repository permissions and retain workflow artifacts without publishing credentials. Trusted publishing jobs use fresh runners, fixed destinations, and verified source refs; they never promote PR artifacts or execute artifact-supplied scripts or binaries.

`__build-bicep-types.yaml` publishes Radius extensions for `build-main.yaml` and `build-release.yaml`; AWS uses `.github/workflows/publish-bicep.yaml`. Main updates `edge` only while its source commit is current. Approved releases stage full-version tags for both extensions, binding Radius to the release source and AWS to the frozen sibling commit. Generation captures inputs and outputs once and reuses them on retries, including AWS schemas.

Both extensions are required release outputs recorded in a durable lock with their source commits, full-version references, and manifest digests. Extend `.github/release-parity/targets.json`, `release-parity-manifest.sh`, `verify-release-manifest.mjs`, and the release-asset allowlists to check Bicep media types and publisher-stamped source annotations against the approved plan. Verification uses staged full-version references, not channel aliases.

After verification and final/patch approval in the `release` environment, the Radius finalizer rechecks both outputs and promotes aliases by locked digest through `release-oci-artifacts.sh`. RCs advance no aliases, and older releases cannot replace newer aliases. The release controller's resume path reuses locked content without rebuilding; historical plans retain their ACR contract.

The publishing jobs log in to GHCR with their repository `GITHUB_TOKEN` and `packages: write`. Docker login supplies the credentials consumed by Bicep's OCI transport.

During compatibility, trusted source-repository mirror jobs use environment-scoped Azure credentials to copy the verified GHCR artifacts by digest to ACR. Disable the external Bicep publishers and dispatches when mirroring starts. ACR development `latest` and release-channel mappings remain unchanged; channel updates follow approved finalization. Backfill required ACR tags into GHCR by digest, without overwriting GHCR's stable `latest` or assigning historical patch-version tags without provenance.

### Bicep toolchain

Radius currently pins Bicep v0.42.1 in `build/tools.yaml` and `pkg/cli/bicep/tools/download_tools.go`. The pin moves to a tested stable version at or above v0.45.6, with checksums updated through the existing tool update path.

The newer Bicep trust model rejects unlisted local registries. Workflows that publish to `localhost` set `BICEP_TRUSTED_REGISTRIES` explicitly rather than disabling registry validation.

`rad bicep publish-extension` enables `ociEnabled` in an isolated configuration for generic non-loopback registry targets without changing the user's configuration. Local-file and loopback targets retain the caller's working directory and configuration; this preserves HTTPS loopback behavior because Bicep's OCI loopback transport uses HTTP. Caller-relative output archives must survive temporary-directory cleanup. GHCR credentials continue to come from the Docker credential store.

### Consumer configuration

The configuration generators change first:

- `pkg/cli/setup/application.go`
- `build/scripts/generate-bicepconfig.sh`
- Radius build and functional-test workflow configuration generation

Checked-in configs, tests, agent assets, documentation, and samples then consume the canonical GHCR references. Obsolete aliases such as `radiusCompute`, `radiusData`, and `radiusSecurity` are replaced by the unified `radius` extension rather than receiving new GHCR packages.

Every effective GHCR config includes `ociEnabled`; development consumers select `edge`, and stable consumers select a release channel, full version, or digest. Long-running tests and versioned docs/samples cut over only after the selected released Radius toolchain passes GHCR publish and restore validation.

## Phased rollout

| Phase                   | Change                                                                                                                                  | Exit criteria                                                                   |
|-------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------|
| 1. Define contract      | Confirm package names, tag mapping, backfill set, and ACR retention policy.                                                             | Package and compatibility contracts are approved.                               |
| 2. Enable OCI toolchain | Upgrade Bicep, enable `ociEnabled`, support trusted local registries, and update `rad bicep publish-extension`.                         | Radius can publish and restore from GHCR and local OCI registries.              |
| 3. Establish GHCR       | Add direct publishing, isolate PR credentials, integrate release verification, and backfill public packages while retaining ACR writes. | Anonymous restore, PR isolation, ACR parity, and RC/final release tests pass.   |
| 4. Cut over Radius      | Change config generators, core CI, tests, and active Radius defaults.                                                                   | A clean Radius install builds Radius and AWS templates using GHCR.              |
| 5. Migrate consumers    | Update active satellite repositories, docs, samples, and release scripts.                                                               | No active default-branch consumer depends on the production ACR references.     |
| 6. Retire ACR writes    | Stop ACR publishing, remove publisher-only Azure credentials, and add a legacy-reference check.                                         | GHCR is the only production publisher; ACR remains read-only for compatibility. |
| 7. Remove ACR           | Remove ACR, including prior-version packages, when the compatibility policy permits it.                                                 | `biceptypes.azurecr.io` no longer exists.                                       |

## Parallel execution and queueing

Work is split by repository and merged through explicit gates.

| Workstream                                    | Can start                                                 | Merge gate                                                 |
|-----------------------------------------------|-----------------------------------------------------------|------------------------------------------------------------|
| Radius Bicep upgrade and local-registry fixes | Immediately                                               | Existing local and cloud Bicep tests pass.                 |
| PR credential isolation                       | After the artifact contract is fixed                      | No PR-executing job receives production-write credentials. |
| Radius generation and publishing              | After the release contract is fixed                       | Main and tag refs publish; PR execution is denied.         |
| AWS generation and publishing                 | After the release contract is fixed; parallel with Radius | Main and tag refs publish; PR execution is denied.         |
| GHCR tag backfill and visibility setup        | After each package is first published                     | Required tags restore successfully.                        |
| Radius canonical consumer changes             | Draft in parallel with publishing                         | Toolchain upgrade and both public packages are ready.      |
| Satellite repository changes                  | Draft in parallel by repository                           | Canonical Radius cutover is validated.                     |
| Documentation and samples                     | Draft in parallel                                         | A released Radius version ships the OCI-capable Bicep CLI. |
| ACR publisher retirement                      | Queue after all cutover work                              | At least one stable release completes on GHCR.             |

## Test plan

- Publish Radius and AWS extensions from allowlisted `main` and `v*` refs and verify the source SHA and destination tag.
- Attempt to create or update a production package version with the token from a PR-executing Radius workflow and require authorization failure.
- Restore each public package anonymously from a clean Bicep cache.
- Compare ACR and GHCR manifest digests during dual publishing and backfill.
- Publish to `--target ./output.tgz` and verify the archive remains in the caller's directory after cleanup.
- Run non-cloud tests with explicit local-registry trust and run cloud and long-running tests against the new public packages.
- Exercise an RC/final cycle with full-version tags, post-approval digest rechecks, alias ordering, and resume without rebuilding. Matching reruns reuse locked content; conflicts and failed lookups block publication.
- Restore by digest after moving a mutable alias and verify that the pinned content remains available.

## Security

Extension packages contain public type metadata and are intentionally public. Consumers restore anonymously.

### Credential isolation prerequisite

The Radius repository has Actions write access to both packages for publishing and finalization; the AWS repository has write access only to its own package. GHCR permissions are repository-scoped, so production access requires removing package-write tokens and publisher App credentials from all PR-executing jobs, including `functional-test-cloud.yaml`. Test uploads use separate trusted jobs with fixed destinations and no execution of artifact-supplied code. The Test-tenant registry remains unchanged.

Publishing jobs accept only verified `main` or release-tag source SHAs. Their `GITHUB_TOKEN` has `contents: read` and `packages: write`; jobs that stage release records additionally need `contents: write`. No personal access token is required. Trusted publishing code is checked out with `persist-credentials: false`, and AWS schema credentials stay in trusted generation jobs.

## Compatibility and risks

- Released Radius versions continue using ACR and require those artifacts to remain readable.
- The GHCR hostname must not be backported without the OCI-capable Bicep CLI and `ociEnabled`.
- Local OCI tests fail after the Bicep upgrade unless their hosts are explicitly trusted.
- A missing `ociEnabled` setting causes Bicep to select its legacy registry transport and fail against GHCR.
- A private GHCR package breaks anonymous restore; package visibility is a release gate.

## Open questions

- How long should the dual-publish phase last?
- When can compatibility for released ACR references end?

## Cleanup after ACR retirement

Deleting `biceptypes.azurecr.io` breaks references in released CLIs, release branches, and user repositories. Cleanup begins only after the compatibility policy explicitly allows that break, all supported releases use GHCR, and the ACR publishers have been disabled.

### Cleanup gates

- All supported Radius releases ship an OCI-capable Bicep CLI and generate GHCR references.
- Required production tags have been copied to GHCR and verified from a clean, anonymous Bicep restore.
- Active default branches contain no production references to `biceptypes.azurecr.io`.
- Production ACR writes have been disabled for the agreed observation period.
- The final ACR catalog, tags, and digests have been recorded.
- Maintainers have approved ending compatibility for released artifacts that still reference ACR.

Anonymous pulls are not attributable without registry diagnostics, so absence of observed traffic is not a sufficient deletion signal. The supported-release policy is the deletion authority.

### Production ACR contents

The following inventory was read from `biceptypes.azurecr.io` on September 9, 2026. Refresh it before deletion.

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

| Repository                       | Cleanup                                                                                                                                                                                                                                                                                                                                                             |
|----------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`          | Keep direct `__build-bicep-types.yaml` publishing and its `build-main.yaml` / `build-release.yaml` callers; remove the temporary ACR mirror and Bicep-only external dispatch/monitoring. Remove the `biceptypes` purge job from `.github/workflows/purge-azure-test-resources.yaml` and active production ACR references in configs, tests, docs, and agent assets. |
| `radius-project/bicep-types-aws` | Keep direct publishing in `.github/workflows/publish-bicep.yaml`; remove the ACR mirror and external Bicep dispatch/monitoring.                                                                                                                                                                                                                                     |
| `azure-octo/radius-publisher`    | Delete `publish-bicep-types.yml` and `publish-bicep-types-aws.yml`. Retain the repository and unrelated Deployment Engine and Static Web App workflows.                                                                                                                                                                                                             |
| `radius-project/wellknown`       | Remove the `BICEPTYPES_*` GitHub secret outputs and examples after the ACR purge job is removed. Retain the separate functional-test registry configuration.                                                                                                                                                                                                        |
| Consumer repositories            | Remove active ACR references from `radius-project/docs`, `samples`, `recipes`, `resource-types-contrib`, `ai-extensions`, `ai-extensions-fixture`, `app-assembly-eval`, `github-extension`, `skills`, `lab`, and `azure-octo/radius-demo`.                                                                                                                          |
| Historical content               | Leave historical release notes, design notes, and `radius-project/blog` content unchanged. Exclude those paths from the legacy-reference check.                                                                                                                                                                                                                     |

Release branches and existing user repositories cannot be rewritten centrally. They remain the reason to retain ACR until the compatibility window ends.

### GitHub configuration cleanup

Remove settings only after their last workflow consumer is gone.

| Repository                       | Remove                                                                                                                                                                                                                                                                                                                                       | Retain or repurpose                                                                                                                                                           |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`          | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, `BICEPTYPES_SUBSCRIPTION_ID`, `BICEP_TYPES_AZURE_CREDENTIALS`, `BICEP_TYPES_REGISTRY_LOGIN_SERVER`, `BICEP_TYPES_REGISTRY_USERNAME`, `BICEP_TYPES_REGISTRY_PASSWORD`, and `BICEP_TYPES_RESOURCE_GROUP`; remove or rename variable `BICEP_TYPES_REGISTRY` after mirroring/purge ends. | Retain `RADIUS_PUBLISHER_BOT_*` while Deployment Engine uses it, and release-controller App settings. Use `GITHUB_TOKEN` for GHCR. Retain the `release` approval environment. |
| `radius-project/bicep-types-aws` | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, and `BICEPTYPES_SUBSCRIPTION_ID` after mirroring stops; `PUBLISH_BICEP_APP_PRIVATE_KEY` and `RADIUS_PUBLISHER_BOT_APP_ID` / `RADIUS_PUBLISHER_BOT_PRIVATE_KEY` once their last external-dispatch consumer is gone.                                                                   | Use `GITHUB_TOKEN` for GHCR; keep only settings needed for trusted schema generation and existing release coordination.                                                       |
| `azure-octo/radius-publisher`    | Secrets `BICEP_TYPES_AZURE_CLIENT_ID`, `BICEP_TYPES_AZURE_TENANT_ID`, and `BICEP_TYPES_AZURE_SUBSCRIPTION_ID` after both Bicep workflows are deleted.                                                                                                                                                                                        | Retain settings used by Deployment Engine or other publisher workflows.                                                                                                       |

Remove Bicep-only publisher App access. Retain `azure-octo/radius-publisher` access and credentials used by Deployment Engine or other workflows. Retain `publish-bicep` only while it holds temporary mirror credentials, and remove it after its last consumer.

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

At inventory time, the registry had no ACR tasks, webhooks, tokens, custom scope maps, private endpoints, resource locks, geo-replications, or managed identity requiring separate cleanup; admin credentials were disabled.

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

## Existing release process

| Area       | Behavior                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Validation | [`build-validation.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-validation.yaml) calls reusable snapshot, CLI, and image workflows. Their build/export jobs use `contents: read` and retain artifacts without registry login.                                                                                                                                                                                                                                                                                             |
| Edge       | [`build-main.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-main.yaml) queues main builds. Separate publishing jobs in [`__build-cli.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-cli.yaml) and [`__build-images.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-images.yaml) check that the source is current before updating `edge`.                                   |
| Releases   | [`__release-controller.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__release-controller.yaml) reconciles approved source commits and creates sibling tags before the Radius tag. [`build-release.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-release.yaml) stages versioned outputs, verifies the manifest and installation, then promotes aliases and publishes. Finals and patches require `release` environment approval; RCs advance no aliases. |

Before the GHCR cutover, [`__build-bicep-types.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-bicep-types.yaml) dispatches Bicep type publication to `azure-octo/radius-publisher`, and the release output contract references ACR. Development builds publish `latest`; final and patch releases update a channel tag such as `0.60`. RCs receive full prerelease-version tags. Full stable-version tags such as `0.60.2` are not published by the ACR path.

`functional-test-cloud.yaml` executes PR-controlled Make targets with `packages: write`. The [credential isolation prerequisite](#credential-isolation-prerequisite) separates that execution from production publishing credentials before GHCR activation.

The `publish-bicep` environment has no protection rules in the audited configuration. Final and patch release approval uses the `release` environment.
