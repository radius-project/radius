# Base Resource Manifest

* **Author**: @nithyatsu

## Overview

Every Radius resource type a contributor authors must today restate the same four "Radius-aware" schema properties — `application`, `environment`, `connections`, and (newly) `codeReference` — in its manifest YAML. This is rote duplication authors get wrong, and it leaks Radius framework concerns into every RRT schema.

This design declares those four common properties in a single repo-owned base resource manifest. The CLI (`rad resource-type create`) and the Bicep extension generator (`bicep-tools`) merge the base into every resource type schema before validation and Bicep-type emission, so an author writes only their type-specific properties.

> **Issue References:** #12233, [adding code reference property to all RRT](https://github.com/radius-project/radius/issues/12114)

## Terms and definitions

| Term | Meaning |
|---|---|
| **Base resource manifest** | The single repo-owned YAML declaring the four common Radius properties and their shapes. Embedded into binaries via `go:embed`. |
| **Common Radius property** | A property whose name and runtime semantics Radius defines, not the type author: `application`, `environment`, `connections`, `codeReference`. |
| **Type-specific property** | Anything else an author declares under `properties:` in their per-type YAML. |

## Objectives

### Goals

- Let an author register a type whose YAML declares none of the common properties and still get a fully-functional Radius type at deploy time.
- Introduce `codeReference` as a standard optional property on every type without each author restating it.

### Non goals

- **No opt-out keyword.** Injection applies to every type; a future "raw" type with none of the four properties would need a separate spec.


### User scenarios

- **Author a new type without boilerplate.** A platform engineer publishes `Radius.Data/postgres` writing only type-specific properties (`size`, `replicaCount`, …). `rad resource-type create` succeeds — the base contributes the four common properties and marks `environment` required. Consumers set `environment` / `application` / `connections` / `codeReference` in Bicep as on any Radius resource.
- **Guarantee a common `codeReference`.** Radius wants every type to expose an optional `codeReference` and a required `environment` without operators hand-copying the "magic" schema into each manifest.

## User Experience

With this feature an author writes only type-specific properties; the four common properties are merged in automatically:

```yaml
namespace: MyOrg.Examples
types:
  widgets:
    apiVersions:
      "2026-06-01-preview":
        schema:
          type: object
          properties:        # type-specific only — no application/environment/connections/codeReference
            size: { type: integer }
            color: { type: string }
          required:
            - size
```

`rad resource-type create -f widget.yaml` — no new flags; the base is embedded in the CLI and merged at registration time. A Bicep consumer can then set all four common properties even though the YAML never declared them:

```bicep
resource w 'MyOrg.Examples/widgets@2026-06-01-preview' = {
  name: 'my-widget'
  properties: {
    application: app.id
    environment: env.id
    connections: { db: { source: pg.id } }
    codeReference: 'https://github.com/myorg/repo/blob/<sha>/app.bicep#L42'
    size: 10
    color: 'red'
  }
}
```

