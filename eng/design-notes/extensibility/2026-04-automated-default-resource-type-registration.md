# Automated Default Registration of Resource Types from resource-types-contrib

* **Author**: Karishma Chawla (@kachawla)

## Overview

Today, resource type manifests for default registration in Radius are manually duplicated from the `resource-types-contrib` repository into the `radius` repository under `deploy/manifest/built-in-providers/`. This creates a maintenance burden — when a resource type schema is updated in `resource-types-contrib`, the corresponding file in `radius` must be manually updated, leading to schema drift, stale definitions, and duplicated effort.

This design introduces a `defaults.yaml` file in the `radius` repo that lists which resource types from `resource-types-contrib` ship as defaults. A `make update-resource-types` target bumps the `resource-types-contrib` Go module dependency and copies the listed manifests from the pinned module cache into `deploy/manifest/built-in-providers/`. The copied files are committed to the `radius` repo, and the existing startup `RegisterDirectory` path picks them up unchanged.

This eliminates manual cross-repo file editing while preserving full content visibility in Radius PRs (the actual YAML diffs appear inline) and Radius-side ownership of the default set. A CI check enforces that the committed copies always match the pinned `resource-types-contrib` version, so drift is caught automatically.

> An earlier iteration of this design proposed embedding manifests via `go:embed` from a `defaults.yaml` in `resource-types-contrib`. After review, we chose this approach to provide Radius maintainers ownership on default resource types. See the [Decision](#decision) note inside the Security section for rationale, and the [Appendix](#appendix-originally-proposed-goembed-approach) for the original design.

## Terms and definitions

| Term | Definition |
|---|---|
| **Resource type manifest** | A YAML file defining the namespace, types, API versions, and schemas for a resource type (e.g., `Radius.Compute/containers`). |
| **Default registration** | The process of registering resource types into UCP at Radius startup so they are available out of the box without user action. |
| **`resource-types-contrib`** | The community repository containing resource type definitions and recipes (`github.com/radius-project/resource-types-contrib`). |
| **UCP** | Universal Control Plane, the Radius component responsible for routing and managing resource providers. |
| **dynamic-rp** | The dynamic resource provider in Radius that handles Radius resource types. |
| **`DefaultDownstreamEndpoint`** | A UCP routing config that provides a fallback endpoint when a resource provider location has no explicit address. Points to dynamic-rp. |
| **`defaults.yaml`** | A file at `radius/deploy/manifest/defaults.yaml` listing which resource type manifests to copy from `resource-types-contrib` and ship as defaults. |
| **`make update-resource-types`** | The make target that bumps the `resource-types-contrib` dependency and refreshes the copied manifest files in `deploy/manifest/built-in-providers/`. |

## Objectives

### Goals

1. **Eliminate schema drift**: Resource type schemas defined in `resource-types-contrib` are the single source of truth. The copies in Radius are mechanically refreshed from a pinned version, with CI enforcement that they match.
2. **Controlled default registration**: Provide a clear, centralized mechanism (`defaults.yaml`) to declare which resource types from `resource-types-contrib` are registered by default in Radius.
3. **Full PR visibility**: Schema and default-set changes appear as YAML diffs in the Radius PR, reviewed under Radius CODEOWNERS.
4. **Version-pinned updates**: Schema updates are applied via standard Go module dependency management, providing version pinning and audit trails.

### Non goals

- **Runtime fetching of manifests**: Manifests are committed files on disk loaded at startup. There is no network dependency at runtime.
- **Migrating non-dynamic-rp providers**: Resource types served by `applications-rp` or the deployment engine (e.g., `Applications.Core`, `Microsoft.Resources`) require explicit `location` addresses and remain as hand-maintained directory-based manifests in `radius`. Migrating them is out of scope.
- **Recipe registration**: This design covers resource type schema registration only, not recipe registration or recipe pack management.
- **Release process for `resource-types-contrib`**: This design assumes a dependency is manually bumped to pick up changes during release process. Establishing a formal release/tagging process for `resource-types-contrib` is out of scope (see [Follow-up Items](#follow-up-items)).

### User scenarios

#### Platform engineer adds a new default resource type

A platform engineer wants `Radius.Networking/loadBalancers` to ship as a default in Radius.

1. They add the YAML manifest at `Networking/loadBalancers/loadBalancers.yaml` in `resource-types-contrib` and merge that PR.
2. In `radius`, they (or a maintainer) open a PR that:
   - Adds `Radius.Networking/loadBalancers` to `deploy/manifest/defaults.yaml`.
   - Runs `make update-resource-types`, which bumps `go.mod` to a version of `resource-types-contrib` containing the new manifest and copies the listed files into `deploy/manifest/built-in-providers/`.
   - Commits the updated `defaults.yaml`, `go.mod`/`go.sum`, and the copied YAML files.
3. The PR is reviewed under Radius CODEOWNERS with the actual YAML content visible in the diff. CI re-runs the copy step (without bumping) and fails if there is any drift.

#### Platform engineer updates a resource type schema

A platform engineer updates the schema for `Radius.Compute/containers` in `resource-types-contrib` and merges that PR. The change flows to Radius when a maintainer opens a PR that runs `make update-resource-types`. The PR diff shows the schema update inline alongside the `go.mod` bump.

## User Experience

N/A. This change is transparent to end users. Resource types continue to be available at startup as they are today. The change is to the internal workflow by which they are kept in sync with `resource-types-contrib`.

## Design

### High Level Design

The design adds `resource-types-contrib` as a Go module dependency of `radius` — using Go's module system purely as a versioned download mechanism. No code is imported from `resource-types-contrib`; no files are embedded at compile time.

A `defaults.yaml` file at `radius/deploy/manifest/defaults.yaml` lists the manifest files to ship as defaults. A `make update-resource-types` target:

1. Bumps the `resource-types-contrib` dependency in `go.mod` (`go get -u`).
2. Reads `defaults.yaml`.
3. Resolves each entry to a path inside the Go module cache for the pinned version.
4. Copies the file into `deploy/manifest/built-in-providers/`.

The copied files are committed to the `radius` repo. At startup, the existing initializer's `RegisterDirectory` function loads them alongside `radius_core.yaml` and `microsoft_resources.yaml` — no new runtime code paths required.

A CI check in `radius` re-runs the copy step (without bumping) on every PR and fails on diff, guaranteeing that the committed copies always match the pinned dependency version. This pattern mirrors the existing `go mod tidy` check.

The `location` field is intentionally omitted from `resource-types-contrib` manifests. When a manifest has no `location`, UCP's existing fallback mechanism routes requests to `DefaultDownstreamEndpoint` (dynamic-rp), which is the correct handler for all UDT-based resource types.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  resource-types-contrib (plain Go module)                   │
│                                                             │
│  go.mod                                                     │
│  Compute/containers/containers.yaml                         │
│  Compute/routes/routes.yaml                                 │
│  Security/secrets/secrets.yaml                              │
│  ...                                                        │
└─────────────────────┬───────────────────────────────────────┘
                      │  go module download (pinned version)
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  radius                                                     │
│                                                             │
│  go.mod ──► github.com/radius-project/resource-types-contrib│
│                                                             │
│  deploy/manifest/defaults.yaml  (lists files to copy)       │
│                      │                                      │
│                      │ make update-resource-types           │
│                      │   1. go get -u && go mod tidy        │
│                      │   2. for each entry in defaults.yaml │
│                      │   3. cp $MOD_DIR/<path> →            │
│                      │      deploy/manifest/built-in-providers/ │
│                      ▼                                      │
│  deploy/manifest/built-in-providers/                        │
│    radius_core.yaml          (existing, hand-maintained)    │
│    microsoft_resources.yaml  (existing, hand-maintained)    │
│    containers.yaml           (copied, committed)            │
│    routes.yaml               (copied, committed)            │
│    secrets.yaml              (copied, committed)            │
│                      │                                      │
│                      │ startup                              │
│                      ▼                                      │
│  pkg/ucp/initializer/service.go                             │
│    └─► manifest.RegisterDirectory(...)                      │
│        (existing path, unchanged)                           │
└─────────────────────────────────────────────────────────────┘
```

### Detailed Design

#### `defaults.yaml`

The file at `radius/deploy/manifest/defaults.yaml` lists which resource types from `resource-types-contrib` are copied into Radius, using canonical `<namespace>/<typeName>` names:

```yaml
defaultRegistration:
  - Radius.Compute/containers
  - Radius.Compute/persistentVolumes
  - Radius.Compute/routes
  - Radius.Data/mySqlDatabases
  - Radius.Data/postgreSqlDatabases
  - Radius.Security/secrets
```

The copy script resolves each entry to a file path in the `resource-types-contrib` module using the convention: strip the `Radius.` prefix from the namespace, then `<namespace>/<typeName>/<typeName>.yaml` (e.g., `Radius.Compute/containers` → `Compute/containers/containers.yaml`). If the resolved path does not exist, the script fails clearly. Logical names are stable across upstream file renames and consistent with how resource types are referenced elsewhere in Radius (CLI, API, logs).

#### `make update-resource-types`

```make
update-resource-types:
	go get -u github.com/radius-project/resource-types-contrib
	go mod tidy
	$(MAKE) sync-resource-types

sync-resource-types:
	@echo "Syncing default resource types from resource-types-contrib..."
	@MODULE_DIR=$$(go mod download -json github.com/radius-project/resource-types-contrib | jq -r '.Dir') && \
	for path in $$(yq '.defaultRegistration[]' deploy/manifest/defaults.yaml); do \
		cp "$$MODULE_DIR/$$path" deploy/manifest/built-in-providers/$$(basename "$$path"); \
		echo "  Copied $$path"; \
	done
	@echo "Done. Review and commit the updated files."
```

`sync-resource-types` is split out so CI can run the copy step alone (without bumping the dependency) to detect drift.

#### Drift detection in CI

A CI workflow runs on Radius PRs that touch `go.mod`, `go.sum`, `deploy/manifest/defaults.yaml`, or `deploy/manifest/built-in-providers/`:

```yaml
on:
  pull_request:
    paths:
      - 'go.mod'
      - 'go.sum'
      - 'deploy/manifest/defaults.yaml'
      - 'deploy/manifest/built-in-providers/**'

jobs:
  verify-resource-types:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Verify resource type copies are in sync with go.mod
        run: |
          make sync-resource-types
          git diff --exit-code deploy/manifest/built-in-providers/
```

If the committed copies do not match what `defaults.yaml` + the pinned `resource-types-contrib` version produce, the PR fails. This catches:
- A `go.mod` bump that wasn't followed by `make sync-resource-types`.
- A manual edit to a copied file that drifts from the upstream definition.
- A new entry added to `defaults.yaml` without re-syncing.

#### Manifest YAML files

Manifests in `resource-types-contrib` contain only `namespace` and `types` (no `location`, no Radius-specific fields). They are copied verbatim into `deploy/manifest/built-in-providers/` and registered alongside the existing `radius_core.yaml` and `microsoft_resources.yaml` files.

At startup, UCP's existing `RegisterDirectory` function loads each file. For a manifest without a `location`, the routing layer falls back to `DefaultDownstreamEndpoint` (dynamic-rp), which is the correct handler for UDT-based resource types.

### Implementation Details

#### resource-types-contrib repository

One change: add a `go.mod` so the repo is consumable as a Go module.

```
module github.com/radius-project/resource-types-contrib

go 1.24
```

No `defaults.yaml`, no generated code, no embed directives. The repo continues to be a flat collection of YAML manifests and recipes.

#### radius repository

| File | Change |
|---|---|
| `go.mod` | Add `github.com/radius-project/resource-types-contrib` dependency. |
| `deploy/manifest/defaults.yaml` | New file listing manifests to copy. |
| `deploy/manifest/built-in-providers/<type>.yaml` | New copied files (one per entry in `defaults.yaml`). |
| `deploy/manifest/built-in-providers/radius_compute.yaml` | **Removed**. Replaced by per-type copied files. |
| `deploy/manifest/built-in-providers/radius_security.yaml` | **Removed**. Replaced by per-type copied files. |
| `deploy/manifest/built-in-providers/radius_core.yaml` | Unchanged. Not in `resource-types-contrib`. |
| `deploy/manifest/built-in-providers/microsoft_resources.yaml` | Unchanged. Not in `resource-types-contrib`. |
| `Makefile` | Add `update-resource-types` and `sync-resource-types` targets. |
| `.github/workflows/...` | Add the drift-detection step described above. |

**No changes to UCP runtime code.** `pkg/cli/manifest/registermanifest.go`, `pkg/ucp/initializer/service.go`, and `pkg/ucp/server/server.go` are untouched. The existing `RegisterDirectory` path picks up the copied files automatically.

### Error Handling

| Scenario | Behavior |
|---|---|
| `defaults.yaml` lists a file not present in the pinned `resource-types-contrib` version | `make sync-resource-types` fails on `cp`, with the missing path identified. CI fails on the PR. |
| `go get -u` fails (network, module resolution) | `make update-resource-types` fails with the underlying Go toolchain error. The maintainer retries or pins manually. |
| Copied manifest YAML has invalid syntax | At startup, the existing `RegisterDirectory` parser returns an error for the specific file. Startup fails. |
| Copied manifest fails schema validation | The existing `validateManifestSchemas` returns an error for the specific file. Startup fails. |
| Drift between committed copies and pinned dependency | CI's drift-detection step shows the diff and fails the PR before merge. |
| `rad upgrade` introduces new default resource types | New types are registered via the existing startup path. Existing types are updated. No error expected. |

**Schema validation failure and release impact:** If a copied manifest fails schema validation at startup, Radius fails to start in a partially configured state. This fail-fast behavior is intentional. If this occurs during a release, the fix is to either correct the manifest in `resource-types-contrib`, re-bump the dependency, and re-run `make update-resource-types`; or pin `go.mod` to the last known good version. This should be documented in the release process alongside the `make update-resource-types` step.

## Test plan

1. **`make sync-resource-types` correctness**:
   - Run on a clean checkout, verify it copies exactly the files listed in `defaults.yaml`.
   - Run twice, verify the second run produces no diff (idempotent).
   - Add a missing path to `defaults.yaml`, verify the script fails clearly.

2. **CI drift detection**:
   - Open a PR that modifies a copied YAML directly without bumping the dependency. Verify CI fails.
   - Open a PR that bumps `go.mod` without running `make sync-resource-types`. Verify CI fails.
   - Open a clean PR that runs `make update-resource-types` end-to-end. Verify CI passes.

3. **Startup registration**:
   - Existing `Test_ResourceProvider_RegisterManifests` continues to pass against the new copied files.
   - Add an integration test that asserts each type listed in `defaults.yaml` is registered after startup.

4. **`rad upgrade` scenario**:
   - Start an older Radius with a smaller default set, then upgrade to a build with an additional default type. Verify the new type is registered and existing types are updated without errors.

## Security

No changes to the security model. The embedded manifests are static YAML files compiled into the binary at build time, so there is no new attack surface for injection or tampering beyond what exists for any compiled-in resource. The `defaults.yaml` file is validated at startup, and invalid entries cause a clear startup failure.

### Security Considerations for Embedded External Manifests

The proposed design has `defaults.yaml` in `resource-types-contrib` and pulls manifests into the Radius binary via a Go module dependency. Since manifests are sourced from a separate repository that may have a broader set of contributors, it is important to ensure that changes are properly reviewed before they are embedded into the Radius binary.

#### Existing safeguards

The following safeguards already mitigate this risk:

1. **PR review in `resource-types-contrib`.** Adding or modifying a manifest and updating `defaults.yaml` requires a PR reviewed and approved by CODEOWNERS of `resource-types-contrib`. A malicious manifest cannot become a default without reviewer approval.

2. **`go.mod` bump requires PR review in `radius`.** Changes in `resource-types-contrib` only reach Radius when a maintainer runs `go get -u` and merges the resulting `go.mod`/`go.sum` change. This is a second review gate.

3. **Manifest parsing and schema validation.** Manifests are parsed using a strict YAML decoder that rejects unknown top-level fields, duplicate keys, and any data that does not conform to the expected `ResourceProvider` structure. Schemas within each manifest are further validated against OpenAPI format; malformed or structurally invalid schemas are rejected at startup. There is no risk of code execution through YAML parsing, as Go YAML parsers do not support executable YAML tags.

4. **Schema runtime behavior.** The `schema` field within each API version accepts arbitrary JSON Schema content (including `additionalProperties: true`), so its contents are not structurally restricted beyond OpenAPI validity. After registration, dynamic-rp reads stored schemas at runtime for request validation and sensitive field identification (encryption). The schema is never passed to Terraform or Bicep recipes, and users always provide resource property values explicitly, so a crafted schema cannot inject values into recipe execution. The residual risks are limited to weakened request validation (overly permissive properties), unnecessary encryption overhead (incorrectly marking fields as sensitive), or performance degradation (very large or deeply nested schemas). These risks are mitigated by the requirement for manifest changes to go through code review, where reviewers can inspect the schema content.

#### Remaining risk

The primary remaining risk is **limited visibility in the Radius PR**. When a maintainer bumps the `resource-types-contrib` dependency, the PR diff in `radius` shows only a `go.mod`/`go.sum` version change. The actual YAML content changes are not visible. A reviewer must manually compare the two commit hashes in `resource-types-contrib` to see what changed.

#### Alternative approach 1: `defaults.yaml` and copied files in `radius`

To address the visibility concern, `defaults.yaml` and the manifest copies can be moved entirely to the `radius` repo. Since the files would already be on disk in the radius repo, no `go:embed` or `RegisterFS` changes are needed. The existing `RegisterDirectory` function handles them at startup, the same way it handles `radius_core.yaml` today.

**How it works:**

1. `resource-types-contrib` remains a plain Go module with `go.mod` and YAML manifest files. No `go:embed`, no `defaults.yaml`, no generated code.
2. `radius` has:
   - `deploy/manifest/defaults.yaml` listing the file paths to copy from `resource-types-contrib`
   - A Makefile target that reads `defaults.yaml`, downloads the pinned version of `resource-types-contrib` from the Go module cache, and copies the listed files into `deploy/manifest/built-in-providers/`
3. The copied YAML files are committed to the `radius` repo and registered at startup by the existing `RegisterDirectory` function.

**Makefile target:**
```make
sync-resource-types:
	@echo "Syncing default resource types from resource-types-contrib..."
	@MODULE_DIR=$$(go mod download -json github.com/radius-project/resource-types-contrib | jq -r '.Dir') && \
	for path in $$(yq '.defaultRegistration[]' deploy/manifest/defaults.yaml); do \
		cp "$$MODULE_DIR/$$path" deploy/manifest/built-in-providers/$$(basename "$$path"); \
		echo "  Copied $$path"; \
	done
	@echo "Done. Review and commit the updated files."
```

**Usage:**
```bash
# After bumping the dependency
go get -u github.com/radius-project/resource-types-contrib
make sync-resource-types
# Review the diff, then commit
```

```
radius/
  deploy/manifest/
    defaults.yaml                          # lists which files to copy
    built-in-providers/
      radius_core.yaml                     # existing, unchanged
      microsoft_resources.yaml             # existing, unchanged
      containers.yaml                      # copied from resource-types-contrib
      routes.yaml                          # copied from resource-types-contrib
      secrets.yaml                         # copied from resource-types-contrib
      ...
```

**Comparison with the proposed approach:**

| | Proposed (defaults.yaml in contrib) | Alternative (defaults.yaml in radius) |
|---|---|---|
| **Who controls what's default** | `resource-types-contrib` maintainers | `radius` maintainers |
| **YAML content visible in Radius PRs** | No (only go.mod/go.sum changes) | Yes (copied files show full diff) |
| **File duplication** | None; files embedded directly from Go module | YAML copies exist in both repos |
| **Security gate** | Two gates: contrib PR + radius go.mod PR (but radius PR has no content visibility) | Two gates: contrib PR + radius PR (with full content visibility) |
| **Adding a new default** | Edit `defaults.yaml` in contrib, run `go generate` there, then bump dependency in radius | Bump dependency in radius, edit `defaults.yaml` in radius, run copy script |
| **Contribution simplicity** | Single-repo change for contrib authors | Cross-repo change (contrib for the type, radius for making it default) |

**Advantages of the alternative:**
- Stronger security posture: `defaults.yaml` changes go through Radius CODEOWNERS review
- Full content visibility in Radius PRs eliminates the opaque `go.mod` bump problem
- Radius maintainers have explicit control over which types ship as defaults

**Disadvantages of the alternative:**
- Reintroduces file copies in the `radius` repo (automated, but still copies)
- Adding a new default requires changes in both repos instead of one
- **Sequential cross-repo workflow.** A contributor's manifest PR in `resource-types-contrib` must be merged before they can open the corresponding PR in `radius` (since `go get -u` pulls from the merged main branch), so it is a sequential two step process across repos.
- **Risk of drift on schema updates.** When an existing default resource type's schema is updated in `resource-types-contrib`, a maintainer must both bump the dependency (`go get -u`) and re-run the copy script to refresh the local YAML copies. If they bump the dependency but forget to re-run the copy script, the YAML files in radius will be stale relative to the pinned dependency version, reintroducing the drift problem in a different form.

#### Alternative approach 2: CI diff report and CODEOWNERS

Rather than moving `defaults.yaml` to `radius`, the visibility and control gaps in the proposed approach can be addressed with two lighter-weight mitigations:

1. **CI diff report in `radius`.** A CI step triggers on `go.mod` changes involving `resource-types-contrib`, fetches both the old and new versions of the module, and posts a comment on the PR with a diff summary of changed YAML files. This gives reviewers full visibility into what changed without any architectural changes, file duplication, or cross-repo complexity.

2. **CODEOWNERS for `defaults.yaml` in `resource-types-contrib`.** Add Radius maintainers as required reviewers for `defaults.yaml` via `resource-types-contrib`'s CODEOWNERS file:
   ```
   /defaults.yaml @radius-project/radius-maintainers
   ```
   This ensures that any change to which types are registered as defaults requires approval from a Radius maintainer, even though the file lives in the contrib repo. This gives Radius maintainers visibility on default registration changes without moving the file to a different repo.

Combined with the existing safeguards (strict parsing, schema validation, contrib CODEOWNERS), these two mitigations address both visibility and review requirements without the complexity of the alternative approach.

#### Recommendation

The proposed approach (defaults.yaml in `resource-types-contrib`) with a **CI diff report** added to `radius` provides the best balance of simplicity, single-repo contribution workflow, and security visibility. The alternative (defaults.yaml in `radius`) offers stronger security control and PR visibility but at the cost of file duplication, cross-repo contribution complexity, and risk of drift if the copy script is not re-run after dependency updates.

### Decision

**Adopt Alternative approach 1: `defaults.yaml` and copied manifest files live in the `radius` repo.** After review, the team prioritized security visibility and Radius-side ownership of the default set over the single-repo contribution workflow.

Rationale:
- **Full content visibility in Radius PRs.** When a default type is added, updated, or removed, the actual YAML diff appears directly in the Radius PR rather than being hidden behind a `go.mod` version bump. This gives reviewers the same level of insight they have for any other code change in the repo.
- **Radius maintainers own the default set.** Adding or removing a default type is a Radius-repo decision gated by Radius CODEOWNERS, eliminating the need for cross-repo CODEOWNERS coordination on `defaults.yaml`.

Mitigating the disadvantages noted in the alternative:
- **Drift risk if the copy script is not re-run after `go get -u`:** Add a CI check in `radius` that runs `make update-resource-types` (copy step only, no `go get`) and fails if it produces a diff. This guarantees the committed YAML files match the pinned `resource-types-contrib` version on every PR, the same way `go mod tidy` checks work.
- **Sequential cross-repo workflow when adding a new default type:** The contributor's PR in `resource-types-contrib` must merge first, then a follow-up PR in `radius` bumps the dependency, runs the copy script, and edits `defaults.yaml`. This is accepted as a worthwhile tradeoff for the visibility and ownership benefits.

## Compatibility

- **No breaking changes**: The existing `RegisterDirectory` startup path is reused as-is. The copied manifests live alongside the existing `radius_core.yaml` and `microsoft_resources.yaml` files.
- **Custom `ManifestDirectory` config**: Continues to work. Operators that point to a custom manifest directory can still override the defaults.
- **Removed files**: `radius_compute.yaml` and `radius_security.yaml` are deleted from `built-in-providers/` because their content is now provided by the per-type files copied from `resource-types-contrib`. Operators relying on these specific filenames should be informed in release notes (the resource types themselves remain registered).

## Monitoring and Logging

No new logs or metrics. The existing `RegisterDirectory` logging applies:
- `"Loaded manifest <path> (namespace: <ns>)"` for each file loaded.
- `"Successfully registered manifests" directory=<dir>` on completion.

Existing startup health checks and log monitoring apply.

## Development plan

1. **PR 1 (resource-types-contrib)**: Add `go.mod` so the repo is consumable as a Go module. No `defaults.yaml`, no generated code, no `go:embed`.
2. **PR 2 (radius)**: Add `resource-types-contrib` to `go.mod`. Add `deploy/manifest/defaults.yaml` listing the manifests to copy. Add the `update-resource-types` and `sync-resource-types` Makefile targets. Run `make update-resource-types` once and commit the copied manifest files into `deploy/manifest/built-in-providers/`. Remove the manually-maintained `radius_compute.yaml` and `radius_security.yaml`. Add the CI drift-detection workflow.
3. **PR 3 (radius)**: Update the Radius release process documentation to include a step for running `make update-resource-types` before each release, along with guidance on handling schema validation failures (fix the manifest in `resource-types-contrib`, re-bump, and re-run `make update-resource-types`; or pin to the last known good version).

### Ensuring the dependency is kept up to date

Until tagged releases and Dependabot automation are in place (see [Follow-up Items](#follow-up-items)), bumping the `resource-types-contrib` dependency in `radius` is a manual step. To ensure this is not forgotten, include a step in the Radius release checklist to run `make update-resource-types` and verify the latest resource type schemas are included before each release.

## Open Questions

None at this time.

## Follow-up Items

### 1. Bicep extension publishing automation

Each default-registered resource type also needs a corresponding Bicep extension published to an OCI registry (ACR) so that users can author Bicep files against the type schemas. Today, `rad bicep publish-extension -f <manifest.yaml> --target br:<registry>/<name>:<tag>` handles this per-file, but there is no automation tying it to the default registration list.

**Work needed:**
- Add a build step (in Radius CI or release pipeline) that reads `defaults.yaml`, groups manifests by namespace, merges them, and calls `rad bicep publish-extension` once per namespace to publish to the shared ACR (e.g., `br:biceptypes.azurecr.io/radius-compute:<version>`).
- Decide whether extensions are published per-namespace (e.g., `radius-compute`, `radius-data`, `radius-security`), per-type (e.g., `radius-compute-containers`), or one extension for all namespaces. One extension for all namespaces is preferred to keep `bicepconfig.json` manageable.
- Ensure extension versions stay in lockstep with the `resource-types-contrib` version pinned in `go.mod`, so Bicep types always match the schemas registered at startup.

### 2. Default recipe registration for embedded resource types

When a new resource type version is pulled into Radius via a `go.mod` bump, the corresponding recipes in `resource-types-contrib` may also need to be updated or registered. This design does not cover recipe registration.

**Work needed:**
- Define how default recipes (e.g., the Kubernetes recipe for `Radius.Compute/containers`) are associated with default-registered resource types.
- Determine whether `defaults.yaml` should also list default recipes per resource type, or whether a separate mechanism (e.g., recipe packs) handles this.

### 3. Tagged releases and automated dependency updates for `resource-types-contrib`

`resource-types-contrib` does not have a formal release or tagging process today. Without tagged releases, Radius depends on Go pseudo-versions (e.g., `v0.0.0-20260408153021-abc123def456`), and dependency updates require a maintainer to manually run `go get -u`. This limits automation and makes it harder to track what changed between versions.

## Alternatives considered

### Copy via GitHub Actions

Push changes from contrib → Radius PR

Pros: 
* No build changes

Cons:
* Operational complexity
* Requires cross-repo PATs
* Duplicates files

## Appendix: Originally proposed `go:embed` approach

> The sections below describe the alternative considered: `defaults.yaml` lives in `resource-types-contrib`, manifests are embedded into the Radius binary at compile time via `go:embed`, and a new `RegisterFS` runtime path loads them at startup. After review, this approach was rejected in favor of the copy-based design above. It is preserved here for context and as a reference for the rejected alternative.

### Central `defaults.yaml` + `go generate`

A `defaults.yaml` file at the root of `resource-types-contrib` lists which resource types should be default-registered using canonical `<namespace>/<typeName>` names. Running `go generate` invokes `gen_embed.go`, which reads this file, resolves each name to its corresponding manifest file path, and produces `manifests_gen.go` with `//go:embed` directives for exactly those files (plus `defaults.yaml` itself). At runtime, `RegisterFS` reads `defaults.yaml` from the embedded FS to know which paths to load.

#### Advantages

- Clean separation of concerns: the `ResourceProvider` struct is not polluted with deployment metadata.
- Minimal binary size: only the listed manifests are embedded.
- Discoverability: a single file shows all defaults at a glance.
- Reviewability: PR diffs for `defaults.yaml` clearly show what's being added or removed.
- No parser coupling: `resource-types-contrib` metadata stays out of the Radius manifest parser.
- Extensible: works for any directory structure; new top-level directories work without changing Go code.

#### Disadvantages

- Requires running `go generate` after editing `defaults.yaml` (mitigated by CI validation).
- Paths in `defaults.yaml` can go stale if files are renamed (mitigated by `go generate` failing on missing files).
- Two-file commit requirement (`defaults.yaml` + `manifests_gen.go`).
- **Limited visibility in the Radius PR.** When a maintainer bumps the `resource-types-contrib` dependency, the PR diff shows only a `go.mod`/`go.sum` version change. The actual YAML content changes are not visible. *This is the disadvantage that ultimately led to the design being rejected.*

### Implementation Details

#### resource-types-contrib repository

| File | Purpose |
|---|---|
| `go.mod` | Makes the repository a Go module (`github.com/radius-project/resource-types-contrib`). |
| `defaults.yaml` | Central list of resource types for default registration, using canonical `<namespace>/<typeName>` names. |
| `gen_embed.go` | Invoked by `go generate`. Reads `defaults.yaml`, resolves each resource type name to its file path, and produces `manifests_gen.go`. Build-tagged `//go:build ignore`. |
| `manifests.go` | Contains `//go:generate go run gen_embed.go` directive and package documentation. |
| `manifests_gen.go` | **Generated**. Contains `//go:embed` directives for `defaults.yaml` and each listed manifest. Exports `DefaultManifests embed.FS`. |

**`defaults.yaml` format:**
```yaml
defaultRegistration:
  - Radius.Compute/containers
  - Radius.Compute/persistentVolumes
  - Radius.Compute/routes
  - Radius.Data/mySqlDatabases
  - Radius.Data/postgreSqlDatabases
  - Radius.Security/secrets
```

`gen_embed.go` resolves each entry to a file path using the convention: strip the `Radius.` prefix from the namespace, then `<namespace>/<typeName>/<typeName>.yaml`. If a file does not exist at the resolved path, `go generate` fails immediately.

#### UCP

**`pkg/cli/manifest/registermanifest.go`**: New `RegisterFS` function that reads `defaults.yaml` from the provided `fs.FS`, parses each listed manifest, validates schemas, merges manifests sharing a namespace, and returns the merged providers for direct database registration.

**`pkg/ucp/initializer/service.go`** (updated): `NewService` accepts an additional `fs.FS` parameter. `Run` calls `RegisterFS` to parse and merge embedded manifests, then registers each merged provider via direct database writes (consistent with how directory-based manifests are registered).

**`pkg/ucp/server/server.go`** (updated): Imports `resource-types-contrib` and passes `resourcetypes.DefaultManifests` to `initializer.NewService`.

### Why this approach was rejected

See the [Decision](#decision) note inside the Security section. The summary:

- **No content visibility on dependency bumps.** A `go.mod` bump shows only a version hash; reviewers must manually compare contrib commits to see the actual schema changes flowing into Radius.
- **Cross-repo CODEOWNERS coordination required** to give Radius maintainers approval rights over `defaults.yaml`.
- **More machinery for the same outcome.** `go:embed`, `RegisterFS`, generated `manifests_gen.go`, and an `fs.FS` parameter on `NewService` are all new concepts versus reusing the existing `RegisterDirectory` path.

The accepted copy-based design preserves the goals of single-source-of-truth and version pinning while eliminating these concerns.