# Graph Visualization Extension — Quickstart

This guide walks you through setting up the graph visualization features added in this branch. It assumes you are already familiar with the browser extension codebase.

---

## 1. Build and Load the Extension

```bash
cd web/browser-extension
npm install
npm run build
```

Load the built extension:

1. Open `chrome://extensions/` (Chrome) or `edge://extensions/` (Edge)
2. Enable **Developer mode**
3. Click **Load unpacked** → select `web/browser-extension/dist/`
4. Pin the extension icon

---

## 2. See It in Action on the Demo Repo

Before setting up your own repo, you can see working examples on the demo repo:

- **Repo root graph tab**: <https://github.com/brooke-hamilton/radius-demo-app> — click the **Application graph** tab next to README
- **PR diff graph**: <https://github.com/brooke-hamilton/radius-demo-app/pull/1> — a color-coded diff graph appears below the PR description (green = added, yellow = modified, red = removed)

Click any node in the graph to see popup links to the source code and `app.bicep` definition.

---

## 3. Set Up Graph Visualization on Your Own Repo

### Prerequisites

- A GitHub repo with an `app.bicep` file **at the repository root** (the extension looks for exactly `app.bicep` at the root — subdirectories won't work)
- The `rad` CLI built from this branch (it has the `rad graph build` subcommand)

### Option A: Automate with GitHub Actions (recommended)

The graph build logic lives in a **reusable workflow** in `radius-project/radius` (`__build-app-graph.yml`). It's a `workflow_call` workflow, so it can't run on its own — your repo needs a small wrapper workflow that calls it. Add this file to your repo:

```yaml
# .github/workflows/build-app-graph.yml
name: Build Application Graph

on:
  push:
    branches: [main]
    paths: [app.bicep]
  pull_request:
    paths: [app.bicep]

permissions:
  contents: write

jobs:
  build-graph:
    uses: radius-project/radius/.github/workflows/__build-app-graph.yml@brooke-hamilton/merge-oidc-branch
    with:
      app_file: app.bicep
      orphan_branch: radius-graph
      workflow_source_ref: brooke-hamilton/merge-oidc-branch
```

> **Note**: The `workflow_source_ref` must point to this branch (`brooke-hamilton/merge-oidc-branch`) until these changes are merged to `main`. After merge, change it to `main` (or remove it — `main` is the default).

This workflow triggers on pushes to `main` and on PRs that modify `app.bicep`. It builds `rad` from the Radius source, installs the Bicep CLI, and runs `rad graph build` with `--orphan-branch`.

### Option B: Manual graph build (workaround)

If the GitHub Actions workflow isn't working or you need to iterate quickly, you can run the graph build locally.

Build `rad` from this branch:

```bash
cd /workspace/radius-project/radius
go build -o /tmp/rad ./cmd/rad
```

Then, from your repo's working directory:

```bash
cd /path/to/your-repo

# Build the graph for main and push it to the radius-graph orphan branch
/tmp/rad graph build \
  --bicep app.bicep \
  --orphan-branch radius-graph \
  --source-branch main
```

This compiles `app.bicep` to ARM JSON, parses resources/connections/diff hashes, and commits `main/app.json` to the `radius-graph` orphan branch.

To build a graph for a feature branch (needed for PR diff visualization):

```bash
git checkout my-feature-branch
/tmp/rad graph build \
  --bicep app.bicep \
  --orphan-branch radius-graph \
  --source-branch my-feature-branch
```

The extension compares `main/app.json` vs `my-feature-branch/app.json` on the `radius-graph` orphan branch to render the PR diff.

### What Gets Stored

The `radius-graph` orphan branch ends up with:

```
main/app.json              # graph artifact for the main branch
my-feature-branch/app.json # graph artifact for a feature branch
```

Each `app.json` contains the resource topology, connections, source line mappings, and diff hashes that the extension uses to render the graph.

---

## 4. How the Extension Uses the Graph

| GitHub Page | What Renders | How It Works |
|---|---|---|
| Repo root | **Application graph** tab (next to README) | Fetches `main/app.json` from the `radius-graph` branch via GitHub Contents API |
| Pull request | Diff graph below PR description | Fetches both base and head `app.json`, compares `diffHash` values, color-codes nodes |

**Important**: The PR diff graph only appears on PRs that modify `app.bicep`. If the PR doesn't touch `app.bicep`, no graph is shown.

---

## Troubleshooting

- **No "Application graph" tab on repo root?** — Confirm `app.bicep` exists at the repo root (not in a subdirectory).
- **PR graph says "Application graph not yet available"?** — The graph artifacts haven't been built yet. Run `rad graph build` manually for both the base and head branches, or wait for CI.
- **Graph doesn't update after merging?** — The CI workflow needs to run on the push to `main` to rebuild `main/app.json`. Check Actions for errors.
- **Bicep compilation fails?** — Make sure the Bicep CLI is on your PATH. The `rad graph build` command calls `bicep build` under the hood.