The [alternative approach](#alternatives-considered) instead makes the author opt in with an explicit `allOf: [{ $ref: "radius:base" }]` line — same end result, different authoring experience. The trade-off is discussed below.

## Design

### High Level Design

Three moving pieces:

1. **`pkg/schema/baseresource/base.yaml`** — the single source of truth: a small YAML declaring the four common properties, their shapes, and `required: [environment]`. Embedded with `go:embed`.
2. **`pkg/schema/baseresource.Apply(schema)`** — the canonical merger. For each base property absent from the per-type schema, it copies the base shape in, then unions the base `required:` into the per-type `required:`. No `allOf` walk, no `$ref`, no URI scheme. The CLI validator (`pkg/cli/manifest/validation.go`) calls it on every schema before validation.
3. **`bicep-tools/pkg/converter/baseresource.go::applyBaseResource(schema)`** — a parallel merger in the standalone `bicep-tools` module, for the published Bicep types.

Plus two small additions for `codeReference`: `pkg/resourceutil/utils.go::BasicProperties` learns the new name so the runtime treats it as framework-owned, and `pkg/dynamicrp/datamodel/dynamicresource.go` gains a `CodeReference()` accessor mirroring `ApplicationID()` / `EnvironmentID()`.

### The base manifest

```yaml
application:
     type: string
     description: "Resource ID of the Radius.Core/applications this resource belongs to."
   environment:
     type: string
     description: "Resource ID of the Radius.Core/environments this resource deploys into."
   connections:
     type: object
     description: "Map of connection name to connection data."
     additionalProperties:
       type: object
       properties:
         source:
           type: string
           description: "Resource ID of the source resource for this connection."
       required: [source]
   codeReference:
     type: string
     description: "Optional URI to the source code of this resource type. ex: https://github.com/radius-project/radius/blob/4fab87e8127adf1db6f43b7029d5235fbe82c5c9/cmd/controller/main.go#L27 "
   icon:
     type: string
     description: "Optional URI pointing to the icon that represents this resource type in app graph"
 required:
   - environment
```

`environment` is the only required entry; the others are optional unless a per-type `required:` adds them. The set excludes `status` and `recipe` (still reserved) and is frozen.

### Why implicit injection (Approach B)

Both approaches produce an identical effective schema and differ only in UX, along three axes:

Advantages of Approach B
- **Authoring.** B requires nothing extra — every type inherits automatically. A requires a per-type `allOf: [{ $ref: "radius:base" }]` opt-in, which is boilerplate the author can forget, silently shipping a type without the base properties.
- **Schema surface.** B adds no keyword to schema. A introduces the $ref and inheritance to schema, but only with a specific URI. If an author uses the same `$ref` keyword to bring in another schema, we are not supporting that yet.

Disadvantage of Approach B
- **Legibility.** B's inheritance is invisible in the per-type YAML, so on-disk YAML and runtime schema diverge (raw-YAML tooling sees less than runtime). A makes inheritance explicit, keeping YAML and runtime in sync.


#### Decision

**Approach B — implicit injection.** It gives the lowest authoring boilerplate and the smallest framework surface, and produces a schema identical to Approach A. The accepted cost — inheritance is invisible in the per-type YAML.

### Key implementation details

**Merger — `pkg/schema/baseresource/loader.go`.** `Apply(schema *openapi3.Schema) error`:

1. Nil schema → no-op.
2. Load the embedded base YAML once (`sync.Once`).
3. For each base property absent from `schema.properties`, copy it in (per-type-wins precedence — an author's own `environment` keeps its shape).
4. Union the base `required:` into the schema's `required:` (deduplicated) — this keeps `environment` mandatory without changing the validator.

`Apply()` is purely lexical (no network, no UCP, no runtime file I/O) and idempotent.

**`bicep-tools` parallel merger.** `applyBaseResource(schema *manifest.Schema)` replicates the semantics in the standalone module (kept separate so the Bicep extension input doesn't pull in the full Radius dependency tree). Notably, **`bicep-tools/pkg/manifest/manifest.go` needs no new fields** — a win over Approach A, which had to extend `Schema` with `AllOf` / `Ref`. A sync test (`TestApplyBaseResource_PropertiesMatchCanonicalYAML`) asserts the duplicated Go literal matches `base.yaml` (both properties and `required:`), failing CI on any drift. The emitter (`bicep-tools/pkg/converter/converter.go`) calls it once per `(provider, type, apiVersion)` before building the `<Type>Properties` Bicep type.

**Validator** `pkg/schema/validator.go`
- still requires every schema to declare `environment`; because `Apply()` runs first, the rule passes for every type automatically.
- The validator should additionally flag a per-type schema that redeclares a common property with a conflicting shape. However, it could be OK to add a base resource property such as `application` to the `required:` section. In this case, the mergers should mark the property as required.



| Component | Change | File(s) |
|---|---|---|
| Schema base manifest (NEW) | `base.yaml` + `Apply()` + `PropertyNames()` | `pkg/schema/baseresource/{base.yaml,loader.go,loader_test.go}` |
| Schema validator | Unchanged; add conflict check | `pkg/schema/validator.go` |
| CLI manifest validator | Calls `Apply()` per schema | `pkg/cli/manifest/validation.go` |
| Generic property util | `BasicProperties` += `codeReference` | `pkg/resourceutil/utils.go` |
| Dynamic-rp adapter | New `CodeReference()` accessor | `pkg/dynamicrp/datamodel/dynamicresource.go` |
| `bicep-tools` converter | Parallel `applyBaseResource()` + sync test (`manifest.go` unchanged) | `bicep-tools/pkg/converter/{baseresource.go,converter.go}` |

Core RP, Portable/Recipes RP, UCP, and the Deployment Engine are unchanged — their static datamodels already declare the common properties; only dynamic-rp and the CLI validate user-authored manifests.

### API and CLI

No HTTP API changes and no new CLI flags or commands. The manifest YAML shape is unchanged (no new keyword); authors may simply delete any common properties they previously declared. The base manifest's frozen wire shape is the only new committed contract.

## Test plan

| Layer | Test | Asserts |
|---|---|---|
| Unit | `pkg/schema/baseresource/loader_test.go` | `Apply()` no-ops on nil; merges all four into a bare schema; per-type-wins on conflicts; unions `required:` so `environment` is required; idempotent. |
| Unit | `pkg/schema/validator_test.go` (regression) | Existing rules still pass unchanged, including "missing `environment` rejected". |
| Unit | `pkg/cli/manifest/validation_test.go` | A bare schema validates end-to-end; a per-type `environment` keeps its custom shape. |
| Unit | `pkg/dynamicrp/datamodel/dynamicresource_test.go` | `CodeReference()` returns value / `""` when absent / `""` on type mismatch. |
| Unit | `bicep-tools/pkg/converter/baseresource_test.go` | Parallel merger matches canonical behavior; sync test covers properties and `required:`. |
| Functional | `test/functional-portable/dynamicrp/noncloud/baseresource_test.go` (NEW) | Register a bare manifest, deploy, assert all four common properties resolve at runtime. |

## Security

No new attack surface. The base is embedded at build time (`go:embed`) and cannot be swapped without rebuilding. The merger performs no network, file, or UCP access. The four properties are non-sensitive metadata; `codeReference` is an author-chosen string the control plane never acts on.

## Compatibility

Our tests/documents have to be updated to remove the Radius-special properties.

## Monitoring and Logging

No new metrics or traces. `Apply()` failures surface through the existing schema-validation error path in CLI/server logs.

## Development plan

| Workstream | Deliverables | Notes |
|---|---|---|
| Schema package | `pkg/schema/baseresource/*` + unit tests | The chokepoint; land first. |
| CLI integration | `validation.go` calls `Apply()` + tests | Existing validator tests must stay green. |
| Runtime wiring | `BasicProperties` + `CodeReference()` + test | Independent; can land in parallel. |
| `bicep-tools` merger | `applyBaseResource()` + sync test | Separate module; sequenced after the schema package. |
| Documentation | Contributor how-to | Ships with the merge. |
| Functional test | End-to-end registration + deploy | Land last. |

## Open Questions

If user defines one of the base property, in the schema, how should validation handle that, if the definition is the same as what we support? [resolved and added details to validation section]

## Alternatives considered

**Approach A — explicit opt-in via `allOf: [{ $ref: "radius:base" }]`.** The schema author adds an explicit `allOf` line referencing the reserved `radius:base` URI; the validator and Bicep generator resolve it, merge the four properties (per-type-wins), union `required:`, and strip the marker. Implemented end-to-end on branch [`210-base-resource-manifest`](https://github.com/radius-project/radius/tree/210-base-resource-manifest). The full B-vs-A comparison is in [Why implicit injection](#why-implicit-injection-approach-b). Rejected for v1 because the keyword adds authoring boilerplate, new vocabulary, a `$ref` error path, and a "forgot the opt-in" footgun — for an identical effective schema. It remains the documented fallback; revisit it if any of these signals appear:

- Radius schema supports "ref"
- Reviewers repeatedly ask where `environment` / `codeReference` come from because the YAML doesn't say.
- Authors ship types meant to be non-app/env-aware but get the properties injected anyway.
- IDE schema tooling or generated docs mislead because on-disk YAML and runtime schema disagree.
- A future feature needs a second base manifest and a way to opt a type into one base but not another.


## Design Review Notes

_To be filled in during review._
