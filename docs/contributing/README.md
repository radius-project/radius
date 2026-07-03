# Contributing documentation

This folder holds the how-to knowledge for contributing to Radius. The documents below are the conventions and authoring guides for the **Agent Ex** system — the shared knowledge base that humans read directly and every supported AI agent (GitHub Copilot in VS Code, Copilot CLI, Copilot Cloud Agent, and Claude Code) reads through a single entry point. See [specs/002-agent-ex/agent-ex-plan.md](../../specs/002-agent-ex/agent-ex-plan.md) for the full plan.

## Agent Ex conventions and authoring guides

- **[contributing-agent-assets.md](./contributing-agent-assets.md)** — the conventions reference: file-strategy rule, file-size budgets, CI gates, naming conventions, and a template for each agent asset type.
- **[authoring-contributing-docs.md](./authoring-contributing-docs.md)** — the standard formats for contributing and architecture docs, with one annotated example of each.
- **[extending-agent-ex.md](./extending-agent-ex.md)** — the "add a capability" decision tree, the live files to update, and the repo-onboarding checklist.

## Documentation index

Every contributing doc, grouped by topic. This is the single index that [AGENTS.md](../../AGENTS.md) and [CONTRIBUTING.md](../../CONTRIBUTING.md) point to — it replaces the older competing link lists.

### Authoring docs & Agent Ex

- [contributing-agent-assets.md](./contributing-agent-assets.md) — conventions reference for every agent asset type.
- [authoring-contributing-docs.md](./authoring-contributing-docs.md) — the standard contributing/architecture doc formats.
- [extending-agent-ex.md](./extending-agent-ex.md) — the "add a capability" decision tree and repo-onboarding checklist.

### Getting started

- [Prerequisites / dev environment](./contributing-code/contributing-code-prerequisites/README.md) — install every tool and bootstrap the repo.
- [First commit walkthrough](./contributing-code/contributing-code-first-commit/README.md) — a guided, end-to-end tutorial for your first change.
- [Working with forks](./contributing-code/contributing-code-forks/index.md) — fork setup and staying in sync with upstream.
- [Code organization](./contributing-code/contributing-code-organization/README.md) — how the repository tree is laid out.
- [Writing Go code](./contributing-code/contributing-code-writing/README.md) — Radius Go conventions.

### Building & running locally

- [Building the repo](./contributing-code/contributing-code-building/README.md) — `make build`, `make generate`, and related targets.
- [Running & debugging the control plane locally](./contributing-code/contributing-code-debugging/radius-os-processes-debugging.md) — run the Radius OS processes and attach a debugger.
- [Running the control plane](./contributing-code/contributing-code-control-plane/README.md) — control-plane run overview.
- [Generating & installing a custom build](./contributing-code/contributing-code-control-plane/generating-and-installing-custom-build.md)
- [Control-plane configuration & settings](./contributing-code/contributing-code-control-plane/configSettings.md) — config schema reference.
- [Control-plane logging](./contributing-code/contributing-code-control-plane/logging.md)
- [Troubleshooting installation](./contributing-code/contributing-code-control-plane/troubleshooting-installation.md)

### Testing

- [Test matrix overview](./contributing-code/contributing-code-tests/README.md) — every test tier and its command.
- [Running functional tests](./contributing-code/contributing-code-tests/running-functional-tests.md)
- [Local test iteration](./contributing-code/contributing-code-tests/testing-local.md)
- [Writing functional tests](./contributing-code/contributing-code-tests/writing-functional-tests.md)
- [Test naming conventions](./contributing-code/contributing-code-tests/tests-naming-conventions.md)
- [Test logging](./contributing-code/contributing-code-tests/tests-logging.md)
- [Pushing test images to GHCR](./contributing-code/contributing-code-tests/tests-images-pushtoghcr.md)

### Schema & API

- [Schema changes (TypeSpec → Swagger → Go)](./contributing-code/contributing-code-schema-changes/README.md)
- [Bicep types migration guide](./bicep-types-migration-guide.md)

### CLI

- [Developing the `rad` CLI](./contributing-code/contributing-code-cli/README.md)

### CI, containers & build automation

- [GitHub Actions workflows](./contributing-code/contributing-code-github-workflows/README.md)
- [Dockerfiles](./contributing-code/contributing-code-dockerfiles/README.md)
- [Bicep files](./contributing-code/contributing-code-bicep/README.md)
- [Shell scripts & Makefiles](./contributing-code/contributing-code-shell-and-make/README.md)

### Pull requests & code review

- [Creating a pull request](./contributing-pull-requests/README.md)
- [Reviewing code](./contributing-code/contributing-code-reviewing/README.md)

### Process & reference

- [Investigating issues](./contributing-issues/README.md)
- [Triage process](./triage/triage-process.md)
- [Releases](./contributing-releases/README.md)
- [Design notes](./contributing-code/contributing-code-design/README.md)

## Capability index

Maps each capability in [agent-ex-features.md](../../specs/002-agent-ex/agent-ex-features.md#capabilities) that this repository owns to its single primary backing doc. Capabilities owned by satellite repos (1.9 resource types, 1.10 dashboard, 1.13 AWS Bicep types) are out of scope here. Parent rows link to the [documentation index](#documentation-index) above rather than a single doc.

| Capability                                | Primary backing doc                                                                                              |
|-------------------------------------------|------------------------------------------------------------------------------------------------------------------|
| 1 Build and test                          | [Documentation index](#documentation-index)                                                                      |
| 1.1 Set up a dev environment              | [contributing-code-prerequisites/README.md](./contributing-code/contributing-code-prerequisites/README.md)       |
| 1.2 Write Go code                         | [contributing-code-writing/README.md](./contributing-code/contributing-code-writing/README.md)                   |
| 1.3 Schema changes                        | [contributing-code-schema-changes/README.md](./contributing-code/contributing-code-schema-changes/README.md)     |
| 1.4 CLI commands                          | [contributing-code-cli/README.md](./contributing-code/contributing-code-cli/README.md)                           |
| 1.5 GitHub workflows                      | [contributing-code-github-workflows/README.md](./contributing-code/contributing-code-github-workflows/README.md) |
| 1.6 Dockerfiles                           | [contributing-code-dockerfiles/README.md](./contributing-code/contributing-code-dockerfiles/README.md)           |
| 1.7 Bicep files                           | [contributing-code-bicep/README.md](./contributing-code/contributing-code-bicep/README.md)                       |
| 1.8 Shell scripts & Makefiles             | [contributing-code-shell-and-make/README.md](./contributing-code/contributing-code-shell-and-make/README.md)     |
| 1.11 Documentation                        | [authoring-contributing-docs.md](./authoring-contributing-docs.md)                                               |
| 1.12 Pull requests                        | [contributing-pull-requests/README.md](./contributing-pull-requests/README.md)                                   |
| 2 Code review                             | [contributing-code-reviewing/README.md](./contributing-code/contributing-code-reviewing/README.md)               |
| 3 Investigate issues                      | [contributing-issues/README.md](./contributing-issues/README.md)                                                 |
| 5 Author and evolve docs and capabilities | [Documentation index](#documentation-index)                                                                      |
| 5.1 Author a new doc                      | [authoring-contributing-docs.md](./authoring-contributing-docs.md)                                               |
| 5.2 Repair drift                          | [extending-agent-ex.md](./extending-agent-ex.md)                                                                 |
| 5.3 Add a capability                      | [extending-agent-ex.md](./extending-agent-ex.md)                                                                 |
