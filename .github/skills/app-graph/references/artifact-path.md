# Building the Graph Artifact Locally

The `rad graph build` subcommand has two output modes:

1. **Local file** (this skill's mode). Default output path is
   `.radius/static/app.json`. Override with `--output <path>`.
2. **Orphan branch** (CI mode, NOT used by this skill). Commits the JSON
   to `{source-branch}/app.json` on a configurable orphan branch.

## Invocation

```bash
rad graph build --bicep ./app.bicep --output ./.radius/static/app.json
```

`--bicep` defaults to `app.bicep` and `--output` defaults to
`.radius/static/app.json`, so the bare command also works when the
file is named `app.bicep` in the current directory:

```bash
rad graph build
```

## Prerequisites

- `bicep` CLI on PATH (or `az bicep` as fallback). `rad graph build`
  shells out to `bicep build --outfile <tmp>` to compile to ARM JSON
  before parsing.
- `rad` CLI from `radius-project/radius` at a ref that contains the
  `graph build` subcommand. Until the feature merges to `main`, build
  from `features/radius-graph`:

  ```bash
  git clone -b features/radius-graph https://github.com/radius-project/radius
  cd radius
  go build -o rad ./cmd/rad
  ```

## Output shape

The output is a `StaticGraphArtifact` (see [schema.md](schema.md)).

## Errors

If `rad graph build` exits non-zero, surface stderr verbatim and stop.
Common failures:

- `bicep build failed` — Bicep CLI not installed or `app.bicep` has
  compile errors.
- `compiling Bicep file` — the Bicep file path is wrong or unreadable.
- `building static graph` — the compiled ARM JSON is missing required
  Radius resource metadata; usually means the input Bicep is not a
  Radius application.
