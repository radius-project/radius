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

### Main Bicep publishing

**Merging the main workflow is the publishing cutover.** `build-main.yaml` publishes Radius types through `__publish-bicep-types.yaml`, with no external-main fallback. It retains the upstream snapshot/edge job graph, main queue and Helm gate. Release workflows, release tag policy, shared `publish-bicep` environment usage and approvals, and consumers remain unchanged.

The new publisher accepts protected canonical main pushes and manual main runs. Successful `only_changed=true` detection permits a skip; failed/missing detection, unexpected skips, and publication failures fail Build Summary. These main-only requirements do not add a precondition to release-tag publishing.

Read-only generation captures a native Bicep OCI tar once, using pinned tools, isolated `ociEnabled=true` configuration, and a local registry without credentials. A fresh trusted uploader validates source/run/artifact identity, native digest verification and the Bicep provider contract, then uses ORAS without executing downloaded code. Standard source/revision annotations retain provenance.

One serialized job publishes `ghcr.io/radius-project/bicep-types-radius:edge`, mirrors the same manifest digest to `biceptypes.azurecr.io/radius:latest`, and verifies both destinations. The first GHCR upload may create the package. GHCR stable `latest`, full-version/RC tags, AWS publishing and existing ACR reads are outside this main-only change. Started pairs finish or fail visibly rather than silently skipping the mirror.

The `bicep-types-radius-<run-id>.tar` snapshot and per-attempt receipts have 30-day retention. Current-main retries reuse the snapshot without rebuilding. A stale first attempt skips before mutation; a superseded retry fails closed because previous writes may be partial. Inspect both destinations and receipts before recovery from current main; missing/expired snapshots require a new approved run.

#### Before merging the publisher

