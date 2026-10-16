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

### Go module downloads

After setting up Go, use the local `.github/actions/go-mod-download` composite action before compiling Go. It runs `go mod download` with HTTP/2 disabled and up to five attempts, waiting 5, 10, 20, and 40 seconds between attempts. Set `working-directory` when the source checkout is not at the workspace root. The `module-directories` input defaults to `.` and accepts one module directory per line, relative to `working-directory`.

Jobs that run `make build`, `make docker-build`, or `make docker-multi-arch-push` must download the independent test modules as well as the root module:

```yaml
- name: Download Go modules
  uses: ./.github/actions/go-mod-download
  with:
    module-directories: |
      .
      test/magpiego
      test/testrp
```

Jobs that compile only the root module, including the current GoReleaser builds, can keep the default. Module downloads do not cover unrelated `go install` commands outside these modules.

### Docker image pulls

The `.github/actions/create-local-registry` composite action uses a cached `registry:2` image when available. Otherwise, it pulls the image with up to five attempts. Each pull has a 60-second timeout. The waits start at 5, 10, 20, and 40 seconds, with a random addition of up to the same delay to spread retries across jobs. Known permanent errors, such as authentication failures and missing manifests, stop without retry. The script preserves Docker's final exit code and output. The pull step has a 10-minute timeout.

### GitHub Packages cleanup

The scheduled `purge-old-images.yaml` workflow uses `.github/scripts/purge-old-images.sh` to remove old `dev/*` package versions with `pr-*` tags. Package and version list requests use the shared bounded HTTP retry policy. Each delete is attempted once, so a transient read failure does not require retrying successful mutations. Authentication, authorization, and other permanent errors fail at once. Run `bash .github/scripts/purge-old-images_test.sh` to check read retries, filtering, and single-attempt deletes without GitHub access.

### Keep change detection separate from executable helper code

[`__changes.yml`](../../../../.github/workflows/__changes.yml) first checks out the caller-selected repository and ref to inspect changes as data. Its Filter step must finish in that checkout at the workspace root. For `pull_request_target` and `workflow_run`, a second checkout then restores `.github/scripts/changes.mjs` from `github.repository` at `github.sha` before Set result imports it. Do not select this executable helper using the comparison inputs or a PR/workflow-run head SHA. A failed trusted checkout or import must fail the job without falling back to PR content.

Before Filter, the workflow checks that the comparison base commit exists locally. It uses `base_sha` when supplied, otherwise the PR or merge-group base SHA. If missing, it fetches that commit and its history from `github.repository` using a command-scoped read token, without changing the checked-out head or persisting credentials. A full-history fork checkout can still lack newer upstream commits; increasing fetch depth does not fix that. A failed base fetch must fail the job rather than continue with an incomplete comparison.

The comparison checkout's `allow-unsafe-pr-checkout: true` permits fetching fork content for inspection; it does not make that content safe to execute. Do not insert repository scripts, local actions, or package installation between comparison checkout and trusted restoration. Ordinary `pull_request` and `merge_group` calls retain their existing behavior. The `files`, `ref`, `repository`, `base_sha`, and string-valued `only_changed` interface is unchanged: PR and merge-group events use the filter result, other events with a base SHA use it, and events without either return `"false"` so downstream work runs.

Run `make test-workflow-changes` for the helper and workflow wiring tests. They cover output/error behavior, import fixtures, and the actual base-fetch command against local Git repositories, including a full-history fork missing its upstream base, an already-present base, and an unavailable base. They do not run the live checkout action.

## Verification

- The workflow you changed runs green on your pull request (open it as a draft first if you want to iterate).
- Any Make target or script called by the workflow runs successfully from the repository root.
- A fork run reaches all steps that do not require organization credentials and skips credential-dependent work with an explicit condition.
- The [github-workflows.instructions.md](../../../../.github/instructions/github-workflows.instructions.md) checklist is satisfied — especially the fork-testability and `permissions:` items.
- Run `bash .github/actions/go-mod-download/test.sh` for changes to the module download action. It checks retries, exponential backoff, failure propagation, directory handling, and input validation without network access. The unit-test workflow also exercises the action on Ubuntu and Windows.
- Run `bash .github/actions/create-local-registry/test.sh` for changes to the image pull retry. It checks cached images, retry limits, random-delay bounds, timeout settings, permanent errors, and exit codes with fake commands, so it needs no network.
- Run `bash .github/scripts/purge-old-images_test.sh` for changes to package cleanup. It checks transient read retries, package and tag filtering, and single-attempt deletes with fake HTTP responses.

## Troubleshooting

- **A workflow does not appear in the Actions tab.** Push the workflow to the branch, wait for GitHub to index it, and confirm that its trigger includes your event or a temporary `workflow_dispatch`.
- **A fork run fails on a secret.** Move the secret-dependent operation behind a repository or event condition; do not replace the missing secret with a fallback value.
- **Logic works in CI but cannot be reproduced locally.** Extract the logic into a Make target or script and keep only GitHub-specific orchestration in the YAML.
- **A reusable workflow change has unexpected callers.** Search `.github/workflows/` and the [`radius-project/.github`](https://github.com/radius-project/.github) repository for every `uses:` reference before changing its inputs, secrets, or outputs.
- **Change detection fails after Filter.** Inspect the trusted checkout and import failure. Do not bypass the failed step or import the helper from the PR to recover.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
