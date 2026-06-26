# `@radius-project/typespec-bicep-types`

A native [TypeSpec emitter](https://typespec.io/docs/extending-typespec/emitters-basics/) that generates Bicep extensibility types (`types.json`, `types.md`, and per-resource `docs/*.md`) **directly from TypeSpec**, replacing the deprecated AutoRest-based [`@autorest/bicep`](../autorest.bicep/) extension.

It is invoked the same way as the other emitters already in this repo (`@azure-tools/typespec-go`, `@azure-tools/typespec-autorest`):

```bash
tsp compile . --emit=@radius-project/typespec-bicep-types
```

## Status: Phase 1 (in progress)

This package is being introduced in phases. See the design note for the full plan and rationale: [`eng/design-notes/tools/2026-06-autorest-bicep-to-typespec-emitter.md`](../../../../eng/design-notes/tools/2026-06-autorest-bicep-to-typespec-emitter.md).

| Phase | Scope                                                                                                                        | State       |
|-------|------------------------------------------------------------------------------------------------------------------------------|-------------|
| 0     | Buildable emitter scaffold wired into the TypeSpec compiler with no AutoRest.                                                | Done        |
| 1     | Port resource discovery + type translation; reach byte-identical golden files for `Applications.Messaging`.                  | In progress |
| 2     | Remaining namespaces (`Applications.{Core,Dapr,Datastores}`, `Radius.Core`); `properties`-bag flatten + discriminated types. | Pending     |
| 3+    | Pipeline wiring, dual-run CI equivalence gate, flip default, delete AutoRest.                                                | Pending     |

Landed so far in Phase 1:

- The toolchain is aligned with the repo: Node 24 (root [`.node-version`](../../../../.node-version)), pnpm, TypeScript 6, vitest 4, and TypeSpec 1.13 / Azure Resource Manager 0.69.
- The upstream `bicep-types` serializer is wired in **from npm** (`bicep-types@0.6.49-g556bf5edad`) - the same commit `autorest.bicep` pins via a git dependency, but published to npm, so it needs no bash-only `postinstall` symlink and is fully cross-platform.
- `$onEmit` discovers ARM resources (`getArmResources`), maps Radius/UCP scopes, builds the standardized `id`/`name`/`type`/`apiVersion` envelope, and writes real `types.json` + `types.md` per `namespace/apiVersion` using the upstream writers.
- `parseType` translates the resource body model's properties - scalars, enums, literal unions, arrays, records (dictionaries), and nested and cyclic models - with `@doc` descriptions, unit-tested against the TypeSpec test host.
- The `properties`-bag `x-ms-client-flatten` hoisting is ported: the bag's children are surfaced as ReadOnly aliases (with an all-or-nothing collision check) alongside the kept wrapper, detected via the same `@extension("x-ms-client-flatten", true)` marker the AutoRest path read.
- Property flags derive from TypeSpec `@visibility`: read-only -> `ReadOnly`, write-only -> `WriteOnly`, read-write -> `Required` when non-optional (no `@visibility` means read-write), matching the AutoRest flag output.
- `list*` resource functions (e.g. `listSecrets`) are emitted: each ARM action (from `operations.actions`) becomes a `FunctionType` - parameters from the request body, output from the response body - attached to the resource's `functions` map. Validated end-to-end against the real `Applications.Messaging` spec.
- Property **ordering** matches the golden byte-for-byte: effective properties are collected own-first then up the base chain (the AutoRest `[schema, ...parents]` order), so the flattened `properties` children and wrapper precede the inherited `tags`/`location`/`systemData`. Confirmed against the real spec - the `rabbitMQQueues` body property order is identical.
- Discriminated (polymorphic) types are emitted as Bicep `DiscriminatedObjectType`s: the `@discriminator` base's shared properties (minus the discriminator) become `baseProperties`, and each subtype (from `derivedModels`) becomes an element keyed by its discriminator value (sorted to match AutoRest). Validated against the real spec.
- Per-resource `docs/*.md` are emitted by a port of the AutoRest `writers/markdown-table.ts` (lodash swapped for native helpers), structurally identical to the golden (181/181 lines for `rabbitMQQueues`).

**Pilot status:** compiling the real `Applications.Messaging` spec yields a `types.json` whose **type-kind census and body property order match the golden**, plus a structurally identical `docs/*.md`. A byte-level diff surfaces a small set of **re-baseline-review items** where the emitter is faithful to the TypeSpec source but the legacy golden differs: descriptions taken from each property's `@doc` (vs the golden's type-level/synthetic docs), `Record<string>` rendered as `Record` (vs the golden's named `TrackedResourceTags`/`RecipeParameters`), extensible enums keeping their `| string` openness, and `location` flagged `Required`. These are reconciled when the goldens are re-baselined.

Remaining:

- Apply the emitter to the remaining namespaces (`Applications.{Core,Dapr,Datastores}`, `Radius.Core`).
- Re-baseline the golden files from the emitter (reviewing the items above) and wire the live `tsp compile` golden check into CI (Phase 3).
- Flip `generate-bicep-types-core` to run the emitter, then delete `autorest.bicep/` and `generator/`.

The unified-index step is already relocated here: `index-builder.ts` + `cmd/rebuild-index.ts` produce a **byte-identical** `index.json`/`index.md`, and the `rebuild-bicep-types-index` Make target now calls this package (so `generator/`'s last AutoRest-free piece is replaced - only its AutoRest driver remains in use).

## Layout

```text
src/
  index.ts              # exports $lib + $onEmit (the TypeSpec library/emitter contract)
  lib.ts                # $lib definition and emitter options
  emitter.ts            # $onEmit: discover resources, emit types.json + types.md + docs/*.md
  bicep.ts              # single import seam for the upstream bicep-types library
  resource-discovery.ts # discover ARM resources from the TypeSpec graph (getArmResources)
  scopes.ts             # ARM resource kind -> Bicep ScopeType
  standardized-props.ts # the id/name/type/apiVersion resource envelope
  type-translator.ts    # parseType (TypeSpec type -> Bicep type) + build the resource type
  index-builder.ts      # build the unified index.json + index.md (relocated from generator/)
  writers/
    markdown-table.ts   # per-resource docs/*.md table writer (ported from AutoRest)
  cmd/
    rebuild-index.ts    # CLI for the `rebuild-bicep-types-index` Make target
test/
  emitter.test.ts            # $lib smoke tests
  standardized-props.test.ts # envelope flags + types.json serialization
  scopes.test.ts             # scope mapping
  type-translator.test.ts    # parseType: scalars, arrays, records, unions, nesting, cycles
  flatten.test.ts            # properties-bag x-ms-client-flatten hoisting
  visibility.test.ts         # @visibility -> ReadOnly/WriteOnly/Required flags
```

Modules still to come with the body-translation increments: `flatten.ts` and `writers/markdown-table.ts` (per-resource `docs/*.md`).

## Develop

```bash
pnpm -C hack/bicep-types-radius/src/typespec-bicep-types install
pnpm -C hack/bicep-types-radius/src/typespec-bicep-types build
pnpm -C hack/bicep-types-radius/src/typespec-bicep-types test
```

The toolchain follows the repo: Node 24 (root [`.node-version`](../../../../.node-version)) via fnm/corepack, with pnpm as the package manager.
