# Providing versions for Terraform module references in Recipe Packs

- **Author**: `(@willdavsmith)`

## Overview

[Direct Recipe Module Support](./2026-05-direct-recipe-modules.md) lets platform engineers point a Recipe's location directly at a standard Terraform module instead of a Radius-specific wrapper. The single most valuable source for this feature is the [Terraform Registry](https://registry.terraform.io) (e.g. `terraform-aws-modules/rds/aws`), because it hosts thousands of production-ready community modules.

Terraform Registry modules differ from every other Terraform source in one important way: **the version is not part of the source address.** A registry address is a bare `<NAMESPACE>/<NAME>/<PROVIDER>` (or `<HOST>/<NAMESPACE>/<NAME>/<PROVIDER>` for private registries) — it is *not* a URL — and Terraform requires the version to be supplied as a separate `version` argument on the `module` block. By contrast, Git sources carry their version in `?ref=`, OCI sources in `:tag`, and HTTP archive sources have no version concept at all.

The new `Radius.Core/recipePacks` `RecipeDefinition` does not (today) carry a version field, so we need a way for a platform engineer to express the version of a registry module. This document lays out the options, their trade-offs, and a recommendation. It is intentionally scoped to the *version-provisioning question* for Terraform; it does not re-open the broader direct-module design.

## Terms and definitions

