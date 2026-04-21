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
| 2026-04-20 | Added full schema reference section (all types from TypeSpec), complete artifact example with frontend+cache+environment, and static vs run-time comparison table. Replaced abbreviated inline JSON example with link to new section. |
| 2026-04-20 | Moved CLI flags table from Detailed Design to UX section. Added missing `--output` flag. Added explanation of local file vs orphan branch mode switching. Populated empty `2023-10-app-graph.md` with original content from archived `design-notes` repo. |
| 2026-04-20 | Added "Concurrent PR handling" section documenting: directory-per-branch isolation, GitHub Actions concurrency group (`build-app-graph-${{ github.ref }}`), fetch-before-write in gitstate, race condition risk (no retry on push failure), and stale directory cleanup gap. |

## Key Findings from Code/Spec Review

- **Sample output in doc is aspirational**: The `rad graph build` output shown ("Parsed 4 resources...") doesn't match the actual code. Real output is just "Static graph artifact written to..." or "committed to...". Marked with `#Q` in doc.
- **Two output modes**: Local file (default `--output .radius/static/app.json`) and orphan branch (`--orphan-branch` + `--source-branch`). The `--output` flag is now documented in the CLI flags table in the UX section.
- **`rad app graph --dry-run` is not from Will's spec**: Will's feature spec defines "Planned app graph (P2)" as just "TBD". The `--dry-run` CLI syntax was invented in the design doc. 
- **Will's spec has UX flows not fully captured**: dedicated app page (`/{owner}/{repo}/radius/app/{name}`), deployed graph status colors (grey/yellow/green/blue/red), cloud console navigation on click, error modal for failed resources, and Copilot-assisted workflow illustration.
- **Diff colors match Will's spec**: Green=added, red=removed, yellow=modified originated from Will's feature spec. Design doc refined with Primer tokens.
- **`codeReference` format**: Will's spec defines validation rules — repo-relative path, forward slashes, optional `#L<number>` anchor, no URL scheme/host/query/traversal. Code has a validator (`coderef-validator.ts`).
