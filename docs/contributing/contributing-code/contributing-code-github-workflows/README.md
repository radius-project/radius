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

## Steps

1. Find the workflow under `.github/workflows/` and identify any reusable workflows, Make targets, or scripts it calls.
2. Put multi-step build, test, or deployment logic in a Make target or script that contributors can run locally. Keep workflow YAML focused on triggers, permissions, runner setup, identity, and orchestration.
3. During development, add or enable `workflow_dispatch` when a safe manual trigger is needed. Do not merge a manual trigger for a workflow that must only run from another event.
4. Gate jobs that require organization secrets or infrastructure so the build and validation portions still run from a fork.
5. Set the smallest explicit `permissions:` block at the workflow or job level.
6. Open the pull request as a draft and run the workflow from your branch. Confirm its trigger, job graph, artifacts, and failure behavior before marking the pull request ready.

### Main Bicep publishing

[`build-main.yaml`](../../../../.github/workflows/build-main.yaml) can generate and publish Radius types directly through [`__publish-bicep-types.yaml`](../../../../.github/workflows/__publish-bicep-types.yaml). This path is **disabled by default**: only the exact repository variable value `BICEP_GHCR_PUBLISH_ENABLED=true` selects it. Unset, `false`, and invalid values retain the existing external ACR publisher; an invalid value produces a notice. The two main publishers are mutually exclusive. Release-tag publishing through `build-release.yaml` and `__build-bicep-types.yaml` remains unchanged.

Direct publication accepts only protected canonical main pushes and approved manual main runs. A successful `only_changed=true` detection intentionally skips publication; failed, missing, or malformed change detection cannot yield a successful publishing skip. Build Summary requires the selected publisher to succeed and checks the direct publication status.

The read-only generation job captures type index/data once as a native Bicep OCI layout and source identity. Bicep runs in an isolated directory with explicit `experimentalFeaturesEnabled.ociEnabled=true`, empty registry credentials, and trusted `localhost`. A fresh publishing job validates the immutable Actions artifact ID, archive digest, owning source run, and every layout blob before copying data. It rejects scripts, executable extension layers, links, foreign URLs, and unsafe archives. It never executes bundle-supplied code or reruns the generator.

The publisher stamps only standard OCI source/revision annotations, preserves the native Bicep config/type layer, and updates `ghcr.io/radius-project/bicep-types-radius:edge`. It copies **that same manifest digest** to `biceptypes.azurecr.io/radius:latest` and verifies both copies. GHCR `latest` is stable and is never updated by this main path. Full-version/RC publication and stable alias promotion belong to the later release integration.

Writers are serialized without cancelling an in-flight pair. A snapshot superseded before publication is recorded as `superseded` without moving either tag. Once a pair starts, a newer main commit does not interrupt the mirror: both destinations finish from the captured digest, or the run fails with a partial receipt. A superseded retry cannot roll a newer pair back or hide unknown prior progress.

The `bicep-types-radius-<run-id>` bundle is reused on retry, with its original generation attempt. Per-attempt `bicep-types-radius-receipt-<run-id>-<attempt>` artifacts record source repository/ref/commit/workflow/run, bundle ID/archive digest, final manifest digest, destination verification, status, and progress. Both have 30-day retention. A missing/expired retry bundle requires a new approved main run; failed API calls are not treated as absence. Missing prior publication progress is an error on a superseded retry. Inspect the receipt and current destinations before recovery; do not regenerate or force stable tags.

#### Activation prerequisites

Keep the flag disabled until maintainers have verified all of the following. This code does not provision or modify these settings:

1. The credential-isolation change is merged and verified, including policy preventing PR-controlled workflow definitions or PR-executing jobs from obtaining production package-write tokens, publisher App credentials, or production Azure identity. Apply equivalent isolation or restrict privileged paths on every maintained release/features branch as well: merging main does not update their older `pull_request_target` definitions. Package access is repository-scoped, not a per-workflow permission boundary; merging the isolation PR alone is not authorization to enable this publisher.
2. The canonical GHCR package is provisioned as public with the intended source-repository Actions access and anonymous restore validated. The publisher fails if package metadata cannot be read or visibility is not public.
3. The shared `publish-bicep` environment has required approval, restrictions to protected main and approved legacy release refs, and no bypass. The environment has no protection rules in the audited configuration; its name alone is not protection. Protect source main and the publishing workflow as well. The retained legacy tag caller is only `v*` gated and executes tag-controlled Python/monitor code with the publisher App key, so approved-tag creation, workflow-definition controls, and App-key isolation remain required until release integration replaces it. Do not mistake the edge-only guard for protection of that legacy route.
4. Environment-scoped `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID`, and `BICEPTYPES_SUBSCRIPTION_ID` identify a least-privilege ACR data-plane publisher. Its federated trust must bind the canonical repository, protected main ref, and trusted `job_workflow_ref` for `__publish-bicep-types.yaml`, as well as the environment and Azure audience. An environment-only OIDC subject is insufficient.
5. The old external **development** Radius Bicep writer is disabled and in-flight main dispatches are drained before enabling the handoff. Keep the existing release-tag writer until release integration replaces it. A rollback must likewise drain active writers before changing the flag.

#### Local verification

Offline contract and workflow tests need Python and Node.js but no credentials:

```bash
python3 -B .github/scripts/bicep-types_test.py
node --test .github/scripts/bicep-types-workflows.test.mjs
actionlint .github/workflows/build-main.yaml .github/workflows/__publish-bicep-types.yaml
```

To verify actual generation, native packaging, exact-digest copy/retry, and anonymous restore/compile with disposable loopback registries:

```bash
make install-bicep install-oras
make generate-bicep-types VERSION=edge
docker pull registry@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373
python3 -B .github/scripts/bicep-types_local_test.py
```

The local test uses the pinned tools on `PATH`, isolates credentials/cache/configuration, and removes only its own containers and temporary files. Generation updates the local generated index version; do not include that test-only `edge` change in an unrelated commit.

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
