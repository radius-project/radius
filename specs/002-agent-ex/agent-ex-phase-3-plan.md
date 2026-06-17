<!-- markdownlint-disable MD060 -->
# Agent Ex — Phase 3 implementation plan (Contributing docs)

This document is the concrete, actionable plan for delivering **Phase 3 — Contributing docs** of the [Agent Ex plan](agent-ex-plan.md#phase-3--contributing-docs) in the `radius/` repository. It turns the phase's deliverables into an ordered, verifiable set of workstreams grounded in an audit of the docs as they exist today.

It assumes the Phase 0 conventions are already live — [contributing-agent-assets.md](../../docs/contributing/contributing-agent-assets.md), [authoring-contributing-docs.md](../../docs/contributing/authoring-contributing-docs.md), and [extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md) — and that Phase 1 (`AGENTS.md` + symlink) has landed. Every doc this phase touches must conform to those conventions.

## North star for the phase

> Every "how do I do X?" question a contributor (human or agent) asks has **exactly one** current, correct, primary doc that answers it, written in the standard **Purpose → Prerequisites → Steps → Verification → Troubleshooting** format, reachable from a single docs index, and mapped from a capability index.

The current docs are old and overlapping. This phase is explicitly a **consolidate / rewrite / refactor** effort, not an additive one. We delete and merge aggressively; we do not leave two docs that answer the same question.

## Scope

- **In scope**: `CONTRIBUTING.md` and everything under `docs/contributing/` in `radius/`. Audit, consolidate, reformat, verify against current code, fill the gaps, and add the docs index + capability index to `docs/contributing/README.md`. Seed the code↔doc path map.
- **Out of scope**: `docs/architecture/` (that is [Phase 4](agent-ex-plan.md#phase-4--architecture-docs-grounded-in-code)); `.github/instructions/*` content audit (that is [Phase 5](agent-ex-plan.md#phase-5--coding-instructions-project-specific-only)); new skills/prompts/agents (that is [Phase 6](agent-ex-plan.md#phase-6--per-workflow-copilot-conveniences)); the satellite repos (they follow `radius/`); and the dev-container / workflow / Makefile setup reconciliation (explicitly out of scope per the [Phase 2 note](agent-ex-plan.md#phase-2--cloud-agent-bootstrap)).
- **The Phase 3 gate**: after the audit, re-evaluate the Phase 5/6 backlog. Many "missing skill" items collapse into "an existing doc is enough." Record that re-evaluation in the Phase 3 PR description so Phases 5 and 6 start from a trimmed backlog.

## 1. Current state (audit findings)

A full audit of `CONTRIBUTING.md` and `docs/contributing/` (excluding the four Phase 0 files) found ~38 docs in 15 topic clusters with substantial overlap and several stale spots. The findings that drive this plan:

### 1.1 Duplicated scope (consolidation targets)

| Cluster                     | Overlapping docs                                                                                                                                                                                                                                                                                                                                                                                                                                               | Problem                                                                                                         |
|-----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| Entry / table of contents   | [CONTRIBUTING.md](../../CONTRIBUTING.md), [how-to.md](../../docs/contributing/how-to.md), [contributing/README.md](../../docs/contributing/README.md)                                                                                                                                                                                                                                                                                                          | Three different top-level link lists with different contents; none is authoritative.                            |
| Prerequisites               | [contributing-code-prerequisites/README.md](../../docs/contributing/contributing-code/contributing-code-prerequisites/README.md), [first-commit-00-prerequisites/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-00-prerequisites/index.md), [first-commit-01-development-tools/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-01-development-tools/index.md) | Tool lists are manually kept "in sync" (the files say so), a known maintenance trap.                            |
| Building                    | [contributing-code-building/README.md](../../docs/contributing/contributing-code/contributing-code-building/README.md), [first-commit-02-building/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-02-building/index.md)                                                                                                                                                                                        | Two build walkthroughs; unclear which is authoritative.                                                         |
| CLI development             | [contributing-code-cli/README.md](../../docs/contributing/contributing-code/contributing-code-cli/README.md), [first-commit-03-working-on-cli/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-03-working-on-cli/index.md), [first-commit-04-debugging-cli/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-04-debugging-cli/index.md)                           | Partial duplication; authoritative source unclear.                                                              |
| Running locally / debugging | [contributing-code-control-plane/running-controlplane-locally.md](../../docs/contributing/contributing-code/contributing-code-control-plane/running-controlplane-locally.md) (deprecated), [contributing-code-debugging/radius-os-processes-debugging.md](../../docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md)                                                                                              | Two parallel guides; the older one is already marked deprecated.                                                |
| Testing                     | [contributing-code-tests/running-functional-tests.md](../../docs/contributing/contributing-code/contributing-code-tests/running-functional-tests.md), [testing-local.md](../../docs/contributing/contributing-code/contributing-code-tests/testing-local.md), plus the [first-commit-05-running-tests/index.md](../../docs/contributing/contributing-code/contributing-code-first-commit/first-commit-05-running-tests/index.md) walkthrough                   | Overlapping "how to run tests" scope with no clear split between the unit/functional/local-iteration audiences. |

### 1.2 Stale content (fix-or-delete targets)

- **Deprecated guide**: `running-controlplane-locally.md` opens with a "Deprecated / Legacy Guide" banner pointing at `radius-os-processes-debugging.md` and says it "will be removed after the transition period."
- **Dead link**: `first-commit-03-working-on-cli/index.md:81` links to `../../contributing-code-cli/running-rad-cli.md`, which does not exist.
- **"Under construction" markers**: `contributing-code-tests/README.md` (integration tests section) and `contributing-code-control-plane/configSettings.md` (legacy environment variables section).
- **Commented-out content**: the GitHub Codespaces section in `contributing-code-prerequisites/README.md` is commented out with no explanation.

### 1.3 Format conformance

Almost no existing doc follows the standard **Purpose → Prerequisites → Steps → Verification → Troubleshooting** format. Some are genuine reference material (config schemas, code organization, naming conventions) or process/philosophy guides (code review, triage, releases) for which the five-section how-to format does not apply — those keep a reference/process shape but still get a one-paragraph **Purpose** and accurate links. The how-to/workflow docs (prerequisites, building, running locally, testing, schema changes, CLI, PRs, first commit) must be reshaped into the five-section format.

### 1.4 Gaps called out by the plan for `radius/`

The [Phase 3 deliverables](agent-ex-plan.md#phase-3--contributing-docs) name three `radius/` gaps. The audit shows the underlying content mostly exists but is fragmented or unverified rather than missing:

- **TypeSpec → Swagger → Go pipeline**: covered by [contributing-code-schema-changes/README.md](../../docs/contributing/contributing-code/contributing-code-schema-changes/README.md) but not in the standard format and not verified end-to-end against current `make` targets.
- **Full test matrix**: spread across the `contributing-code-tests/` files with an "under construction" integration-test section; needs one authoritative overview that names every test tier and its command.
- **Local dev environment setup**: covered by the prerequisites doc + `radius-os-processes-debugging.md`; needs to be the single, verified path once the deprecated guide is removed.

So for `radius/`, Phase 3 is overwhelmingly **consolidation + verification + reformat**, with only small net-new writing.

## 2. Target structure

The consolidation collapses the overlaps into one primary doc per topic. Authoritative source per topic after this phase:

| Topic                           | Single authoritative doc                                         | Action                                                                                                                                  |
|---------------------------------|------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| Contribution overview / entry   | [CONTRIBUTING.md](../../CONTRIBUTING.md)                         | Trim to process + DCO + one link to the docs index. Fold [how-to.md](../../docs/contributing/how-to.md) into it and delete `how-to.md`. |
| Docs index + capability index   | [docs/contributing/README.md](../../docs/contributing/README.md) | Extend with a docs index and the capability index (new content this phase).                                                             |
| Prerequisites / dev environment | `contributing-code-prerequisites/README.md`                      | Make it the single source of truth. First-commit prereq/dev-tools steps shrink to a link.                                               |
| Building                        | `contributing-code-building/README.md`                           | Authoritative; first-commit building step links to it.                                                                                  |
| Running & debugging locally     | `contributing-code-debugging/radius-os-processes-debugging.md`   | Authoritative; **delete** `running-controlplane-locally.md`.                                                                            |
| Testing                         | `contributing-code-tests/README.md`                              | Becomes the authoritative test-matrix overview; sub-pages cover one tier each with explicit audience/scope.                             |
| Schema changes                  | `contributing-code-schema-changes/README.md`                     | Reformat + verify the TypeSpec → Swagger → Go pipeline.                                                                                 |
| CLI development                 | `contributing-code-cli/README.md`                                | Authoritative; first-commit CLI steps link to it; fix the dead `running-rad-cli.md` link.                                               |
| Pull requests                   | `contributing-pull-requests/README.md`                           | Reformat.                                                                                                                               |
| Code review                     | `contributing-code-reviewing/README.md`                          | Keep as process guide; add Purpose + accurate links.                                                                                    |
| Issues / triage                 | `contributing-issues/README.md`, `triage/triage-process.md`      | Keep; add Purpose + accurate links.                                                                                                     |
| Releases                        | `contributing-releases/README.md`                                | Keep as process guide; add Purpose + accurate links.                                                                                    |
| First-commit walkthrough        | `contributing-code-first-commit/**`                              | Stays a tutorial series, but each step **links to** the authoritative topic doc instead of duplicating it.                              |

The first-commit series is deliberately preserved as a guided tutorial — its value is sequencing, not content. The fix is to make every step a thin walkthrough that defers to the authoritative topic doc, killing the sync burden.

## 3. Workstreams

Ordered so that destructive consolidation happens before reformatting (no point formatting a doc we are about to delete), and the indexes come last (they reference the final paths).

### WS1 — Consolidate and delete (do this first)

1. Delete `running-controlplane-locally.md`; update `contributing-code-control-plane/README.md` and any inbound links to point to `radius-os-processes-debugging.md`.
2. Fold `how-to.md` into `CONTRIBUTING.md`; delete `how-to.md`; update inbound links.
3. De-duplicate prerequisites: `contributing-code-prerequisites/README.md` is canonical; first-commit prereq/dev-tools steps shrink to an intro + link; remove the "keep in sync" notes.
4. De-duplicate building and CLI: first-commit building/CLI steps link to `contributing-code-building/` and `contributing-code-cli/`.
5. Resolve the testing overlap: define the audience split (unit vs. functional vs. local-iteration) and cross-link; remove duplicated setup prose.

### WS2 — Fix stale content

1. Fix the dead `running-rad-cli.md` link in `first-commit-03-working-on-cli/index.md` (point to the CLI guide section, or create the target if the content is genuinely needed).
2. Resolve the two "under construction" markers (`contributing-code-tests/README.md` integration tests; `configSettings.md` legacy env vars): either complete the section or replace the marker with an accurate current-state statement.
3. Decide Codespaces: restore the commented-out section with working instructions, or delete it and state why.

### WS3 — Reformat how-to docs to the standard format

Apply **Purpose → Prerequisites → Steps → Verification → Troubleshooting** (per [authoring-contributing-docs.md](../../docs/contributing/authoring-contributing-docs.md)) to: prerequisites, building, running/debugging locally, testing (overview + per-tier), schema changes, CLI, pull requests, and each first-commit step. Reference/process docs (organization, writing, config settings, logging, code review, triage, releases, design) get a one-paragraph **Purpose** and verified links but keep their reference/process shape.

### WS4 — Verify against current code

For every reformatted doc, run or read-the-source-for every command, path, flag, and `make` target. Confirm `make generate`, `make build`, `make test`, and the functional-test commands exist and behave as documented. Spot-check file paths against the tree. Fix the doc, not the code.

### WS5 — Docs index in `docs/contributing/README.md`

Add a docs index section that `AGENTS.md` links to: a grouped list of every contributing doc by topic (Getting started, Building & running, Testing, Schema & API, CLI, PRs & review, Process). This replaces the three competing link lists with one.

### WS6 — Capability index in `docs/contributing/README.md`

Add the capability index table that maps each leaf capability in [agent-ex-features.md](agent-ex-features.md#capabilities) that `radius/` owns to its single primary backing doc. This is the contractual Phase 3 artifact (see §4). Every `radius/`-owned leaf must have exactly one primary doc; a leaf with none is a Phase 3 gap to fill, not a row to omit.

### WS7 — Seed the code↔doc path map

Populate the (currently empty) [code↔doc path map](../../docs/contributing/contributing-agent-assets.md#code--doc-path-map) with the `<code-glob> ↔ <doc-path>` rows implied by the capability index (for example `typespec/** ↔ contributing-code-schema-changes/README.md`, `pkg/cli/** ↔ contributing-code-cli/README.md`). This feeds the Phase 0 docs-drift code-review step and the Phase 7 drift workflow.

## 4. Capability index design

The index lives in `docs/contributing/README.md`. It lists every leaf capability `radius/` owns and maps it to exactly one primary doc. Parent rows (for example capability 1, "Build and test") are the union of their children and link to the docs index rather than a single doc. Capabilities owned by satellite repos (1.9 resource types, 1.10 dashboard, 1.13 AWS Bicep types) are out of `radius/`'s index. Proposed mapping:

| Capability                    | Primary backing doc                                                                  |
|-------------------------------|--------------------------------------------------------------------------------------|
| 1.1 Set up a dev environment  | `contributing-code-prerequisites/README.md`                                          |
| 1.2 Write Go code             | `contributing-code-writing/README.md`                                                |
| 1.3 Schema changes            | `contributing-code-schema-changes/README.md`                                         |
| 1.4 CLI commands              | `contributing-code-cli/README.md`                                                    |
| 1.5 GitHub workflows          | *gap — confirm whether a doc or the instruction file is the primary; fill if needed* |
| 1.6 Dockerfiles               | *gap — confirm primary doc; fill if needed*                                          |
| 1.7 Bicep files               | *gap — confirm primary doc; fill if needed*                                          |
| 1.8 Shell scripts & Makefiles | *gap — confirm primary doc; fill if needed*                                          |
| 1.11 Documentation            | `authoring-contributing-docs.md`                                                     |
| 1.12 Pull requests            | `contributing-pull-requests/README.md`                                               |
| 2 Code review                 | `contributing-code-reviewing/README.md`                                              |
| 3 Investigate issues          | `contributing-issues/README.md`                                                      |
| 5.1 Author a new doc          | `authoring-contributing-docs.md`                                                     |
| 5.2 Repair drift              | `extending-agent-ex.md`                                                              |
| 5.3 Add a capability          | `extending-agent-ex.md`                                                              |

Rows 1.5–1.8 are the real open question for this phase: today these capabilities are backed only by `.github/instructions/*` files, but the file-strategy rule requires a *primary contributing doc* per leaf. Phase 3 resolves each by either (a) confirming an existing contributing doc as primary, or (b) writing a short primary doc that the instruction file complements. This decision is part of WS6, not deferred to Phase 5.

## 5. Verification

Matches the [plan's Phase 3 verification](agent-ex-plan.md#phase-3--contributing-docs) and the [CI gates](agent-ex-plan.md#6-ci-gates-deterministic-run-on-every-pr):

- **Deterministic**:
  - Every how-to doc uses the five-section format; reference/process docs have a Purpose paragraph.
  - Markdown link check passes over `CONTRIBUTING.md` and `docs/contributing/` (no dead links — specifically the `running-rad-cli.md` link is resolved).
  - `make spellcheck` (`cspell`) passes; new technical terms added to `.cspellignore`.
  - Markdown lint passes (run the `markdown-lint` skill on changed files).
  - `docs/contributing/README.md` capability index: every `radius/`-owned leaf capability appears exactly once and every linked path resolves.
  - No deprecated/duplicate docs remain (`running-controlplane-locally.md` and `how-to.md` are gone; no "under construction" markers remain unresolved).
- **Prompt-based** (manual acceptance): in a fresh agent session, ask the five canonical questions and confirm each answer cites the one authoritative doc and is correct:
  1. "How do I set up a dev environment for Radius?"
  2. "How do I build the repo and run the unit tests?"
  3. "How do I run the control plane locally and debug it?"
  4. "How do I make an API/schema change (TypeSpec → Swagger → Go)?"
  5. "How do I open a pull request?"

## 6. Task list

Dependency-ordered. Each task is a small, reviewable PR-sized unit; WS1 lands before WS3 so we never reformat a doc we then delete.

- [ ] **T1 (WS1)**: Delete `running-controlplane-locally.md`; repoint inbound links to `radius-os-processes-debugging.md`.
- [ ] **T2 (WS1)**: Fold `how-to.md` into `CONTRIBUTING.md`; delete `how-to.md`; fix inbound links.
- [ ] **T3 (WS1)**: Consolidate prerequisites; shrink first-commit prereq/dev-tools steps to links; remove "keep in sync" notes.
- [ ] **T4 (WS1)**: Consolidate building + CLI; first-commit steps link to the authoritative docs.
- [ ] **T5 (WS1)**: Resolve testing overlap; define audience split; cross-link.
- [ ] **T6 (WS2)**: Fix the dead `running-rad-cli.md` link.
- [ ] **T7 (WS2)**: Resolve the two "under construction" markers and the Codespaces comment block.
- [ ] **T8 (WS3)**: Reformat the how-to docs to the five-section format.
- [ ] **T9 (WS3)**: Add a Purpose paragraph + verified links to the reference/process docs.
- [ ] **T10 (WS4)**: Verify every command/path/flag/`make` target against current code.
- [ ] **T11 (WS6)**: Resolve the 1.5–1.8 primary-doc question; write any short primary docs needed.
- [ ] **T12 (WS5)**: Add the docs index to `docs/contributing/README.md`.
- [ ] **T13 (WS6)**: Add the capability index to `docs/contributing/README.md`.
- [ ] **T14 (WS7)**: Seed the code↔doc path map in `contributing-agent-assets.md`.
- [ ] **T15 (gate)**: Re-evaluate the Phase 5/6 backlog in light of the audit; record the trimmed backlog in the Phase 3 PR.
- [ ] **T16 (verify)**: Run the deterministic checks (link check, `make spellcheck`, markdown lint) and the five canonical prompt questions.

## 7. Risks and notes

- **Inbound links from outside `docs/contributing/`**: deleting `how-to.md` and `running-controlplane-locally.md` may break links in `AGENTS.md`, the dashboard/docs repos, or blog posts. Grep the repo for every inbound link before deleting and update all of them in the same PR.
- **First-commit screenshots**: the walkthrough relies on co-located images; keep them when trimming prose so the tutorial still reads.
- **Reference vs. how-to judgment**: do not force genuine reference docs (config schemas, naming conventions) into the five-section how-to shape — apply the [authoring-contributing-docs.md](../../docs/contributing/authoring-contributing-docs.md) split. A doc that "spans two formats" is two docs.
- **Spec Kit**: per [plan §8](agent-ex-plan.md#8-execution-order), this phase can be driven through `/speckit.specify` scoped to these deliverables. This document is the human-readable plan that prompt should encode.
- **Do not edit the planning docs as part of capability work**: per [extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md), ongoing capability work updates the live docs and the capability index only — not `agent-ex-features.md` or `agent-ex-plan.md`. This Phase 3 plan document is itself a planning artifact and is not edited by later capability work.
