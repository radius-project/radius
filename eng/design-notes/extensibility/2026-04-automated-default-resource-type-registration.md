# Automated Default Registration of Resource Types from resource-types-contrib

* **Author**: Karishma Chawla (@kachawla)

## Overview

Today, resource type manifests for default registration in Radius are manually duplicated from the `resource-types-contrib` repository into the `radius` repository under `deploy/manifest/built-in-providers/`. This creates a maintenance burden - when a resource type schema is updated in `resource-types-contrib`, the corresponding file in `radius` must be manually updated, leading to schema drift, stale definitions, and duplicated effort.

This design introduces a mechanism to automatically embed resource type manifests from `resource-types-contrib` as a Go module dependency of `radius`. A central configuration file (`defaults.yaml`) in `resource-types-contrib` declares which resource types should be default-registered. At build time, only those manifests are embedded into the Radius binary via `go:embed`. At startup, the UCP initializer reads the embedded manifests and registers them alongside any existing directory-based manifests.

This eliminates the need to copy files between repositories, ensures schemas stay in sync via standard Go dependency management, and provides a clear, reviewable way to control which resource types ship as defaults.

## Terms and definitions

| Term | Definition |
|---|---|
| **Resource type manifest** | A YAML file defining the namespace, types, API versions, and schemas for a resource type (e.g., `Radius.Compute/containers`). |
| **Default registration** | The process of registering resource types into UCP at Radius startup so they are available out of the box without user action. |
| **`resource-types-contrib`** | The community repository containing resource type definitions and recipes (`github.com/radius-project/resource-types-contrib`). |
| **UCP** | Universal Control Plane, the Radius component responsible for routing and managing resource providers. |
| **dynamic-rp** | The dynamic resource provider in Radius that handles Radius resource types. |
| **`DefaultDownstreamEndpoint`** | A UCP routing config that provides a fallback endpoint when a resource provider location has no explicit address. Points to dynamic-rp. |
| **`go:embed`** | A Go compiler directive that embeds files into the compiled binary at build time, accessible via the `embed.FS` type. Used here to include resource type manifests in the Radius binary without runtime file I/O. |
| **`go generate`** | A Go toolchain command that runs code generation scripts declared via `//go:generate` comments. Used here to produce `//go:embed` directives from `defaults.yaml`. |

## Objectives

### Goals

1. **Eliminate schema drift**: Resource type schemas defined in `resource-types-contrib` should be the single source of truth. Radius should consume them directly rather than maintaining copies.
2. **Controlled default registration**: Provide a clear, centralized mechanism to declare which resource types from `resource-types-contrib` are registered by default in Radius.
3. **Minimal binary bloat**: Only embed the manifests needed for default registration, not the entire `resource-types-contrib` repository (which includes recipes, tests, and documentation).
4. **Simple contribution workflow**: Adding a new default resource type should require editing a single configuration file, with no Go code changes.
5. **Version-pinned updates**: Schema updates are applied to Radius via standard Go dependency management (`go get -u`), providing version pinning and audit trails.

### Non goals

- **Runtime fetching of manifests**: Manifests are embedded at build time, not downloaded at runtime. This avoids network dependencies during startup.
- **Migrating non-dynamic-rp providers**: Resource types served by `applications-rp` or the deployment engine (e.g., `Applications.Core`, `Microsoft.Resources`) require explicit `location` addresses and remain as directory-based manifests in `radius`. Migrating them is out of scope.
- **Recipe registration**: This design covers resource type schema registration only, not recipe registration or recipe pack management.
- **Release process for `resource-types-contrib`**: This design assumes a Radius maintainer manually bumps the `resource-types-contrib` dependency in `go.mod` to pick up changes. Establishing a formal release/tagging process for `resource-types-contrib` is out of scope.

### User scenarios

#### Platform engineer adds a new default resource type

A platform engineer creates a new resource type `Radius.Networking/loadBalancers` in `resource-types-contrib`. To make it a default in Radius:

1. They add the YAML manifest at `Networking/loadBalancers/loadBalancers.yaml`.
2. They add the resource type to `defaults.yaml`:
   ```yaml
   defaultRegistration:
     - Radius.Networking/loadBalancers
   ```
3. They run `go generate` and commit `defaults.yaml` along with the auto-generated `manifests_gen.go` (which contains the `//go:embed` directives that tell the Go compiler which files to embed in the binary). `gen_embed.go` resolves each resource type name to its corresponding file path by scanning the directory tree.
4. A Radius maintainer manually bumps the dependency by running `go get -u github.com/radius-project/resource-types-contrib` in the `radius` repository and merging the resulting `go.mod` change. Since `resource-types-contrib` does not have tagged releases today, Go resolves a pseudo-version based on the latest commit (e.g., `v0.0.0-20260408153021-abc123def456`).