1. Retain the upstream read-only snapshot architecture, downstream manual-writer restrictions and cloud credential isolation. Restrict elevated workflow definitions and production authority to trusted code, including maintained PR base branches; repository package grants are not per-workflow ACLs.
2. Allow the source repository's `GITHUB_TOKEN` to create the canonical package; the trusted uploader requests `packages: write`. No PAT or separate publisher repository is needed.
3. The caller forwards only existing repository secrets `BICEPTYPES_CLIENT_ID`, `BICEPTYPES_TENANT_ID` and `BICEPTYPES_SUBSCRIPTION_ID` to required callee declarations. Only the trusted publisher's Azure login references them. Verify the existing identity's ACR authorization and configure a federation binding scoped to this new main workflow. The current subject `repository_owner_id:93291507:repository_id:340522752:environment:publish-bicep` omits ref/workflow identity and alone cannot prove that scope. Establish the new mirror trust without changing legacy tag policy, existing issuer bindings or shared environment approval behavior. No live identity configuration is included here.
4. Before merging [#13119](https://github.com/radius-project/radius/pull/13119), separately authorize the development guard in `azure-octo/radius-publisher/.github/workflows/publish-bicep-types.yml`: reject normalized production `radius` (including omitted/default `radius`) for `source_ref: refs/heads/main` or `rel_channel: edge`/`latest`, before checkout or cloud credentials. Pause new development work and drain queued/running caller and private receiver jobs before enabling the replacement. Preserve legacy tagged channel/RC publication, explicit `test/radius`, and existing environment behavior. The AWS receiver guard alone does not stop Radius dispatches. Only after all unpublished legacy Radius/AWS plans finish, at the R3 release handoff, extend this Radius guard to all production `radius` dispatches, including old tags; keep `test/radius` intact. See the [ordered merge and writer handoff](../../contributing-releases/README.md#ordered-merge-and-writer-handoff) for both receiver guards and their distinct activation points. These private guard changes remain outstanding operational gates, not changes deployed by this PR.

#### After the first upload

The workflow verifies both uploaded digests before checking public visibility. Private/internal visibility or lookup failure fails the run but retains an `uploaded` receipt containing both digests. A package admin must use **Packages -> bicep-types-radius -> Package settings -> Change visibility -> Public**, then rerun and verify anonymous restore before switching consumers. Visibility is never changed automatically.

Run the focused checks without registry credentials:

```bash
node --test .github/scripts/bicep-types.test.mjs
actionlint .github/workflows/build-main.yaml .github/workflows/__publish-bicep-types.yaml
```

### GHCR consumer cutover

Active Radius configs read `ghcr.io/radius-project/bicep-types-radius` and `ghcr.io/radius-project/bicep-types-aws` with `experimentalFeaturesEnabled.ociEnabled=true`. Development uses `edge`, not the former ACR development `latest`; release channels, full version/RC tags and digest pins retain their meaning. New CLI scaffolds and Bicep image configs use the same defaults; existing user configs are not rewritten. A new CLI release must ship these defaults; already released binaries do not change.

Keep the consumer cutover draft until all gates are satisfied:

1. The upstream release stack has landed and the dependent migration stack targets `main`.
2. Radius publishers and release integration, plus the actual AWS companion publisher/release changes, are complete.
3. Both canonical GHCR packages are public and anonymous restore/build succeeds for required `edge`, channel and full version/RC references. Backfill needed old channels and RCs with their exact historical manifest digests before changing pinned consumers; do not rebuild or invent historical provenance.
4. Verify a newly built CLI's generated configs and arrange a CLI release containing the changes. Do not infer generated defaults or public package availability from a Bicep binary version alone.

Keep ACR resources and the compatibility mirror. Remaining production ACR references in main/release publisher code are mirror destinations; release parity baselines, design inventories and historical release verification are intentionally retained. Private `TEST_BICEP_TYPES_REGISTRY`/`test/radius`, `testresources`, and unrelated recipe/image registries are not part of this cutover.

The secure CI registry certificate already covers `localhost` and `radius-registry`. Native Bicep OCI publication treats loopback names as HTTP, so Bicep extension and recipe targets use `radius-registry:5000`; Docker/ORAS capture retains `localhost:5000`. The secure Kind action adds the registry's network address to CoreDNS because Docker's alias alone is not visible to pods; workloads retain the existing `global.rootCA.cert` trust mount. Exercise native `bicep publish-extension`, `rad bicep publish-extension`, restore/build and recipe publication with isolated certificates; JSON checks alone do not verify transport. A failed loopback publication does not imply loopback restore fails. Local generic OCI success is not proof that the public GHCR packages or AWS release artifacts exist.

## Verification

- The workflow you changed runs green on your pull request (open it as a draft first if you want to iterate).
- Any Make target or script called by the workflow runs successfully from the repository root.
- A fork run reaches all steps that do not require organization credentials and skips credential-dependent work with an explicit condition.
- The [github-workflows.instructions.md](../../../../.github/instructions/github-workflows.instructions.md) checklist is satisfied — especially the fork-testability and `permissions:` items.
- With Node.js, `yq` and OpenSSL installed, run `make test-publishing-isolation test-build-summary` for offline trust-boundary, producer-tag, retry, TLS configuration, and required-summary checks; no registry credentials or cloud resources are used.

## Troubleshooting

- **A workflow does not appear in the Actions tab.** Push the workflow to the branch, wait for GitHub to index it, and confirm that its trigger includes your event or a temporary `workflow_dispatch`.
- **A fork run fails on a secret.** Move the secret-dependent operation behind a repository or event condition; do not replace the missing secret with a fallback value.
- **Logic works in CI but cannot be reproduced locally.** Extract the logic into a Make target or script and keep only GitHub-specific orchestration in the YAML.
- **A reusable workflow change has unexpected callers.** Search `.github/workflows/` and the [`radius-project/.github`](https://github.com/radius-project/.github) repository for every `uses:` reference before changing its inputs, secrets, or outputs.
- **Cloud tests report a missing private-module credential.** Have a maintainer configure and verify the narrowly scoped read token above; do not restore the status App key to candidate jobs. Dispatch the cloud controller from main and supply the candidate branch through its `branch` input.
- **An upload rejects an artifact.** Inspect the validation error; rerun the producer for missing/expired inputs rather than relaxing source, digest or destination checks. Downstream-only reruns reuse the same run's validated inputs.

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make` targets that CI invokes.
- [Testing](../contributing-code-tests/README.md) — the test tiers the workflows run.
- [Documentation index](../../README.md) — every contributing doc.
