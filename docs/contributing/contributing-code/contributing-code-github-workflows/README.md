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

### Manual tests with registry write access

`long-running-azure` publishes Bicep test recipes to GHCR, and `repo-radius-state-e2e` writes and deletes GHCR state artifacts. Both require `packages: write`; a test package name does not limit the token to that package. In the canonical repository, scheduled and manual runs of these workflows require protected main so arbitrary dispatched branches do not execute with that authority. Fork-local manual state tests remain available against the fork's own package.

These restrictions do not change the release pipelines, release tag requirements, snapshot artifacts, or main CLI `edge` publication. Cloud functional-test credential isolation is separate. Workflow conditions supplement, rather than replace, repository protections and registry authorization policies.

### Cloud publishing credential isolation

Cloud builds have read-only tokens and generate images, recipes and types in a TLS-local registry, then hand off a raw OCI archive through native Actions artifacts. Fresh trusted jobs check the exact run/source/artifact identity and fixed test destinations, reject unsupported OCI content, and use ORAS to copy data without executing candidate code. Images/recipes retain `ghcr.io/radius-project/dev` paths; test types remain in `crradfunctest1b2s.azurecr.io/test/radius`.

The build owns the canonical `test-<UNIQUE_ID>-<run_id>-<producing_attempt>` tag, where `UNIQUE_ID` is `func` followed by ten lowercase hexadecimal characters. It exports `REL_VERSION` for both upload jobs and all test image/recipe consumers. Validators bind this tag to the trusted unique ID and authenticated producing run/attempt, including archive metadata and publication receipts. Rerunning only uploads or tests reuses the successful producer's tag; rebuilding generates a new attempt tag even if setup is not rerun. Publication receipts and the final report identify the actual producer tag. Old `pr-func...` artifacts are rejected: start a fresh workflow run rather than retrying old-format work.

**Before merging cloud isolation:** configure `FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN` as a fine-grained token with only **Contents: read** and **Metadata: read** on `radius-project/terraform-private-modules`; verify its scope and read access. Missing configuration fails early. The broad functional-test App key stays on trusted controller/reporting runners, never candidate tests. Per-suite results keep the existing checks/PR-scoped `GITHUB_TOKEN`, with contents and packages read-only. Cloud tests retain contributor approval and necessary Test-tenant identities. Verify the full new controller path **after merge from protected main**, or in an explicitly approved isolated environment, not by privileged dispatch from a PR ref.

Cloud isolation does not alter upstream release pipelines, tag requirements, artifact or alias behavior, or the ordinary snapshot build architecture. It adds no protected-release-tag precondition. Before the direct main/edge Bicep publisher's registry cutover, retain the separate manual-writer restrictions and cloud credential boundary, restrict elevated workflow definitions and production credentials to trusted publishers, and ensure Test identities have no production rights. GHCR grants are repository-scoped; read-only defaults, `dev/` prefixes and environment names are not package ACLs. These workflows do not configure secrets, registries, or policies, and there is no publisher enablement flag to defer the prerequisites until after cutover.

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

The scheduled `purge-old-images.yaml` workflow uses `.github/scripts/purge-old-images.sh` to remove old `dev/*` package versions with `pr-*` tags. Package and version list requests use the shared bounded HTTP retry policy. Deletes use the same selective retry policy; HTTP 404 is accepted because a previous attempt may have completed before its response failed. HTTP 400 is skipped only when the response is a single valid JSON object whose `.message` exactly equals `Publicly visible package versions with more than 5000 downloads cannot be deleted. Contact GitHub support for further assistance.` or `You cannot delete the last tagged version of a package. You must delete the package instead.` The script warns that the version is protected, keeps the version and package, does not count it as deleted or already deleted, and continues with later versions and packages. All other HTTP 400 responses, plus HTTP 409 and 422, fail the run and log the GitHub error message. HTTP 403 with `Retry-After` is retried within the existing 300-second request budget, unless `x-ratelimit-remaining` is 0. If the wait exceeds that budget, retries are exhausted, or the primary rate limit is reached, the script writes a warning and stops without failing; the next scheduled run starts cleanup again. Authentication, authorization, and other permanent errors fail at once. Error messages from the GitHub API are written to the log. Manual workflow runs default to dry-run mode so contributors can verify the selected versions before enabling deletion. Run `bash .github/scripts/purge-old-images_test.sh` to check retries, filtering, pagination, and deletes without GitHub access.

