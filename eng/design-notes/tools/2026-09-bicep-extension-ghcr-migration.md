# Migrate Radius Bicep extensions from ACR to GHCR

- **Author**: Will Smith (@willdavsmith)
- **Tracking issue**: [#12937](https://github.com/radius-project/radius/issues/12937)
- **Release baseline**: [#12970](https://github.com/radius-project/radius/pull/12970), the stack ending at `dp/release-snapshot-hardening`, reviewed at [`20bbd89a`](https://github.com/radius-project/radius/tree/20bbd89a8311659ade5db65a50fb5ed98ff674fa).

## Overview

Publish the Radius and AWS Bicep extensions directly from their source repositories to GitHub Container Registry (GHCR). Reuse the release stack's snapshot builds, immutable staging, verification, and alias promotion rather than introducing a separate publisher repository or another dispatch protocol.

The referenced stack is an implementation dependency, not the current `main` workflow layout. Bicep extensions remain separate from the GoReleaser binary/image builds, but join the same release transaction. Existing Azure Container Registry (ACR) artifacts remain readable until maintainers approve ending compatibility.

## Release-stack baseline

| Area       | Behavior to reuse                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Validation | [`build-validation.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-validation.yaml) calls reusable snapshot, CLI, and image workflows. Their build/export jobs use `contents: read` and retain artifacts without registry login.                                                                                                                                                                                                                                                                                             |
| Edge       | [`build-main.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-main.yaml) queues main builds. Separate publishing jobs in [`__build-cli.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-cli.yaml) and [`__build-images.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-images.yaml) check that the source is still current before updating `edge`.                             |
| Releases   | [`__release-controller.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__release-controller.yaml) reconciles approved source commits and creates sibling tags before the Radius tag. [`build-release.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/build-release.yaml) stages immutable outputs, verifies the manifest and installation, then promotes aliases and publishes. Finals and patches require `release` environment approval; RCs advance no aliases. |

The stack still sends Bicep types through [`__build-bicep-types.yaml`](https://github.com/radius-project/radius/blob/20bbd89a8311659ade5db65a50fb5ed98ff674fa/.github/workflows/__build-bicep-types.yaml) to `azure-octo/radius-publisher`. Its output contract still points at ACR. Those are extension-specific changes for this migration.

The stack does not finish the shared CI credential isolation: `functional-test-cloud.yaml` still executes PR-controlled Make targets with `packages: write`. The isolation work below is a prerequisite, not something this design assumes the stack already provides.

## Scope

Move public extension publishing, release verification, generated configurations, documentation, and samples to GHCR. Preserve supported ACR consumers during transition and remove only obsolete Bicep publishing automation and credentials.

### Non-goals

- Replacing the release controller or creating another publisher repository.
- Redesigning release version selection beyond adopting the referenced stack's policy.
- Updating historical release notes, design notes, or blog posts.
- `radiusdeploymentengine.azurecr.io` ACR migration.
- Migrating the Test-tenant ACR selected by `TEST_BICEP_TYPES_REGISTRY`. Its [separate design](./2026-09-test-bicep-extension-ghcr-migration.md) remains optional; its registry, identities, and configuration are retained.

## Artifact contract

| Extension | Source repository                | Current reference                       | GHCR reference                                       |
|-----------|----------------------------------|-----------------------------------------|------------------------------------------------------|
| Radius    | `radius-project/radius`          | `br:biceptypes.azurecr.io/radius:<tag>` | `br:ghcr.io/radius-project/bicep-types-radius:<tag>` |
| AWS       | `radius-project/bicep-types-aws` | `br:biceptypes.azurecr.io/aws:<tag>`    | `br:ghcr.io/radius-project/bicep-types-aws:<tag>`    |

Both packages are public for anonymous restore. Use the stack's `release-version.sh` policy, not the removed `get_release_version.py`.

| Source                             | GHCR publication                                                                                                                                                          |
|------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| PR, merge queue, or validation run | Workflow artifacts only; no production registry writes.                                                                                                                   |
| Current protected `main`           | Update `edge` from that commit's snapshot.                                                                                                                                |
| Approved `vX.Y.Z-rc.N`             | Stage immutable `X.Y.Z-rc.N`; no channel or `latest` promotion.                                                                                                           |
| Approved final or patch `vX.Y.Z`   | Stage immutable `X.Y.Z`; promote eligible `X.Y` and `latest` aliases from its locked digest after verification and approval. Older releases cannot replace newer aliases. |

This intentionally replaces the old Bicep mapping of development builds to `latest`: GHCR `edge` follows main and GHCR `latest` follows the newest stable release. Update edge consumers with that change. Do not change historical ACR tag meanings or rewrite old release references.

Released consumers use their release channel or full-version tag. GHCR configurations require Bicep v0.45.6 or later and `ociEnabled` while the feature remains gated:

```json
{
  "experimentalFeaturesEnabled": {
    "ociEnabled": true
  },
  "extensions": {
    "radius": "br:ghcr.io/radius-project/bicep-types-radius:<release-channel>",
    "aws": "br:ghcr.io/radius-project/bicep-types-aws:<release-channel>"
  }
}
```

## Design

### Source-repository builds and publishing

Replace the outbound dispatch in `__build-bicep-types.yaml` with generation and publishing jobs, following the reusable CLI/image workflows. Generation produces the index, type files, and source metadata as one workflow artifact. It has read-only repository permissions, no publishing credentials, and no registry dependency in PR validation.

Separate jobs on fresh runners publish using the repository `GITHUB_TOKEN` and Docker credentials. Check out publishing code from the protected default branch or approved release commit with `persist-credentials: false`, not from the generated artifact. Destinations are fixed by the workflow; source ref, commit, and release identity come from the trusted run or approved release plan. Never promote PR artifacts or execute artifact-supplied scripts or binaries. Keep reusable workflow callers read-only when they are used for validation.

`build-main.yaml` publishes only `edge`, using its queued execution and current-main check before mutation. `build-release.yaml` stages the full version without changing aliases. Reuse pinned tools, disabled credential-bearing build caches, explicit failure reporting, and local script tests from the stack. Bicep packaging stays a dedicated step rather than pretending type metadata is a GoReleaser image.

In `radius-project/bicep-types-aws`, replace the external dispatch in `.github/workflows/publish-bicep.yaml` with the same build/stage pattern. The existing controller still creates the AWS tag at the plan's frozen sibling commit. Capture schema inputs and generated output once; retries must reuse that output rather than query changing AWS schemas again. Any AWS schema-download credentials belong only in the trusted generation path, not PR builds or GHCR upload jobs.

### Release verification and promotion

Add a durable Bicep extension lock, using the stack's immutable release-asset pattern and registering it in the release-asset allowlists. Record each extension's source repository, source commit, version, full-version reference, manifest digest, and type-content checksum. Each trusted publisher stamps `org.opencontainers.image.source` and `org.opencontainers.image.revision` in the final OCI manifest before publishing the immutable version. Radius binds to the release source; AWS binds to the separately frozen AWS commit. Verify those annotations against the plan; matching Git tags alone do not prove artifact provenance. A missing required output or conflicting existing tag stops publication. Matching locked content is reused without rebuilding.

Extend these existing release surfaces together:

- `.github/release-parity/targets.json` and prepared plans: describe the two GHCR extension outputs.
- `release-parity-manifest.sh` and `verify-release-manifest.mjs`: observe full-version artifacts, verify Bicep media types and locked content/source identity, and require both before installation verification.
- `release-oci-artifacts.sh` and `promote-release-aliases`: add extension-aware digest verification and promotion without applying container-image assumptions to Bicep metadata.
- `build-release.yaml`: include both extensions in the existing verification, post-approval recheck, finalization, published-release reconciliation, and Build Summary gates.

Verification must not read `X.Y` before finalization: that alias still represents the previous release. Use the staged extension references in isolated compile/deployment verification without rewriting published consumer defaults. Recheck locked digests after approval, then let Radius's existing finalizer promote aliases for both packages. Grant that job's repository access to the AWS package, as the stack already requires for sibling image promotion; the AWS staging workflow does not independently advance release aliases.

Wait for the expected AWS full-version output with bounded retries and verify its planned source, not merely a successful remote run. Reuse the existing release identity, locks, summaries, and resume gateway. A partial promotion resumes from the same verified digests; it never rebuilds or silently selects the newest artifact. Select verification inputs from the frozen plan: new plans adopt GHCR, while historical plans and parity baselines keep their original ACR contract.

### Credential isolation prerequisite

Apply the snapshot/publish separation to every source-repository job that executes PR-controlled code, including the remaining cloud functional-test path. Those jobs receive neither package-write tokens nor publisher App credentials. If tests need uploaded images or recipes, use a separate trusted upload job with fixed test destinations, validated artifacts, and no execution of PR-generated scripts or binaries. Keep the Test-tenant registry unchanged.

Grant the Radius repository Actions write access to its own package and to the AWS package for finalization; grant the AWS repository write access only to its own package. GHCR Actions access is repository-scoped, not workflow- or tag-scoped. An environment or `dev/` prefix alone does not isolate a token. Protected workflow/ref controls and the negative authorization tests below must be in place before granting production package access. This is shared CI hardening, coordinated with the release stack, rather than a second Bicep release system.

### Compatibility and dual publishing

Build and package once, then copy the verified OCI artifact by digest to the corresponding ACR compatibility tag. Run this temporary mirror in a trusted source-repository job with environment-scoped Azure credentials. Stop the old external Bicep publishers when the mirror is activated so there is only one writer for each tag. Mirror current-main `edge` to ACR's existing development `latest` tag; update ACR release-channel aliases only through approved finalization. Preserve existing ACR mappings rather than copying GHCR's new `latest` meaning onto ACR.

Backfill the required ACR tags into GHCR with verified OCI copies, recording original digests rather than inventing source provenance. Do not import the old development `latest` over GHCR's stable `latest`. Keep historical aliases separate from new immutable full-version publication. Do not assume two independent generations, especially AWS generations, produce identical artifacts.

Upgrade the build/container and `rad bicep download` pins together to a tested Bicep version at or above v0.45.6. Keep caller configuration and caller-relative local output paths intact in `rad bicep publish-extension`. Enable OCI in an isolated configuration for generic remote targets; preserve caller transport for loopback targets because Bicep's OCI loopback transport uses HTTP. Scope local-registry trust explicitly and retain TLS verification.

Switch CLI scaffolding, `generate-bicepconfig.sh`, active configs, and satellite consumers only with that toolchain. Every effective GHCR config needs `ociEnabled`; edge consumers select `edge`, and released consumers retain their matching channel or full version. Consolidate obsolete per-namespace aliases into `radius`. Long-running release tests and versioned docs/samples wait for a compatible released Radius; updating main alone is insufficient.

## Phased rollout

| Phase                    | Change                                                                                                              | Exit criteria                                                                               |
|--------------------------|---------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| 1. Align prerequisites   | Land the release-stack dependencies, complete PR credential isolation, and upgrade Bicep.                           | Snapshot jobs cannot publish; ACR and local-registry compatibility pass.                    |
| 2. Stage direct outputs  | Add Radius/AWS generation and direct GHCR staging, package visibility, locks, and verification.                     | Immutable source-bound outputs and anonymous restore pass; validation runs do not publish.  |
| 3. Integrate the release | Extend manifests and the existing finalizer; backfill tags and replace external ACR writers with the digest mirror. | A real RC/final cycle passes verification, approval, promotion, and resume with ACR parity. |
| 4. Switch consumers      | Update Radius defaults, edge consumers, docs, samples, and other active repositories.                               | Compatible released clients restore GHCR types; old supported clients still use ACR.        |
| 5. Stop ACR writes       | End mirroring after the agreed observation period and remove Bicep-only Azure credentials.                          | GHCR is the sole production write path; ACR remains readable.                               |
| 6. Remove ACR            | Apply the cleanup inventory after maintainers approve the compatibility cutoff.                                     | Production ACR and dedicated dependencies are removed; Test-tenant resources remain.        |

## Parallel execution and queueing

| Workstream                         | Parallel work                                                                       | Gate                                                               |
|------------------------------------|-------------------------------------------------------------------------------------|--------------------------------------------------------------------|
| Toolchain and CLI wrapper          | Prepare independently of the release stack.                                         | Local output, ACR, and TLS/OCI behavior remain compatible.         |
| Shared CI isolation                | Coordinate with the release-stack author; do not duplicate their workflow refactor. | No PR-controlled execution receives production-write credentials.  |
| Radius and AWS staging             | Implement in parallel against the same tag and lock contract.                       | Reusable workflow layout and credential isolation are ready.       |
| Release verification and promotion | Extend the stack's scripts and fixtures while staging is prepared.                  | Both source-bound outputs exist; a real release cycle succeeds.    |
| Consumer/docs/sample changes       | Prepare drafts in parallel, including the GHCR `edge` mapping.                      | Public packages and a compatible released toolchain are available. |
| ACR retirement                     | Queue last.                                                                         | Observation period and explicit deletion approval are complete.    |

The previously prepared external-publisher dispatch work is superseded. Reuse the toolchain and wrapper changes; adapt workflow and consumer drafts to this stack instead of adding jobs to the removed `build.yaml`.

## Verification

- Extend the stack's snapshot, release-cutover, OCI artifact, manifest, and publication fixtures; include Bicep media types and full-version references.
- Prove PR/merge-queue/manual-validation jobs cannot write production versions, and artifact inputs cannot redirect a trusted test upload.
- Verify current-main edge publication, stale-main skips, RC alias exclusion, post-approval digest rechecks, out-of-order final releases, partial failures, and resume without rebuilding.
- Verify Radius and AWS digests and source identities, ACR mirror parity, anonymous clean-cache restore, and staged compile/deployment before alias promotion.
- Exercise relative local output survival, preserved caller auth/configuration, HTTPS loopback compatibility, and the actual released toolchain used by long-running tests.

Only trusted publishing/finalization jobs receive `packages: write`; builds are read-only. Keep `contents: write` only where staging release records requires it, and retain existing release App permissions for sibling tagging and coordination. No new publisher App is needed. Public GHCR packages have no download credential requirement.

## Open questions

- Which release will introduce the GHCR contract and satisfy the first RC/final rollout gate?
- How long should ACR mirroring and read-only compatibility last before deletion?

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

| Repository                       | Remove                                                                                                                                                                                                                                                                                                                                       | Retain or repurpose                                                                                                                                                                   |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `radius-project/radius`          | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, `BICEPTYPES_SUBSCRIPTION_ID`, `BICEP_TYPES_AZURE_CREDENTIALS`, `BICEP_TYPES_REGISTRY_LOGIN_SERVER`, `BICEP_TYPES_REGISTRY_USERNAME`, `BICEP_TYPES_REGISTRY_PASSWORD`, and `BICEP_TYPES_RESOURCE_GROUP`; remove or rename variable `BICEP_TYPES_REGISTRY` after mirroring/purge ends. | Retain `RADIUS_PUBLISHER_BOT_*` while Deployment Engine uses it, and release-controller App settings. Use `GITHUB_TOKEN` for GHCR. Retain the stack's `release` approval environment. |
| `radius-project/bicep-types-aws` | Secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, and `BICEPTYPES_SUBSCRIPTION_ID` after mirroring stops; `PUBLISH_BICEP_APP_PRIVATE_KEY` and `RADIUS_PUBLISHER_BOT_APP_ID` / `RADIUS_PUBLISHER_BOT_PRIVATE_KEY` once their last external-dispatch consumer is gone.                                                                   | Use `GITHUB_TOKEN` for GHCR; keep only settings needed for trusted schema generation and existing release coordination.                                                               |
| `azure-octo/radius-publisher`    | Secrets `BICEP_TYPES_AZURE_CLIENT_ID`, `BICEP_TYPES_AZURE_TENANT_ID`, and `BICEP_TYPES_AZURE_SUBSCRIPTION_ID` after both Bicep workflows are deleted.                                                                                                                                                                                        | Retain settings used by Deployment Engine or other publisher workflows.                                                                                                               |

Remove Bicep-only publisher App access, but retain `azure-octo/radius-publisher` access and credentials used by Deployment Engine or other workflows. Do not create a new publisher App or repository. Repurpose `publish-bicep` only for temporary mirror credentials, or remove it after its last consumer; it had no protection rules at audit time and is not an existing approval boundary.

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

1. End source-repository ACR mirroring; confirm the old external Bicep publishers are disabled and record the final inventory.
2. Remove remaining active ACR consumers, Bicep-only external dispatches, purge automation, and obsolete GitHub settings.
3. Wait for the approved compatibility period and confirm GHCR remains the only production write path.
4. Remove the dedicated `Radius-bicep-types` application identity and its federated credentials.
5. Delete the `bicep-types` resource group.
6. Verify the Azure resources and obsolete GitHub settings are absent and the code-search allowlist contains only historical references.
