# Contract: CI Workflow — Build App Graph

**Version**: 1.0.0
**File**: `radius/.github/workflows/__build-app-graph.yml` (reusable) plus a thin consumer wrapper workflow
**Purpose**: Install a released `rad` binary, run `rad graph build`, and commit `.radius/static/app.json` on every push that changes `app.bicep`

## Workflow Definition

```yaml
# Consumer repository wrapper
name: build-app-graph

on:
  push:
    paths:
      - app.bicep

jobs:
  build-graph:
    permissions:
      contents: write
    uses: radius-project/radius/.github/workflows/__build-app-graph.yml@main
    with:
      app_file: app.bicep
      artifact_path: .radius/static/app.json
```

```yaml
# Radius reusable workflow (high-level shape)
name: __build-app-graph

on:
  workflow_call:
    inputs:
      app_file:
        required: true
        type: string
      artifact_path:
        required: true
        type: string

jobs:
  build-graph:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4

      - name: Install rad release
        run: |
          curl -fsSL -o rad.tar.gz https://github.com/radius-project/radius/releases/download/${RAD_VERSION}/rad-linux-amd64.tar.gz
          tar -xzf rad.tar.gz
          sudo mv rad /usr/local/bin/rad

      - name: Build static graph
        run: |
          mkdir -p .radius/static
          rad graph build --bicep "${{ inputs.app_file }}" --output "${{ inputs.artifact_path }}"

      - name: Commit graph artifact
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add "${{ inputs.artifact_path }}"
          git diff --cached --quiet || git commit -m "chore: update app graph artifact [skip ci]"
          git push
```

## Trigger Conditions

| Event | Condition | Action |
|-------|-----------|--------|
| Push to any branch in consumer repo | `app.bicep` modified | Invoke reusable Radius workflow and commit graph artifact |
| Push to any branch in consumer repo | `app.bicep` not modified | Skip (paths filter) |

## Outputs

| Artifact | Path | Description |
|----------|------|-------------|
| Static graph JSON | `.radius/static/app.json` | Committed to the branch |

## Concurrency

- One build per branch at a time
- In-progress builds cancelled when a new push arrives on the same branch
- `[skip ci]` in commit message prevents infinite loop from the artifact commit

## Error Handling

| Failure | Behavior |
|---------|----------|
| `rad graph build` fails | Job fails; no updated artifact committed; PR status check fails |
| Git push fails (race condition) | Job fails; next push will regenerate |

## Dependencies

- **rad CLI**: Downloaded from Radius releases and contains `graph build`
- **Bicep CLI**: Managed internally by `rad graph build` or installed as a runtime dependency by that command

## Notes

- The reusable workflow lives in the Radius repo and follows the existing reusable-workflow naming convention (`__` prefix).
- Core graph logic lives in the `rad` CLI, not in workflow YAML, so contributors can run the same command locally.
- Consumer repos add only a thin wrapper workflow, avoiding duplicated CI logic.
