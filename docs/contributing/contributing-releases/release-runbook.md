# Release runbook

## Purpose

Use this checklist to run a Radius RC, final, or patch release. The [full release guide](./README.md) covers setup and exceptional recovery. Commands below use channel `0.62` as an example; replace it with your target channel and use the version that Prepare Release generates.

## Prerequisites

- Have maintainer access, an authenticated GitHub CLI (`gh auth status`), and an available reviewer for the `release` environment.
- Complete the [one-time setup](./README.md#prerequisites), including App permissions, package access, environment protection, branch rules, and the current release pipeline on the target branch. Follow the [first-production readiness checks](./README.md#confirm-readiness-before-the-first-production-cycle); green CI alone is not that evidence.
- If resource types or recipe packs changed, [release them and merge the generated Radius update](./README.md#release-resource-type-namespaces-and-recipe-packs) first. Unchanged, already-pinned releases need no action.
- For an existing channel, label selected changes `backport release/X.Y` and **rebase-merge** their generated backports before preparation. Every `supported` version in [`versions.yaml`](../../../versions.yaml) must already have its Radius tag; finish any pending release first.

## Steps

### 1. Run Prepare Release

Choose the release type:

| Type    | When to use it                                                                                                                                   |
|---------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| `rc`    | First or subsequent candidate for a new minor release.                                                                                           |
| `final` | Promote the last RC after its docs upmerge, samples upmerge, and sample tests succeed. Do not add untested product or Deployment Engine changes. |
| `patch` | Release fixes on an existing stable channel.                                                                                                     |

Run [Prepare Release](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) from `main`:

```bash
gh workflow run prepare-release.yaml --repo radius-project/radius --ref main -f release-type=rc -f channel=0.62
```

For a final or patch, change `release-type=rc` to `release-type=final` or `release-type=patch`. Preparation computes the full version; do not choose an RC number yourself. For an existing channel, optionally select merged main PRs with `-f backport-pr-numbers="12345,12346"`, using their actual numbers.

For a first RC, preparation freezes the checked-out `main` commit. To select an earlier reachable cutoff, add `-f product-commit="<full-40-character-SHA>"`; preparation verifies that its release metadata and build tooling are compatible. Later `main` or merge-queue changes do not enter that release. This override is not available for subsequent RCs, finals, or patches.

Open the run and follow **Summary > Release pull request**. If the CLI did not return a run URL, find your invocation in that workflow's Actions page.

While the first-RC PR remains open, rerunning with the same type/channel retains its cutoff, sibling commits, release date, and curated Highlights/Upgrading. Normally omit `product-commit` on reruns; a different cutoff for the same open preparation is rejected.

### 2. Review and merge the release PR

Review the generated draft PR: version, product commit, sibling commits, selected backports, and expected outputs in `.github/release-plans/`. For RC/final notes, edit only **Highlights** and **Upgrading**; retain the image-pinning warning. Patch notes are fully generated.

Mark the PR ready, obtain approval, and **squash-merge into `main`**. A frozen first-RC plan does not need regeneration just because `main` advanced.

For an existing `release/X.Y` channel, also review and **rebase-merge the generated metadata backport**. The controller waits for this; the main PR merge alone is not enough. For a first RC, automation creates the release branch.

### 3. Approve release reconciliation

Open the automatically started [Release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml). Review its version, Radius source commit, Deployment Engine commit, and frozen sibling commits. Record the exact **version and source commit** from its summary for recovery.

In **Review deployments**, approve **Approve release reconciliation** in the `release` environment.

For a frozen first RC, the controller then creates or reuses a signed release-App commit based on the frozen product cutoff, adding only the four reviewed release metadata files, including the PM notes. The release branch, tag, builds, and artifact locks use this resolved commit, not the later `main` squash commit. Record the resolved source SHA from the summary for subsequent recovery.

Automation creates the Deployment Engine tag when missing, waits up to 15 minutes for its image while unlocked, locks the image digest, reconciles sibling tags, and creates the Radius tag last. **Do not manually create tags or dispatch individual publishers on the normal path.**

### 4. Check the build and approve stable publication

Open the [release build workflow](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) and select the run for the approved version. Confirm its source commit matches the controller summary. Wait for **Verify release manifest and staged installation** to succeed and inspect its `release-manifest.json` evidence.

- **RC:** publication continues automatically after verification.
- **Final or patch:** approve **Approve final or patch publication** in the `release` environment only for the verified version/source pair.

Automation rechecks artifacts, promotes eligible stable aliases for finals/patches, and publishes the GitHub Release. RCs do not move stable aliases.

### 5. Finish downstream work

Follow the exact downstream run and PR links in the release summary:

| Type  | Maintainer action                                                                                                                                                                                                                      |
|-------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| RC    | Review and squash-merge the docs and samples upmerge PRs into `edge`, if created. Then [resume the same release](#troubleshooting) so sample tests run. Require all three successful coordination receipts before preparing the final. |
| Final | Confirm automated docs publication, samples publication, and sample tests succeed. Also publish the final's Bicep recipes using the command below.                                                                                     |
| Patch | Confirm sample tests succeed; no docs/samples release-branch recut is needed.                                                                                                                                                          |

**Final only:** confirm `resource-types-contrib` `main` contains the intended recipe source, then run [Publish Bicep Recipes](https://github.com/radius-project/resource-types-contrib/actions/workflows/publish-bicep-recipes.yaml). Replace `0.62.0` with the actual final version **without `v`**:

```bash
gh workflow run publish-bicep-recipes.yaml --repo radius-project/resource-types-contrib --ref main -f release_version=0.62.0
```

Wait for success and record the run URL. This is a separate maintainer action, not dispatched by Radius; it publishes release-version recipe tags and moves recipe `latest` tags.

## Verification

- [ ] The release build and required downstream stages succeeded; no approval or upmerge is still pending.
- [ ] The GitHub Release is public with the correct version, and `release-manifest.json` and staged-installation evidence match the approved source.
- [ ] RC: all three coordination receipts succeeded. Final/patch: eligible aliases were promoted from verified digests. Final: Bicep recipe publication succeeded.
- [ ] Save the version, source commit, PR links, workflow URLs, and verification evidence in the Teams release thread. A public GitHub Release alone does not mean downstream work is complete.

## Troubleshooting

**Resume a failed or interrupted release:** fix the reported cause, then use [Resume Release](https://github.com/radius-project/radius/actions/workflows/resume-release.yaml) from `main`. Prefer the exact command in the failed summary. Otherwise replace both example inputs below with that summary's version and full source SHA:

```bash
gh workflow run resume-release.yaml --repo radius-project/radius --ref main -f version=v0.62.0-rc.1 -f source-commit="<source-commit-from-summary>"
```

For a frozen first RC, use the resolved release commit from the summary. Before resolution, use the merged metadata PR commit; replaying that same approval identity reuses the original resolved commit. Legacy first RCs and existing-channel releases retain their original release PR squash or metadata-backport commit. **Never substitute the raw product cutoff or current branch tip.** Resume preserves matching tags and locked digests; approvals may be required again.

**Older open first-RC preparation:** after frozen-source support is deployed, rerun preparation once with the same type/channel to migrate an unmerged `productRef: HEAD` plan. This retains its cutoff and PM curation and returns the PR to draft for renewed review. Do not migrate or reprepare a merged or tagged release; resume its existing identity.

**Dispatch outcome unknown:** inspect the correlated publisher run and destination artifacts before resuming, including after cancellation. Do not blindly redispatch; see [lost dispatch recovery](./README.md#lost-dispatch-response).

**Controller never started after merge:** use [Approve Release](https://github.com/radius-project/radius/actions/workflows/approve-release.yaml) instead, with the same version/source inputs. This starts reconciliation; it does not bypass environment approval.

**Preparation blocked:** merge missing backports or finish the preceding release, then rerun Prepare Release with the same inputs. Never edit the generated plan to bypass validation.

Never move approved tags, delete locks, overwrite published artifacts, or disable verification. Follow [break-glass recovery](./README.md#break-glass-recovery) for conflicts or fixes needed in already-tagged build code.
