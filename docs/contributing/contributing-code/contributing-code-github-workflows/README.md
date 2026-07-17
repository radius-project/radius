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

## Repo Radius GHCR state test

[`repo-radius-state-e2e.yaml`](../../../../.github/workflows/repo-radius-state-e2e.yaml) is a standalone daily end-to-end test of the OCI state archive. It deploys an application through one ephemeral Radius control plane to a separate persistent k3d target cluster, saves Radius state to a private GHCR package, replaces the control plane, restores the saved state, and updates the existing target-cluster workload.

The scheduled run uses `ghcr.io/radius-project/radius-repo-state-e2e`. This package must be precreated as private and linked to `radius-project/radius`; a workflow in the public repository would otherwise create a public package, which Radius refuses to use for state.

Provision or verify the package with a GitHub CLI credential that has `write:packages`:

```bash
$ make install-oras
$ export PATH="$HOME/.local/bin:$PATH"
$ ./.github/scripts/precreate-repo-radius-state-package.sh \
    --package ghcr.io/radius-project/radius-repo-state-e2e \
    --source-repository https://github.com/radius-project/radius
```

The script is idempotent. It creates or verifies a harmless `bootstrap` version, private/internal visibility, and repository linkage. It never changes a public package to private.

The workflow runs daily at 04:17 UTC from the default branch. For manual testing, dispatch another ref and optionally select a different precreated package under that repository owner:

```bash
$ gh workflow run repo-radius-state-e2e.yaml \
    --ref <branch> \
    -f state_package=<package-name>
```

Each lifecycle phase is a separate workflow step. On failure, inspect the failed step first, then download the `repo-radius-state-e2e-diagnostics` artifact. Scheduled failures in the upstream repository also create an issue labeled `test-failure`; manual and fork runs do not. Successful runs record the saved/restored digest in the job summary and delete only their run-specific state version; the private package and bootstrap version remain.

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

## Troubleshooting

- **A workflow does not appear in the Actions tab.** Push the workflow to the branch, wait for GitHub to index it, and confirm that its trigger includes your event or a temporary `workflow_dispatch`.
- **A fork run fails on a secret.** Move the secret-dependent operation behind a repository or event condition; do not replace the missing secret with a fallback value.
- **Logic works in CI but cannot be reproduced locally.** Extract the logic into a Make target or script and keep only GitHub-specific orchestration in the YAML.
- **A reusable workflow change has unexpected callers.** Search `.github/workflows/` and the [`radius-project/.github`](https://github.com/radius-project/.github) repository for every `uses:` reference before changing its inputs, secrets, or outputs.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
