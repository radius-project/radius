# Migrating `@autorest/bicep` to a Native TypeSpec Emitter

- **Author**: TBD
- **Status**: Proposed (Draft)
- **Related issue**: [#11425 - Retire AutoRest from the Bicep types pipeline](https://github.com/radius-project/radius/issues/11425)
- **Builds on**: [PR #12053](https://github.com/radius-project/radius/pull/12053) (pinned AutoRest core + modelerfour for determinism after the 2026-07-01 AutoRest retirement)
- **Affected code**: [`hack/bicep-types-radius/`](../../../hack/bicep-types-radius/), [`build/generate.mk`](../../../build/generate.mk), [`typespec/*/tspconfig.yaml`](../../../typespec/)

## TL;DR

`@autorest/bicep` is the last remaining consumer of deprecated AutoRest in this repository. It is an AutoRest *extension* that reads the modelerfour **CodeModel** (derived from OpenAPI, which is itself emitted from TypeSpec) and produces Bicep extensibility types (`types.json`, `types.md`, per-resource `docs/*.md`).

This proposal replaces that extension with a **native TypeSpec emitter** - a standard `tsp` library, tentatively `@radius-project/typespec-bicep-types`, invoked as `tsp compile . --emit=@radius-project/typespec-bicep-types`. The emitter reads the TypeSpec **type graph directly** (via the compiler API and the `@typespec/http` / `@azure-tools/typespec-azure-resource-manager` typekits) and reuses the **same** upstream `bicep-types` TypeScript library that already serializes `types.json`/`types.md` today. Only the *reader* changes: CodeModel -> TypeSpec type graph.

The result lets us delete the entire AutoRest dependency chain (AutoRest core, `@autorest/modelerfour`, `@autorest/extension-base`, `@autorest/codemodel`, the `autorest.bicep` extension, and the AutoRest-invoking generator driver) and removes OpenAPI/Swagger as an intermediate artifact on the Bicep critical path.

**Recommendation:** Option A (native TypeSpec emitter). It is the only option that is genuinely "a fully supported TypeSpec emitter," it removes the most code, and it collapses a three-stage pipeline (TypeSpec -> OpenAPI -> AutoRest CodeModel -> Bicep) into one stage (TypeSpec -> Bicep).

## Goals

- Eliminate every runtime and build-time dependency on AutoRest for Bicep type generation.
- Generate Bicep types **directly** from TypeSpec, with no OpenAPI/Swagger intermediate on the Bicep path.
- Produce **byte-identical** (or intentionally re-baselined) `types.json`, `types.md`, and `docs/*.md` for every namespace that flows through the AutoRest path today.
- Leave the published REST API contract (Swagger) and the Go SDK generation untouched.
- End with a single, owned, testable TypeScript codebase that the team can evolve.

## Non-goals

- Changing the on-disk Bicep extension format (the upstream `bicep-types` schema is unchanged).
- Changing the manifest-driven Go path (`bicep-tools/`); it is already AutoRest-free.
- Removing `@azure-tools/typespec-autorest`. It still emits Swagger for the API contract and examples validation - it is simply no longer on the Bicep critical path.
- Changing the Go client SDK generation, which already uses `@azure-tools/typespec-go` directly from TypeSpec.

## Background: how Bicep types are generated today

There are two **independent** pipelines that both produce the same artifact shape (`types.json` + index + docs), unified at the end into one publishable Bicep extension.

```mermaid
flowchart TD
    subgraph CORE["CORE path - generate-bicep-types-core (DEPRECATED AutoRest)"]
        TSP1[TypeSpec specs] -->|@azure-tools/typespec-autorest| OAPI[OpenAPI / Swagger]
        OAPI -->|AutoRest core 3.10.8| CM[modelerfour CodeModel]
        CM -->|autorest.bicep extension| TJ1[types.json + types.md + docs]
    end
    subgraph CONTRIB["CONTRIB path - generate-bicep-types-contrib (AutoRest-free)"]
        MAN[Resource-type manifests] -->|Go bicep-tools + bicep-types-go| TJ2[types.json + index.json + index.md + docs]
    end
    TJ1 --> IDX[rebuild-bicep-types-index -> unified index.json/index.md]
    TJ2 --> IDX
    IDX --> PUB[bicep publish-extension]
```

### What flows through the AutoRest path

Every namespace that has full TypeSpec + OpenAPI goes through `autorest.bicep`. Confirmed by output shape (`types.json` + `types.md` + `docs/`, *without* a per-namespace `index.json`/`index.md`):

| Namespace                 | API version          | Generated output                                                                                                                         |
|---------------------------|----------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `Applications.Core`       | `2023-10-01-preview` | [`generated/applications/applications.core/...`](../../../hack/bicep-types-radius/generated/applications/applications.core/)             |
| `Applications.Dapr`       | `2023-10-01-preview` | [`generated/applications/applications.dapr/...`](../../../hack/bicep-types-radius/generated/applications/applications.dapr/)             |
| `Applications.Datastores` | `2023-10-01-preview` | [`generated/applications/applications.datastores/...`](../../../hack/bicep-types-radius/generated/applications/applications.datastores/) |
| `Applications.Messaging`  | `2023-10-01-preview` | [`generated/applications/applications.messaging/...`](../../../hack/bicep-types-radius/generated/applications/applications.messaging/)   |
| `Radius.Core`             | `2025-08-01-preview` | [`generated/radius/radius.core/...`](../../../hack/bicep-types-radius/generated/radius/radius.core/)                                     |

The manifest/Go path (`Radius.Compute`, `Radius.Security`, plus contrib types) is **out of scope** - it never touched AutoRest. Its presence matters only because the new emitter's output must keep slotting into the same `rebuild-bicep-types-index` unification step.

### What `autorest.bicep` actually does

The extension is ~1,900 lines of TypeScript ([`hack/bicep-types-radius/src/autorest.bicep/src/`](../../../hack/bicep-types-radius/src/autorest.bicep/src/)):

| File                                                                                                             | Lines | Responsibility                                                                                                                                                                                                                                             |
|------------------------------------------------------------------------------------------------------------------|------:|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [`main.ts`](../../../hack/bicep-types-radius/src/autorest.bicep/src/main.ts)                                     |    89 | AutoRest plugin host; iterates provider definitions; writes `types.json`, `types.md`, `docs/*.md`.                                                                                                                                                         |
| [`resources.ts`](../../../hack/bicep-types-radius/src/autorest.bicep/src/resources.ts)                           |   599 | **Reader.** Walks the CodeModel: groups operations by ARM path, detects scope (tenant/subscription/resource group/management group/extension), pairs PUT/GET bodies, finds `list*` POST actions, parses `{name}` segments.                                 |
| [`type-generator.ts`](../../../hack/bicep-types-radius/src/autorest.bicep/src/type-generator.ts)                 |   895 | **Translator.** CodeModel schemas -> `bicep-types` via `TypeFactory`: primitives, enums, objects, dictionaries, arrays, `any`, constants, discriminated/polymorphic types, property flags, `x-ms-client-flatten` hoisting, standardized resource envelope. |
| [`utils.ts`](../../../hack/bicep-types-radius/src/autorest.bicep/src/utils.ts)                                   |    30 | `Result` helpers.                                                                                                                                                                                                                                          |
| [`writers/markdown-table.ts`](../../../hack/bicep-types-radius/src/autorest.bicep/src/writers/markdown-table.ts) |   345 | Per-resource `docs/*.md` table writer.                                                                                                                                                                                                                     |

Crucially, it **already depends on the upstream `bicep-types` library** for the output side (`TypeFactory`, `writeTypesJson`, `writeMarkdown`, all the `TypeBaseKind`/`ObjectTypePropertyFlags` enums). Only the *input* side is AutoRest-specific.

### Why this must change

- AutoRest is **deprecated** and retires 2026-07-01. PR #12053 pinned versions so the pipeline keeps working, but it is frozen, unsupported code.
- The pipeline is needlessly indirect: TypeSpec is the source of truth, yet Bicep types are derived from a *third-generation* artifact (TypeSpec -> OpenAPI -> CodeModel -> Bicep). Information is lost and re-inferred at each hop (e.g. ARM scope is reverse-engineered from URL paths in `resources.ts`).
- Maintaining an AutoRest extension requires understanding modelerfour's CodeModel, a niche skill set.

## What "a fully supported TypeSpec emitter" means

TypeSpec's [emitter framework](https://typespec.io/docs/extending-typespec/emitters-basics/) defines an emitter as a TypeSpec library that exports `$onEmit(context)`. It is invoked via `tsp compile` (the same command already used for `@azure-tools/typespec-go` and `@azure-tools/typespec-autorest`) and reads the program's type graph through the compiler API and **typekits**:

- `@typespec/http/experimental/typekit` adds `httpOperation` - path, verb, parameters, request/response bodies - exactly the information `resources.ts` currently reconstructs from OpenAPI URL strings.
- `@azure-tools/typespec-azure-resource-manager` exposes ARM metadata (resource kind, scope, `properties` envelope) as first-class decorators/state, removing the need to pattern-match `/providers/...` paths.

This is the same mechanism the Azure Bicep team itself uses; there is no AutoRest involved.

## Proposed target architecture

```mermaid
flowchart TD
    subgraph NEW["CORE path - native TypeSpec emitter (AutoRest-free)"]
        TSP[TypeSpec specs] -->|tsp compile --emit=@radius-project/typespec-bicep-types| EMIT[typespec-bicep-types emitter]
        EMIT -->|reuses upstream bicep-types TypeFactory + writers| TJ[types.json + index.json + index.md + docs]
    end
    subgraph CONTRIB["CONTRIB path - Go bicep-tools (unchanged)"]
        MAN[Manifests] --> TJ2[types.json + index.json + index.md + docs]
    end
    TSP -. unchanged .->|@azure-tools/typespec-autorest| OAPI[Swagger - API contract only]
    TJ --> IDX[rebuild-bicep-types-index]
    TJ2 --> IDX
    IDX --> PUB[bicep publish-extension]
```

### New package: `@radius-project/typespec-bicep-types`

Scaffolded with `tsp init --template emitter-ts`. Proposed home: `hack/bicep-types-radius/src/typespec-bicep-types/` (replacing `src/autorest.bicep/`).

```text
src/typespec-bicep-types/
  package.json            # tsp emitter library; deps: @typespec/compiler, @typespec/http,
                          #   @azure-tools/typespec-azure-resource-manager, bicep-types
  src/
    index.ts              # exports $lib + $onEmit
    emitter.ts            # $onEmit: discover resources, drive TypeFactory, write files
    resource-discovery.ts # PORT of resources.ts using httpOperation/ARM typekits
    type-translator.ts    # PORT of type-generator.ts: TypeSpec Type -> bicep-types
    standardized-props.ts # id/name/type/apiVersion envelope (carried over verbatim)
    flatten.ts            # properties-bag flattening to match x-ms-client-flatten output
    writers/markdown-table.ts  # carried over ~unchanged (operates on bicep-types, not CodeModel)
  test/
    *.test.ts             # uses @typespec/compiler test host (in-memory, no AutoRest)
```

`$onEmit` skeleton:

```ts
import { EmitContext } from "@typespec/compiler";
import "@typespec/http/experimental/typekit";
import { $ } from "@typespec/compiler/typekit";
import { TypeFactory } from "bicep-types";

export async function $onEmit(context: EmitContext<BicepEmitterOptions>) {
  if (context.program.compilerOptions.noEmit) return;
  for (const provider of discoverProviders(context.program)) {       // resource-discovery.ts
    const factory = new TypeFactory();
    const types = translateProvider(context.program, provider, factory); // type-translator.ts
    await writeArtifacts(context, provider, types);                   // types.json/types.md/docs
  }
}
```

### Reader mapping: AutoRest CodeModel -> TypeSpec type graph

The hard work is porting the reader. The mapping is direct because TypeSpec *is* the source the OpenAPI was generated from:

| `autorest.bicep` (CodeModel) concept                              | TypeSpec equivalent                                                                                |
|-------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| `CodeModel.operationGroups[].operations[]`                        | `$(program).httpOperation` over operations in the ARM namespace                                    |
| `HttpRequest.path`, `.method`                                     | `httpOperation.get(op).path` / `.verb`                                                             |
| Scope inferred by regex on `/providers/...`, `/subscriptions/...` | `@azure-tools/typespec-azure-resource-manager` resource metadata (scope is declared, not inferred) |
| PUT/GET body pairing (`getPutSchema`/`getGetSchema`)              | `httpOperation` request body + 200 response body for the resource's CRUD operations                |
| `list*` POST actions (`ResourceListActionDefinition`)             | ARM action operations (`@armResourceAction`) / POST `httpOperation`s                               |
| `{name}` path segment -> name schema                              | resource key property via ARM `@key` / path parameter                                              |
| `ObjectSchema` / `Property`                                       | `Model` / `ModelProperty` (typekit `model`, `modelProperty`)                                       |
| `ChoiceSchema` / `SealedChoiceSchema`                             | `Union` / `Enum` (typekit `union`, `enum`)                                                         |
| `DictionarySchema` (additionalProperties)                         | `Record<T>` (typekit `record`)                                                                     |
| `ArraySchema`                                                     | array / `T[]` (typekit `array`)                                                                    |
| `ConstantSchema` / string literal                                 | string literal types (typekit `literal`)                                                           |
| `x-ms-client-flatten` on the `properties` bag                     | the ARM `properties` model property; flatten replicated in `flatten.ts`                            |
| Property `required` / `readOnly`                                  | `modelProperty` optionality + `@visibility`/`@readonly`                                            |
| `language.default.description`                                    | `$(program).type.getDoc(type)`                                                                     |

### Semantics that must be preserved exactly

These behaviors in `type-generator.ts`/`resources.ts` are the re-baselining risk surface; each needs a focused test:

1. **Standardized resource envelope** - `id`, `name`, `type`, `apiVersion` properties with the correct flags (`ReadOnly`, `DeployTimeConstant`, `Identifier`). Carried over near-verbatim.
2. **Name schema parsing** - the resource name property (literal vs. parameterized) and its `Required` flag.
3. **`properties`-bag flattening** - the trickiest. AutoRest emits `x-ms-client-flatten: true` on the ARM `properties` envelope; `autorest.bicep` then *hoists flattened children as ReadOnly aliases while keeping the wrapper property*. The new emitter sees the nested `properties` model directly and must reproduce the identical hoisted-alias output. This is the single most important golden-file check.
4. **Discriminated/polymorphic types** - discriminator property + `elements` map, with each subtype's discriminator literal injected.
5. **Primitive/enum/dictionary/array/any mapping** - including `int`/`number`/`unixtime` -> integer, `byte-array` -> `any[]`, `uri`/`date`/`uuid`/`duration` -> string.
6. **List actions -> resource functions** - `list*` POST operations become `ResourceTypeFunction`s with request/response schemas.
7. **Scope detection** - tenant / subscription / resource group / management group / extension. Simpler here because ARM scope is declared in TypeSpec rather than inferred from URLs.
8. **Markdown output** - `types.md` and per-resource `docs/*.md` byte-for-byte (the writer already operates on `bicep-types`, so it ports with minimal change).

### Output shape: converge on the Go path

Today the AutoRest path emits `types.json` + `types.md` + `docs/`, and the unified index is built separately. The Go path additionally emits per-namespace `index.json` + `index.md`. **Recommendation:** have the new emitter also emit per-namespace `index.json` + `index.md` (via `bicep-types` `buildIndex`/`writeIndexJson`/`writeIndexMarkdown`, exactly as the Go converter does). This eliminates the last shape divergence between the two paths and lets `rebuild-bicep-types-index` treat all namespaces uniformly.

## Alternatives considered

### Option A - Native TypeSpec emitter (RECOMMENDED)

Port the reader to the TypeSpec type graph; keep the `bicep-types` output library.

- **Pros:** Directly answers the ask ("fully supported TypeSpec emitter"). Removes the most code. One-stage pipeline. Reuses the already-trusted `bicep-types` serializer and the existing markdown writers. Tests run on the in-memory TypeSpec test host (fast, hermetic, no AutoRest). Same approach as the Azure Bicep team.
- **Cons:** The `properties`-flatten and discriminator semantics must be matched exactly against golden files. Net-new emitter code (though smaller than today's extension, since scope/path inference largely disappears).

### Option B - Converge onto the Go `bicep-tools` + `bicep-types-go`

Route the CORE namespaces through the existing AutoRest-free Go converter.

- **Pros:** Reuses owned, tested Go code; single toolchain for both paths.
- **Cons:** `bicep-tools` is **manifest-driven**, not TypeSpec-driven. To feed it from TypeSpec we'd need either (a) an OpenAPI->`bicep-types-go` reader (re-introducing the OpenAPI intermediate we're trying to drop), or (b) a manifest projection of the full Applications.\* / Radius.Core schemas (large, lossy for discriminators/flatten). This does **not** produce "a TypeSpec emitter" and keeps a non-TypeSpec hop. Rejected for this goal, though it remains attractive as a longer-term *unification* of both paths.

### Option C - Status quo (pinned AutoRest)

Keep PR #12053's pinned versions indefinitely.

- **Pros:** Zero work now.
- **Cons:** Permanently frozen on deprecated, unsupported software; does not satisfy #11425. Rejected.

## Migration plan (phased, incremental, reversible)

Each phase is independently mergeable. The AutoRest path stays the source of truth until Phase 4, so `main` is never broken.

### Phase 0 - Scaffold (no behavior change)

- Scaffolded `@radius-project/typespec-bicep-types` at [`hack/bicep-types-radius/src/typespec-bicep-types/`](../../../hack/bicep-types-radius/src/typespec-bicep-types/) following the `tsp emitter-ts` template (ESM, `@typespec/compiler` + `@typespec/http` peer deps, `vitest`). Builds with `tsc`, tested with `vitest`.
- Toolchain aligned with the repo: Node 24 (root [`.node-version`](../../../.node-version)), pnpm, TypeScript 6, vitest 4, TypeSpec 1.13 / Azure Resource Manager 0.69.

### Phase 1 - First namespace to parity

- **`bicep-types` is published on npm** as `bicep-types@0.6.49-g556bf5edad` - the `g556bf5edad` suffix is the exact `Azure/bicep-types` commit that `autorest.bicep` pins via a git dependency. The emitter depends on it straight from npm, so it needs **no** bash-only `postinstall` symlink and is fully cross-platform - a strict simplification over the path it replaces.
- Landed: `getArmResources`-based resource discovery (replacing the OpenAPI path-parsing of `resources.ts`), Radius/UCP scope mapping (`ScopeType.None`, matching the golden `readableScopes`/`writableScopes` of `0`), the standardized `id`/`name`/`type`/`apiVersion` envelope (flags `10`/`25`/`10`/`10`, byte-matching the golden files), and real `types.json` + `types.md` emission through the upstream writers. Unit-tested.
- Landed: `parseType` - body-property translation reading the TypeSpec type graph directly (scalars, enums, literal unions, arrays, records/dictionaries, nested and cyclic models), with `@doc` descriptions. Unit-tested against the TypeSpec core test host (no AutoRest, no ARM libraries needed).
- Landed: the `properties`-bag `x-ms-client-flatten` hoisting - detected via the same `@extension("x-ms-client-flatten", true)` marker the AutoRest path read (from `getExtensions`), it surfaces the bag's children as ReadOnly aliases (all-or-nothing collision check, non-object/discriminated bags skipped) alongside the kept writable wrapper. Unit-tested against the TypeSpec test host with `@typespec/openapi`.
- Landed: property flags from TypeSpec `@visibility` (lifecycle) - read-only -> `ReadOnly`, write-only -> `WriteOnly`, read-write -> `Required` when non-optional (no `@visibility` means read-write); ReadOnly is never also Required. This is the TypeSpec equivalent of AutoRest's PUT/GET-schema flag inference and fixes the flags of envelope props like `systemData`/`provisioningState`. Unit-tested.
- Landed: `list*` resource functions - each ARM action (from `getArmResources(...).operations.actions`) becomes a `FunctionType` (parameters from the request body's properties, output from the 2xx response body) attached to the resource's `functions` map. Validated end-to-end: the real `Applications.Messaging` spec now emits `functions: { listSecrets }`.
- Landed: property **ordering** - effective properties are collected own-first then up the base chain (AutoRest's `[schema, ...parents]` order), so the flattened `properties` children and wrapper precede the inherited `tags`/`location`/`systemData`. Confirmed against the real spec: the `rabbitMQQueues` body property order is now **byte-identical** to the golden (`ORDER MATCH: true`).
- Landed: discriminated (polymorphic) types - a `@discriminator` model becomes a Bicep `DiscriminatedObjectType` whose `baseProperties` are the base's shared properties (minus the discriminator) and whose `elements` are the subtypes (`derivedModels`, recursing through nested discriminators) keyed by discriminator value and sorted to match AutoRest. Validated against the real spec.
- Landed: per-resource `docs/*.md` writer - the AutoRest `writers/markdown-table.ts` ported into the emitter (it already operated on `bicep-types`; only lodash was swapped for native helpers). Structurally identical to the golden (181/181 lines for `rabbitMQQueues`).
- **Pilot: structurally at parity, with re-baseline items.** Compiling the actual `Applications.Messaging` project through the emitter (via the compiler `compile()` API - the workspace's `tsp` CLI currently hits a `yargs` bug) produces, with **0 diagnostics**, a `types.json` whose **type-kind census and body property order match the golden** and a structurally identical `docs/*.md`. A byte-level diff of the doc surfaces a small set of **re-baseline-review items** where the emitter is faithful to the TypeSpec source but the legacy AutoRest golden differs: per-property `@doc` descriptions (vs the golden's type-level/synthetic docs), `Record<string>` rendered as `Record` (vs the golden's named `TrackedResourceTags`/`RecipeParameters`), extensible enums keeping their `| string` openness, and `location` flagged `Required`. These are decided during the golden re-baseline.
- Remaining: apply the emitter to the other namespaces (`Applications.{Core,Dapr,Datastores}`, `Radius.Core`), then re-baseline the goldens (reviewing the items above). Folding the compile-and-diff into a repeatable golden check is the Phase 3 pipeline wiring (the ARM package ships no test library, so the in-package unit tests use the core/HTTP/OpenAPI test hosts instead).

### Phase 2 - Remaining namespaces + flatten/discriminators

- Bring up `Applications.Datastores`, `Applications.Dapr`, `Applications.Core`, then `Radius.Core`.
- Implement `flatten.ts` and discriminated-type handling; iterate against golden files.
- Any intentional output differences are reviewed and the golden files re-baselined in a dedicated, clearly-labeled commit.

### Phase 3 - Pipeline wiring + CI gate

**Status (executed 2026-06):** satisfied by the repo's existing generated-code gate; no separate dual-run was needed.

- The emitter is wired into `make generate` through `generate-bicep-types` -> `generate-bicep-types-core` (the [`compile-projects`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/cmd/compile-projects.ts) driver), so the Bicep types regenerate in the same pass as every other generated artifact.
- The gate is the repo's standard [`lint.yaml`](../../../.github/workflows/lint.yaml) "Check code is up-to-date" job: it runs `make generate`, then `git diff --exit-code` (captured via a `continue-on-error` step), and a follow-up `Check failures` step fails the build on any drift. Because the committed [`generated/`](../../../hack/bicep-types-radius/generated/) trees are the contract, any emitter change that alters output must be committed as an explicit, reviewed re-baseline.
- The originally-planned **dual-run** (old AutoRest vs new emitter) equivalence gate is moot: it was only meaningful while both implementations coexisted, and AutoRest has been deleted. The single-run golden-file gate (the emitter must reproduce the committed trees) is the equivalent, simpler check.

### Phase 4 - Flip the default and delete AutoRest

- Make the emitter the default for `generate-bicep-types-core`.
- Delete the AutoRest extension and the AutoRest-invoking driver (see deletion list below).
- Update [`build/generate.mk`](../../../build/generate.mk) so `generate-bicep-types-core` runs the emitter and no longer builds/installs `autorest.bicep`. Implemented as a small programmatic driver, [`src/cmd/compile-projects.ts`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/cmd/compile-projects.ts), rather than `tsp compile --emit`: the projects' `tspconfig.yaml` already emit `@azure-tools/typespec-autorest` for the OpenAPI contract, so driving Bicep emission separately through the compiler API keeps the two outputs decoupled and guarantees a single shared compiler instance.

### Phase 5 - Dependency cleanup

- Remove AutoRest packages from `package.json`/lockfiles.
- Confirm no remaining references to `autorest`, `modelerfour`, `@autorest/*`, `readme.bicep.md`, or the OpenAPI-on-the-Bicep-path in Makefiles, scripts, and docs.

## What gets fully removed (proof the current can be deleted)

**Status (executed 2026-06):** done. `generate-bicep-types-core` now runs the emitter through the [`compile-projects`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/cmd/compile-projects.ts) driver, the committed `generated/` `types.json` were re-baselined from the emitter, and both directories below were deleted. The re-baseline (rather than byte-for-byte matching) was taken because the residual differences from the old AutoRest output are all modelerfour artifacts where the emitter reads TypeSpec faithfully or better: type **ordering**, structural **de-duplication** of identical types, synthesized `Record` names (e.g. `TrackedResourceTags`), and `$ref` **description** propagation. Two genuine behaviors were preserved as emitter fixes: a top-level `location` is never `Required`, and extensible enums render as the closed literal set.

The following are gone with no remaining references:

**Directories / files**

- `hack/bicep-types-radius/src/autorest.bicep/` - the entire extension (`main.ts`, `resources.ts`, `type-generator.ts`, `utils.ts`, `writers/`, its `package.json`, lockfile, jest/eslint config).
- The **entire** `generator/` package - its AutoRest driver (`cmd/generate.ts` + `config.ts`: `readme.bicep.md` generation, `autorest` invocation, `--specs-dir` walking) plus its index step. The index step was first **relocated** into the emitter package ([`src/index-builder.ts`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/index-builder.ts) + [`src/cmd/rebuild-index.ts`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/cmd/rebuild-index.ts), validated byte-identical), then the whole package was deleted.

**npm dependencies** (from the extension's `package.json`)

- `autorest`, `@autorest/extension-base`, `@autorest/codemodel`, `@autorest/modelerfour` (pinned by PR #12053), plus AutoRest-only transitive/dev deps no longer referenced.

**Build steps** (in [`build/generate.mk`](../../../build/generate.mk))

- The `pnpm -C .../autorest.bicep install && run build` step and the AutoRest invocation inside `generate-bicep-types-core`. (The `rebuild-bicep-types-index` target is **already rewired** to the emitter package's relocated `rebuild-index`.)

**Intermediate artifacts**

- OpenAPI/Swagger is no longer required *for Bicep*. (The `@azure-tools/typespec-autorest` emit stays for the REST API contract and example validation - that is a separate consumer.)

**What stays** (intentionally): the upstream `bicep-types` library, the **relocated** unified-index step (now [`src/index-builder.ts`](../../../hack/bicep-types-radius/src/typespec-bicep-types/src/index-builder.ts) in the emitter package, not `generator/`), the Go `bicep-tools` path, `@azure-tools/typespec-autorest` (for Swagger), and all `generated/` golden files (re-baselined from the emitter).

## Validation strategy

- **Golden-file diff** - the primary gate. The committed [`generated/`](../../../hack/bicep-types-radius/generated/) trees are the contract. CI compares emitter output against them; any change is an explicit, reviewed re-baseline.
- **Dual-run equivalence (Phase 3)** - run AutoRest and the emitter on the same specs in CI and assert identical trees before flipping the default.
- **Emitter unit tests** - use the `@typespec/compiler` test host to compile focused fixtures (a flattened `properties` bag, a discriminated union, a `list*` action, each scope type) and assert the produced `bicep-types` objects.
- **End-to-end** - `bicep publish-extension` on the unified index plus an existing Bicep-based functional test still deploys.

## Risks and mitigations

| Risk                                                                                                                                                        | Mitigation                                                                                                                                                    |
|-------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `properties`-flatten output drifts from `x-ms-client-flatten`                                                                                               | Dedicated golden fixtures; dual-run equivalence gate in Phase 3; flatten isolated in `flatten.ts` with unit tests.                                            |
| Discriminated-type ordering/keys differ                                                                                                                     | Compare against golden `types.json`; sort deterministically as the Go converter already does.                                                                 |
| TypeSpec emitter framework is still marked experimental                                                                                                     | We use only the **stable** compiler API + `@typespec/http` typekit (already a dependency for SDK generation), not the experimental Alloy/JSX rendering layer. |
| Hidden coupling to OpenAPI-only constructs (e.g. KeyVault `additionalFiles` in [`config.ts`](../../../hack/bicep-types-radius/src/generator/src/config.ts)) | Audit during Phase 2; Radius specs do not pull external Azure readmes, so this AutoRest-specific shim is expected to be deletable.                            |
| Re-baseline churn obscures real diffs                                                                                                                       | Re-baselines land in isolated, labeled commits separate from logic changes.                                                                                   |

## Open questions

- **Emit wiring** - add the Bicep emitter to each existing `tspconfig.yaml`, or a separate `tspconfig.bicep.yaml` per provider to keep Swagger/Go/Bicep emit concerns isolated?
- **Package publishing** - keep `@radius-project/typespec-bicep-types` internal to the repo (workspace-only), or publish it for reuse by downstream resource-provider authors?
- **Long-term unification** - after Option A ships, is there appetite to also pursue Option B's convergence (one converter for both TypeSpec-backed and manifest-backed types), or do the two readers stay separate behind the shared `bicep-types`/`bicep-types-go` serializers?

## Effort

Relative sizing (not a schedule): Phase 1 (pilot to parity) is the bulk of the discovery/translation work; Phase 2 is mostly flatten + discriminator fidelity; Phases 3–5 are mechanical wiring and deletion. The net new emitter is expected to be **smaller** than today's 1,900-line extension because ARM scope/path inference (most of `resources.ts`) is replaced by declared TypeSpec metadata.
