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
- Be consistent — to the extent possible — with the existing [Terraform module version design](./2023-07-terraform-template-version.md) and the [direct-recipe-modules spec](./2026-05-direct-recipe-modules.md).

### Non-goals

- Changing how Bicep recipes are versioned. Bicep modules use OCI references and carry the version in the `:tag`; nothing here changes that.
- Automatic version bumping or floating version constraints. Platform engineers pin explicit versions, consistent with the direct-module design non-goals.
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

1. The **environment-recipe** model (`Applications.Core/environments`) has a first-class, optional `templateVersion` API field (see the [2023-07 design](./2023-07-terraform-template-version.md)). That design also explicitly **rejected** encoding source + version as a single "stringified" (OCI-like) value, calling it "overly complex".
2. The **Recipe Packs** model (`Radius.Core/recipePacks`) introduced in [2025-08-recipe-packs.md](./2025-08-recipe-packs.md) has no per-recipe version field. Its `RecipeDefinition` is `recipeKind`, `recipeLocation`, `parameters`, `outputs`, `plainHTTP`.

So the gap is specific to Recipe Packs: there is currently nowhere to put a registry module's version.

## Design

Three options are considered. In all three, non-registry sources are unaffected because their version is already in the source string or absent.

### Option A — Stringified `<source>@<version>` convention (implemented in this PR)

Encode the version as an `@<version>` suffix on `recipeLocation`, and split it inside Radius before generating the Terraform config:

```bicep
'Test.Resources/userTypeAlpha': {
  recipeKind: 'terraform'
  recipeLocation: 'terraform-aws-modules/rds/aws@6.1.0'
}
```

Radius splits this into `source = "terraform-aws-modules/rds/aws"` and `version = "6.1.0"` (`configloader.parseTerraformModuleSource`). The split only fires when the source has no `://` scheme and the `@` is after the last `/`, so it does not collide with `git@host` SSH addresses or URLs that legitimately contain `@`.

#### Advantages of Option A

- No API, schema, or codegen change — the smallest possible blast radius.
- A single field for a platform engineer to set.

#### Disadvantages of Option A

- **Previously rejected.** This is the "stringified source + version" representation the [2023-07 design](./2023-07-terraform-template-version.md) explicitly turned down as overly complex.
- **Diverges from the approved spec.** The [direct-recipe-modules design](./2026-05-direct-recipe-modules.md) shows a first-class `version` field (`source: 'terraform-aws-modules/rds/aws'`, `version: '5.9.0'`), not an `@` suffix.
- **Not Terraform-native.** Terraform itself does not accept `source@version`; the syntax is a Radius-only invention that platform engineers must learn.
- **Inconsistent across IaC kinds.** Bicep uses OCI `:tag` and Terraform Git uses `?ref=`; this adds a third, Radius-specific spelling only for Terraform registries.
- Requires custom parsing with collision rules that must be kept correct over time.

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

- **Matches the approved direct-module spec** ([2026-05](./2026-05-direct-recipe-modules.md)) and the original [2023-07 design](./2023-07-terraform-template-version.md).
- **Terraform-native semantics** — `source` and `version` map one-to-one onto the generated `module` block, exactly how a Terraform author thinks.
- **Explicit and discoverable** — the field is in the schema, documented, and validatable (for example, "version is only valid for registry sources").
- No fragile string parsing or collision rules.

#### Disadvantages of Option B

- API/schema change to `Radius.Core/recipePacks` plus codegen (TypeSpec → swagger → models → datamodel → conversion → bicep types).
- Slightly larger blast radius than Option A.
- Needs a clear rule for non-registry sources (omit `version`, or error if both a `?ref=`/`:tag` and `version` are supplied), matching the 2023-07 behavior.

### Option C — Support only non-registry sources (version always in the source)

Do not provide any version mechanism for registries; support Git (`?ref=`), OCI (`:tag`), and HTTP only.

#### Advantages of Option C

- Zero schema change; Radius stays a pure pass-through to Terraform.

#### Disadvantages of Option C

- **Excludes the Terraform public registry**, which is the headline use case for direct modules (AWS, Azure, and GCP community modules are registry-hosted). This guts the feature's value proposition and is not acceptable.

### Proposed Option

**Option B (first-class `version` field).** It matches the approved direct-module spec and the long-standing Terraform version design, is Terraform-native, explicit, validatable, and avoids a convention that was already weighed and rejected. Option A's only real advantage is blast radius, which Option B can match closely because the rendering plumbing (`templateVersion` → module `version`) already exists — only the API surface and codegen need to be added.

**Status in this PR:** Option A is implemented as a minimal, no-codegen interim so the end-to-end direct-module path is exercisable, pending this design decision. If the team accepts Option B, the change is contained: add `version?` to the `recipePacks` `RecipeDefinition`, populate `EnvironmentDefinition.TemplateVersion` from it in the config loader, and remove `parseTerraformModuleSource`.

## API design

| Option | API change                                                 | Generated Terraform                                                           |
|--------|------------------------------------------------------------|-------------------------------------------------------------------------------|
| A      | None                                                       | `source` parsed from `recipeLocation` split on `@`; `version` from the suffix |
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

- **Unit** — version rendering into the generated `module` block for: registry with version, registry without version, Git `?ref=`, OCI `:tag`, and HTTP (no version). Option A additionally needs the `@`-split collision tests, which exist in `configloader` today.
- **Functional** — noncloud functional tests cannot reach the public Terraform registry (no network egress, no OSS registry in-cluster). They continue to use the HTTP archive module server, consistent with the [2023-07 design](./2023-07-terraform-template-version.md). Registry-plus-version resolution remains a manual or cloud-E2E verification regardless of option.

## Compatibility

- Options A and C are additive and do not change existing wrapped-recipe behavior.
- Option B is an additive, optional API field and is backward compatible. If the interim Option A `@<version>` convention has shipped, switching to Option B would make `recipeLocation: '…/rds/aws@6.1.0'` no longer split automatically; this should be called out as a behavior change before Option A reaches a stable API version.

## Open Questions

1. Should `version` be **rejected** (validation error) for non-registry sources, matching Terraform's own behavior, or silently ignored?
2. Should this field be unified with Bicep so a single `version` works across IaC kinds (Bicep ignoring it in favor of the OCI `:tag`), or kept Terraform-only?
3. Does the [rename in #11879](https://github.com/radius-project/radius/issues/11879) (`recipeLocation` → `source`) change the recommendation? It does not — the version question is independent of the field name.

## Alternatives considered

See Options A and C above. The "stringified source + version" idea (Option A) was also previously considered and rejected in the [2023-07 Terraform module versions design](./2023-07-terraform-template-version.md) for the environment-recipe model.