### Keep change detection separate from executable helper code

[`__changes.yml`](../../../../.github/workflows/__changes.yml) first checks out the caller-selected repository and ref to inspect changes as data. Its Filter step must finish in that checkout at the workspace root. For `pull_request_target` and `workflow_run`, a second checkout then restores `.github/scripts/changes.mjs` from `github.repository` at `github.sha` before Set result imports it. Do not select this executable helper using the comparison inputs or a PR/workflow-run head SHA. A failed trusted checkout or import must fail the job without falling back to PR content.

Before Filter, the workflow checks that the comparison base commit exists locally. It uses `base_sha` when supplied, otherwise the PR or merge-group base SHA. If missing, it fetches that commit and its history from `github.repository` using a command-scoped read token, without changing the checked-out head or persisting credentials. A full-history fork checkout can still lack newer upstream commits; increasing fetch depth does not fix that. A failed base fetch must fail the job rather than continue with an incomplete comparison.

The comparison checkout's `allow-unsafe-pr-checkout: true` permits fetching fork content for inspection; it does not make that content safe to execute. Do not insert repository scripts, local actions, or package installation between comparison checkout and trusted restoration. Ordinary `pull_request` and `merge_group` calls retain their existing behavior. The `files`, `ref`, `repository`, `base_sha`, and string-valued `only_changed` interface is unchanged: PR and merge-group events use the filter result, other events with a base SHA use it, and events without either return `"false"` so downstream work runs.

Run `make test-workflow-changes` for the helper and workflow wiring tests. They cover output/error behavior, release-metadata and mixed-input file sets, required-check failure propagation, import fixtures, and the actual base-fetch command against local Git repositories, including a full-history fork missing its upstream base, an already-present base, and an unavailable base. They do not run the live checkout action.

### Filter release metadata without skipping release validation

The shared skip allowlist includes `CHANGELOG.md`, `versions.yaml`, `docs/release-notes/v*.md`, and `.github/release-plans/v*.yaml`. All changed paths must match the allowlist before consumers skip expensive work, including deletions, type changes, and both sides of renames. The existing changed-files action counts all paths and matching paths; its `only_changed` output is insufficient because it omits deletions. Metadata mixed with source, build inputs, tests, or any workflow still runs the full work, including workflows that would otherwise match the existing skip patterns. Selection depends on paths, not a PR's title, author, labels, or number.

For matching-only release-metadata PRs and merge groups, snapshot/CLI/image builds, full Go and Windows CLI tests, cloud and non-cloud functional tests, Bicep validation, and reference-doc generation skip. The required `Lint` job instead runs `test-workflow-changes`, `test-release-version-format`, `test-changelog-range`, `test-changelog-config`, `test-prepare-release`, and `test-release-plan`. It does not compile Go, install Helm, or regenerate code. The shared `release_metadata` output limits this path to release-related changes: ordinary documentation, configuration, and publisher-only changes retain their existing skip behavior, including skipping `Lint`. Run the focused targets locally with the [basic prerequisites](../contributing-code-prerequisites/), Node.js 24, yq, jq, and git-cliff available. The `make install-yq install-jq install-git-cliff` targets install the pinned tools.

