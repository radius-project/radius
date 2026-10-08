# Review note: PR 13 - Tag-build cutover to GoReleaser

- **Pull request**: [#12828](https://github.com/radius-project/radius/pull/12828)
- **Plan phase**: [PR 13](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-13-tag-build-cutover-to-goreleaser)
- **Stack index**: [README](./README.md)

## Verdict

The layer is the cutover the plan describes, and it implements the design's immutability constraints rather than approximating them. One GoReleaser job stages the CLI assets, the split checksums, the five production images under their full-version tags, and a source-bound draft release that carries the prepared notes; the retained Bicep and test images keep their Make path but now publish full-version tags; Helm publication reuses an identical chart or pushes and re-pulls to verify; and a single finalize job promotes the channel and `latest` aliases from recorded digests, for images with buildx and for the CLI artifacts with oras, before it publishes the draft. Every mutation is preceded by a query. Intents and digest locks are immutable release assets, so a rerun at the same source adopts a complete image set, re-stages a partial one, and refuses a set that another source pushed; a published release is only verified. Aliases never regress: a release cannot take `latest` from a newer stable release, or its channel alias from a newer patch, and finalization is serialized across versions with a queued concurrency group. The old CLI matrix, image publisher, checksum loop, release job, shadow verifier, and payload manifest tool are deleted, and `main` publishes only `edge`. Two things I checked outside the repository hold: buildx returns the original index bytes when it creates a tag from a single source without annotations, so the digest the promotion step verifies is the one it pushed; and the `queue` property of a concurrency group is documented GitHub behavior, so the finalize job's `queue: max` with `cancel-in-progress: false` is valid even though actionlint 1.7.12 does not know the key yet. The suites cover reruns, partial and conflicting state, source mismatches, retries, registry ambiguity, checksum normalization, Helm reuse, and the workflow contract; all thirteen pass, and ShellCheck, Prettier, markdownlint, and cspell are clean.

## Changes made in this review

### 1. A rerun with locked images no longer fails its own verification

- **What changed**: the `goreleaser-release` Make target passes the same `GORELEASER_VERIFY_ARGS` to the post-release verifier that the snapshot target already passes, and the cutover contract test asserts that both verifier calls carry it.
- **Why**: when the image lock already exists, or the adopted image set is complete, the workflow runs GoReleaser with `--skip=docker`. The verifier then finds no image entries in `artifacts.json` and fails with "no built images found", which is exactly the message the snapshot path avoids by passing `--skip-images`. The release target never passed the flag, so the resume path this layer was built for stopped at its last step. `make -n` with `--skip=docker` showed the snapshot call with the flag and the release call without it.
- **Value**: a rerun after an interrupted CLI stage completes instead of failing after GoReleaser has already re-uploaded the assets.
- **Impact**: one line in the Makefile; the fresh-release path is unchanged because the flag is empty when Docker is not skipped.

## Findings left as-is

- **Eleven internal JSON assets on every release**: the intents, digest locks, CLI lock, core lock, and complete image lock are the durable store that makes reruns idempotent, and the parity collector allows them. Their cost is that every public release page lists them beside the seven binaries and their checksums. Folding them into one manifest needs a merge-immutable upload, one that verifies the existing sections are unchanged before appending a new one, and touches this layer, the controller, and the publication gate, so it is evaluated at PR 16, where the release manifest is introduced (cross-layer finding 2). A per-version OCI artifact in GHCR is the alternative store.
- **Concurrency `queue: max`**: valid on GitHub, rejected by the actionlint release in this environment, and not run in CI. Anyone linting locally will see the false positive until actionlint learns the key.
- **RC release notes**: an RC now publishes the prepared notes rendered from the channel boundary rather than GitHub's delta since the previous candidate, the behavior change noted at PR 12.
- **Shell style**: the OCI script and the cutover tests mix the two shfmt profiles used in this repository; nothing enforces either.
- **Draft gate**: the pull request stays in draft until an RC-plus-final cycle runs with no unexplained parity difference, which is the plan's exit criterion and cannot be satisfied by review.

## Verification

- Shell: digest capture, cutover (8), OCI artifacts (13), Helm publication, checksum normalization, parity manifest, preparation (17), and snapshot verifier suites pass; ShellCheck is clean for the eleven new or changed scripts. The new assertion fails against the unfixed Makefile and passes with the fix.
- Node: draft release (4), publication (9), and asset (12) suites pass.
- `goreleaser check` validates the configuration in the real checkout; actionlint reports only the `queue` key; Prettier with the repository configuration passes for the workflows and the Node scripts; markdownlint and cspell pass for the runbook, the chart README, and these notes.
- buildx source (`util/imagetools/create.go`): a single index source with no annotations returns the original bytes. GitHub docs (`actions-group-concurrency`): `queue: max` allows up to 100 pending runs and is incompatible only with `cancel-in-progress: true`.
