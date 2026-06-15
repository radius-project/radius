<!-- markdownlint-disable MD060 -->
# Agent Ex — Agent Experience Features

**Vision**: Humans and AI agents collaborate as peers across all Radius repositories. Every capability is powered by the same knowledge base — humans read the contributing and architecture docs directly; every supported agent reads the same docs through a single entry point (`AGENTS.md`). Tool-specific UX (skills, prompts, custom agent modes) is convenience over that shared knowledge — never the only place a capability lives.

## Supported AI tools

- GitHub Copilot in VS Code
- GitHub Copilot CLI
- GitHub Copilot Cloud Agent
- Claude Code

See [agent-ex-plan.md](agent-ex-plan.md) for the implementation plan.

---

## Scope

This initiative covers these Radius repositories:

| Repo | Primary tech | Key agent workflows |
|---|---|---|
| `radius/` | Go, TypeSpec, Bicep | Control plane, CLI, API types, schema changes, testing |
| `dashboard/` | TypeScript, React | Backstage plugins, UI components |
| `docs/` | Markdown, Hugo | Documentation authoring, content review |
| `resource-types-contrib/` | YAML, Terraform, Bicep | Resource type definitions, recipes |
| `bicep-types-aws/` | Go, TypeScript | AWS type generation pipeline |

---

## Success Criteria