| Term              | Definition                                                                                                                                                                                                                 |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Source            | The location of a Terraform module. May be a registry address, Git URL, HTTP URL, OCI reference, or local path. See [Module Sources](https://developer.hashicorp.com/terraform/language/modules/sources#module-sources).   |
| Registry module   | A module referenced by the Terraform registry protocol using a bare `<HOST>/<NAMESPACE>/<NAME>/<PROVIDER>` address. Versioned with SemVer; the version is a **separate** `version` argument, never embedded in the source. |
| `recipeLocation`  | The field on a `recipePacks` `RecipeDefinition` that carries the module source. (Tracked for rename to `source` in [#11879](https://github.com/radius-project/radius/issues/11879).)                                       |
| `templateVersion` | The internal `EnvironmentDefinition` field that Radius renders as the Terraform module `version` argument.                                                                                                                 |

## Objectives

> **Issue Reference:** Design spec [#11876](https://github.com/radius-project/radius/pull/11876); direct-module design note [2026-05-direct-recipe-modules.md](./2026-05-direct-recipe-modules.md).

### Goals

- Allow platform engineers to pin a specific version of a Terraform **registry** module referenced directly (no wrapper) from a Recipe Pack.
- Keep non-registry sources (Git `?ref=`, OCI `:tag`, HTTP) working as pure pass-through, exactly as they do for wrapped recipes today.

### Non-goals

- Changing how Bicep recipes are versioned. Bicep modules use OCI references and carry the version in the `:tag`; nothing here changes that.
- Automatic version bumping or floating version constraints. Platform engineers pin explicit versions.
- Local filesystem sources, which are out of scope for direct modules.

## Background: how versions flow today

For a Terraform recipe, Radius renders an `EnvironmentDefinition` into a generated `module` block. The `version` key is only emitted when `templateVersion` is non-empty (`pkg/recipes/terraform/config/config.go`):

```hcl
module "default" {
  source  = "terraform-aws-modules/rds/aws"
  version = "6.1.0"
}
```

Two facts shape the options below:

1. The **environment-recipe** model (`Applications.Core/environments`) has a first-class, optional `templateVersion` API field (see the [2023-07 design](./2023-07-terraform-template-version.md)).
2. The **Recipe Packs** model (`Radius.Core/recipePacks`) introduced in [2025-08-recipe-packs.md](./2025-08-recipe-packs.md) has no per-recipe version field. Its `RecipeDefinition` is `recipeKind`, `recipeLocation`, `parameters`, `outputs`, `plainHTTP`.

So the gap is specific to Recipe Packs: there is currently nowhere to put a registry module's version.

## Design

Three options are considered. In all three, non-registry sources are unaffected because their version is already in the source string or absent.

### Option A — Stringified `<source>:<version>` convention (implemented in this PR, proposed in [#12086](https://github.com/radius-project/radius/issues/12086))

Encode the version as a `:<version>` suffix on `recipeLocation` — the same spelling Bicep/OCI recipes use for their image tag — and split it inside Radius before generating the Terraform config:

```bicep
'Test.Resources/userTypeAlpha': {
  recipeKind: 'terraform'
  recipeLocation: 'terraform-aws-modules/rds/aws:6.1.0'
}
```

Radius splits this into `source = "terraform-aws-modules/rds/aws"` and `version = "6.1.0"` (`configloader.parseTerraformModuleSource`). The split only fires when the source has no `://` scheme and the `:` is in the final path segment (after the last `/`), so a registry `host:port` such as `my.registry.com:8443/ns/name/aws` is not mistaken for a version, and Git/HTTP URLs are left untouched.

#### Advantages of Option A

- No API, schema, or codegen change — the smallest possible blast radius.
- A single field for a platform engineer to set.
- **Uniform spelling with Bicep/OCI.** The version is appended after a `:` exactly as for an OCI recipe tag, so a platform engineer pins a version the same way regardless of IaC kind.

#### Disadvantages of Option A

- **Not Terraform-native.** Terraform's `module` block takes `source` and `version` as separate arguments and does not accept `source:version`; the combined spelling is a Radius convention that platform engineers must learn.
- **Looks like an OCI tag but isn't.** An OCI `:tag` is part of the native image reference; a Terraform registry version never is (it is always a separate argument), so the shared spelling hides a semantic difference.
- Requires custom parsing whose collision rules (registry `host:port`, SCP-style Git, URL schemes) must be kept correct over time.

### Option B — First-class `version` field on `RecipeDefinition` (spec design)

Add an optional `version` (rendered to `templateVersion`) to the Recipe Packs `RecipeDefinition`, mirroring the environment-recipe model and the direct-module spec:

```bicep
'Test.Resources/userTypeAlpha': {
  recipeKind: 'terraform'
  recipeLocation: 'terraform-aws-modules/rds/aws'
  version: '6.1.0'
}
```

#### Advantages of Option B

- **Terraform-native semantics** — `source` and `version` map one-to-one onto the generated `module` block, exactly how a Terraform author thinks.
- **Explicit and discoverable** — the field is in the schema, documented, and validatable (for example, "version is only valid for registry sources").
- No fragile string parsing or collision rules.

#### Disadvantages of Option B

- API/schema change to `Radius.Core/recipePacks` plus codegen (TypeSpec → swagger → models → datamodel → conversion → bicep types).
- Slightly larger blast radius than Option A.
- Needs a clear rule for non-registry sources (omit `version`, or error if both a `?ref=`/`:tag` and `version` are supplied).

### Option C — Support only non-registry sources (version always in the source)

Do not provide any version mechanism for registries; support Git (`?ref=`), OCI (`:tag`), and HTTP only.

#### Advantages of Option C

- Zero schema change; Radius stays a pure pass-through to Terraform.

#### Disadvantages of Option C

- **Excludes the Terraform public registry**, which is the headline use case for direct modules (AWS, Azure, and GCP community modules are registry-hosted). This guts the feature's value proposition and is not acceptable.

### Proposed Option

Issue [#12086](https://github.com/radius-project/radius/issues/12086) proposes **Option A with the `:` separator**, which is what this PR implements: platform engineers pin a version with the same `:<version>` spelling they already use for Bicep/OCI recipe tags, and no API change is required. **Option B (a first-class `version` field)** is the more explicit, Terraform-native alternative and the natural follow-up if the team wants schema-level validation (for example, rejecting a version on non-registry sources). The decision is a trade-off — blast radius and uniform spelling (A) versus explicitness and validatability (B) — and is left for design review.

**Status in this PR:** Option A (the `:` convention from [#12086](https://github.com/radius-project/radius/issues/12086)) is implemented so the end-to-end direct-module path is exercisable. Moving to Option B would be contained: add `version?` to the `recipePacks` `RecipeDefinition`, populate `EnvironmentDefinition.TemplateVersion` from it in the config loader, and remove `parseTerraformModuleSource`.

## API design

| Option | API change                                                 | Generated Terraform                                                           |
|--------|------------------------------------------------------------|-------------------------------------------------------------------------------|
| A      | None                                                       | `source` parsed from `recipeLocation` split on `:`; `version` from the suffix |
| B      | Add optional `version` to `recipePacks` `RecipeDefinition` | `source` = `recipeLocation`, `version` = `version`                            |
| C      | None                                                       | `source` = `recipeLocation`; no `version`                                     |

For Option B the `RecipeDefinition` becomes:

```typespec
model RecipeDefinition {
  recipeKind: RecipeKind;
  recipeLocation: string;
  @doc("Version of the module. Required for Terraform registry sources; omitted for Git/OCI/HTTP sources.")
  version?: string;
  parameters?: Record<unknown>;
  outputs?: Record<string>;
  plainHTTP?: boolean;
}
```

## Test plan

- **Unit** — version rendering into the generated `module` block for: registry with version, registry without version, Git `?ref=`, OCI `:tag`, and HTTP (no version). Option A additionally needs the `:`-split collision tests (including a registry `host:port`), which exist in `configloader` today.
- **Functional** — noncloud functional tests cannot reach the public Terraform registry (no network egress, no OSS registry in-cluster). They continue to use the HTTP archive module server. Registry-plus-version resolution remains a manual or cloud-E2E verification regardless of option.

## Compatibility

- Options A and C are additive and do not change existing wrapped-recipe behavior.
- Option B is an additive, optional API field and is backward compatible. If the interim Option A `:<version>` convention has shipped, switching to Option B would make `recipeLocation: '…/rds/aws:6.1.0'` no longer split automatically; this should be called out as a behavior change before Option A reaches a stable API version.

## Open Questions

1. Should `version` be **rejected** (validation error) for non-registry sources, matching Terraform's own behavior, or silently ignored?
2. Should this field be unified with Bicep so a single `version` works across IaC kinds (Bicep ignoring it in favor of the OCI `:tag`), or kept Terraform-only?
3. Does the [rename in #11879](https://github.com/radius-project/radius/issues/11879) (`recipeLocation` → `source`) change the recommendation? It does not — the version question is independent of the field name.

## Alternatives considered

See Options A and C above.
