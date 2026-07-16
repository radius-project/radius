# Sensitive Fields in the Application Graph

Status: Draft
Author: (fill in)
Date: 2026-07-15

## Problem statement

Resource-type schemas can mark individual properties as sensitive via the
`x-radius-sensitive: true` annotation (see
[`bicep-tools/pkg/manifest/manifest.go`](../../../bicep-tools/pkg/manifest/manifest.go)
and [`pkg/schema/annotations.go`](../../../pkg/schema/annotations.go)). Radius
encrypts those values at rest and redacts them from the LIST/GET API
responses that dynamic-rp serves. However, we need to confirm and codify how
those redacted values appear in the **application graph** (the
`Radius.Core/applications/getGraph@2025-08-01-preview` response) and answer a
disambiguation question the current wire contract cannot answer:

> When a graph consumer sees `"password": null` in a resource's `properties`
> map, is that because the property was **never set** by the user, or because
> it **was set but redacted** for security?

Both cases produce identical JSON today, which makes it impossible for a
downstream consumer (CLI text renderer, browser extension, third-party
tooling) to render "[REDACTED]" correctly and equally impossible to tell a
platform engineer "this field is empty, please set it" without risk of that
being a security surprise.

## What exists today

### 1. Schema-declared sensitive paths

- `x-radius-sensitive: true` on a property (or on an array's `items` /
  `additionalProperties`) marks that path as sensitive.
- [`schema.ExtractSensitiveFieldPaths`](../../../pkg/schema/annotations.go)
  walks the OpenAPI schema and returns dot-notation paths, e.g.:
  - `credentials.password`
  - `secrets[*].value`
  - `config[*]`
- Handles object properties, array items, and additionalProperties (maps).

### 2. Redaction happens at the read boundary

The dynamic-rp read paths all fetch sensitive paths from the schema and pass
them to [`schema.RedactFields`](../../../pkg/schema/annotations.go), which
walks the data and sets each matching leaf to `nil` — which the autorest
serializer emits as JSON `null`:

- LIST: [`pkg/dynamicrp/frontend/listresources.go`](../../../pkg/dynamicrp/frontend/listresources.go) (`ListResourcesWithRedaction`).
- GET: [`pkg/dynamicrp/frontend/getresource.go`](../../../pkg/dynamicrp/frontend/getresource.go).
- Encryption filter: [`pkg/dynamicrp/frontend/encryptionfilter.go`](../../../pkg/dynamicrp/frontend/encryptionfilter.go).
- Backend controller: [`pkg/portableresources/backend/controller/createorupdateresource.go`](../../../pkg/portableresources/backend/controller/createorupdateresource.go).

Core-RP resource types (containers, gateways, environments, applications) do
not inline secrets in their LIST responses at all — secrets live behind
dedicated `/listSecrets` endpoints. So the graph never sees a raw secret
value for core-RP types.

### 3. How the graph consumes this

The graph pipeline in
[`pkg/corerp/frontend/controller/applications/graph_util.go`](../../../pkg/corerp/frontend/controller/applications/graph_util.go)
calls the ARM LIST endpoint per resource type, then hands the already-redacted
property bag to `getResourceTypeSpecificProperties`, which copies it verbatim
(minus `provisioningState`, `connections`, `status`) into the graph node's
`properties` map. The doc comment on `getResourceTypeSpecificProperties`
records the contract: it depends on upstream LIST handlers having redacted
secrets first.

Net effect: **on the happy path the graph is free of raw
`x-radius-sensitive` values today**, but only implicitly, via LIST redaction.
The graph layer itself performs no schema-based redaction as a safety net.

## The disambiguation problem

The current wire contract is:

| Case                                    | JSON wire form         |
| --------------------------------------- | ---------------------- |
| Property never set                      | `"foo": null` (or key absent) |
| Property set but marked sensitive       | `"foo": null`          |
| Property set to a non-nil value         | `"foo": <the value>`   |

The two `null` cases are indistinguishable from the wire. A consumer cannot
choose between rendering "not configured" and "[REDACTED]" without a
side-channel signal.

### Why not a sentinel string?

- `"foo": "[REDACTED]"` works only for string properties. For int, bool,
  object, or array properties, `"[REDACTED]"` breaks the wire type and any
  strict-schema client (TypeScript, autorest) rejects the response.
- Type-aware sentinels (`"[REDACTED]"` for strings, `null` for everything
  else) make the redaction asymmetric — consumers still have to special-case
  and still can't tell "null number: unset or redacted?".

### Why not a wrapper envelope?

- Replacing every sensitive value with `{"$redacted": true}` is uniform but
  breaks the wire type for every property, regardless of kind. Strict-schema
  clients reject it universally.

