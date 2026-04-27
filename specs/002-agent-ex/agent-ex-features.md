<!-- markdownlint-disable MD060 -->
# Agent Ex — Agent Experience Features

**Vision**: Humans and AI agents collaborate as peers across all Radius repositories. Every capability is powered by the same knowledge base — humans read the contributing docs; agents consume the same knowledge through skills and instructions.

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

- [ ] A contributor to any repo can ask Copilot "how do I build this?" and get a correct answer
- [ ] An agent can build, test, and submit a PR for a code change in any repo
- [ ] Every high-value developer workflow has a skill backed by a contributing doc
- [ ] Contributing docs are accurate and verified against current code
- [ ] Cloud agent can bootstrap and work in any repo out of the box
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
| 5 | [**Improve agent effectiveness**](#5-continuously-improve-agent-effectiveness) | Automated review of session logs to curate skills and instructions |

### 1. Build and test features and bug fixes

An agent can pick up an issue, build the project, implement a change, test it, and submit a pull request — guided by the same docs a human contributor would follow. This is the highest-value capability and spans many sub-scenarios below.

#### 1.1 Set up a development environment

An agent (local or cloud) can bootstrap a working environment in any Radius repo without trial-and-error tool installation. Dev containers and [`copilot-setup-steps.yml`](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/customize-the-agent-environment#customizing-copilots-development-environment-with-copilot-setup-steps) will give agents the same turnkey setup humans get. These do not exist yet — see Phase 0 in [agent-ex-plan.md](agent-ex-plan.md).

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

### 5. Continuously improve agent effectiveness

An automated agent periodically reviews agent session logs to assess the effectiveness of skills, instructions, and agent definitions. It identifies what's working, what's failing, and what's missing — then recommends additions, edits, or removals.

**Sub-scenarios**:

- Detect repeated agent failures that indicate a missing skill or instruction
- Identify skills that are never used (wasted context budget)
- Flag skills that produce incorrect or outdated guidance
- Recommend new skills based on observed friction patterns

**Validation**: Given synthetic session logs with known issues, the agent produces correct add/edit/remove recommendations matching the evidence.

---

## Design Principles

1. **Start minimal, add from friction.** Don't build a comprehensive library up-front. Start with the minimum, observe what agents get wrong, add skills only when real pain justifies the context cost.

2. **Documentation is the durable foundation.** Skills are ephemeral agent interfaces; contributing docs are the durable knowledge base. Invest in docs first; skills are thin wrappers on top.

3. **Skills encode project-specific, non-obvious knowledge.** Skills are valuable when they encode knowledge the model cannot infer: custom pipelines, internal conventions, cross-repo coordination. They are low-value for standard workflows (`go test`, `npm install`). A skill must satisfy ≥2 of: project-specific, multi-step/non-obvious, frequently repeated, error-prone without guidance.

4. **Context budget is finite.** Always-on context costs tokens before your task starts. Targets: [`copilot-instructions.md` < 2 pages](https://docs.github.com/en/copilot/customizing-copilot/adding-repository-custom-instructions-for-github-copilot#asking-copilot-cloud-agent-to-generate-a-copilot-instructionsmd-file), [instruction files ~< 200 lines](https://code.claude.com/docs/en/memory#write-effective-instructions), [skill files ~< 500 lines](https://code.claude.com/docs/en/skills#add-supporting-files). When in doubt, put knowledge in a doc the agent reads on-demand.

5. **Enforce deterministically first.** Linters, CI, and formatters always catch violations; skills are probabilistic. Use deterministic enforcement as primary; use skills for guidance that can't be automated.

6. **Dual-audience authoring.** One source of truth, two interfaces. Skills reference contributing docs; they don't duplicate them.

7. **Plan for obsolescence.** Review quarterly; prune aggressively. Stale skills give outdated guidance and waste context.

8. **Environment-agnostic by default.** Skills should work in dev containers, Codespaces, and cloud agent alike.

---

