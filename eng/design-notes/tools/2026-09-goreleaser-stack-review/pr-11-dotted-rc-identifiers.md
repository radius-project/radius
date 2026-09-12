# Review note: PR 11 - Adopt `-rc.N` identifiers

- **Pull request**: [#12793](https://github.com/radius-project/radius/pull/12793)
- **Plan phase**: [PR 11](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-11-adopt--rcn-identifiers)
- **Stack index**: [README](./README.md)

## Verdict

The layer does what the plan asks and keeps the scope tight. The version policy now lives in one sourced helper: a release version is `X.Y.Z`, `X.Y.Z-rc.N`, or the historical `X.Y.Z-rcN`, with the RC number starting at one and carrying no leading zero, and nothing else. Version selection, release verification, and the long-running release checkout all use that helper instead of three private regular expressions, the selector also requires the `v` prefix and warns when a legacy form is selected, and the generic SemVer validator keeps its acceptance while gaining a proper full match and clean exit codes. The tag parser needed no code change because it already used the SemVer reference expression; the new format test proves that a dotted tag flows into `REL_VERSION`, `REL_CHANNEL`, and `CHART_VERSION` unchanged, and that the same holds for a historical tag. The runbook and the design's examples are updated, and the format test greps the runbook so a legacy identifier cannot creep back into the instructions. The Helm chart's `rc` substring detection is untouched and the chart suite passes with a dotted image tag. The Go upgrade preflight already orders `rc.2` below `rc.10` through the SemVer library, and the tests now prove it while keeping a legacy `rc4` to `rc5` case. No `rcN` emission remains outside the recorded parity baseline for `v0.60.0-rc3`, which is historical data. The transition is clean: channel `0.60` is complete at `v0.60.2`, so the first dotted candidate is `v0.61.0-rc.1`.

## Changes made in this review

### 1. The no-mixing rule is stated where the policy lives

- **What changed**: the policy helper, the runbook's transition paragraph, and the design's RC decision now say that the two forms are never mixed within one version and that the dotted form starts with the first RC of a version. A preflight test pins the reason: an upgrade from `0.61.0-rc1` to `0.61.0-rc.2` is rejected as a downgrade.
- **Why**: SemVer compares the first prerelease identifier lexically, and `rc` sorts before `rc1`, so every dotted candidate of a version orders below every legacy candidate of the same version. The repository's own SemVer library confirms it. Nothing in the layer stated the constraint, and a maintainer following "use the dotted form for new tags" partway through an RC series would have produced a candidate that `rad upgrade` refuses to install over its predecessor.
- **Value**: the rule is visible to the maintainer writing `versions.yaml`, to the reader of the design, and to the Go developer changing the preflight, and PR 12's version computation has an explicit constraint to satisfy (cross-layer finding 9).
- **Impact**: comments, documentation, and one test case; no behavior change.

## Findings left as-is

- **Spacing in the new format test**: it writes `> /dev/null` with a space, the style of the repository's `.editorconfig` shfmt profile, while the sibling scripts of this layer follow `shfmt -i 4 -ci`. Nothing in CI runs shfmt, and PR 18 rewrites the affected lines, so normalizing them here would only create a conflict.
- **Pre-existing lint findings**: shfmt disagrees with `checkout-release-codebase.sh` and `release-verification.sh` exactly as it did before this layer, actionlint's ShellCheck pass reports old `run` blocks in the functional test workflows, and Prettier wants a trailing comma removed at one place in `functional-test-noncloud.yaml`. This layer touched those files only in comments and examples.
- **Generic SemVer validator**: `validate_semver.py` still accepts `X.Y.Z-rc.0` or `X.Y.Z-beta.1` because it validates SemVer, not the Radius policy; the policy is enforced by the helper in every release path, and the release-verification workflow is the only caller of the generic check.
- **Legacy warning text**: the selector recommends the dotted equivalent when a legacy RC is selected. With the no-mixing rule that advice applies only to a new version, which the runbook now says.

## Verification

- Shell: format (24), selection (11), and reconciliation (18) suites pass; ShellCheck is clean for the six scripts.
- Go: the preflight tests pass with the added case; the ordering of `0.61.0-rc1`, `0.61.0-rc.2`, and `0.61.0-rc.10` was checked directly against the `Masterminds/semver` library the preflight uses.
- Helm: 134 chart tests pass. Python 3.12 parses both scripts and the validator behaves as the format test expects.
- Docs: markdownlint, markdown-table-formatter, and cspell pass for the runbook, the design note, and these review notes.
