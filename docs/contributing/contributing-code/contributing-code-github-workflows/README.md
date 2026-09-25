# Contributing to GitHub Actions workflows

## Purpose

This is the primary doc for adding or changing the CI/CD workflows that build, test, and release Radius. It is reference material for anyone editing the automation under `.github/workflows/`. The detailed, rule-by-rule conventions live in the [GitHub Workflows instruction file](../../../../.github/instructions/github-workflows.instructions.md), which Copilot applies automatically to any `.github/workflows/*.yml`/`*.yaml` file you edit; this doc gives the map of where the workflows live and how to change them safely.

## Where these files live

- `.github/workflows/` — the workflow definitions that run on every push, pull request, and release.
- Reusable/shared workflows are referenced from the org-level [`radius-project/.github`](https://github.com/radius-project/.github) repository (for example the spellcheck and linter workflows), so a fix there can affect every repo.

## Conventions

Follow the [GitHub Workflows instruction file](../../../../.github/instructions/github-workflows.instructions.md). Its emphasis for Radius:

- **Fork-testability** — a workflow must be runnable from a fork without access to repository secrets; gate secret-dependent steps rather than assuming they exist.
- **Least privilege** — set explicit `permissions:` blocks; default to read-only and grant write only where needed.
- **Pin and cache** — pin action versions and cache dependencies to keep runs fast and reproducible.

### Publishing credential isolation

`build-validation.yaml` uses read-only CLI and image builds; Actions artifacts do not require `packages: write`. Main publishes CLI binaries in `__publish-cli.yaml` on fresh runners without executing the downloaded binaries. Production image/Helm publication and canonical maintenance jobs require protected main or an approved protected release tag, as appropriate. Use `build-validation` for arbitrary branch builds. `long-running-azure` and canonical `repo-radius-state-e2e` manual runs must select main; fork-local state tests remain available.

Cloud tests still require contributor authorization and retain their Test-tenant Azure/AWS identities. Their source branch is resolved to a commit before building. Candidate Makefiles, Docker builds, type generation, and `rad bicep publish` run without publishing credentials, using a job-local registry. Fresh upload jobs use the resolved protected-main uploader code and validate immutable Actions artifact IDs, producing run/repository/commit metadata, archive digests, safe paths, and bounded OCI descriptors before copying data. No candidate executables or Docker builds run in those jobs. Images and recipes keep their existing `ghcr.io/radius-project/dev` paths and `pr-func...` tags; test types stay in `crradfunctest1b2s.azurecr.io/test/radius`. Registry variables must match these allowlists. The OCI handoff permits eight images, one Radius extension, and up to 128 recipes, with a 4 GiB archive and 12 GiB expanded-data limit.

**Before merging this isolation change**, maintainers must configure `FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN` as a fine-grained token limited to **Contents: read** and **Metadata: read** on `radius-project/terraform-private-modules` only, then verify its scope and private-module read access. Do not grant access to Radius source/workflows or any other repository. Candidate tests pass this credential through the existing `GH_TOKEN` Terraform authentication path. A missing credential fails explicitly; there is no fallback to the functional-test App key. That App has registered contents/workflows write authority, so its private key and status-comment/check tokens remain only on trusted controller/reporting runners. Never pass credentials or private module contents through Actions artifacts. After merging, run the complete new cloud-test path from protected main and verify the private-module test; before merging, use only offline fixtures or an explicitly approved isolated validation environment, not a privileged workflow dispatched from an untrusted PR ref.

**Production publisher activation is a separate policy gate.** GHCR Actions package access is repository-scoped: read-only workflow defaults, `dev/` prefixes, CODEOWNERS alone, and unprotected environments are not package authorization boundaries. Before granting or enabling production extension publication, enforce restrictions that prevent non-publisher-trusted actors from creating or running elevated same-repository workflow definitions, including new workflow files. Protect main, approved release tags, and uploader code; restrict any bypass to publishing principals, using fork contributions for other actors. `github.ref_protected` records that a rule applies, not that its rules are sufficient. Verify production secret/environment and Azure OIDC trust policies reject candidate contexts, and that retained Test identities have no production ACR rights. These are live administrator prerequisites, not settings this workflow change applies; keep the companion publisher disabled until verified.

Apply the isolation contract to every maintained branch that can execute PR workflows before activation: `pull_request_target` uses the PR base branch's workflow, so changing main does not repair an older release/feature branch. Legacy Bicep dispatch through `__build-bicep-types.yaml` retains its implementation and credentials, but both main and release callers now require the publication-ref preflight. Keep its App credentials restricted to approved workflows/refs until its separate integration.

## Steps

1. Find the workflow under `.github/workflows/` and identify any reusable workflows, Make targets, or scripts it calls.
2. Put multi-step build, test, or deployment logic in a Make target or script that contributors can run locally. Keep workflow YAML focused on triggers, permissions, runner setup, identity, and orchestration.
3. During development, add or enable `workflow_dispatch` when a safe manual trigger is needed. Do not merge a manual trigger for a workflow that must only run from another event.
4. Gate jobs that require organization secrets or infrastructure so the build and validation portions still run from a fork.
5. Set the smallest explicit `permissions:` block at the workflow or job level.
6. Open the pull request as a draft and run the workflow from your branch. Confirm its trigger, job graph, artifacts, and failure behavior before marking the pull request ready.

## Verification

- The workflow you changed runs green on your pull request (open it as a draft first if you want to iterate).
- Any Make target or script called by the workflow runs successfully from the repository root.
- A fork run reaches all steps that do not require organization credentials and skips credential-dependent work with an explicit condition.
- The [github-workflows.instructions.md](../../../../.github/instructions/github-workflows.instructions.md) checklist is satisfied — especially the fork-testability and `permissions:` items.
- With Python 3, Node.js, `yq`, `oras`, and OpenSSL installed, run `make test-publishing-isolation test-build-summary` for offline permission/guard, artifact-tampering, certificate/configuration, local OCI-copy, and required-summary coverage. This does not publish registry artifacts or deploy cloud resources. Cloud summaries treat failed change detection, rejected approval, missing inputs, failed uploads, and cancelled jobs as non-success; only a successful, explicit docs-only decision permits skipping the test phases.

## Troubleshooting

- **A workflow does not appear in the Actions tab.** Push the workflow to the branch, wait for GitHub to index it, and confirm that its trigger includes your event or a temporary `workflow_dispatch`.
- **A fork run fails on a secret.** Move the secret-dependent operation behind a repository or event condition; do not replace the missing secret with a fallback value.
- **Logic works in CI but cannot be reproduced locally.** Extract the logic into a Make target or script and keep only GitHub-specific orchestration in the YAML.
- **A reusable workflow change has unexpected callers.** Search `.github/workflows/` and the [`radius-project/.github`](https://github.com/radius-project/.github) repository for every `uses:` reference before changing its inputs, secrets, or outputs.
- **Cloud tests report a missing private-module credential.** Have a maintainer configure and verify the narrowly scoped read token above; do not restore the status App key to candidate jobs. Dispatch the cloud controller from main and supply the candidate branch through its `branch` input.
- **An upload rejects an artifact or destination.** Inspect the producing run and validation error; re-run the producer for missing/expired inputs. Do not relax path, digest, source, or registry checks to make an upload pass. Downstream-only reruns can reuse validated artifacts from an earlier successful attempt of the same run.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