- [ ] Every repo has an `AGENTS.md` (symlinked to `.github/copilot-instructions.md`) that orients any agent in any tool
- [ ] A contributor to any repo can ask any agent "how do I build this?" and get a correct answer
- [ ] An agent can build, test, and submit a PR for a code change in any repo
- [ ] Contributing docs are accurate, current, and follow a single template
- [ ] Architecture docs cover every major subsystem and are grounded in real code references
- [ ] Cloud Agent can bootstrap and work in any repo out of the box (`copilot-setup-steps.yml`)
- [ ] Every leaf capability in the [Capabilities](#capabilities) table maps to exactly one primary backing contributor doc, surfaced through a capability index in `docs/contributing/README.md`
- [ ] Every skill, custom agent, and VS Code prompt links to a backing doc that any other tool can read
- [ ] Any contributor can ask an agent to draft a new contributing or architecture doc and get a usable first draft
- [ ] Code changes that affect documented workflows are flagged in code review (Copilot Code Review and the docs-drift code-review instructions) so docs don't drift; any drift that slips through is caught by the scheduled `docs-drift-review.yml` workflow
- [ ] Adding a new capability to the agent-ex system itself is a documented, contributor-runnable workflow
- [ ] The skill lifecycle agent detects and recommends improvements from real usage data

---

## Capabilities

| # | Capability | Summary |
|---|---|---|
| 1 | [**Build and test**](#1-build-and-test-features-and-bug-fixes) | Pick up an issue, build, implement, test, and submit a PR |
| 1.1 | [Set up a dev environment](#11-set-up-a-development-environment) | Bootstrap any repo without trial-and-error |
| 1.2 | [Write Go code](#12-write-and-modify-go-code) | Idiomatic Go following Radius conventions |
| 1.3 | [Schema changes](#13-modify-api-type-definitions-schema-changes) | TypeSpec → Swagger → Go code generation pipeline |
| 1.4 | [CLI commands](#14-add-or-update-cli-commands) | Add/update `rad` CLI commands (Cobra) |
| 1.5 | [GitHub workflows](#15-edit-github-actions-workflows) | Explain, modify, refactor, and debug CI/CD |
| 1.6 | [Dockerfiles](#16-write-and-modify-dockerfiles) | Multi-stage builds, minimal images, security |
| 1.7 | [Bicep files](#17-write-and-modify-bicep-files) | Radius Bicep naming and deployment patterns |
| 1.8 | [Shell scripts & Makefiles](#18-write-shell-scripts-and-makefiles) | Safe Bash, well-structured Make targets |
| 1.9 | [Resource types (contrib)](#19-add-or-update-resource-type-definitions-contrib) | Scaffold YAML definitions, recipes, and tests |
| 1.10 | [Dashboard plugins](#110-develop-dashboard-plugins) | Backstage plugins in TypeScript/React |
| 1.11 | [Documentation](#111-author-and-edit-documentation) | Hugo-based docs with frontmatter and shortcodes |
| 1.12 | [Pull requests](#112-create-pull-requests-and-manage-contributions) | Branch handling, fork-aware defaults, clean PRs |
| 1.13 | [AWS Bicep types](#113-generate-aws-bicep-types) | AWS type generation pipeline in `bicep-types-aws/` |
| 2 | [**Code review**](#2-review-code) | File-by-file review for bugs, style, and test coverage |
| 3 | [**Investigate issues**](#3-investigate-issues) | Analyze issues, find relevant code, produce technical summaries |
| 4 | [**Explain architecture**](#4-explain-architecture-and-design) | Answer "how does X work?" with code references and Mermaid diagrams |
| 5 | [**Author and evolve docs and capabilities**](#5-author-and-evolve-documentation-and-capabilities) | Human-triggered, AI-executed authoring of docs and extension of the agent-ex system itself |
| 5.1 | [Author a new doc](#51-author-a-new-contributor-or-architecture-doc) | Draft a contributor or architecture doc from a topic |
| 5.2 | [Repair drift](#52-keep-docs-in-sync-with-code) | Detect and fix docs that fall behind code/tooling changes |
| 5.3 | [Add a capability](#53-add-a-new-capability-to-the-agent-ex-system) | Author or extend a contributing doc, scaffold optional wrappers (instruction/skill/prompt/agent), update the capability index |
| 6 | [**Improve agent effectiveness**](#6-continuously-improve-agent-effectiveness) | Automated review of session logs to curate skills and instructions |

### 1. Build and test features and bug fixes

An agent can pick up an issue, build the project, implement a change, test it, and submit a pull request — guided by the same docs a human contributor would follow. This is the highest-value capability and spans many sub-scenarios below.

#### 1.1 Set up a development environment

An agent (local or cloud) can bootstrap a working environment in any Radius repo without trial-and-error tool installation. Dev containers and [`copilot-setup-steps.yml`](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/customize-the-agent-environment#customizing-copilots-development-environment-with-copilot-setup-steps) will give agents the same turnkey setup humans get. These do not exist yet — see Phase 2 in [agent-ex-plan.md](agent-ex-plan.md).

**Validation**: "Build this project and run the unit tests." — the agent succeeds without manual intervention in any repo.

#### 1.2 Write and modify Go code

An agent writes idiomatic Go that follows Radius conventions: error handling, naming, package layout, test patterns. Instructions encode project-specific conventions that go beyond what the model knows from training data.

**Sub-scenarios**:

- Implement a new feature in the control plane or resource providers
- Fix a bug identified in a GitHub issue
- Add or update unit tests and functional tests
- Refactor code while preserving existing test coverage

**Validation**: Agent-authored Go code passes linting, existing tests, and follows patterns visible in the surrounding codebase.

#### 1.3 Modify API type definitions (schema changes)

An agent can execute the full TypeSpec → Swagger → Go code generation pipeline when API types change. This is a multi-tool, multi-step workflow that is error-prone without guidance.

**Sub-scenarios**:

- Add a new field to an existing resource type
- Create a new resource type definition
- Update generated client/server code after a schema change
- Validate that generated code compiles and tests pass

**Validation**: Agent runs the pipeline end-to-end in the correct order and produces compilable, test-passing output.

#### 1.4 Add or update CLI commands

An agent can add a new `rad` CLI command or modify an existing one, following the Cobra command structure, flag conventions, and output formatting patterns used in the project.

**Sub-scenarios**:

- Add a new top-level or nested command
- Add flags and arguments following existing conventions
- Write unit tests for the command
- Update help text and documentation

**Validation**: New CLI command builds, tests pass, and `rad --help` shows correct output.

#### 1.5 Edit GitHub Actions workflows

The Radius CI/CD workflows are complex, interconnected, and have accumulated significant incidental complexity. An agent must deeply understand the existing workflows — their triggers, job dependencies, reusable workflow calls, matrix strategies, secret handling, and artifact flows — before making any changes. The goal is not just to edit workflows but to explain, simplify, and refactor them safely.

**Sub-scenarios**:

- **Explain**: Describe what a workflow does end-to-end — triggers, job graph, conditional logic, secret usage, and downstream effects. Answer questions like "what happens when a PR is merged to main?" or "which workflows run on a fork PR and what's skipped?"
- **Modify**: Add or change workflow steps, jobs, or triggers while preserving correctness. Follow Radius conventions: fork-testability, extracting complex logic into Make targets or shell scripts, proper secret and permission scoping, and reusable workflow patterns.
- **Refactor and simplify**: Identify redundancy, dead code, overly complex conditional logic, and unnecessary coupling between workflows. Propose and implement simplifications without introducing regressions — changes must preserve the same set of triggers, artifacts, and deployment behaviors.
- **Optimize**: Reduce CI run time by identifying parallelization opportunities, unnecessary job dependencies, and cacheable steps. Improve reliability by reducing flaky patterns.
- **Debug failures**: Given a failed CI run, trace the failure back to the specific job, step, and root cause. Distinguish between infrastructure flakes, test failures, and workflow configuration errors.

**Validation**: Modified workflows pass `actionlint`. Refactored workflows produce the same observable behavior (triggers fire, artifacts are created, deployments succeed) as the originals. The agent can accurately describe the job graph of any workflow when asked.

#### 1.6 Write and modify Dockerfiles

An agent follows container best practices — multi-stage builds, minimal images, proper layer ordering, security scanning — when creating or editing Dockerfiles.

**Validation**: Docker images build successfully, follow multi-stage patterns, and pass basic security checks.

#### 1.7 Write and modify Bicep files

An agent follows Radius Bicep conventions for naming, parameter design, and deployment patterns.

**Validation**: Bicep files compile and follow project naming conventions.

#### 1.8 Write shell scripts and Makefiles

An agent writes safe, portable Bash scripts (`set -euo pipefail`) and well-structured Make targets following Radius conventions.

**Validation**: Scripts pass `shellcheck`, Make targets work in CI and local environments.

#### 1.9 Add or update resource type definitions (contrib)

An agent can scaffold a new resource type in `resource-types-contrib/` — YAML definition, recipe templates, and tests — following the project-specific schema and conventions.

**Sub-scenarios**:

- Create a new resource type YAML definition
- Add Terraform or Bicep recipe templates
- Write tests for the resource type
- Update documentation

**Validation**: Resource type passes schema validation and recipe tests.

#### 1.10 Develop dashboard plugins

An agent can create or modify Backstage plugins in the `dashboard/` repository, following the TypeScript and React patterns used in the project.

**Validation**: Plugin builds, tests pass, and integrates correctly with the dashboard app.

#### 1.11 Author and edit documentation

An agent can create or update Hugo-based documentation in the `docs/` repository, following frontmatter conventions, linking patterns, and shortcode usage.

**Validation**: Docs site builds without errors, links resolve, frontmatter is valid.

#### 1.12 Create pull requests and manage contributions

An agent can create well-formed pull requests with proper branch handling, fork-aware defaults, and descriptive titles/bodies. This is the final step of every build-and-test workflow — the agent follows the contribution process from branch creation through PR submission.

**Sub-scenarios**:

- Create a PR with the correct base branch and clean commit history
- Write a descriptive PR title and body that references the originating issue
- Handle fork-aware defaults (upstream vs. origin)

**Validation**: Created PRs have correct base branches, clean commit history, and descriptive titles.

#### 1.13 Generate AWS Bicep types

An agent can run the AWS type generation pipeline in `bicep-types-aws/` — converting AWS CloudFormation schemas into Bicep type definitions.

**Validation**: Generated types compile and match the expected schema output.

---

### 2. Review code

An agent can perform thorough code reviews on pull requests: analyzing changes file-by-file, checking for bugs, evaluating test coverage, verifying adherence to project conventions, and assessing documentation impact.

**Sub-scenarios**:

- Review a PR for correctness, style, and test coverage
- Identify documentation that needs updating based on code changes
- Post actionable, line-specific review comments

**Validation**: Agent-authored review catches the same categories of issues a senior maintainer would flag, with accurate file paths and line numbers.

---

### 3. Investigate issues

An agent can analyze a GitHub issue, explore the codebase to identify relevant code paths, gather references (related issues, PRs, architecture docs), and produce a focused technical summary that helps a developer understand and evaluate the issue efficiently.

**Validation**: "Investigate issue #N" — the agent identifies the correct code areas, relevant tests, and prior art.

---

### 4. Explain architecture and design

An agent can answer questions about how the Radius system works — component relationships, request flows, data models — grounded in architecture documents and actual source code. It can generate Mermaid diagrams for visual explanation.

**Sub-scenarios**:

- Explain how a request flows through UCP to a resource provider
- Generate a component diagram of the control plane
- Describe the relationship between resource types and recipes
- Answer "how does X work?" questions grounded in code, not speculation

**Validation**: Explanations reference actual code paths and architecture docs. Mermaid diagrams render correctly and reflect current system structure.

---

### 5. Author and evolve documentation and capabilities

The agent-ex system has to grow with the codebase. Any contributor must be able to ask an agent to draft a new doc, repair a stale one, or add a brand-new capability — and the resulting changes must follow the same conventions as everything that came before. This is the meta-capability that keeps every other capability current.

#### 5.1 Author a new contributor or architecture doc

A human picks a topic ("running the dashboard locally", "how the deployment engine works") and triggers the agent. The agent drafts a doc in the standard format (Purpose → Prerequisites → Steps → Verification → Troubleshooting for contributing docs; Entry points → Packages → Flow diagram → Change-safety for architecture docs), grounded in real code references and existing patterns. The human reviews and merges.

**Sub-scenarios**:

- Draft a new contributing doc from a topic and a starting code reference
- Draft a new architecture doc for a subsystem from its entry point
- Add the new doc to the appropriate index (`docs/contributing/README.md`, `docs/architecture/README.md`, `AGENTS.md`)

**Validation**: A reviewer can merge the draft after one round of edits, not a rewrite.

#### 5.2 Keep docs in sync with code

When code changes, the docs that describe it should change with it. Code review (Copilot Code Review on GitHub.com and any agent surface running a review) flags PRs that touch documented workflows (CLI commands, build targets, schema generation, release process) without touching the matching doc, and suggests a concrete patch. The author updates the doc, asks the agent to draft the update, or replies in the review with the reason no doc update is needed. Drift that slips past per-PR review is caught by a weekly scheduled job.

**Sub-scenarios**:

- A reviewer flags a PR that changes `cmd/rad/cmd/*.go` without changing the CLI doc and proposes a concrete patch
- An agent reads the diff and the affected doc, then drafts the patch the reviewer suggested
- A weekly job walks the code↔doc path map, files an issue listing pairs where code has moved but the doc has not, and optionally drafts patches

**Validation**: Docs do not silently fall behind code. Drift gaps produce visible review comments or weekly issues, never silent rot.

#### 5.3 Add a new capability to the agent-ex system

When a new contributor workflow emerges (new tool added to the Go workflow, new repo joins the scope, new agent surface appears), a contributor follows the "add a capability" walkthrough in `docs/contributing/extending-agent-ex.md`. The agent walks them through: decide where it lives (doc only, instruction, skill, prompt, custom agent — using the decision tree in that doc); author or extend the primary contributing doc; scaffold any optional wrappers; add a row to the capability index in `docs/contributing/README.md`; wire any new top-level entry into `AGENTS.md`. The planning docs `agent-ex-features.md` and `agent-ex-plan.md` describe the original buildout and are not edited as part of this workflow.

**Sub-scenarios**:

- Add a new tool to an existing workflow (e.g., a new linter for Go) — extends the relevant contributing doc and, if needed, the matching path-scoped instruction
- Add a new top-level capability (e.g., performance tuning) — authors a new primary contributing doc, adds a row to the capability index, and adds any skill, prompt, or custom agent justified by the rules
- Onboard a new repo into the agent-ex scope — follows the repo-onboarding checklist in `docs/contributing/extending-agent-ex.md` (`AGENTS.md` from the template, `copilot-setup-steps.yml`, dev-container post-create script, contributing docs index, capability index for the capabilities the repo owns, and the docs-drift addition to `code-review.instructions.md`)

**Validation**: The process is captured in `docs/contributing/extending-agent-ex.md` (created in Phase 0 of the plan). Following it produces a coherent set of file changes that pass the CI gates in [agent-ex-plan.md, Section 6](agent-ex-plan.md#6-ci-gates-deterministic-run-on-every-pr) without manual cleanup.

---

### 6. Continuously improve agent effectiveness

An automated agent periodically reviews agent session logs to assess the effectiveness of skills, instructions, and agent definitions. It identifies what's working, what's failing, and what's missing — then recommends additions, edits, or removals.

**Sub-scenarios**:

- Detect repeated agent failures that indicate a missing skill or instruction
- Identify skills that are never used (wasted context budget)
- Flag skills that produce incorrect or outdated guidance
- Recommend new skills based on observed friction patterns

**Validation**: Given synthetic session logs with known issues, the agent produces correct add/edit/remove recommendations matching the evidence.

---

## Design Principles

1. **One knowledge base, two audiences, every supported tool.** Humans and the four supported agents (GitHub Copilot in VS Code, GitHub Copilot CLI, GitHub Copilot Cloud Agent, Claude Code) read the same Markdown. `AGENTS.md` at the repo root is the entry point and is symlinked from `.github/copilot-instructions.md` so the Copilot family resolves to the same content the other tools read directly.

2. **Capability lives in docs. Tool-specific UX is just a wrapper.** Anything an agent must know to do its job belongs in `AGENTS.md` or in a doc it links to (`CONTRIBUTING.md`, `docs/contributing/`, `docs/architecture/`). Skills and custom agents are Copilot conveniences (read by VS Code, Cloud Agent, and CLI); prompts are VS Code shortcuts. None of them are the only place a capability lives — they wrap the same knowledge that the docs hold.

3. **Every capability is backed by one primary contributor doc.** Each leaf row in the capability table above maps to a single contributor doc under `CONTRIBUTING.md` or `docs/contributing/` that walks an agent (or a human) through the capability end-to-end. That doc may link to architecture docs or sibling contributor docs for depth, but the contributor reads one entry point per capability. Parent rows (e.g., capability 1, "Build and test") are the union of their sub-capability rows. Skills, prompts, and custom agents are optional wrappers; the primary doc is mandatory. The mapping itself lives in a capability index in `docs/contributing/README.md` (created in Phase 3) and is what the docs-drift code-review instructions and the Phase 7 drift workflow consult. Architecture docs are reference material that contributor docs link to, not capability backing on their own.

4. **The system maintains itself.** The agent-ex system ships its own meta-tooling: templates, conventions, authoring skills, an "add a capability" agent mode, and CI gates that detect drift. The meta-tooling is built **first** so every later artifact (including `AGENTS.md` itself) is produced through it. Adding a new capability, repairing a stale doc, onboarding a new repo, and adapting to new AI tools or coding practices all follow the same documented workflow — there is no separate, undocumented process for evolving the system.

5. **Start minimal, add from friction.** Don't build a comprehensive library up-front. Add skills, prompts, and instructions only when real pain justifies the context cost.

6. **Skills, prompts, and custom agents encode project-specific, non-obvious knowledge.** A skill, prompt, or custom agent must satisfy ≥2 of: project-specific, multi-step/non-obvious, frequently repeated, error-prone without guidance. Standard workflows (`go test`, `npm install`) belong in docs, not in any of these wrappers.

7. **Context budget is finite.** Always-on context costs tokens before your task starts. Budgets: `AGENTS.md` ≤ 2 pages, [instruction files ~< 200 lines](https://code.claude.com/docs/en/memory#write-effective-instructions), [skill files ~< 500 lines](https://code.claude.com/docs/en/skills#add-supporting-files). When in doubt, put knowledge in a doc the agent reads on-demand.

8. **Enforce deterministically first.** Linters, CI, formatters, and link checks always catch violations; agents are probabilistic. Use deterministic enforcement as primary; use guidance for what can't be automated.

9. **Plan for obsolescence.** Stale docs and skills give outdated answers. Run a continuous review loop (the lifecycle phase of the plan) and prune aggressively.

10. **Environment-agnostic by default.** Workflows should work in dev containers, Codespaces, GHA runners, and the Cloud Agent — driven by the same version files and post-create script.

---
