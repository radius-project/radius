# Review note: PR 12 - Prepare Release workflow and backport automation

- **Pull request**: [#12823](https://github.com/radius-project/radius/pull/12823)
- **Plan phase**: [PR 12](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-12-prepare-release-workflow-and-backport-automation)
- **Stack index**: [README](./README.md)

## Verdict

The layer delivers the plan's preparation phase and does it with an unusually strong trust model. `Prepare Release` computes the version from the policy table and the repository state, renders the changelog section and release notes with git-cliff, updates `versions.yaml` and `CHANGELOG.md`, records the release plan as a structured block in the pull request body and as an artifact, and opens a signed draft pull request with the release App. The plan check regenerates everything from the trusted base commit and compares, on pull requests and again in the merge group, without ever executing pull request code. Backports are label driven, serialized per channel, cherry-picked with `-x`, keep the contributor as author with the App as committer, and hand a conflict off as a draft with exact commands instead of committing markers; the release-branch check enforces Conventional Commit titles and the recorded base and source markers. The `generate_release_note` job is gone, RC releases now publish the prepared notes, and the runbook is rewritten around the new flow with a break-glass appendix. Repository settings already match the plan: rebase and squash merges are enabled and merge commits are not. Eight suites cover it, and with the pinned yq they all pass; ShellCheck, actionlint, and Prettier are clean.

Two things were wrong and are fixed here. The signed Deployment Engine tag check used the release App token, which is installed on `radius-project`; `azure-octo/deployment-engine` is private, so that token gets a 404 and the script told the maintainer to create a tag that may already exist. And `versions.yaml` handling deprecated the current stable release the moment a first RC was cut, which contradicts every recorded release: during the 0.60 candidates `v0.59.0` stayed supported and moved to `deprecated` only when `v0.60.0` shipped.

## Changes made in this review

### 1. The Deployment Engine tag check can actually read the repository

- **What changed**: `Prepare Release` mints a publisher App token for `azure-octo/deployment-engine` with Contents read and uses it for the tag check. The script first proves it can read the repository and reports a token without access as such; only a readable repository with no tag produces the "create the signed tag" instruction. The plan's token table gains the row, the PR 12 section names the App extension, and the runbook lists it as a prerequisite. A test covers the unreadable repository.
- **Why**: the repository is private, so a token without access receives the same 404 as a missing tag. With the release App or `GITHUB_TOKEN` the check can never pass, and its failure message pointed at the wrong fix. The plan's token policy had no identity for this read, so the check had to be given one that the design already reserves for `azure-octo`.
- **Value**: the fail-fast gate the design asks for works, and its two failure modes are named correctly.
- **Impact**: the publisher App installation on `azure-octo` must include `deployment-engine` with Contents read before the first run; the token step fails visibly otherwise. PR 15's controller runs the same script with `GITHUB_TOKEN` and is cross-layer finding 10.

### 2. `versions.yaml` follows the recorded release history

- **What changed**: a first RC is inserted at the top of `supported` and nothing is deprecated; a final release moves every other supported entry, in order, to the top of `deprecated`; later RCs and patches still replace the channel's version in place. Two tests replace the fixed-window test, and the runbook states the rule.
- **Why**: the previous logic kept the supported list the same size, so with today's single supported entry the first `0.61` candidate would have deprecated `v0.60.2`, leaving an unvalidated RC as the only supported version. Every release since the file existed did the opposite, as the `v0.60.0-rc1` and `v0.60.0` snapshots of the file show.
- **Value**: the generated file states the support policy the project actually applies, and the plan check regenerates it the same way.
- **Impact**: generated `versions.yaml` content changes for first RCs and finals; no consumer in this repository reads anything but `.supported[].version` for tag selection.

### 3. A dotted candidate never follows a historical one

- **What changed**: preparing another RC fails when the channel's current RC or its highest RC tag uses the historical `rcN` form, with a message that points at the final release or a new version. A test covers it. This closes cross-layer finding 9.
- **Why**: SemVer orders `rc.3` before `rc2`, so the successor the script would have computed is a downgrade for `rad upgrade`. No channel is in that state today, so the guard costs nothing and removes the only path that could emit a mixed series.
- **Value**: the rule stated in PR 11 is enforced where RC numbers are produced.
- **Impact**: none for current channels.

### 4. Smaller corrections

- The release pull request body no longer ends with the hard-coded "Generated for #12814." line (cross-layer finding 4).
- The plan text uses "serializing", and the dictionary entry added for the British spelling is dropped; "korthout" stays because it is a GitHub account.

## Findings left as-is

- **Patch releases now need a Deployment Engine tag**: the check runs for every release type, and the runbook says so, but `azure-octo/deployment-engine` has no `v0.60.2` tag, so this is a new demand on maintainers rather than an existing practice. It follows the design's plan-validation rule and is flagged here for the maintainers' attention.
- **RC changelog sections cover the whole channel**: each candidate's section, and the final's, is rendered from the channel boundary, so `rc.2` repeats `rc.1`'s entries plus the backports. The previous GitHub-generated RC notes listed only the delta since the last candidate. The full list is the more useful view for validating a candidate and matches what the final section contains; noted as a behavior change, not changed.
- **yq version**: the preparation scripts need the pinned yq (4.53.6), which `make install-yq` provides in CI; the 4.44 release in my environment mis-evaluated an expression and failed a test in a confusing way. Not worth a version check in the script.
- **Shell style**: the thirteen new scripts follow the repository's `.editorconfig` shfmt profile, while older release scripts follow `shfmt -i 4 -ci`; nothing in CI enforces either, so they are left alone.
- **Plan tables**: the implementation plan was never formatted with the table formatter, so the added row keeps the file's own style rather than reformatting every table in this layer.

## Verification

- Shell: preparation (17), plan validation (10), merge group (4), backport collection (3), backport creation (3), and Deployment Engine tag (5) suites pass with yq 4.53.6; ShellCheck is clean for the thirteen new scripts.
- Node: backport selection (5) and Conventional Commit validation (13) pass.
- actionlint passes for the six new or changed workflows; Prettier with the repository configuration passes for the workflows and the Node scripts; markdownlint and cspell pass for the runbook, the release-notes documentation, the plan, and these notes.
- The token problem was reproduced by running the tag check with a token that cannot read the repository: it printed the create-the-tag instruction for a tag that exists.