### Why not schema-based disambiguation?

Every consumer could fetch the resource-type schema, look at whether the
path is `x-radius-sensitive`, and render accordingly. Zero wire change,
architecturally cleanest. Downsides:

- Every consumer (CLI, browser extension, external tooling) needs schema
  fetch logic and a per-`(namespace, typeName, apiVersion)` schema cache.
- One extra UCP call per distinct type per consumer session.
- Consumers written before the annotation existed can't opt in.

## Recommended solution: `redactedPaths` sidecar

Add an optional `redactedPaths: string[]` field to each
`ApplicationGraphResource` in the v20250801preview response. It enumerates
the dot-notation paths whose values have been redacted for security in the
current response.

### Wire example

```json
{
  "id": "/planes/radius/local/.../mySqlDatabases/db",
  "type": "Radius.Data/mySqlDatabases",
  "properties": {
    "host": "db.example.com",
    "port": 3306,
    "username": "admin",
    "password": null,
    "readReplica": null
  },
  "redactedPaths": ["password"]
}
```

### Consumer disambiguation rules

- Path listed in `redactedPaths` → **was set, hidden for security** → render
  "[REDACTED]" (or type-appropriate placeholder) in the UI.
- Path absent from `redactedPaths` and value is `null` → **unset**.
- Path absent from `redactedPaths` and value is present → real value.

### Why this works

- **Wire-safe.** No property-value type changes. Existing clients that don't
  know about `redactedPaths` simply ignore the extra field.
- **Type-agnostic.** Works uniformly for string, int, bool, object, array
  properties — the value is still `null`, the sidecar is the signal.
- **No consumer schema fetch.** Consumers get the disambiguation signal
  in-band with the graph response.
- **Cheap to compute.** The LIST redaction path already knows the sensitive
  path list per API version (it fetched them from the schema to do the
  nil-out). It publishes that list on each returned record; the graph layer
  copies it through, same way it copies `properties`.
- **Backwards compatible.** Additive on the v20250801preview surface. The
  legacy v20231001preview surface stays unchanged.

### Optional refinement: shared map

If the same sensitive-path list is repeated across many nodes of the same
type, we could dedupe to a top-level

```
"redactedPaths": {
  "Radius.Data/mySqlDatabases": ["password"],
  "Radius.Security/secrets":    ["data[*]"]
}
```

on the `ApplicationGraphResponse`. Slightly less wire but every consumer
needs the type→paths lookup. Per-resource is simpler to consume and rarely
larger in practice.

## Belt-and-suspenders: redact at the graph layer too

Independent of how we render redacted values, we should call
`schema.RedactFields` inside `getResourceTypeSpecificProperties` even though
LIST already redacts. Rationale:

- The graph layer's contract today ("LIST is the redaction boundary")
  depends on every upstream LIST handler getting redaction right.
- A second call at the graph layer costs one schema-cache lookup plus one
  map walk per node, and it guarantees the graph itself cannot leak a
  sensitive value even if a LIST handler misses.
- The `redactedPaths` sidecar is computed as a natural byproduct of that
  call, so the two changes ship together.

## Rejected alternatives (summary)

| Option                                              | Verdict                                                  |
| --------------------------------------------------- | -------------------------------------------------------- |
| Keep only `null`, no signal                         | Blocks the disambiguation the user needs.                |
| `"[REDACTED]"` sentinel for all types               | Breaks wire type for int/bool/object/array.              |
| Type-aware sentinel                                 | Still ambiguous for non-strings; asymmetric render.      |
| Envelope wrapper `{"$redacted": true}`              | Breaks wire type universally.                            |
| Schema-based disambiguation                         | Cleanest architecturally; requires every consumer to fetch and cache schemas. Viable as a follow-up. |
| **`redactedPaths` sidecar** (recommendation)        | **Wire-safe, type-agnostic, zero schema fetch on the consumer.** |

## Follow-ups / open questions

1. **Should `redactedPaths` live on each resource or on the response?** —
   Draft assumes per-resource. Response-level dedupe is a possible refinement
   if size matters.
2. **Encoding for paths inside `additionalProperties` / arrays** — reuse the
   same `[*]` and `[N]` grammar already produced by
   `ExtractSensitiveFieldPaths`; no new syntax needed.
3. **CLI rendering** — once the sidecar ships, `rad app graph`'s text
   renderer and the browser extension should show `[REDACTED]` for listed
   paths. Both are consumer-side and not blocked by the wire change.
4. **Do we redact core-RP inline containers today?** — Core-RP LIST does not
   inline secrets (they're behind `/listSecrets`). The doc contract on
   `getResourceTypeSpecificProperties` should be revisited if any core-RP
   type ever starts inlining sensitive data.
