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
3. They run `go generate` and commit `defaults.yaml` along with the auto-generated `manifests_gen.go` (which contains the `//go:embed` directives that tell the Go compiler which files to embed in the binary).
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
3. Registers the merged resource providers with UCP.
4. Proceeds to register any additional directory-based manifests as before.

The `location` field is intentionally omitted from `resource-types-contrib` manifests. When a manifest has no `location`, UCP's existing fallback mechanism routes requests to `DefaultDownstreamEndpoint` (dynamic-rp), which is the correct handler for all UDT-based resource types.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  resource-types-contrib (Go module)                         │
│                                                             │
│  defaults.yaml ─── lists paths ──► go generate              │
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

A `defaults.yaml` file at the repo root lists which manifest paths should be default-registered. A `go generate` script reads this file and produces `manifests_gen.go` with `//go:embed` directives for exactly those files (plus `defaults.yaml` itself). At runtime, `RegisterFS` reads `defaults.yaml` from the embedded FS to know which paths to load.

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
| `defaults.yaml` | Central list of manifest paths for default registration. |
| `gen_embed.go` | `go generate` script that reads `defaults.yaml` and produces `manifests_gen.go`. Build-tagged `//go:build ignore`. |
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

**Manifest YAML files** remain unchanged: no `location` field, no `defaultRegistration` field. They contain only `namespace` and `types`.

#### UCP

**`pkg/cli/manifest/registermanifest.go`**: New `RegisterFS` function:
- Reads `defaults.yaml` from the provided `fs.FS` to get the list of resource type names.
- For each entry, resolves the resource type name to the corresponding embedded manifest file.
- Reads and parses the manifest using the existing `ReadBytes` function.
- Validates schemas using the existing `validateManifestSchemas` function.
- Merges manifests sharing a namespace (e.g., three `Radius.Compute` files) into a single `ResourceProvider` with all types under one `Types` map.
- Registers each merged provider using the existing `RegisterResourceProvider` function.

**`pkg/ucp/initializer/service.go`** (updated):
- `NewService` accepts an additional `fs.FS` parameter for embedded manifests.
- `Run` calls `manifest.RegisterFS` for embedded manifests **before** `manifest.RegisterDirectory` for directory-based manifests.
- If both embedded and directory manifests exist, both are registered. Directory-based manifests can override embedded ones (last-write-wins via UCP's `CreateOrUpdate`).

**`pkg/ucp/server/server.go`** (updated):
- Imports `resource-types-contrib` and passes `resourcetypes.DefaultManifests` to `initializer.NewService`.

**`deploy/manifest/built-in-providers/`** (removed files):
- `radius_compute.yaml` (now embedded from `resource-types-contrib`)
- `radius_security.yaml` (now embedded from `resource-types-contrib`)

Remaining files (`applications_core.yaml`, `applications_dapr.yaml`, `applications_datastores.yaml`, `applications_messaging.yaml`, `microsoft_resources.yaml`, `radius_core.yaml`) stay because they are not included in resource-types-contrib.

### Error Handling

| Scenario | Behavior |
|---|---|
| `defaults.yaml` missing from embedded FS | `RegisterFS` returns error: `"failed to read defaults.yaml"`. Startup fails. |
| `defaults.yaml` lists a non-existent manifest path | `RegisterFS` returns error: `"failed to read manifest <path> listed in defaults.yaml"`. Startup fails. |
| Manifest YAML has invalid syntax | `ReadBytes` returns parse error. Startup fails with the specific file identified. |
| Manifest schema validation fails | `validateManifestSchemas` returns error. Startup fails with the specific file identified. |
| `defaults.yaml` is empty (no entries) | `RegisterFS` logs a message and returns nil. Startup continues with directory-based manifests only. |
| UCP not reachable at startup | Existing `waitForServer` timeout behavior. No change from current behavior. |
| 409 conflict during registration | Existing retry logic with exponential backoff. No change from current behavior. |

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

## Compatibility (optional)

- **No breaking changes**: The existing `ManifestDirectory` config continues to work. Directory-based manifests are registered after embedded manifests, so existing deployments that set a custom manifest directory will continue to function.
- **Removed files**: `radius_compute.yaml` and `radius_security.yaml` are removed from `deploy/manifest/built-in-providers/`. Any tooling or scripts that reference these files directly would need to be updated. The `copy-manifests` Makefile target will copy fewer files but continues to work.
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

## Open Questions

1. **`go generate` enforcement**: Should `resource-types-contrib` CI block merges if `manifests_gen.go` is out of date, or should CI auto-regenerate and commit?

   - **Option A: CI blocks merges (proposed).** CI runs `go generate` and `git diff --exit-code manifests_gen.go`. If the file is stale, the PR fails. Contributors must run `go generate` locally before pushing. This keeps generated files explicitly reviewed in PRs and avoids hidden auto-commits.
   - **Option B: CI auto-regenerates and commits.** CI runs `go generate` and pushes the updated file back to the PR branch. This is more convenient but obscures changes behind automated commits and can cause unexpected push conflicts.

2. **Defaults key format**: Should `defaults.yaml` entries be file paths (e.g., `Compute/containers/containers.yaml`) or logical resource type names (e.g., `Radius.Compute/containers`)?

   - **Option A: File paths.** Directly resolvable by `go:embed` and `fs.ReadFile` with no lookup step. Breakage on renames is mitigated by `go generate` failing immediately on missing files, making stale paths easy to catch. This is simpler to implement and aligns with how `go:embed` patterns work.
   - **Option B: Logical resource type names (proposed).** Uses the canonical `<namespace>/<typeName>` format (e.g., `Radius.Compute/containers`) which is stable across file renames and consistent with how resource types are referenced elsewhere in Radius (CLI, API, logs). The `go generate` script resolves names to file paths by scanning the directory tree for matching `namespace` and type entries. This adds a small amount of generator complexity but decouples `defaults.yaml` from the repository's directory layout.

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
- Define how default recipes (e.g., the Kubernetes recipe for `Radius.Compute/containers`) are associated with default-registered resource types. Today, recipes are registered separately via recipe packs or manual `rad recipe register` commands.
- Determine whether `defaults.yaml` should also list default recipes per resource type, or whether a separate mechanism (e.g., recipe packs) handles this.

### 3. Tagged releases and automated dependency updates for `resource-types-contrib`

`resource-types-contrib` does not have a formal release or tagging process today. Without tagged releases, Radius depends on Go pseudo-versions (e.g., `v0.0.0-20260408153021-abc123def456`), and dependency updates require a maintainer to manually run `go get -u`. This limits automation and makes it harder to track what changed between versions.

**Work needed:**
- Establish a tagging/release process for `resource-types-contrib` (e.g., semver tags like `v0.1.0`, `v0.2.0`).
- Enable Dependabot in the `radius` repository for the `resource-types-contrib` Go module dependency. With tagged releases, Dependabot will automatically open PRs in `radius` when a new version is available.
- Alternatively (or additionally), add a scheduled or event-driven GitHub Actions workflow in `radius` that runs `go get -u`, runs tests, and opens a PR. This provides automation even before tagged releases are in place.
- Define a versioning policy for `resource-types-contrib`: when to bump major/minor/patch, and whether schema-breaking changes require a major version bump.

## Alternatives considered

### Copy via GitHub Actions

Push changes from contrib → Radius PR

Pros: 
* No build changes

Cons:
* Operational complexity
* Requires cross-repo PATs
* Duplicates files