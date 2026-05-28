# Flat property references for Radius resources

**Status**: implemented
**Authors**: @willdavsmith
**PR**: [#12001](https://github.com/radius-project/radius/pull/12001)
**Issue**: [#12000](https://github.com/radius-project/radius/issues/12000)

## Summary

Radius resources follow the ARM resource envelope: user-defined fields live
under `properties`. Bicep references therefore must include `.properties.`,
e.g. `gateway.properties.url` or `container.properties.container.image`.
This is a small but high-visibility UX wart.

This note describes how flat references (`gateway.url`, `ctnr.container.image`)
are enabled while keeping the wire format and existing templates unchanged.

## Non-goals

- **Authoring flat is not supported.** Templates still write
  `resource r 'Applications.Core/containers@…' = { properties: { … } }`.
  This keeps all ~145 existing Radius `.bicep` files (and recipes) working
  unmodified.
- **No OpenAPI/wire-contract change.** The on-wire shape still has `properties`
  as the canonical field; aliases are additive convenience.

## Design

Two coordinated changes, neither of which requires changes to upstream
Bicep or Azure.

### 1. Bicep type generator emits read-only aliases

`hack/bicep-types-radius/src/autorest.bicep/src/type-generator.ts` honors the
`x-ms-client-flatten: true` extension on `TrackedResourceRequired<TProperties>.properties`.
For each child of `properties`, the generator emits a top-level alias on the
resource body with the `ReadOnly` flag set (and `Required` stripped). The
original `properties` field is *also* emitted unchanged, so existing authoring
keeps working and Bicep's type checker rejects `ctnr.container = {...}`
(write to a `ReadOnly` field).

### 2. Radius RP hoists `properties` children on response

`pkg/armrpc/rest/flatten.go` defines `flattenPropertiesAliases` which is
called from `OKResponse.Apply`, `CreatedResponse.Apply`,
`CreatedAsyncResponse.Apply`, and `AcceptedAsyncResponse.Apply`. The helper:

1. Decodes the marshaled body into `map[string]any`.
2. Walks the value. For any object that contains a `"properties"` map, each
   child key is copied onto the parent object as a top-level alias. The
   original `properties` object is preserved.
3. For paginated list bodies (`{value: [...], nextLink}`), recurses into
   each element of `value`.
4. Re-marshals.

#### Rules

- **Reserved envelope keys are never overwritten or splatted.** The reserved
  set is: `id, name, type, location, tags, properties, systemData, kind, etag,
  sku, identity, plan, managedBy, extendedLocation, zones`.
- **Existing top-level keys win on collision.** If a key on the envelope
  already shadows a child of `properties`, the alias is skipped.
- **Shallow.** Only one level of hoisting; nested objects are copied by
  reference (alias and `properties.<key>` point at the same object).
- **Best-effort.** If flatten fails (it shouldn't, post-`json.Marshal`), the
  unflattened body is returned and the request still succeeds.

### Why this works end-to-end

ARM's `reference(x)` function returns the literal JSON body the RP returned
from its GET. With the alias splatted at the top level, `reference(ctnr).application`
resolves at deploy time. Bicep is a literal translator — it does not rewrite
property-access expressions — so the alias *must* exist on the wire.

## Alternatives considered

### A. Implement flatten purely in Bicep

Add a `flattenedFrom` metadata field to `ObjectTypeProperty` in
`bicep-types` and teach `Bicep.Core/Intermediate/ExpressionBuilder.cs` to
rewrite `r.x` ⇒ `reference(r).properties.x`. ~1-2 weeks of upstream work
in `Azure/bicep`, plus release/version-bump lead time, plus risk of
maintainer rejection. **Rejected** because the response-side flatten is
orders of magnitude smaller and unblocks the feature today.

### B. Per-type generated code

Rather than a generic JSON walk, change the autorest-generated marshalers
for each resource type to emit aliases. **Rejected**: many files, fragile,
no benefit over the generic walk.

### C. Outputs-only helper modules

Ship a Bicep module per resource that re-exports `.properties.*` at the
top level. **Rejected**: only solves the output-reference case, requires
adoption per template, and adds a new artifact to publish.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Response size roughly doubles for the `properties` payload | Aliases share references with `properties` in memory; only the JSON serialization is larger. If measured to be a problem, gate behind an `Accept`/query opt-out. |
| Strict OpenAPI consumers see "unknown" fields | All typed SDK clients (Go/C#/Python autorest-generated) ignore unknown fields. The OpenAPI spec can be amended later to declare each child as a read-only mirror via TypeSpec composition if formal contract alignment is desired. |
| Collision with envelope keys | Reserved-key list (above) prevents this. New envelope keys (if ARM ever adds any) must be added to the reserved set. |
| Async operation status bodies look superficially like resource envelopes | Async status bodies have no `properties` key and so are pass-through. Verified by unit test. |

## Testing

- `pkg/armrpc/rest/flatten_test.go` — 14 table-driven unit tests + an
  alias-identity test covering: single resource, reserved-key skip, collision
  skip, no-properties pass-through, non-object `properties`, null
  `properties`, paginated list, empty list, async status pass-through,
  empty body, malformed JSON, non-object top-level.
- `test/functional-portable/corerp/noncloud/resources/container_flatten_test.go` —
  end-to-end: deploys two containers; `ctnr2` consumes
  `ctnr.container.image`, `ctnr.container.ports.web.containerPort`,
  `ctnr.application` via flat aliases.
