# Release runbook

## Purpose

Steps to ship a Radius RC, final, or patch release. For background, setup, and recovery, see the [release guide](./README.md).

Examples use channel `0.62`. Replace it, and every `<placeholder>`, with your values.

## Before you start

- [ ] `gh auth status` succeeds and you can approve the `release` environment.
- [ ] One-time setup is done (first release only): [setup and readiness](./README.md#prerequisites).
- [ ] Every `supported` version in [`versions.yaml`](../../../versions.yaml) already has a Radius Git tag.
- [ ] For an existing channel: each fix is merged to `main`, labeled `backport release/X.Y`, and its generated backport PR is **rebase-merged**. Do the same in `bicep-types-aws`, `dashboard`, and `recipes` if they have fixes.

## Steps

### 1. Release resource types and recipe packs (only if they changed)

1. In `radius-project/resource-types-contrib`, run [Release Namespace](https://github.com/radius-project/resource-types-contrib/actions/workflows/release-namespace.yaml) from `main` for each changed namespace in [`deploy/manifest/defaults.yaml`](../../../deploy/manifest/defaults.yaml). Set `bump`; leave `prerelease_label` empty, `dry_run` and `force` off.
2. Run [Release Recipe Pack](https://github.com/radius-project/resource-types-contrib/actions/workflows/release-recipe-pack.yaml) from `main` for each changed pack, with the same settings.
3. Merge the generated `chore(resource-types-contrib): updates` PR (branch `bot/update-resource-types`) into `radius` `main`.
4. For a later RC or patch, label that PR `backport release/X.Y` and rebase-merge its backport.

### 2. Pin Radius in `bicep-types-aws` (only if it has a `radius` submodule)

1. Get the product commit: the head of Radius `release/X.Y`, or for a first RC run step 3 first and read `source.productCommit` from the release PR's plan.
2. Pin it:

   ```bash
   git clone https://github.com/radius-project/bicep-types-aws.git
   cd bicep-types-aws
   git switch -c pin-radius-0.62 origin/release/0.62
   git update-index --cacheinfo "160000,<product-commit>,radius"
   git commit -s -m "chore: pin Radius to <product-commit>"
   git push origin pin-radius-0.62
   gh pr create --repo radius-project/bicep-types-aws --base release/0.62 --fill
   ```

   If `release/0.62` does not exist, create it first: `git push origin origin/main:refs/heads/release/0.62`.
3. Merge the PR, then rerun step 3 with the same inputs.

### 3. Prepare the release

Pick `rc`, `final`, or `patch`. Before a final, the last RC's docs/samples upmerges and sample tests must be done.

```bash
gh workflow run prepare-release.yaml --repo radius-project/radius --ref main \
  -f release-type=rc -f channel=0.62
```

Optional flags:

- Existing channel: `-f backport-pr-numbers="12345,12346"`
- First RC only, earlier cutoff: `-f product-commit="<full-40-char-SHA>"`

Open the run, then **Summary > Release pull request**.

### 4. Review and merge the release PR

1. Check the version, product commit, sibling commits, and backports.
2. For RC/final notes, edit only **Highlights** and **Upgrading**. Keep the image-pinning warning. Patch notes are generated.
3. Mark ready, get approval, and **squash-merge** into `main`.
4. Existing channel only: also **rebase-merge** the generated metadata backport PR into `release/X.Y`.

### 5. Approve reconciliation

1. Open the [Release controller](https://github.com/radius-project/radius/actions/workflows/release-controller.yaml). Existing channel: use the run started by the metadata backport, showing **State: validated and ready to reconcile**.
2. Record the **version** and **source commit** from its summary.
3. Until v0.63.0 final ships, confirm the Deployment Engine commit includes azure-octo/deployment-engine#705 (`e537d7e4b`).
4. Under **Review deployments**, approve **Approve release reconciliation**.

### 6. Check the build and approve publication

1. Open the [release build](https://github.com/radius-project/radius/actions/workflows/build-release.yaml) run for the version. Its source commit must match the controller summary.
2. Wait for **Verify release manifest and staged installation** to pass.
3. **RC:** nothing to approve. **Final/patch:** approve **Approve final or patch publication** in the `release` environment.
4. Wait for **Publish sibling GitHub Releases** to pass.

### 7. Finish downstream work

| Type  | Do this                                                                                                                              |
|-------|--------------------------------------------------------------------------------------------------------------------------------------|
| RC    | Squash-merge the docs and samples upmerge PRs into `edge`, then resume the release (below) so sample tests run. Confirm all three coordination receipts succeeded. |
| Final | Confirm docs publication, samples publication, and sample tests passed, then run the Bicep recipes command below.                    |
| Patch | Confirm sample tests passed.                                                                                                         |

Resume the release (RC step above), using the version and source commit from step 5:

```bash
gh workflow run resume-release.yaml --repo radius-project/radius --ref main \
  -f version=v0.62.0-rc.1 -f source-commit="<source-commit>"
```

Final only (version without `v`):

```bash
gh workflow run publish-bicep-recipes.yaml --repo radius-project/resource-types-contrib --ref main \
  -f release_version=0.62.0
```

## Verification

- [ ] Release build and all downstream stages succeeded; no approval or upmerge pending.
- [ ] The GitHub Release is public with the right version.
- [ ] RC: three coordination receipts succeeded. Final: Bicep recipes published.
- [ ] Version, source commit, PR links, and run URLs posted in the Teams release thread.

## Troubleshooting

| Symptom                                    | Fix                                                                                                                                                       |
|--------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| A stage failed or was interrupted          | Fix the cause, then run the resume command from step 7 using the failed summary's version and source commit. |
| Controller never started after merge       | [Manual-start recovery](./README.md#controller-never-started-after-merge), then run Approve Release.                                                      |
| Preparation blocked                        | Merge the missing backport or finish the previous release, then rerun step 3 with the same inputs.                                                        |
| Plan check fails on the `bicep-types-aws` pin | Redo step 2 with the commit named in the error, then rerun step 3.                                                                                     |
| Controller fails on the `bicep-types-aws` pin | Release PR merged without a passing check; follow [break-glass recovery](./README.md#break-glass-recovery).                                             |
| Dispatch outcome unknown                   | Inspect the publisher run first; do not redispatch. See [lost dispatch recovery](./README.md#lost-dispatch-response).                                     |
| Upmerge PR closed or conflicting           | [Upmerge recovery](./README.md#closed-or-conflicting-upmerge-pull-requests).                                                                              |
| Sample tests failed                        | Merge the fix to `samples` `edge`, then resume. See [retrying sample tests](./README.md#retrying-failed-sample-tests).                                    |
| Attestation or verification failed         | Transient error: resume. Missing or mismatched attestation: investigate.                                                                   |
| Fix needed in already-tagged build code    | [Break-glass recovery](./README.md#break-glass-recovery).                                                                                                 |