#### Platform engineer updates a resource type schema

A platform engineer updates the schema for `Radius.Compute/containers` in `resource-types-contrib`. The change flows to Radius when a maintainer bumps the dependency by running `go get -u github.com/radius-project/resource-types-contrib` and merging the `go.mod` change. No file copying or sync scripts are needed.

## User Experience

N/A. This change is transparent to end users. Resource types continue to be available at startup as they are today. The change is to the internal mechanism by which they are loaded.

## Design

### High Level Design

The design introduces `resource-types-contrib` as a Go module dependency of `radius`. Resource type manifests are embedded into the Radius binary using Go's `embed.FS` mechanism. A central `defaults.yaml` file in `resource-types-contrib` lists which manifests should be embedded and registered by default.

At startup, the UCP initializer service:
1. Reads `defaults.yaml` from the embedded filesystem to discover which manifests to load.
2. Parses each listed manifest, validates its schema, and merges manifests sharing a namespace into a single resource provider.
3. Registers the merged resource providers directly to the database, consistent with the existing startup registration path.
4. Proceeds to register any additional directory-based manifests as before.

The `location` field is intentionally omitted from `resource-types-contrib` manifests. When a manifest has no `location`, UCP's existing fallback mechanism routes requests to `DefaultDownstreamEndpoint` (dynamic-rp), which is the correct handler for all UDT-based resource types.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  resource-types-contrib (Go module)                         │
│                                                             │
│  defaults.yaml ─── lists types ───► go generate             │
│                                        │                    │
│  Compute/containers/containers.yaml    ▼                    │
│  Compute/routes/routes.yaml       manifests_gen.go          │
│  Security/secrets/secrets.yaml    (//go:embed directives)   │
│  ...                                   │                    │
│                                        ▼                    │
│                              embed.FS DefaultManifests      │
└─────────────────────┬───────────────────────────────────────┘
                      │  Go module dependency
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  radius                                                     │
│                                                             │
│  go.mod ──► github.com/radius-project/resource-types-contrib│
│                                                             │
│  pkg/ucp/server/server.go                                   │
│    └─► initializer.NewService(options, DefaultManifests)     │
│                                                             │
│  pkg/ucp/initializer/service.go                             │
│    └─► Run():                                               │
│         1. manifest.RegisterFS(embeddedManifests)            │
│         2. manifest.RegisterDirectory(manifestDir)           │
│                                                             │
│  pkg/cli/manifest/registermanifest.go                       │
│    └─► RegisterFS():                                        │
│         - Read defaults.yaml for manifest paths             │
│         - Parse & validate each manifest                    │
│         - Merge by namespace                                │
│         - Register with UCP                                 │
│                                                             │
│  deploy/manifest/built-in-providers/                        │
│    └─► radius_compute.yaml (REMOVED, now embedded)          │
│    └─► radius_security.yaml (REMOVED, now embedded)         │
└─────────────────────────────────────────────────────────────┘
```

### Detailed Design

#### Option 1: Per-file annotation (`defaultRegistration: true` in YAML)

Add a `defaultRegistration` boolean field to each manifest YAML and to the `ResourceProvider` Go struct. Embed all manifest YAMLs (`*/*/*.yaml`) and filter at runtime.

##### Advantages

- Flag is co-located with the type it describes; self-contained.
- Easier scaling as the list of default types grows.
- No central file to maintain.

##### Disadvantages

- **Schema pollution**: Leaks a deployment concern (`defaultRegistration`) into the schema.
- **Binary bloat**: Embedding all YAMLs (`*/*/*.yaml`) includes every resource type in the binary, even though only a handful are defaults right now. As `resource-types-contrib` grows to dozens or hundreds of types, this wastes binary space.
- **Discoverability**: Requires grepping across many files to determine which types are defaults.
- **Accidental removal**: The flag could be silently dropped during a type refactor.
- **Visibility**: Harder to view full default set.

#### Option 2: Central `defaults.yaml` + `go generate` (Proposed)

A `defaults.yaml` file at the repo root lists which resource types should be default-registered using canonical `<namespace>/<typeName>` names. Running `go generate` invokes `gen_embed.go`, which reads this file, resolves each name to its corresponding manifest file path, and produces `manifests_gen.go` with `//go:embed` directives for exactly those files (plus `defaults.yaml` itself). At runtime, `RegisterFS` reads `defaults.yaml` from the embedded FS to know which paths to load.

##### Advantages

- **Clean separation of concerns**: The `ResourceProvider` struct is not polluted with deployment metadata.
- **Minimal binary size**: Only the listed manifests are embedded.
- **Discoverability**: A single file shows all defaults at a glance.
- **Reviewability**: PR diffs for `defaults.yaml` clearly show what's being added or removed.
- **No parser coupling**: `resource-types-contrib` metadata stays out of the Radius manifest parser.
- **Extensible**: Works for any directory structure; new top-level directories (e.g., `Networking/`) work without changing Go code.

##### Disadvantages

- Requires running `go generate` after editing `defaults.yaml` (mitigated by CI validation).
- Paths in `defaults.yaml` can go stale if files are renamed (mitigated by `go generate` failing on missing files).
- Two-file commit requirement (`defaults.yaml` + `manifests_gen.go`).

#### Proposed Option

**Option 2: Central `defaults.yaml` + `go generate`**, for the reasons described above.

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

`gen_embed.go` resolves each entry to a file path using the convention: strip the `Radius.` prefix from the namespace, then `<namespace>/<typeName>/<typeName>.yaml` (e.g., `Radius.Compute/containers` resolves to `Compute/containers/containers.yaml`). If a file does not exist at the resolved path, `go generate` fails immediately.

**Manifest YAML files** remain unchanged: no `location` field, no `defaultRegistration` field. They contain only `namespace` and `types`.

#### UCP

**`pkg/cli/manifest/registermanifest.go`**: New `RegisterFS` function:
- Reads `defaults.yaml` from the provided `fs.FS` to get the list of resource type names.
- For each entry, resolves the resource type name to the corresponding embedded manifest file path.
- Reads and parses the manifest using the existing `ReadBytes` function.
- Validates schemas using the existing `validateManifestSchemas` function.
- Merges manifests sharing a namespace (e.g., three `Radius.Compute` files) into a single `ResourceProvider` with all types under one `Types` map.
- Returns the merged providers to the initializer for direct database registration.

**`pkg/ucp/initializer/service.go`** (updated):
- `NewService` accepts an additional `fs.FS` parameter for embedded manifests.
- `Run` processes embedded manifests by calling `RegisterFS` to parse and merge them, then registers each merged provider using `registerResourceProviderDirect` (direct database writes), consistent with how directory-based manifests are already registered at startup. This avoids HTTP round-trips, async operation queues, and polling.
- If both embedded and directory manifests exist, both are registered. Directory-based manifests can override embedded ones (last-write-wins via direct database save).

**`pkg/ucp/server/server.go`** (updated):
- Imports `resource-types-contrib` and passes `resourcetypes.DefaultManifests` to `initializer.NewService`.

**`deploy/manifest/built-in-providers/`** (removed files):
- `radius_compute.yaml` (now embedded from `resource-types-contrib`)
- `radius_security.yaml` (now embedded from `resource-types-contrib`)

Remaining files ( `radius_core.yaml`, `microsoft_resources.yaml`) stay because they are not included in resource-types-contrib.

### Error Handling

| Scenario | Behavior |
|---|---|
| `defaults.yaml` missing from embedded FS | `RegisterFS` returns error: `"failed to read defaults.yaml"`. Startup fails. |
| `defaults.yaml` lists a non-existent manifest path | `RegisterFS` returns error: `"failed to read manifest <path> listed in defaults.yaml"`. Startup fails. |
| Manifest YAML has invalid syntax | `ReadBytes` returns parse error. Startup fails with the specific file identified. |
| Manifest schema validation fails | `validateManifestSchemas` returns error. Startup fails with the specific file identified. |
| `defaults.yaml` is empty (no entries) | `RegisterFS` logs a message and returns nil. Startup continues with directory-based manifests only. |
| `rad upgrade` introduces new default resource types | New types are registered via direct database save on startup. Existing types are updated. No error expected. |

**Schema validation failure and release impact:** If any embedded manifest fails schema validation, startup fails entirely and no resource types are registered. This fail-fast behavior is intentional to prevent Radius from starting in a partially configured state. If this occurs during a release, the fix would be to either update the manifest in `resource-types-contrib` and re-bump the dependency, or pin `go.mod` to the last known good version of `resource-types-contrib` until the issue is resolved. This should be documented in the release process alongside the `make update-resource-types` step, so that maintainers know how to handle validation failures when bumping the dependency.

## Test plan

1. **Unit tests for `RegisterFS`**:
   - Test with a valid `fs.FS` containing `defaults.yaml` and matching manifests; verify correct registration calls.
   - Test namespace merging: multiple manifests with the same namespace produce a single provider with all types.
   - Test missing `defaults.yaml`: returns appropriate error.
   - Test invalid manifest YAML: returns parse error with file path.
   - Test empty `defaults.yaml`: returns nil without registering anything.
   - Test manifest path listed in `defaults.yaml` but missing from FS: returns appropriate error.

2. **Integration tests**:
   - Existing `Test_ResourceProvider_RegisterManifests` continues to work (tests directory-based registration).
   - New test that passes an `embed.FS` to `NewService` and verifies the resource provider is registered correctly.
   - Test `rad upgrade` scenario: register an initial set of embedded types, then simulate an upgrade with an updated `embed.FS` containing an additional default type. Verify the new type is registered and existing types are updated without errors.

3. **CI validation for `manifests_gen.go`**:
   - In `resource-types-contrib` CI: run `go generate` and verify no diff to ensure the generated file is up to date.
   ```bash
   go generate ./...
   git diff --exit-code manifests_gen.go
   ```

4. **Build verification**:
   - Verify Radius binary size doesn't increase significantly (only a handful of small YAML files are embedded).

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

## Compatibility

- **No breaking changes**: The existing `ManifestDirectory` config continues to work. Directory-based manifests are registered after embedded manifests, so existing deployments that set a custom manifest directory will continue to function.
- **Redundant registration**: If a deployment provides the same resource type via both embedded manifests and a directory-based manifest, the directory-based one will overwrite the embedded one (UCP uses `CreateOrUpdate`). This is harmless and provides an escape hatch.

## Monitoring and Logging

The initializer service logs at each stage:
- `"Loaded manifest <path> (namespace: <ns>)"` for each embedded manifest loaded.
- `"Registering resource provider <ns> from embedded manifests"` for each merged provider.
- `"Successfully registered default resource type manifests"` on completion of embedded registration.
- `"Successfully registered manifests" directory=<dir>` on completion of directory registration (existing).

No new metrics are added. Existing startup health checks and log monitoring apply.

## Development plan

1. **PR 1 (resource-types-contrib)**: Add `go.mod`, `defaults.yaml`, `gen_embed.go`, `manifests.go`, `manifests_gen.go`. Add CI step to validate `manifests_gen.go` is up to date.
2. **PR 2 (radius)**: Add `resource-types-contrib` to `go.mod`. Add `RegisterFS` to the manifest package. Update `initializer.Service` and `server.NewServer`. Remove `radius_compute.yaml` and `radius_security.yaml` from `built-in-providers/`. Add unit/integration tests.
3. **PR 3 (radius)**: Update the Radius release process documentation to include a step for running `make update-resource-types` before each release, along with guidance on handling schema validation failures (fix the manifest and re-bump, or pin to the last known good version).

### Makefile

Add a target in `resource-types-contrib` for convenience:

```make
generate-defaults:
	go generate ./...
```

In `radius`, a target to bump the dependency:

```make
update-resource-types:
	go get -u github.com/radius-project/resource-types-contrib
	go mod tidy
```

### Ensuring the dependency is kept up to date

Until tagged releases and Dependabot automation are in place (see Follow-up Item #3), bumping the `resource-types-contrib` dependency in `radius` is a manual step. To ensure this is not forgotten, include a step in the Radius release checklist to run `make update-resource-types` and verify the latest resource type schemas are included before each release.

## Open Questions

1. **`go generate` enforcement**: Should `resource-types-contrib` CI block merges if `manifests_gen.go` is out of date, or should CI auto-regenerate and commit?

   - **Option A: CI blocks merges (proposed).** CI runs `go generate` and `git diff --exit-code manifests_gen.go`. If the file is stale, the PR fails. Contributors must run `go generate` locally before pushing. This keeps generated files explicitly reviewed in PRs and avoids hidden auto-commits.
   - **Option B: CI auto-regenerates and commits.** CI runs `go generate` and pushes the updated file back to the PR branch. This is more convenient but obscures changes behind automated commits and can cause unexpected push conflicts.

2. **Defaults key format**: Should `defaults.yaml` entries be file paths (e.g., `Compute/containers/containers.yaml`) or logical resource type names (e.g., `Radius.Compute/containers`)?

   - **Option A: File paths (proposed).** Directly resolvable by `go:embed` and `fs.ReadFile` with no lookup step. Breakage on renames is mitigated by `go generate` failing immediately on missing files, making stale paths easy to catch. This is simpler to implement and aligns with how `go:embed` patterns work.
   - **Option B: Logical resource type names.** Uses the canonical `<namespace>/<typeName>` format (e.g., `Radius.Compute/containers`) which is stable across file renames and consistent with how resource types are referenced elsewhere in Radius (CLI, API, logs). The `go generate` script resolves names to file paths by scanning the directory tree for matching `namespace` and type entries. This adds generator complexity and couples the generator to the manifest schema format.

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