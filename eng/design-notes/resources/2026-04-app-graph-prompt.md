# Application Graph Design Spec — Working Prompt

This file captures the iterative prompts and context used to develop and refine `2026-04-app-graph.md`.

## Source Branches

| Branch | Repo | What it contains |
|---|---|---|
| `github-demo` | `origin` / `upstream` | Working graph builder CLI (`pkg/cli/graph/`), browser extension source (`web/browser-extension/src/`), built extension (`web/browser-extension/dist/`) |
| `design-update-app-graph` | `origin` | Current design doc updates (this branch) |
| `filesystem-state` | `upstream` | `rad shutdown`, orphan branch persistence (`radius-state`), `gitstate` package — the repo/CI workflow this feature targets |
| `app-graph-viz-gh-feature-spec` | `willtsai/radius` | Feature spec by Will Tsai |
| `staticgraph` | `origin` | Static graph work branch |
| `pr-graph-view-fix` | `origin` | PR graph view fixes |
| `002-github-app-graph-viz` | `upstream` | Upstream tracking branch |

## Original Prompt

> OK. go over the below and update `2026-04-app-graph.md` so that the design provides much more details.
> `github-demo` branch from upstream, my current `2026-04-app-graph.md` in this repo, feature spec from
> https://github.com/willtsai/radius/tree/app-graph-viz-gh-feature-spec and sylvain's
> https://github.com/radius-project/radius/tree/filesystem-state is the branch that captures repo radius
> for which a lot of this feature is focused on. First step you will create a new branch. Then you run
> your analysis on each of these branches and then use your findings to update the design spec
> `2026-04-app-graph.md`. Use the design template to cover all topics necessary. Also, can you make a
> spec file that captures these instructions and then work based on that file?

## Known Issues in the Design Doc

- ~~**DiffHash algorithm mismatch**~~: Fixed 2026-04-20. Changed all 5 occurrences of "BLAKE2b" to "SHA-256" to match the actual Go implementation (`crypto/sha256` in `pkg/cli/graph/diffhash.go`).
- **Asterisk typo**: Was only in attachment preview; actual file is correct.

## Code Locations (on `github-demo` branch)

### Go CLI — `pkg/cli/graph/`

| File | Purpose |
|---|---|
| `build.go` | `BuildStaticGraph()` — ARM JSON parsing, resource/connection extraction |
| `build_test.go` | Unit tests for graph building |
| `diffhash.go` | `ComputeDiffHash()` — SHA-256 hashing of review-relevant properties |
| `diffhash_test.go` | Unit tests for hash determinism and stability |
| `doc.go` | Package documentation |

### Browser Extension — `web/browser-extension/src/`

| File | Purpose |
|---|---|
| `background/service-worker.ts` | Extension lifecycle |
| `content/inject.ts` | Page detection, dispatch to handlers |
| `content/repo-tab.ts` | "Application graph" tab on repo root |
| `content/pr-graph.ts` | PR diff graph orchestrator |
| `content/app-page.ts` | Dedicated app graph page |
| `content/graph-renderer.ts` | Cytoscape.js rendering with dagre layout |
| `content/graph-diff.ts` | Diff computation (added/removed/modified/unchanged) |
| `content/graph-diff.test.ts` | Unit tests for diff computation |
| `content/graph-navigation.ts` | Node click popups with code links |
| `content/coderef-validator.ts` | Code reference validation |
| `content/coderef-validator.test.ts` | Tests for code reference validation |
| `popup/popup.ts` | Extension popup UI |
| `shared/api.ts` | Token storage, GitHub client factory |
| `shared/github-api.ts` | GraphGitHubAPI class |
| `shared/github-client.ts` | GitHub API client |
| `shared/graph-types.ts` | TypeScript types mirroring Go schema |
| `shared/device-flow.ts` | GitHub App device flow authentication |
| `shared/radius-api.ts` | Radius API client |
| `shared/types.ts` | General types |
| `app-create/app-create.ts` | App creation UI |
| `styles/graph.css` | Primer-aligned styling |

## Iteration Log

| Date | Action |
|---|---|
| 2026-04-16 | Initial design doc created on `design-update-app-graph` branch |
| 2026-04-17 | Updated with comprehensive details from `github-demo`, `filesystem-state`, and feature spec analysis |
| 2026-04-20 | Identified BLAKE2b/SHA-256 mismatch — fixed in design doc. Created prompt file. Created `design-with-code` branch from `github-demo` with both design files. |
