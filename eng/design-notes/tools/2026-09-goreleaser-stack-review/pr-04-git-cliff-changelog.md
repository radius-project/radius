# Review note: PR 4 - git-cliff configuration and changelog bootstrap

- **Pull request**: [#12743](https://github.com/radius-project/radius/pull/12743)
- **Plan phase**: [PR 4](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-4-git-cliff-configuration-and-changelogmd-bootstrap)
- **Stack index**: [README](./README.md)

## Verdict

The layer meets the PR 4 exit criteria. Rendering the real unreleased range (124 commits since the 0.60 branch point) exactly as the preview workflow does, and a fixture repository with one commit of every kind, produced the grouping the plan asks for: a `Breaking changes` section first, with a `chore!` commit kept by `protect_breaking_commits`; `feat`, `fix`, `perf` and `refactor` under the Keep a Changelog headings; Dependabot's `chore(deps)` and `ci(deps)` under `Dependencies`; reverts in their own section; the excluded types absent; legacy subjects under `Other changes`; ISO dates and comparison links in the footer. The range script picks the 0.60 branch point on `main` and a release branch's own tag on that branch, and its six tests pass. The four pinned installer checksums match the downloaded assets and the upstream SHA-512 files. The bootstrapped `CHANGELOG.md` has the `## [Unreleased]` heading and `[Unreleased]:` link that PR 12's release preparation later splices into.

## Changes made in this review

### 1. One pull request reference per entry, and the pull request author credited

- **What changed**: a template macro renders each entry. When the GitHub integration knows the pull request, the `(#NNN)` suffix that a squash merge leaves in the subject is dropped and the number is rendered once as a link; offline renders keep the plain suffix. The author is `commit.remote.pr_author` with `commit.remote.username` as the fallback, the form the git-cliff documentation recommends for squash merges, where the commit can resolve to the merging account.
- **Why**: 83 of the entries in the real preview read like "Fix the thing (#12608) by @user in [#12608](...)", with the number twice.
- **Value**: release notes read the way GitHub's generated notes do, with one link per change and the right person credited.
- **Impact**: verified on the real range: 101 linked entries, no duplicates, and one backport that keeps its original `(#12688)` reference on purpose because the link points at the backport. The fixture render is byte-identical before and after the macro when offline.

### 2. git-cliff pinned to v2.14.1

- **What changed**: `build/tools.yaml` pins v2.14.1 with checksums computed from the assets and verified against the upstream SHA-512 files; `build/tools.generated.mk` was regenerated with the tool updater.
- **Why**: v2.14.1 is the current release. Its three breaking notes do not touch this layer: the workflow always passes `--config`, `commit.remote.username` is unchanged, and the range is passed after every option. The release also adds `pr_author`, which change 1 uses.
- **Value**: the stack lands on the current release, and the rendered output is byte-identical between v2.13.1 and v2.14.1 for both the real range and the fixture.
- **Impact**: none on output; the installer's exact version comparison already handles the bump.

### 3. A committed render test for the configuration

- **What changed**: `changelog-config_test.sh` renders `cliff.toml` against a fixture repository with every allowed type, a breaking `chore!`, the three dependency prefixes, a revert, and a legacy subject, then checks section order, placement, exclusions, single rendering of breaking commits, and the tagged heading and comparison link. `make test` runs it through `test-changelog-config`, which installs the pinned git-cliff first, the same shape as the OCI artifact test in PR 14.
- **Why**: the pull request describes these fixtures being run by hand. Nothing guards the template, and PR 8 and a later layer both edit it.
- **Value**: a broken parser or template fails `make test` instead of the next release preparation.
- **Impact**: about two seconds and one binary download in `make test`.

### 4. The preview workflow runs on pull requests that change what it renders

- **What changed**: `changelog-preview.yaml` also triggers on `pull_request` for `cliff.toml`, the range script, and itself, with the same read-only permissions.
- **Why**: manual dispatch only catches a template error when someone remembers to dispatch it.
- **Value**: a rendering problem is visible on the pull request that introduces it, with the preview in the job summary.
- **Impact**: one short job on the few pull requests that touch those files.

## Findings left as-is

- **`deps` parser**: still valid in this layer because `deps` is an allowed title type until PR 8 removes it; recorded as cross-layer finding 5.
- **Non-standard headings**: `Dependencies`, `Reverted changes`, and `Other changes` are outside the Keep a Changelog set. The plan chose them deliberately for dependency and revert commits, and the last one is the only honest place for history written before the title policy.

## Verification

- `changelog-config_test.sh` and `changelog-range_test.sh` pass with git-cliff v2.14.1 installed through the repository installer; shfmt and ShellCheck are clean; actionlint and Prettier pass for the workflow; the tooling package tests pass after the manifest regeneration.
- The real preview rendered with v2.13.1 and v2.14.1 is identical for both the original and the revised configuration.
