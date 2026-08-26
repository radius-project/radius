# GoReleaser Release Lifecycle: Implementation Plan

- **Author**: Dariusz Porowski
- **Design**: [GoReleaser Release Lifecycle](./2026-03-goreleaser-release-lifecycle.md)

## Scope and decisions

This plan sequences the migration described in the design document into pull requests. Each phase is one PR against `radius-project/radius` unless a companion PR in another repository is explicitly listed. Phases are ordered from least impact on the current workflow and contributor input to most impact, so every phase leaves the existing release process fully operational until the cutover phases replace it.

The following decisions are locked and encoded throughout the plan:

- **Change input**: Conventional Commits, enforced on pull request titles only, because the repository squash-merges every PR. Enforcement rolls out advisory first, then becomes a required status check.
- **Version policy**: commit metadata never selects the version. Every scheduled full release bumps the minor version of the latest channel, even when breaking changes are present, while Radius is `0.x`. Patch releases bump the patch version of their channel. Conventional Commit types drive changelog grouping only; `!` or a `BREAKING CHANGE` footer routes the entry into a mandatory `Breaking changes` section and never causes a major bump.
- **Changelog tooling**: [git-cliff](https://git-cliff.org/) renders `CHANGELOG.md` and per-release notes from Conventional Commit PR titles using a Keep a Changelog template. A small preparation workflow computes the version by policy and opens the release PR. Release Please is not used: with the bump fixed by policy, versioning-by-commit adds no value, and git-cliff gives full control over the RC and release-branch flow.
- **RC identifiers**: new releases use the dotted `-rc.N` form (`v0.61.0-rc.1`), which sorts correctly under SemVer and is native to the tooling. Existing `-rcN` tags are historical and are not rewritten.
- **`CHANGELOG.md` is canonical**: the GitHub Release body for a version is rendered from the corresponding changelog section plus the curated template sections (Highlights, Upgrading).
- **Mutable tag semantics**: `edge` is the mutable tag for every merge to `main`, published by the edge workflow outside the release transaction. `latest` always points at the most recent stable release and is promoted only during release finalization, like the channel aliases. Because `latest` tracks `main` today, the plan dual-publishes `edge` beside `latest` with a deprecation notice first (PR 6) and repoints `latest` at cutover (PR 13).
- **Cleanup travels with the change**: every PR deletes the code, workflow jobs, scripts, and runbook sections it supersedes, in the same PR. Nothing is parked as "disabled" for a later sweep. Phases that only add parallel machinery (PRs 1-4, 7, and 14) supersede nothing yet; every other phase lists its deletions in a Cleanup note, and the final phase handles only code whose last consumer disappears there.
- **Identity policy**: workflows use the default `GITHUB_TOKEN` with an explicit least-privilege `permissions:` block wherever it is sufficient; short-lived GitHub App installation tokens are used only where the built-in token cannot work. The complete operation-by-operation mapping is in the Token and identity policy section below.

## Version policy specification

The preparation workflow and release-plan validation implement exactly these rules:

| Release type | Rule | Example (latest channel `0.60`, latest tag `v0.60.0`) |
| -------------- | ------ | -------------------------------------------------------- |
| First RC of a new channel | next minor, `-rc.1` | `v0.61.0-rc.1` |
| Subsequent RC | same version, increment RC number | `v0.61.0-rc.2` |
| Final | last validated RC version without prerelease | `v0.61.0` |
| Patch | increment patch of the channel | `v0.61.1` |

Validation rejects any proposed version that does not follow from these rules; a deviation requires the break-glass path, never a silent override. Breaking changes affect the changelog and release notes, not the version.

## Token and identity policy

The built-in `GITHUB_TOKEN` cannot push to another repository, cannot emit events that trigger other workflows (a tag or PR created with it starts no workflow and receives no CI checks), and cannot cross the organization boundary into `azure-octo`. Those three cases - and only those - use a GitHub App installation token, minted per job with `actions/create-github-app-token`, scoped to the narrowest repository list and permission set the job needs. Everything else uses `GITHUB_TOKEN`. No long-lived personal access tokens exist anywhere in the release path.

| Operation | Repository | Identity | Why |
| ----------- | ------------ | ---------- | ----- |
| Checkout, CI validation, snapshot builds, PR-title check annotations | `radius` | `GITHUB_TOKEN` | Same-repo reads and PR comments; nothing downstream needs to trigger |
| GHCR image, Helm chart, and CLI OCI pushes | `radius` packages | `GITHUB_TOKEN` (`packages: write`) | Same-repo package publishing, exactly as today |
| Draft GitHub Release creation and publication by GoReleaser | `radius` | `GITHUB_TOKEN` (`contents: write`) | Same-repo release; post-release work is dispatched directly by the controller, so nothing depends on the release event triggering another workflow |
| Release PR, backport PR, and changelog/`versions.yaml` commits from workflows | `radius` | Release GitHub App (`RADIUS_RELEASE_BOT`) | A PR opened with `GITHUB_TOKEN` receives no CI checks; App-authored PRs get full validation and a consistent bot identity |
| Radius `v*` tag push | `radius` | Release GitHub App | A tag pushed with `GITHUB_TOKEN` does not trigger the GoReleaser release workflow |
| Sibling branch and tag reconciliation | `recipes`, `dashboard`, `bicep-types-aws` | Release GitHub App (`contents: write`, scoped to the four repos) | Cross-repository pushes, and those tag pushes must trigger each repository's own workflows |
| Docs and samples workflow dispatch and monitoring | `docs`, `samples` | Release GitHub App (`actions: write`, `metadata: read`) | Cross-repository `workflow_dispatch`; requires extending the App installation to both repositories, done in PR 16 |
| Deployment Engine and Bicep types publisher dispatch and monitoring | `azure-octo/radius-publisher` | Publisher GitHub App (`RADIUS_PUBLISHER_BOT`: `actions: read`, `contents: write`) | Cross-organization dispatch; the App keeps tokens short-lived and scoped to a single repository |

Both Apps already exist with the listed scopes except where a phase explicitly extends them; every extension is named in the phase that introduces it, so the App inventory is auditable from this document alone.

## Phase overview

| PR | Title | Impact on current workflow | Contributor input | Design phase |
| ---- | ------- | ---------------------------- | ------------------- | -------------- |
| 1 | Parity manifest tooling | None | None | 1 |
| 2 | `.goreleaser.yaml` with check and snapshot CI | None (adds CI job) | None | 1 |
| 3 | Conventional Commit title check, advisory | None (non-blocking) | Optional | 5 (pulled forward) |
| 4 | git-cliff configuration and `CHANGELOG.md` bootstrap | None | None | 1 |
| 5 | Split `build.yaml` by trigger | None (mechanical refactor) | None | 2 |
| 6 | Introduce `edge` tags and deprecate `latest`-as-edge | Additive dual publication | None | 2 |
| 7 | Shadow GoReleaser release on tags | None (parallel job) | None | 2 |
| 8 | Conventional Commit title check becomes required | PR titles must conform | Required | 5 (pulled forward) |
| 9 | Idempotent tag and branch reconciliation | None (same trigger, safer internals) | None | 3 |
| 10 | Release-identifier correlation for remote dispatch | None | None | 3 |
| 11 | Adopt `-rc.N` identifiers | Release engineers type `rc.1` | None | 3 |
| 12 | `Prepare Release` workflow and backport automation | Replaces manual release PRs and cherry-picks | None | 3 |
| 13 | Tag-build cutover to GoReleaser | Artifact production authority changes; `latest` repointed to stable | None | 2 |
| 14 | SBOM generation | None (additive assets) | None | 5 (pulled forward) |
| 15 | Release controller and trigger swap | Release operating model changes | None | 3 |
| 16 | Publication gate, verification, and notifications | Publication becomes gated | None | 4 |
| 17 | Helm chart immutable image tags | User-visible patch-pickup change | None | 4 |
| 18 | Final sweep, last consumers, and hardening | Last orphaned paths deleted | None | 5 |

Each PR merges only after the previous one is validated; PRs 7 and 13 additionally require observing at least one full RC-plus-final cycle.

## Phase details

### PR 1: Parity manifest tooling

Add a script (for example `.github/scripts/release-parity-manifest.sh`) that captures the complete observable output of a release into a machine-readable manifest: raw CLI asset names and sidecar checksums, Go version and linker metadata fields, production and test image names with platforms, tags, and digests, Helm chart name, version, and app version, GitHub Release classification and note source, and downstream outputs. Add a manually dispatched workflow that runs it against a given version. Generate and commit the manifest for the latest release as the baseline.

- **Exit criteria**: manifest generated for the most recent RC and final release; reviewed for completeness against the design's compatibility list.
- **Rollback**: none needed; purely additive.

### PR 2: `.goreleaser.yaml` with check and snapshot CI

Add the full GoReleaser configuration: six builds (`rad`, `ucpd`, `applications-rp`, `dynamic-rp`, `controller`, `pre-upgrade`), raw binary asset naming, split checksums, `dockers_v2` definitions with `linux/amd64`, `linux/arm64`, and `linux/arm/v7` platforms, draft-release settings, explicit retry budget, and external release-note input. Add `Dockerfile.goreleaser` files for components whose Dockerfiles assume the Make dist layout, preserving base images, packages, non-root users, and the UCP manifests via `extra_files`. Pin the GoReleaser version (v2.14 minimum). Wire `goreleaser check` and `goreleaser release --snapshot --clean` into PR and main CI as a non-required job that diffs snapshot output against the PR 1 manifest for binary names, checksum format, and image definitions.

- **Exit criteria**: snapshot job green on PRs; binary and checksum layout matches the parity manifest with every difference explained.
- **Rollback**: remove the CI job; the config file is inert without it.

### PR 3: Conventional Commit title check, advisory

Add a PR-title validation job using the pinned `amannn/action-semantic-pull-request` action in non-blocking mode that annotates non-conforming titles with a fix-it comment. Use the action's Conventional Commit defaults (`feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`; scope optional; `!` allowed). Dependency updates use the existing `ci(deps):` and `chore(deps):` prefixes. Update the contributing documentation with the type table, examples, and the policy statement that types affect the changelog, never the version. Verify the repository squash-commit message setting is "pull request title" so merged commits actually carry the convention; change it if not.

- **Exit criteria**: check runs on all PRs; contributing docs merged; squash-message setting confirmed.
- **Rollback**: remove the job.

### PR 4: git-cliff configuration and `CHANGELOG.md` bootstrap

Add `cliff.toml` mapping commit types to Keep a Changelog headings (`feat` to Added, `fix` to Fixed, `perf` and `refactor` to Changed, dependency-scoped `ci` and `chore` commits to Dependencies, `revert` handled natively; `docs`, `style`, `test`, `build`, other `ci`, and other `chore` commits excluded), with a `Breaking changes` section rendered first for `!` or `BREAKING CHANGE` entries, ISO 8601 dates, comparison links, and GitHub integration for contributor and first-time-contributor lists (this replaces the manual "new contributors" paste in the release-notes template). Bootstrap `CHANGELOG.md` with an `Unreleased` section and entries starting from the current channel; older history stays on GitHub Releases with a pointer. Add a manually dispatched preview workflow that renders the unreleased section.

- **Exit criteria**: preview renders correct grouping against recent real commits; template sections agreed with maintainers.
- **Rollback**: none needed; not yet wired into the release.

### PR 5: Split `build.yaml` by trigger

Mechanically split the current `build.yaml` into trigger-scoped workflows with identical behavior: PR and merge-queue validation; main-branch push (snapshot validation plus the existing edge publication of `latest` CLI OCI artifacts, test images, and edge Helm); tag push (the current release jobs, unchanged). No logic changes; the diff is job relocation plus shared pieces extracted into reusable workflows where duplication would otherwise grow.

- **Cleanup**: the monolithic `build.yaml` is deleted in this PR; its jobs move wholesale into the trigger-scoped workflows.
- **Exit criteria**: one PR cycle, one main merge, and one tag build produce byte-identical behavior to before (verified against the parity manifest).
- **Rollback**: revert restores the single workflow.

### PR 6: Introduce `edge` tags and deprecate `latest`-as-edge

Give the mutable tags their target semantics ahead of the cutover. The main-branch edge workflow starts publishing `edge` tags for the CLI OCI artifacts and images alongside the existing `latest` tags with identical content, the Helm chart's edge mapping switches from `latest` to `edge`, and the documentation announces that `latest` will be repointed to the most recent stable release at cutover, directing main-branch consumers to `:edge`. Dual publication keeps every existing consumer working through the deprecation window.

- **Cleanup**: the chart's edge-to-`latest` mapping is replaced by edge-to-`edge`; `latest` publication from `main` survives only as a deprecation shim and is deleted in PR 13.
- **Exit criteria**: after a `main` merge, `edge` and `latest` resolve to the same digests; the deprecation notice is published in the docs.
- **Rollback**: revert the chart mapping; the extra `edge` tags are harmless.

### PR 7: Shadow GoReleaser release on tags

Add a non-blocking job to the tag workflow that runs GoReleaser for the real tag in shadow mode: images pushed only to the existing `ghcr.io/radius-project/dev` namespace with full-version tags, binaries and checksums uploaded as workflow artifacts, no GitHub Release created. The job diffs every output against the parity manifest and fails loudly on unexplained differences without affecting the production release.

- **Exit criteria**: at least one full RC-plus-final cycle with zero unexplained parity differences, including image platforms, labels, runtime file layout, and binary version metadata.
- **Rollback**: remove the job.

### PR 8: Conventional Commit title check becomes required

Flip the PR 3 check to a required status check once the advisory period shows a high conformance rate. Include merge-queue coverage.

- **Cleanup**: the advisory-mode comment path from PR 3 is removed; the check either passes or blocks.
- **Exit criteria**: check required on `main`; no long-lived PRs blocked without a documented rename path.
- **Rollback**: unmark as required.

### PR 9: Idempotent tag and branch reconciliation

Replace `release-create-tag-and-branch.sh` internals with the design's reconciliation rules while keeping the current trigger and callers: push exactly one named tag (never `git push --tags`), treat an existing tag at the planned commit as success and at any other commit as a hard conflict, verify branch state by commit reachability, and verify every destination after mutation. Fix the timeout inversion so every monitor budget fits inside its job timeout.

- **Cleanup**: the previous script logic (`git push --tags`, existing-tag-as-error) is deleted, not kept as a fallback.
- **Exit criteria**: rerunning the release workflow after a simulated partial failure (tag exists in `radius`, missing in `recipes`) completes the remaining repositories instead of failing.
- **Rollback**: revert the script.

### PR 10: Release-identifier correlation for remote dispatch

Add a stable release identifier (`<version>-<source-sha>`) to the Deployment Engine and Bicep dispatch payloads, and rewrite `monitor-remote-workflow.mjs` to match runs by that identifier instead of by workflow name and start time, with the query-before-dispatch rule to prevent duplicates.

- **Companion PR**: `azure-octo/radius-publisher` - include the identifier in `run-name` and the concurrency group of both publisher workflows. The companion PR lands and deploys first, so the monitor switches directly to identifier matching. Dispatch continues to use the existing publisher App (`RADIUS_PUBLISHER_BOT`) per the token policy; no scope change is needed.
- **Cleanup**: the time-window matching logic in `monitor-remote-workflow.mjs` is deleted in this PR; no transitional fallback remains.
- **Exit criteria**: two concurrent dispatches correlate to their own runs in a test exercise.
- **Rollback**: reverting this PR restores time-window matching; the companion PR's run-name change is harmless on its own.

### PR 11: Adopt `-rc.N` identifiers

Update `release-get-version.sh`, `validate_semver.py` acceptance, the release documentation, and any tag pattern matching to accept and prefer `-rc.N`. The Helm chart's `rc` substring detection is unaffected. Takes effect at the next RC; historical tags are untouched.

- **Cleanup**: every `-rcN` emission path is removed; parsing of historical `-rcN` tags remains because previous-tag computation needs it, so it is not dead code.
- **Exit criteria**: an `-rc.N` version flows through the existing release automation end to end.
- **Rollback**: both forms validate during the transition, so reverting to `-rcN` input is non-breaking.

### PR 12: `Prepare Release` workflow and backport automation

Add the version-preparation workflow (manual dispatch with inputs: release type `rc | final | patch` and channel). It computes the version by the policy table, runs git-cliff to produce the changelog section and assembles the release-notes file from the curated template (generated changelog and contributors filled in; Highlights and Upgrading left for maintainer edit), updates `versions.yaml` and `CHANGELOG.md`, and opens the release PR against `main` using the release App identity so the PR receives full CI validation. The release plan (version, source commit, channel, chart version, expected outputs, included backports) is recorded in the release PR body as a structured block and emitted as a workflow artifact.

**Selecting commits for a subsequent RC or patch** - the mechanics that replace manual `git log` archaeology and `git cherry-pick`:

- A maintainer marks a merged `main` PR for inclusion by adding a `backport release/<channel>` label, at merge time or retroactively.
- A backport workflow reacts to the label: it cherry-picks the squash commit with `-x` onto a branch cut from the release branch and opens a backport PR with the release App identity, preserving the original Conventional Commit title and linking the source PR. On a conflict it still opens the PR and comments with the exact local commands to resolve and push, so the engineer resolves the conflict but never re-derives what to pick.
- The backport commit keeps the source commit's author and records the release App as the committer, so credit stays with the contributor. GitHub's bot signature verification is deliberately not used for backports: it only applies when the request carries no custom author, and `release/*` allows only rebase merges, which add commits to the base branch without signature verification. The signature would therefore be discarded when the backport lands, while the author survives. The release PR opened by `Prepare Release` is the opposite case - it carries no human author and targets `main`, which requires signatures and squash-merges - so it keeps bot signing.
- Backport PRs to `release/*` branches are rebase-merged (rebase merging is enabled repository-wide in this PR) so each cherry-picked commit keeps its own Conventional Commit title for the release-branch changelog; a validation job on release branches enforces commit-message conformance.
- `Prepare Release` for a subsequent RC or patch verifies that every labeled PR has a merged backport before proposing the version, lists the included commits in the release PR body, and also accepts an explicit list of PR numbers as a dispatch input for one-off inclusions.
- A new runbook section, "Backporting changes to a release branch", documenting the label flow, conflict resolution, and verification, lands in this PR.

[korthout/backport-action](https://github.com/korthout/backport-action) was evaluated as a replacement for the backport scripts and rejected. It cherry-picks with `-x`, resolves the merge method automatically, and preserves the source author while committing as a bot, all of which match this design. It was rejected on three other grounds: it opens every labelled backport simultaneously against the current branch head rather than serialising one per channel; its `draft_commit_conflicts` mode commits conflict markers, which the completion check deliberately rejects; and it has no equivalent of the approved-base pinning or the completion verification that most of the custom code implements. Adopting it would replace roughly the cherry-pick mechanics alone. Revisit if `release/*` gains a strict up-to-date merge requirement, which is what the one-open-backport-per-channel rule currently compensates for.

The existing `versions.yaml`-triggered automation still runs unchanged when the release PR merges, so this PR only automates what release engineers previously did by hand.

- **Cleanup**: the `generate_release_note` job in `release.yaml` (sticky PR comments with GitHub-generated notes) is superseded by the git-cliff notes in the release PR and deleted; the runbook's manual `versions.yaml`-editing and cherry-pick sections are replaced by the new instructions, with the old manual path retained only as a clearly marked break-glass appendix until PR 18.
- **Exit criteria**: one RC and one final prepared through the workflow, including at least one label-driven backport, with the old automation completing the release; generated notes require only Highlights editing.
- **Rollback**: release engineers fall back to the break-glass appendix; nothing downstream changed.

### PR 13: Tag-build cutover to GoReleaser

In the tag workflow, replace the CLI matrix, image publication, checksum, and GitHub Release jobs with a single GoReleaser release job: raw CLI assets and split checksums, immutable full-version production images, and a draft GitHub Release using the prepared notes from the release PR. A finalize job publishes the draft and promotes the mutable aliases - channel tags and `latest`, for images and CLI OCI artifacts alike (via `docker buildx imagetools create` and `oras` from the verified immutable tags) - only after the Helm, Bicep, and CLI jobs succeed, a minimal `needs`-based gate ahead of the full gate in PR 16. From this PR onward `latest` always points at the most recent stable release, per the decided tag semantics. PR-build and edge behavior from PRs 5 and 6 are untouched except that the edge workflow stops publishing `latest`; artifact upload points required by functional tests are preserved. The old tag-path jobs are deleted, not disabled. GoReleaser's release creation and image pushes run on `GITHUB_TOKEN` per the token policy; no App token enters this workflow.

- **Cleanup**: the tag-path CLI matrix, image publication, checksum, and `publish-release` jobs are deleted, along with any Make targets whose only consumers were those jobs, and the PR 6 deprecation shim (`latest` publication from `main`) is removed. `docker-multi-arch-push` survives because main-branch edge publication still uses it until PR 18.
- **Exit criteria**: first production release with GoReleaser matches the parity manifest; install verification passes against the published release.
- **Rollback**: revert the PR to restore the previous tag workflow; immutable extra tags in GHCR are harmless.

### PR 14: SBOM generation

Enable GoReleaser's native syft integration to publish SBOMs for the raw CLI binaries as GitHub Release assets, and generate image SBOMs through `dockers_v2`. Purely additive: no existing asset changes, and the parity comparison treats the new files as explained additions. A short documentation note records what is published, in which format, and where consumers find it.

- **Cleanup**: nothing superseded; net-new assets.
- **Exit criteria**: SBOM assets present and well-formed on the draft release of one RC.
- **Rollback**: remove the config block.

### PR 15: Release controller and trigger swap

Introduce the release controller workflow and switch the trigger: merging the release PR (or an explicit approval dispatch) starts the controller, which validates the release plan and `versions.yaml` consistency, verifies the signed Deployment Engine tag and publishes its image, reconciles sibling branches and tags in `recipes`, `dashboard`, and `bicep-types-aws`, and creates the Radius tag last using the GitHub App identity (App token required so the tag push triggers the GoReleaser workflow). Remove the `versions.yaml` push trigger from `release.yaml`. Add the `Resume Release` workflow accepting version and source commit, and release-level concurrency keyed by both with cancellation disabled. The five-minute-timeout release job and its ordering problems disappear with the old trigger. All cross-repository operations (sibling reconciliation, the Radius tag push, publisher dispatch) use the two existing Apps exactly as mapped in the token policy; the controller's own validation and verification steps run on `GITHUB_TOKEN`.

- **Cleanup**: `release.yaml` is deleted outright - its release job, branch-existence special case, and Deployment Engine dispatch move into the controller - and the runbook's trigger documentation is updated in the same PR.
- **Exit criteria**: injected failures after each stage recover through `Resume Release` without duplicate tags or dispatches; a conflicting version/source pair is rejected.
- **Rollback**: revert restores the `versions.yaml` trigger; PRs 9 and 10 improvements are independent and remain.

### PR 16: Publication gate, verification, and notifications

Complete the design's verify and finalize stages: generate the release manifest, verify every asset name, checksum, image digest and platform set, Helm chart version and image references, external outputs, and binary metadata against the plan; run installation verification with the staged local `rad` binary before publication; move channel-alias promotion and draft publication behind this gate with the release-environment approval for finals and patches. The mandatory output set is identical for every release type; RCs publish automatically once the gate passes, per the decided gate policy. Dispatch and correlate docs and samples workflows (RC upmerge and sample tests as final-release gates; publication workflows post-release). Post stage transitions and the final summary to the Teams release channel through an incoming webhook, replacing manual transcription. Dispatching the docs and samples workflows requires extending the release App installation to those two repositories with `actions: write` and `metadata: read`; this is the only App-scope extension in the plan and is recorded in the token policy table.

- **Cleanup**: the runbook's manual dispatch instructions for release verification, docs upmerge, samples upmerge, and sample tests are deleted; the standalone post-publication release-verification workflow stays until PR 18 as the migration cross-check.
- **Exit criteria**: a seeded verification failure (wrong digest) blocks publication and aliases; the release summary answers every troubleshooting question in the design's monitoring section.
- **Rollback**: gate checks can be individually demoted to warnings via configuration while a false positive is fixed; the break-glass workflow cannot bypass the manifest check.

### PR 17: Helm chart immutable image tags

Change `deploy/Chart/templates/_helpers.tpl` so final and patch charts reference full-version image tags instead of the truncated `major.minor` channel tag (RC behavior is already full-version). Channel aliases continue to be published for backward compatibility. Call out the patch-pickup behavior change prominently in the release notes of the release that ships it.

- **Exit criteria**: chart install from a final release pins full-version images; upgrade path from a channel-tag install verified.
- **Rollback**: revert the template; both tag forms exist in the registry.

### PR 18: Final sweep, last consumers, and hardening

Because every phase deletes what it supersedes, this PR handles only the code whose last consumer disappears here. Migrate the PR-build container artifacts and main-branch edge publication from the Make path to GoReleaser snapshot outputs, then delete `get_release_version.py`, `release-get-version.sh`, the remaining multi-arch targets in `docker.mk`, release-only parts of `version.mk`, and any workflow remnants, retaining developer, Bicep, and separate-module test targets. Per the decided test-image policy, move `testrp` and `magpiego` publication fully into test workflows with test-specific immutable tags once the consumer inventory confirms nothing pulls their channel tags, and stop their release-channel tags. Consolidate the runbook edits from PRs 12-16 into the final short procedure (approval, monitoring, resume, break-glass), remove the manual-fallback appendix, and retire the standalone post-publication release-verification dispatch now that the publication gate covers it.

- **Cleanup**: this entire PR is cleanup; nothing new is added except the snapshot wiring for the migrated consumers.
- **Exit criteria**: one full release cycle executed by a release engineer using only the new runbook; repository search finds no callers of removed scripts or targets.
- **Rollback**: individual deletions revert independently.

## Cross-cutting notes

- **Deployment Engine**: the manually signed DE tag remains a prerequisite until the decided replacement lands ([azure-octo/deployment-engine#456](https://github.com/azure-octo/deployment-engine/issues/456)). The decided mechanism is a GitHub-App-created tag through the API, contingent on a validation spike proving it satisfies the `azure-octo` verified-tag requirement (GitHub's web-flow signing covers commits, not annotated tag objects); the fallback is a bot GPG signing key in the DE release workflow. Until one of those ships in the DE repository, PR 12's `Prepare Release` checks for the tag early and prints the exact command, and PR 15's controller hard-blocks before any mutation.
- **Cadence fit**: with a monthly release train, PRs 1-6 can land within one cycle, PR 7 must observe a full cycle, and PRs 13, 15, and 16 should each ship at least one release apart. Realistic end-to-end duration is four to five release cycles.
- **Parity manifest is the contract**: every cutover PR (7, 13, 17) treats an unexplained difference from the PR 1 manifest as a failure, per the design's test plan.
- **Runner duration budget**: the release summary records total release duration each cycle; the GoReleaser Pro split-and-merge evaluation reopens only if duration exceeds the agreed budget, per the decided runner topology.
- **Script language boundary**: work that is a GitHub API problem belongs in an ESM module executed by [`actions/github-script`](https://github.com/actions/github-script), which supplies a pre-authenticated paginating Octokit plus `context` and `core`; work that is local git, registry, or artifact inspection stays in shell, because `github-script` would only wrap the same commands in `exec` while losing pipeline ergonomics and the real-git test fixtures. `monitor-remote-workflow.mjs` and `select-release-backports.mjs` sit on the API side of that line; the tag, branch, changelog, and verification scripts sit on the git side. The boundary is not a signing decision: no GitHub API creates a verified annotated tag object, and signed commits come from the GraphQL `createCommitOnBranch` mutation that `peter-evans/create-pull-request` already performs, so moving git plumbing into `github-script` would gain neither.