The trusted `release-plan.yaml` workflow keeps the actual changed-file list separate from the files the validator reads. Both PR and merge-group snapshots download the changed files plus `CHANGELOG.md`, once each, from the captured PR head SHA. An RC leaves the root changelog unchanged, but validation still compares its contents. Do not add that unchanged file to the changed-file list or fetch it from current `main`; missing or unreadable snapshot inputs must fail validation.

Run `bash .github/scripts/validate-release-plan_test.sh --snapshots-only` to exercise both production snapshot scripts with mocked GitHub responses, fresh head directories, and the real validator. These regressions also run through `make test-release-plan`; the full suite additionally checks both snapshots against real release preparation.

`Build Check`, `Lint`, and the app-owned `Functional Test Run` still complete for filtered changes and fail if change detection fails. An absent or invalid path count is an error, not permission to skip; an empty diff runs the full work. Cloud status aggregation also includes authorization, setup, and skip-report failures. The independent trusted `release-plan.yaml` validator, PR policy checks, and tag-build, staged-installation, approval, and publication gates remain unchanged. Pushes, schedules, and dispatches without a comparison base still run the full work.

After a filter change merges, refresh an existing generated release PR through Prepare Release rather than rerunning an old workflow attempt. PR workflows need the updated revision, and cloud tests use the trusted base revision. A plan whose product source is `HEAD` must be regenerated when `main` advances; the release-plan validator deliberately rejects the stale source pin.

## Verification

- The workflow you changed runs green on your pull request (open it as a draft first if you want to iterate).
- Any Make target or script called by the workflow runs successfully from the repository root.
- A fork run reaches all steps that do not require organization credentials and skips credential-dependent work with an explicit condition.
- The [github-workflows.instructions.md](../../../../.github/instructions/github-workflows.instructions.md) checklist is satisfied — especially the fork-testability and `permissions:` items.
- Run `bash .github/actions/go-mod-download/test.sh` for changes to the module download action. It checks retries, exponential backoff, failure propagation, directory handling, and input validation without network access. The unit-test workflow also exercises the action on Ubuntu and Windows.
- Run `bash .github/actions/create-local-registry/test.sh` for changes to the image pull retry. It checks cached images, retry limits, random-delay bounds, timeout settings, permanent errors, and exit codes with fake commands, so it needs no network.
- Run `bash .github/scripts/purge-old-images_test.sh` for changes to package cleanup. It checks transient read and delete retries, package and tag filtering, pagination, protected-version skips, rejected versions, primary and secondary rate limits, and permanent failure handling with fake HTTP responses.
- With Node.js, `yq` and OpenSSL installed, run `make test-publishing-isolation test-build-summary` for offline trust-boundary, producer-tag, retry, TLS configuration, and required-summary checks; no registry credentials or cloud resources are used.

## Troubleshooting

- **A workflow does not appear in the Actions tab.** Push the workflow to the branch, wait for GitHub to index it, and confirm that its trigger includes your event or a temporary `workflow_dispatch`.
- **A fork run fails on a secret.** Move the secret-dependent operation behind a repository or event condition; do not replace the missing secret with a fallback value.
- **Logic works in CI but cannot be reproduced locally.** Extract the logic into a Make target or script and keep only GitHub-specific orchestration in the YAML.
- **A reusable workflow change has unexpected callers.** Search `.github/workflows/` and the [`radius-project/.github`](https://github.com/radius-project/.github) repository for every `uses:` reference before changing its inputs, secrets, or outputs.
- **Change detection fails after Filter.** Inspect the trusted checkout and import failure. Do not bypass the failed step or import the helper from the PR to recover.
- **Cloud tests report a missing private-module credential.** Have a maintainer configure and verify the narrowly scoped read token above; do not restore the status App key to candidate jobs. Dispatch the cloud controller from main and supply the candidate branch through its `branch` input.
- **An upload rejects an artifact.** Inspect the validation error; rerun the producer for missing/expired inputs rather than relaxing source, digest or destination checks. Downstream-only reruns reuse the same run's validated inputs.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
