# Static-Graph Sensitive-Field Redaction

Status: Proposed
Author: (fill in)
Date: 2026-07-15
Companion to: [2026-07-sensitive-fields-in-app-graph.md](./2026-07-sensitive-fields-in-app-graph.md)

## Scope

- **Namespace:** `Radius.*` only (types authored under `Applications.Core/*` are out of scope).
- **API version:** the `Radius.Core/applications/getGraph@2025-08-01-preview` response shape (the type this static graph produces). The static graph itself does **not** call any API — it is built entirely from the compiled Bicep template. The API-version qualifier applies only to the response model it targets.
- **In scope:** the static (modeled) graph produced by `rad app graph <app.bicep>` — [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go).
- **Out of scope:** `Applications.Core/*`, older API versions (`v20231001preview` and predecessors), the runtime graph handler (see [Why the runtime graph needs no change](#why-the-runtime-graph-needs-no-change)), and any server-side redaction code.

## Deliverable

Two changes shipped together:

1. **Populate `ApplicationGraphResource.Properties` on every static-graph node.** The field exists on the response model but is left `nil` today; this effort starts populating it from the compiled Bicep resource body, using the same drop-list (`provisioningState`, `connections`, `status`) the runtime graph applies in `getResourceTypeSpecificProperties`.
2. **Redact `x-radius-sensitive` paths in that newly-populated `Properties` bag** using the schema information carried by the Bicep extension `types.tgz` for each resource type.

Redaction is only meaningful once (1) is done — the whole point of populating `Properties` is that operators want to see resource-specific configuration in the graph, and sensitive properties must be nulled out along that path. The two ship as one unit; there is no intermediate state where properties are populated but not redacted.

## Problem statement

The static graph is built entirely client-side from the compiled ARM JSON of a Bicep file — no control-plane calls, no APIs. Today `buildModeledResource` in [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go) does not populate `ApplicationGraphResource.Properties` at all; the field is left `nil`. As a result the static graph shows resource identity, connections, and dependencies but not the resource-specific configuration operators actually need in the diff view.

The desired behavior is:

> Populate `Properties` per resource — matching the shape of the runtime graph — **and** null out any property marked `x-radius-sensitive` in the resource type's schema.

Because the static graph is offline, we cannot look up the schema over the network at graph-build time.

## Why the runtime graph needs no change

Two-layer guarantee already in place:

1. **Write path redacts at rest.** `dynamicrp` encrypts sensitive fields on create/update; the record's `Properties` bag is stored with sensitive keys already `null`. Both `GetResourceWithRedaction` ([pkg/dynamicrp/frontend/getresource.go:71-77](../../../pkg/dynamicrp/frontend/getresource.go)) and `ListResourcesWithRedaction` ([pkg/dynamicrp/frontend/listresources.go:76-82](../../../pkg/dynamicrp/frontend/listresources.go)) fast-path `Succeeded` resources for this reason.
2. **Read path redacts explicitly for non-`Succeeded`.** Both controllers call `schema.RedactFields(resource.Properties, sensitiveFieldPaths)` for `Updating` / `Accepted` / `Failed` states.

The runtime graph calls those LIST endpoints via `listAllResourcesByApplication` / `listAllResourcesByEnvironment`, then copies the already-redacted properties into `ApplicationGraphResource.Properties` at [pkg/corerp/frontend/controller/applications/graph_util.go:328](../../../pkg/corerp/frontend/controller/applications/graph_util.go). No additional graph-layer redaction is needed for the runtime path.

## Approach: read `x-radius-sensitive` from the local Bicep extension cache

Every Bicep extension declared at the top of an `app.bicep` (`extension radius`, `extension aws`, `extension myprovider`, etc.) is packaged as a `types.tgz` archive containing two files:

```text
index.json    # { "resources": { "Namespace/Type@Version": { "$ref": "types.json#/N" }, ... }, "settings": { ... } }
types.json    # the full type registry with "sensitive": true on marked properties
```

The `"sensitive": true` marker is emitted by [bicep-tools/pkg/converter/converter.go](../../../bicep-tools/pkg/converter/converter.go) directly from the `x-radius-sensitive` annotation in the source YAML manifest. Same source of truth `dynamic-rp` uses server-side.

The Bicep CLI resolves each extension into a content-addressed local cache under `~/.bicep/`:

```text
~/.bicep/local/sha256_<hash>/types.tgz    # local files (../out/x.tgz)
~/.bicep/br/<registry-path>/**/types.tgz  # OCI-resolved (br:registry/name:tag)
```

Because `bicep build` is invoked as one of the first steps of `runModeled` (in [pkg/cli/cmd/app/graph/graph.go](../../../pkg/cli/cmd/app/graph/graph.go)), the cache is populated for every extension referenced by the app **before** the static graph is built. This gives us schema-driven redaction without a UCP call.

## Design

### New package: `pkg/cli/bicep/extensions`

Single public interface:

```go
// SensitivePathResolver resolves the dot-notation paths of properties marked
// `x-radius-sensitive` for a fully-qualified Bicep resource type, e.g.
// "Radius.Data/sqlServerDatabases@2025-08-01-preview".
type SensitivePathResolver interface {
    Resolve(ctx context.Context, typeAndVersion string) ([]string, error)
}
```

Returned paths use the same dot-notation format as [pkg/schema/annotations.go](../../../pkg/schema/annotations.go) so the caller can hand them straight to `schema.RedactFields` — the same function the runtime path uses.

### `CacheResolver`

The default production implementation:

1. **Locate** the Bicep cache root. Default `~/.bicep/`; overridable for tests.
2. **On first call in a graph build**, walk `~/.bicep/local/**/types.tgz` and `~/.bicep/br/**/types.tgz` and build an in-memory index `type@version → (tgz path, $ref)`. One walk per graph build; results memoized.
3. **Resolve one type**:
   - Look up `type@version` in the index. If missing → cache-refetch fallback (below).
   - Load and cache `types.json` from the target tgz.
   - Follow the `$ref` (e.g., `types.json#/18`) to the type definition.
   - Recursively walk `properties`, collecting dot-paths for every property whose target type carries `"sensitive": true`. Includes nested objects and array items.
4. **Return** `([]string, nil)` on success, or `ErrTypeNotFound` on miss.

### Cache-refetch fallback

Per the requirement *"if for some reason the cache location is wrong or misses a type, I want the code to fetch the extension again"*:

- On the first `ErrTypeNotFound` for a given `BuildModeledGraph` invocation, call `bicep restore --force <app.bicep>` via the existing `bicep.Interface.Call` helper the CLI already uses to shell out to Bicep. This re-resolves and re-downloads every extension declared in the file's `bicepconfig.json` scope.
- Rescan the cache after `bicep restore` completes. If the type is now indexed, use it. If still not indexed → surface `ErrTypeNotFound`.
- The refetch is attempted **at most once per graph build**, gated by a `sync.Once`, so a bicepconfig-level misconfiguration cannot trigger a fetch storm.

### Failure semantics — fail-closed on Properties, never fail the graph

Per the requirement *"if this fails for some reason, I want it to just stick to what we display for static graph today and not bring in resource-specific properties"*:

For each resource in `buildModeledResource`:

```text
paths, err := resolver.Resolve(ctx, typeAndVersion)
switch {
case err == nil:
    Properties = redact(bicepProperties, paths)   // populated + redacted
case errors.Is(err, ErrTypeNotFound):
    Properties = nil                              // fall back to today's shape
    logOnce(warning, "no cached schema for %s")
default:
    Properties = nil                              // any other resolver error
    logOnce(warning, "sensitive-path resolution failed for %s: %v")
}
```

The graph itself — ID, name, type, connections, output resources, diff hash, icon hash — is always emitted. Redaction failure never fails the command. This preserves the "static graph works everywhere" invariant.

### Changes to `pkg/cli/graph/modeled.go`

1. **Preserve the API version.** [`stripAPIVersion`](../../../pkg/cli/graph/modeled.go) currently discards the `@version` suffix. The graph builder needs the fully-qualified `type@version` string to key into the cache. Introduce a small helper `splitTypeAndVersion(t string) (typeName, version, fullKey string)` and store the full key alongside the stripped type on the working entry.
2. **Widen `BuildModeledGraph`.** New signature:

   ```go
   func BuildModeledGraph(
       ctx context.Context,
       template map[string]any,
       resolver extensions.SensitivePathResolver,
   ) (*corerpv20250801preview.ApplicationGraphResponse, error)
   ```

   The single caller in [pkg/cli/cmd/app/graph/graph.go](../../../pkg/cli/cmd/app/graph/graph.go) constructs a `CacheResolver` (with the workspace-defined bicep interface for the refetch path) and passes it through. Test callers use a fake resolver — no I/O.
3. **Populate `Properties`.** In `buildModeledResource`, after `ComputeDiffHash`, build a `Properties` bag from the raw Bicep properties minus the same set of keys the runtime graph drops (`provisioningState`, `connections`, `status`) — matching `existingKeys` in [pkg/corerp/frontend/controller/applications/graph_util.go](../../../pkg/corerp/frontend/controller/applications/graph_util.go). Apply the resolver + `schema.RedactFields`. On resolver failure, leave `Properties` `nil`.

### Order of operations for `DiffHash`

`DiffHash` is computed over the **authored** properties (pre-redaction), matching the runtime side. Rationale: the hash exists to detect authored changes; nulling sensitive values before hashing would make the hash stable across secret rotations, which is the wrong signal for the diff-detection use case. The hash itself is one-way and does not surface plaintext.

## Testing

All offline, no I/O to the real `~/.bicep`:

- **Unit tests for `CacheResolver`** with a temp-dir cache seeded from synthetic `types.tgz` files built at test time (Go's `archive/tar` + `compress/gzip`). Covers happy path, cache miss with successful refetch (mocked `bicep restore` writes a new tgz), and cache miss with failed refetch.
- **Unit tests for `BuildModeledGraph`** with an in-memory `SensitivePathResolver` fake. Covers: populated + redacted, `ErrTypeNotFound` → nil Properties, other error → nil Properties, `existingKeys` drops, and the graph itself renders regardless.
- **One end-to-end test** that seeds a temp Bicep cache dir with a small synthetic tgz declaring a `x-radius-sensitive: true` property and asserts the full pipeline connects.

## What this design deliberately does not do

- **No `redactedPaths` sidecar** on the wire response. Redaction is expressed as plain `null` per user directive; consumers cannot distinguish "unset" from "redacted" — that is accepted for now.
- **No `bicepconfig.json` parsing.** The compiled ARM JSON already lists every type actually used in the app; there is no need to enumerate declared extensions.
- **No UCP call.** Ever.
- **No changes to the runtime graph** — already redacted upstream.
- **No changes to `Applications.Core/*` or older API versions.**

## Files touched

- New: `pkg/cli/bicep/extensions/resolver.go`, `pkg/cli/bicep/extensions/cache_resolver.go`, corresponding `_test.go`.
- Modified: `pkg/cli/graph/modeled.go` (widen `BuildModeledGraph`, preserve API version, populate + redact `Properties`), `pkg/cli/graph/modeled_test.go` (fake resolver).
- Modified: `pkg/cli/cmd/app/graph/graph.go` (construct + pass the resolver).
- No changes to `pkg/corerp/frontend/controller/applications/**`, `pkg/dynamicrp/**`, or any API-generated code.
