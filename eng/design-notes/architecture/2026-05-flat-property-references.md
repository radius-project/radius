# Flat property references for Radius resources

**Status**: implemented (Radius side); pending deployment-engine release
**Authors**: @willdavsmith
**PR**: [#12001](https://github.com/radius-project/radius/pull/12001)
**Issue**: [#12000](https://github.com/radius-project/radius/issues/12000)
**Deployment-engine PR**: companion change on `azure-octo/deployment-engine`
branch `willdavsmith/flatten-properties-aliases`

## Summary

Radius resources follow the ARM resource envelope: user-defined fields live
under `properties`. Bicep references therefore have to include `.properties.`,
e.g. `gateway.properties.url` or `container.properties.container.image`. This
is a small but high-visibility UX wart that shows up across recipes, tests,
and docs.

This note describes how flat references (`gateway.url`,
`ctnr.container.image`) are enabled while keeping the wire format and existing
templates unchanged.

## Non-goals

- **Authoring flat is not supported.** Templates still write
  `resource r 'Applications.Core/containers@…' = { properties: { … } }`.
  This keeps all existing Radius `.bicep` files (and recipes) working
  unmodified.
- **No OpenAPI/wire-contract change.** The on-wire shape still has
  `properties` as the canonical field; aliases are additive convenience.
- **No upstream Bicep change.** Upstream issue
  [Azure/bicep#2052](https://github.com/Azure/bicep/issues/2052) (open since
  2021, no progress) explicitly defers this to RPs.

## Design

Two coordinated changes, neither of which requires changes to upstream Bicep
or Azure:

### 1. Bicep type generator emits read-only flat aliases

`hack/bicep-types-radius/src/autorest.bicep/src/type-generator.ts` honors the
existing `@extension("x-ms-client-flatten", true)` annotation on the
`TrackedResourceRequired.properties` model. For every child of the
flattened `properties` object, the generator emits a hoisted property on the
resource type as a **`ReadOnly` projection** (the `Required` flag is
stripped, so it never participates in authoring validation).

- `properties` itself is preserved unchanged, so authoring continues to use
  `resource r '…' = { properties: { application: app.id, … } }`.
- Writing to a hoisted alias at the top level (e.g.
  `r.container = {…}`) is correctly rejected by Bicep because the alias is
  ReadOnly.
- Reading from a hoisted alias (`r.container.image`) type-checks because
  Bicep sees the alias as a valid property on the resource.

This makes templates **compile** against flat references. By itself it is
not enough — Bicep emits the alias name verbatim into the ARM JSON
expression (`[reference('r').container.image]`), and the deployment engine
needs to be able to resolve that expression at runtime.

### 2. Deployment engine flattens reference() lookups

The deployment engine (`azure-octo/deployment-engine`, an Radius-owned fork
of the ARM deployment engine) is responsible for evaluating ARM template
expressions, including `reference('foo')`. Internally it stores the result
of each resource deployment as a `JToken` in a `referenceValueLookup`
dictionary, then performs property lookups on that token when expressions
are evaluated.

By default, the stored `JToken` is produced by `metadata.Resource.ToJToken()`,
which serializes only the standard envelope keys (`id`, `name`, `type`,
`location`, `tags`, `properties`, `systemData`, …). Any top-level aliases on
the wire are dropped before they reach the expression engine, which is why
RP-side flatten is not sufficient.

The companion deployment-engine change (`JTokenFlattenExtensions`) hoists
each non-reserved child of `properties` onto the top level of that `JToken`
as a deep-cloned read-only alias, after the typed serialization:

- Reserved envelope keys (`id`, `name`, `type`, `location`, `tags`,
  `properties`, `systemData`, `kind`, `etag`, `sku`, `identity`, `plan`,
  `managedBy`, `extendedLocation`, `zones`, `resources`) are never splatted
  and never overwritten.
- Existing top-level fields are never overwritten.
- The hoist is shallow (one level) and is idempotent.
- Wired into `TryGetResourceReferenceFromSequencerAction` at the two
  non-extensible code paths (`DeploymentResourceJob`, `DeploymentUCPJob`);
  extensible-resource branches construct a fixed `{ Properties: … }` shape
  and are unaffected.

With both changes in place, `reference('r').container.image` resolves to the
same value as `reference('r').properties.container.image`.

## What was tried and rejected

- **Pure Radius-side response flatten** (initial attempt). Decoding each
  resource response on the wire and splatting `properties` to the top level
  has no effect on `reference()` resolution because the deployment engine
  rebuilds the resource shape from its typed model before evaluating
  expressions. The wire-level aliases are silently dropped. This approach
  was implemented, tested against CI, observed to fail end-to-end, and
  reverted in favor of the deployment-engine change.
- **Upstream Bicep `x-ms-client-flatten` support**
  ([Azure/bicep#2052](https://github.com/Azure/bicep/issues/2052)). The
  Bicep maintainers explicitly deferred this design back to RPs in 2021 and
  there has been no movement since. Not a viable path on any reasonable
  timeline.

## Change-safety guidance

- **Adding a new property under `properties`**: nothing to do — the type
  generator picks it up automatically; the DE flatten is generic.
- **Adding a property whose name collides with a reserved envelope key**:
  it will be skipped by the DE flatten; flat references won't work for that
  field. Avoid such names if a flat alias is desired.
- **Adding a property whose name collides with an existing top-level
  envelope field on a specific resource**: the existing top-level value
  wins; the flat alias is skipped. This is rare in practice.
- **Resources without a `properties` object** (e.g. proxy resources that
  don't define one): the flatten is a no-op.

## Open questions

- Should the reserved-key list live in a shared place between the type
  generator (TypeScript) and the deployment engine (C#)? Currently
  duplicated, which is fine for a stable list but worth revisiting if it
  starts to drift.
