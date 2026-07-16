# Static-Graph Sensitive-Field Redaction

Status: Proposed
Author: (fill in)
Date: 2026-07-15
Companion to: [2026-07-sensitive-fields-in-app-graph.md](./2026-07-sensitive-fields-in-app-graph.md)

## Scope

- **In scope:** the static (modeled) graph produced by `rad app graph <app.bicep>` — [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go). No API calls, no control plane. Static graph is Bicep-side only.
- **Out of scope:** the runtime graph (see [Why the runtime graph needs no change](#why-the-runtime-graph-needs-no-change)), `Applications.Core/*` handlers, older API versions, all server-side redaction code.

## Deliverable

Two changes shipped together:

1. **Populate `ApplicationGraphResource.Properties` on every static-graph node.** The field exists on the response model ([pkg/corerp/api/v20250801preview/zz_generated_models.go](../../../pkg/corerp/api/v20250801preview/zz_generated_models.go)) but is left `nil` today. Populate it from the compiled Bicep resource body, minus the runtime keys the graph itself surfaces first-class (`provisioningState`, `connections`, `status`) — matching the runtime graph's `getResourceTypeSpecificProperties`.
2. **Null out sensitive values in that newly-populated `Properties` bag** using two simple rules:
   - **Rule A — Secure-parameter tracing.** Values that come from a Bicep `@secure() param` are marked in the compiled ARM JSON via `parameters.<name>.type == "secureString"` (scalar secrets) or `"secureObject"` (structured secrets). Any property value that references such a parameter — directly, nested inside an expression, or via field access on a secureObject — is nulled.
   - **Rule B — Name-based blocklist.** Any property whose key (case-insensitive, at any nesting depth) matches a well-known secret name is nulled — regardless of source.

Redaction is only meaningful once population is done; the two ship as one change.

## Why the runtime graph needs no change

The runtime graph already relies on two-layer redaction upstream:

1. **Write path redacts at rest.** `dynamicrp` encrypts `x-radius-sensitive` fields on create/update; storage carries them already-nulled. Both `GetResourceWithRedaction` ([pkg/dynamicrp/frontend/getresource.go](../../../pkg/dynamicrp/frontend/getresource.go)) and `ListResourcesWithRedaction` ([pkg/dynamicrp/frontend/listresources.go](../../../pkg/dynamicrp/frontend/listresources.go)) fast-path `Succeeded` resources for this reason.
2. **Read path redacts explicitly for non-`Succeeded`.** Both controllers call `schema.RedactFields(resource.Properties, sensitiveFieldPaths)` for `Updating` / `Accepted` / `Failed` states.

The runtime graph's `Properties` bag is populated from those redacted LIST responses at [pkg/corerp/frontend/controller/applications/graph_util.go:328](../../../pkg/corerp/frontend/controller/applications/graph_util.go). No additional graph-layer redaction is needed and none is added here.

## Approach

### Rule A — Secure-parameter tracing

Bicep's `@secure()` decorator applies to both string and object parameters and compiles to two ARM types:

```json
{
  "parameters": {
    "adminPassword":    { "type": "secureString" },   // @secure() param adminPassword string
    "credentialsBlob":  { "type": "secureObject" }    // @secure() param credentialsBlob object
  }
}
```

Both types are treated identically as sensitive sources — the redaction contract does not distinguish scalar from structured secrets, and mixing the two rules would let a `secureObject` field like `credentialsBlob.password` flow through the graph unredacted. This mirrors how [pkg/recipes/driver/bicep/bicep.go](../../../pkg/recipes/driver/bicep/bicep.go) treats `securestring` / `secureobject` outputs identically for recipe-response secret routing.

Any resource property value that draws from a secure param appears in the compiled template as an ARM expression referencing `parameters('name')` — either directly, e.g. `"password": "[parameters('adminPassword')]"`, or nested inside another function, e.g. `"connectionString": "[format('server=x;pwd={0}', parameters('adminPassword'))]"`, or via property access on a secureObject, e.g. `"clientId": "[parameters('credentialsBlob').clientId]"`.

**Algorithm.** Extract the set `secureParams = { name | template.parameters[name].type ∈ {"secureString","secureObject"} }` (case-insensitive on the type). When populating a resource's `Properties`, walk recursively; for each **string** leaf value:

- If the string is an ARM expression (`^\[.*\]$`) that contains a `parameters('<name>')` reference where `<name> ∈ secureParams`, null the value.

The match is intentionally coarse: a single sensitive substring nulls the entire value. Trying to redact a substring inside a `format(...)` expression would produce a partially-decoded value the user cannot interpret; nil is the safer signal.

**What this does NOT catch:**

- Sensitive values that are literal strings hard-coded in the Bicep source. Rule B is the backstop for this — the naming heuristic catches values named `password` / `apiKey` / etc. even when the author bypassed `@secure()`.
- Values assigned via `output` chaining across module boundaries. Modules aren't in scope for the static graph today; when they are, this rule extends naturally by walking the module's own parameter table.

### Rule B — Name-based blocklist

Regardless of where the value came from, if the **property key** (case-insensitive) matches one of the well-known secret names below, its value is set to `nil`. Applied at every nesting depth inside the `Properties` bag.

**Blocklist:**

```text
password
connectionString
apiKey
secret
token
privateKey
sasToken
```

**Why these specifically.** Every entry is either universally understood as sensitive (industry consensus, e.g. `password`, `privateKey`) or appears verbatim as a property key on multiple existing Radius resource types (`connectionString`, `sasToken`). The list intentionally excludes generic names like `key`, `data`, or `config` that are frequently used for non-sensitive purposes.

**Match rules:**

- Case-insensitive exact match on the key. `"Password"`, `"password"`, `"PASSWORD"` all match; `"passwordHash"` does not.
- Applies to keys in maps at every depth, including inside arrays' object items.
- The value is set to `nil` regardless of its type (string, object, number, array).

**False-positive tolerance.** Occasionally a legitimate non-sensitive property will collide (e.g. a hypothetical `password` field on a rate-limiter config that stores the number of password attempts). We accept that risk. The list is short, curated, and conservative; a false positive redacts one graph cell but never leaks a real secret. The alternative — no naming rule — would let hard-coded plaintext secrets flow through unredacted.

### Order

Both rules run on the same in-place walk. A cell that matches either rule is nulled; the rules are OR-composed, not sequential.

### DiffHash

`DiffHash` is computed over the **authored** properties (pre-redaction), matching the runtime graph. Rationale: the hash exists to detect authored changes; nulling sensitive values before hashing would make the hash stable across secret rotations, which is the wrong signal for the diff-detection use case. The hash itself is one-way and does not surface plaintext.

## Design

### Changes to [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go)

1. **`BuildModeledGraph`** keeps its existing `(template map[string]any)` signature — no context, no resolver, no injected dependencies. All state comes from the template itself.
2. New unexported helper `sensitiveParamNames(template) map[string]struct{}` returns the set of parameter names declared as `secureString` or `secureObject` (case-insensitive on the type).
3. `buildModeledResource` accepts the param set, calls new `resolveGraphProperties(authored, secureParams)` which:
   - Clones the authored map minus the runtime keys.
   - Walks the clone recursively, applying Rules A and B to each leaf value.
   - Returns the redacted clone (or `nil` if the input was empty).
4. Two small unexported helpers:
   - `containsSecureParamReference(value string, secureParams map[string]struct{}) bool` — literal substring check for `parameters('name')` occurrences, robust to whitespace variants seen in real Bicep output.
   - `isSensitiveKey(key string) bool` — case-insensitive membership check against the blocklist.

### No changes required to callers

`BuildModeledGraph`'s signature is unchanged, so [pkg/cli/cmd/app/graph/graph.go](../../../pkg/cli/cmd/app/graph/graph.go) and all existing tests continue to work without edits.

### Testing

All offline, no I/O, no fake schemas:

- **Rule A**: secure-param tracing with direct `[parameters('x')]`, nested inside `format(...)`, mixed with non-secure params (nulls the whole value), and unreferenced secure params (no-op).
- **Rule B**: each blocklisted name at top level, nested inside an object, inside an array item, and case variants (`Password`, `PASSWORD`).
- **Population**: runtime keys (`provisioningState`/`connections`/`status`) dropped; empty properties → empty `Properties` map; missing properties → `nil` `Properties`.
- **DiffHash**: identical for two graphs of the same app where one has secure params and one doesn't (redaction happens after `ComputeDiffHash`).

## What this design deliberately does not do

- **No Bicep type registry / `types.tgz` parsing.** Considered and rejected — see [Alternatives considered](#alternatives-considered) below.
- **No `redactedProperties` list** on the wire response — tracked as a future enhancement; redaction is expressed as plain `null` in this milestone. See [Future enhancements](#future-enhancements).
- **No `bicepconfig.json` parsing.**
- **No UCP calls, no filesystem I/O, no network.** Pure template inspection.
- **No changes to the runtime graph handler**, `Applications.Core/*`, or older API versions.

## Alternatives considered

### Alternative 1: Bicep extension type-registry parsing (rejected)

The `x-radius-sensitive` annotation authored in a resource-type YAML manifest is preserved end-to-end into the Bicep type registry that ships alongside each extension:

```text
~/.bicep/local/sha256_<hash>/types.tgz    # extensions resolved from a local ../out/x.tgz
~/.bicep/br/<registry-path>/**/types.tgz  # extensions resolved from an OCI registry (br:host/name:tag)
```

Each `types.tgz` contains two files: an `index.json` that maps `Namespace/Type@Version → $ref` and a `types.json` that carries the resolved type nodes with `"sensitive": true` markers on the individual properties (see [bicep-tools/pkg/converter/converter.go](../../../bicep-tools/pkg/converter/converter.go) — the converter that emits those markers from the source YAML).

The rejected approach would have added a `pkg/cli/bicep/extensions` package with:

- A cache walker that scans `~/.bicep/**/types.tgz`, indexes types by fully-qualified name, and lazy-loads `types.json` per archive.
- A tree walker over each type's `ObjectType → outer body → inner properties` structure to compute the dot-notation path list of sensitive properties (`credentials.password`, `secrets[*].value`, etc.) using the same encoding `pkg/schema.RedactFields` accepts.
- A refetch fallback that runs `bicep restore --force` at most once per graph build when the cache lacked a requested type.
- Fail-closed semantics: `Properties = nil` when the resolver could not decide.

**Why we rejected it.** Three reasons, in decreasing weight:

1. **Redundant with `@secure()`.** Any resource property authored with `x-radius-sensitive` is expected to be assigned only from a `@secure()` Bicep parameter — that is the documented authoring pattern in [docs/architecture/extensibility.md](../../../docs/architecture/extensibility.md). **Bicep itself warns when a sensitive-typed property is assigned a plaintext literal**, pushing authors toward `@secure()` naturally. So the secure-param tracing rule (Rule A) already catches every value the extension approach would have caught, without needing the schema.
2. **Complexity budget.** The abandoned approach required ~600 LOC across a new package (`resolver.go`, `cache_resolver.go`, `walker.go`, three test files), one integration point in `pkg/cli/graph`, one integration point in `pkg/cli/cmd/app/graph`, plus a refetch loop that itself has failure modes. The two-rule approach lives in one file, ~120 LOC. Every line saved is a line that cannot regress.
3. **Offline / cache-consistency footguns.** The refetch fallback existed because a fresh cache can miss a type the current app.bicep declares. Getting `bicep restore --force` to leave the cache in a state that a second walk of the same directory would find the type is not obvious across OS variants, filesystem casing, and OCI-vs-local resolution paths. Adding this complexity to protect a case that Rule A already covers was not worth it.

**The kept git ref.** The full implementation of the abandoned approach lives on branch [`x-radius-senstive-graph-extn-walkthrough`](https://github.com/radius-project/radius/tree/x-radius-senstive-graph-extn-walkthrough) for reference — walker, resolver, cache scanner, refetch loop, and tests are all there. If a future need forces us to reconsider (e.g., an extension author who bypasses `@secure()` intentionally and Bicep changes its warning policy), that branch is the starting point.

### Alternative 2: UCP schema fetch at graph-build time (rejected)

We considered having the static graph call the same `pkg/schema.GetSensitiveFieldPaths` helper the runtime redaction path uses, contacting UCP to fetch each resource type's schema. Rejected because it violates the "static graph works offline" invariant — the graph is often built in CI, in a workflow that runs before the target cluster exists.

## Future enhancements

### Emit a per-resource `redactedProperties` list on the graph response (deferred)

With redaction expressed as plain `null`, downstream consumers cannot distinguish two states:

- **Unset** — the author never assigned a value to this property.
- **Redacted** — the author assigned a value that this design nulled out.

Both serialize as `"password": null`. A dashboard rendering the graph today has no way to display "🔒 redacted" vs. "—" (empty), and diff tooling cannot tell the two apart when comparing two graphs.

**Sketch of the proposal.** Add a `redactedProperties: []string` field to `ApplicationGraphResource` in the `2025-08-01-preview` model. When [`resolveGraphProperties`](../../../pkg/cli/graph/modeled.go) nulls a value, append its dot-notation path (matching the format `schema.RedactFields` uses — `credentials.password`, `secrets[*].value`, etc.) to that list on the emitted node. The redacted `Properties` map continues to serialize as `null` for each entry so the wire is backward-compatible; the new field is additive and always non-nil (empty when nothing was redacted).

**Why deferred.**

- Requires a schema/TypeSpec change on `Radius.Core/applications` at `2025-08-01-preview`, which the current milestone is trying to keep additive-free.
- No consumer today needs the disambiguation. Adding the field now would be speculative; adding it when a real client asks for it means we get to shape the field around the actual use case (list of paths vs. structured entries with a reason code, etc.).
- The runtime graph would want the same field for symmetry, which pulls the change into the shared response type and the runtime handler — out of scope for the static-graph milestone.

Tracked in [radius-project/radius#12451](https://github.com/radius-project/radius/issues/12451).

## Why the two-rule approach is sufficient

The design leans on three facts that make Rule A + Rule B a practically complete redactor for the static graph:

1. **Bicep already gates the sensitive-authoring path.** A resource property declared `x-radius-sensitive` in its type manifest surfaces in Bicep's type system with the equivalent of `@secure()`. Bicep raises a compile-time warning (and in some future versions, an error) when an author hard-codes a literal value into that property, pushing every legitimate assignment through a `@secure() param`. Rule A catches those.
2. **Name-based redaction is the safety net for authors who bypass `@secure()`.** Even if Bicep's warning is ignored or the author uses a non-annotated type with an obviously-sensitive property name, Rule B catches the common cases (`password`, `connectionString`, etc.). The list is short, curated, and universally understood.
3. **The runtime graph is protected by the server, not us.** Any leakage of `x-radius-sensitive` at runtime is a server bug ([pkg/dynamicrp/frontend/getresource.go](../../../pkg/dynamicrp/frontend/getresource.go), [pkg/dynamicrp/frontend/listresources.go](../../../pkg/dynamicrp/frontend/listresources.go)), not something the CLI needs to compensate for.

Together, the two rules cover the observable static-graph attack surface with a much smaller design and a much shorter path to first review.

## Files touched

- Modified: [pkg/cli/graph/modeled.go](../../../pkg/cli/graph/modeled.go) — populate + redact Properties.
- Modified: [pkg/cli/graph/modeled_test.go](../../../pkg/cli/graph/modeled_test.go) — new cases for the two rules and population.
- No new files. No changes to any server-side code, CLI wiring, or API-generated code.
