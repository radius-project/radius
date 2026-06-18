# Radius — Agent and Contributor Guide

This is the single entry point that orients any agent (GitHub Copilot in VS Code, Copilot Cloud Agent, Copilot CLI, Claude Code) or human contributor working in this repository. It points to directories and docs rather than listing individual files. Read the linked docs on demand for depth.

## 1. What this repo is

Radius is a cloud-native application platform that lets developers and the platform engineers who support them collaborate on delivering and managing cloud-native applications across Kubernetes, private cloud, Microsoft Azure, and Amazon Web Services. Radius is a [CNCF sandbox project](https://www.cncf.io/sandbox-projects/).

`radius-project/radius` is the main repository. It contains the Radius control plane, the `rad` CLI, the resource providers, and the API type definitions — essentially all of the Radius code and its contributor and architecture documentation.

## 2. Tech stack and repo layout

**Tech stack.** The control plane, resource providers, and CLI are written in Go. API types are authored in [TypeSpec](/typespec/) and compiled to Swagger/OpenAPI, which generates Go client and server code. Deployments use Bicep and Terraform recipes; packaging uses Helm charts and Docker images. The build and test tooling is GNU Make (split into includes under [build/](/build/)) plus Bash scripts.

**Repo layout.** [cmd/](/cmd/) holds the executable entry points (`applications-rp`, `dynamic-rp`, `ucpd`, `controller`, `rad`, and others). [pkg/](/pkg/) holds the core library packages used by those binaries. [typespec/](/typespec/) and [swagger/](/swagger/) define and generate the API surface. [bicep-tools/](/bicep-tools/) builds Radius Bicep extensions. [deploy/](/deploy/) holds the Helm chart and install scripts. [test/](/test/) holds functional and integration tests. [docs/](/docs/) holds contributor and architecture documentation. [.github/](/.github/) holds CI workflows plus the agent assets described below.

## 3. How to build and test

Start with [CONTRIBUTING.md](/CONTRIBUTING.md), which links to the full how-to documentation under [docs/contributing/](/docs/contributing/): [prerequisites](/docs/contributing/contributing-code/contributing-code-prerequisites/), [building the repo](/docs/contributing/contributing-code/contributing-code-building/), and [running tests](/docs/contributing/contributing-code/contributing-code-tests/README.md). The repo is driven by Make — run `make help` for the available targets.

## 4. How the system works

The architecture documentation under [docs/architecture/](/docs/architecture/README.md) explains every major subsystem with code references and diagrams. Start with the [service interaction map](/docs/architecture/service-interaction-map.md) for how the binaries fit together, then the [shared runtime and ARM-RPC framework](/docs/architecture/shared-runtime-and-armrpc.md), [UCP](/docs/architecture/ucp.md), the [dynamic resource provider](/docs/architecture/dynamic-rp.md), and the [deployment engine](/docs/architecture/deployment-engine.md).

## 5. Conventions

Path-scoped coding conventions live in [.github/instructions/](/.github/instructions/) and are applied automatically by Copilot based on the file you are editing. They cover [Go](/.github/instructions/golang.instructions.md), [Bicep](/.github/instructions/bicep.instructions.md), [Docker](/.github/instructions/docker.instructions.md), [GitHub Workflows](/.github/instructions/github-workflows.instructions.md), [Make](/.github/instructions/make.instructions.md), [Markdown](/.github/instructions/markdown.instructions.md), [shell scripts](/.github/instructions/shell.instructions.md), and [code review](/.github/instructions/code-review.instructions.md). Follow the instruction file that matches the file type you are changing.

## 6. Copilot agent surface users

Agents running in Copilot agent surfaces (VS Code, Cloud Agent, CLI) have additional convenience wrappers over the same knowledge in the docs above:

- **Skills** in [.github/skills/](/.github/skills/) wrap multi-step Radius workflows such as building the CLI, building and pushing images, installing from custom images, code review, and documenting architecture.
- **Custom agents** in [.github/agents/](/.github/agents/) provide specialized modes such as issue investigation.

VS Code users additionally have slash-command **prompts** in [.github/prompts/](/.github/prompts/).

## 7. How to contribute

Read [CONTRIBUTING.md](/CONTRIBUTING.md) for the contribution process, the Developer Certificate of Origin sign-off requirement, and links to the issue and pull-request guides. New contributors should follow the [first commit walkthrough](/docs/contributing/contributing-code/contributing-code-first-commit/).
