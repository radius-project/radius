# Contributing schema changes

## Purpose

This guide explains how to make a change to the Radius REST API — for example adding a property to an existing resource or adding a new resource type. The Radius application model and API are defined in [TypeSpec](https://typespec.io/) under [`typespec/`](../../../../typespec/); the build pipeline compiles that TypeSpec into OpenAPI (Swagger) specs under [`swagger/`](../../../../swagger/) and into Go API client code under `pkg/`. This is the **TypeSpec → Swagger → Go** pipeline. Follow it whenever you touch the API surface so the spec, the generated clients, and the Bicep types stay in sync. It is for contributors changing the API; it does not cover resource-provider business logic beyond the generated types.

## Prerequisites

- The standard build prerequisites from [contributing-code-prerequisites](../contributing-code-prerequisites/README.md): Go, Node.js, and `pnpm` (enabled through `corepack`). The TypeSpec compiler (`tsp`) and emitters are installed into [`typespec/`](../../../../typespec/) on first use by the `make generate` targets, so no global install is needed.
- A working clone of the repo where you can run `make` targets. `make generate` runs the `tsp` toolchain and `go generate` (mocks), so a working Go and Node toolchain is required.
- Familiarity with the namespace you are changing. Each API namespace has its own folder under [`typespec/`](../../../../typespec/) (for example `typespec/Applications.Core`, `typespec/Radius.Core`, `typespec/UCP`).

## Steps

### 1. Update the TypeSpec definitions

1. Create or update the applicable `.tsp` files (named after the resource type) inside the namespace folder under [`typespec/`](../../../../typespec/), for example `typespec/Applications.Core`.
2. Check the formatting of your TypeSpec:

   ```bash
   make tsp-format-check
   ```

   This runs `pnpm -C typespec exec tsp format --check "**/*.tsp"`. To apply the formatter instead of just checking, run `pnpm -C typespec exec tsp format "**/*.tsp"` from the repo root.

### 2. Generate the OpenAPI specs and Go clients

Run the umbrella target from the repo root:

```bash
make generate
```

`make generate` runs the full pipeline: it deletes stale generated code, compiles every namespace's TypeSpec to its OpenAPI spec (`make generate-openapi-spec`), runs the TypeSpec Go emitter for each namespace's client, runs `go generate ./...` (mockgen), generates the Bicep extensibility types, and generates the CRDs. The two halves of the pipeline are:

- **TypeSpec → Swagger.** The [`@azure-tools/typespec-autorest`](https://github.com/Azure/typespec-azure) emitter writes each namespace's OpenAPI document to `swagger/specification/<service>/resource-manager/<service-name>/<status>/<version>/openapi.json`. The output directory is set per namespace by the `emitter-output-dir` option in that namespace's `tspconfig.yaml` (for example `typespec/Applications.Core/tspconfig.yaml` emits to `swagger/specification/applications`).
- **TypeSpec → Go.** The [`@azure-tools/typespec-go`](https://github.com/Azure/typespec-azure) emitter writes generated client code to a temporary `.tsp-go-tmp` folder, which the per-namespace `make generate-rad-<namespace>-client` targets copy into the matching `pkg/<namespace>/api/<version>/` directory and run `go fmt` over. Generated files are prefixed `zz_generated_`.

#### Alternative: generate a single namespace manually

You normally only need `make generate`. To regenerate one namespace by hand, run these from the repo root. Generation depends on the `tsp` toolchain being installed; running `make generate` once (or `make generate-tsp-installed`) installs it.

1. Compile the OpenAPI spec for one namespace:

   ```bash
   cd typespec/Applications.Core && pnpm exec tsp compile .
   ```

2. Generate the Go client for that namespace with the TypeSpec Go emitter:

   ```bash
   cd typespec/Applications.Core && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
   ```

   The emitter configuration lives in each namespace's `tspconfig.yaml` (under the `@azure-tools/typespec-go` options block). The generated files land in `.tsp-go-tmp` and must be copied into the matching `pkg/<namespace>/api/<version>/` directory; the per-namespace `make generate-rad-<namespace>-client` targets (for example `make generate-rad-corerp-client`) automate that copy-and-format step, so prefer them over copying by hand.

### 3. Wire up and test the change

1. Add any changes to the Radius resource provider needed to support the new or updated types.
2. Add or update tests as needed.
3. Open a pull request in the Radius repo. (See the local-testing and merge-order steps below before you expect all checks to pass.)

### 4. (Optional) Test the schema change locally with Bicep

To confirm your schema compiles in a Bicep template, publish the generated Bicep types to a local target and point `bicepconfig.json` at them.

1. Install the [Bicep CLI](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/install). If you already have the Radius CLI installed, you can use the Bicep binary it downloads to `./.rad/bin/bicep` instead.
2. Generate the Bicep types (already done if you ran `make generate`):

   ```bash
   make generate-bicep-types
   ```

   This writes the type files under `hack/bicep-types-radius/generated/` and rebuilds the unified index at `hack/bicep-types-radius/generated/index.json`.
3. Publish the unified `radius` extension to a target of your choice (a local file path or an OCI registry):

   ```bash
   make publish-bicep-extension BICEP_PUBLISH_TARGET=<target>
   ```

   `<target>` is either a local path (for example `./bin/radius-types.tgz`) or an OCI reference (for example `br:biceptypes.azurecr.io/radius:latest`). The target requires the `bicep` CLI on your `PATH`.
4. Update the root `bicepconfig.json` to reference your published extension:

   ```json
   {
       "extensions": {
           "radius": "<target>",
           "aws": "br:biceptypes.azurecr.io/aws:latest"
       }
   }
   ```

   Once Bicep restores the new extension, your schema changes are available in Bicep templates.

### 5. Update docs and samples, then merge in order

1. Open PRs in the [docs](https://github.com/radius-project/docs/) and [samples](https://github.com/radius-project/samples/) repositories with the corresponding resource changes. Some checks fail until the PRs below start merging.
2. Merge in this order once all three PRs (radius, docs, samples) are ready and approved:
   1. **Samples** — because of a cyclic dependency between samples and radius (the "Test Quickstarts" task in the samples pipeline runs against the `main` branch of radius, which does not yet contain your changes), a repo admin must force-merge the samples PR.
   2. **Radius** — after the samples PR merges, re-run the radius PR checks and merge.
   3. **Docs** — re-run any failed checks and merge the docs PR with the updated Bicep files.

## Verification

- `make tsp-format-check` reports `OK` with no formatting diffs.
- After `make generate`, the regenerated OpenAPI spec for your namespace under `swagger/specification/.../openapi.json` reflects your change.
- The regenerated `zz_generated_*.go` files under `pkg/<namespace>/api/<version>/` reflect your change.
- The repo still builds and tests pass:

  ```bash
  make build
  make test
  ```

- `git status` shows the generated spec, client, and Bicep type files changed alongside your `.tsp` edits — generated output must be committed, not left out of the PR.

## Troubleshooting

- **`tsp` or `pnpm` not found.** `make generate` installs the TypeSpec toolchain into `typespec/` via `corepack`. Ensure Node.js is installed and on your `PATH`, then re-run `make generate` (or `make generate-tsp-installed`). See [contributing-code-prerequisites](../contributing-code-prerequisites/README.md).
- **`make tsp-format-check` fails.** Run `pnpm -C typespec exec tsp format "**/*.tsp"` to apply the formatter, then re-run the check.
- **Generated files keep reappearing as changes.** Generated `zz_generated_*.go` and `openapi.json` files are committed artifacts. Run `make generate`, then commit the regenerated output so it matches your TypeSpec.
- **`make publish-bicep-extension` errors that the index does not exist.** Run `make generate-bicep-types` first; the target publishes `hack/bicep-types-radius/generated/index.json`, which that command creates.
- **`make publish-bicep-extension` cannot find `bicep`.** Install the [Bicep CLI](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/install) and ensure it is on your `PATH`, or use the binary at `./.rad/bin/bicep` from a Radius CLI install.
