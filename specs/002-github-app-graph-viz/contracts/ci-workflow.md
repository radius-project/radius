# Contract: CI Workflow — Build App Graph

**Version**: 1.0.0
**File**: `radius/.github/workflows/__build-app-graph.yml` (reusable) plus a thin consumer wrapper workflow
**Purpose**: Build `rad` from the requested Radius source ref, run `rad graph build`, and commit the graph artifact to the `radius-graph` orphan branch on every push that changes `app.bicep`

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
      orphan_branch:
        required: false
        type: string
        default: radius-graph
      workflow_source_ref:
        required: false
        type: string
        default: main

jobs:
  build-graph:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/checkout@v4
        with:
          repository: radius-project/radius
          ref: ${{ inputs.workflow_source_ref }}
          path: .radius-workflow-src

      - name: Build rad from Radius workflow source
        run: |
          cd .radius-workflow-src
          go build -o "$RUNNER_TEMP/rad" ./cmd/rad
          sudo mv "$RUNNER_TEMP/rad" /usr/local/bin/rad

      - name: Build static graph and commit to orphan branch
        run: |
          SOURCE_BRANCH="${GITHUB_HEAD_REF:-${GITHUB_REF#refs/heads/}}"
          rad graph build \
            --bicep "${{ inputs.app_file }}" \
            --orphan-branch "${{ inputs.orphan_branch }}" \
            --source-branch "$SOURCE_BRANCH"
```

## Trigger Conditions

| Event | Condition | Action |
|-------|-----------|--------|
| Push to any branch in consumer repo | `app.bicep` modified | Invoke reusable Radius workflow and commit graph artifact |
| Push to any branch in consumer repo | `app.bicep` not modified | Skip (paths filter) |

## Outputs

| Artifact | Path | Description |
|----------|------|-------------|
| Static graph JSON | `{source-branch}/app.json` on `radius-graph` orphan branch | Committed to the orphan branch, never pollutes source branches |

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

- **rad CLI**: Built from the requested `workflow_source_ref` in `radius-project/radius`
- **Bicep CLI**: Managed internally by `rad graph build` or installed as a runtime dependency by that command

## Notes

- The reusable workflow lives in the Radius repo and follows the existing reusable-workflow naming convention (`__` prefix).
- Core graph logic lives in the `rad` CLI, not in workflow YAML, so contributors can run the same command locally.
- Consumer repos add only a thin wrapper workflow, avoiding duplicated CI logic.
