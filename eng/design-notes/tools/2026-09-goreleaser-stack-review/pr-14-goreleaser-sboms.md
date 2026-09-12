# Review note: PR 14 - SBOM generation

- **Pull request**: [#12869](https://github.com/radius-project/radius/pull/12869)
- **Plan phase**: [PR 14](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-14-sbom-generation)
- **Stack index**: [README](./README.md)

## Verdict

The layer is the additive follow-up the plan asks for, and it is careful in the two places where an SBOM addition can quietly change a release. GoReleaser's native Syft integration produces one SPDX 2.x JSON document per raw `rad` binary, attached to the draft release under the binary's name plus `.sbom.json`, and the checksum pipeline stays restricted to the binaries, so the seven `.sha256` sidecars remain exactly the published contract; the verifier fails if a checksum for an SBOM ever appears. The five production images gain BuildKit SBOM attestations through `dockers_v2`, and every path that trusts an image lock now also requires one valid SPDX document per locked platform: after staging, at finalization, and when a published release is reconciled. The parity collector classifies the seven CLI documents as explained additions. Syft is pinned in the tool manifest with per-platform checksums verified against the upstream checksum file, installed by a script that mirrors the GoReleaser installer, and the SBOM contract suite pins the configuration, the workflow wiring, the tool pin, and the documentation together. Enrichment is narrowed to Go module data, which is the right scope for a static Go binary. The runbook explains where each kind of SBOM lives and how to read an image attestation by digest. All suites pass, `goreleaser check` validates the configuration, the generated tool metadata matches the manifest, and ShellCheck, Prettier, markdownlint, and cspell are clean.

## Changes made in this review

### 1. Syft pinned to the current patch release

- **What changed**: the manifest pins Syft v1.51.1 with the four upstream checksums, and the generated Make metadata is regenerated from it.
- **Why**: v1.51.1 shipped on 27 August, fourteen days before this review and past the manifest's seven-day cooldown, so the tool updater would have proposed it anyway. It fixes the Go remote license search, which this configuration enables through `--enrich golang`, so that standard-library modules are no longer looked up as if they were dependencies, and it remediates three vulnerabilities in Syft's own dependencies, two of them rated high.
- **Value**: the first release with SBOMs uses a scanner without a known defect in the exact code path the configuration exercises.
- **Impact**: none on the generated document structure; the contract suite only requires a Syft creator, and the installer verifies the new checksums.

## Findings left as-is

- **Network use during snapshots**: `--enrich golang` lets Syft query the Go module proxy for licenses, so every pull request snapshot now performs remote lookups for seven binaries. The runs stay well inside their budgets and the data is worth having; noted so a future timeout is not misread.
- **Image attestation scanner**: BuildKit generates the image SBOMs with its own bundled scanner, independent of the pinned Syft, which the runbook says explicitly. Pinning that scanner would mean a `generator` image reference in every `dockers_v2` entry; deferred until a reason appears.
- **Draft gate**: the pull request stays in draft until an RC draft release shows all seven CLI documents and valid attestations on every production platform, which is the plan's exit criterion.

## Verification

- Shell: SBOM contract, OCI artifacts (15, including absent, partial, and malformed attestations), snapshot verifier, parity manifest, cutover (8), and digest capture suites pass; ShellCheck is clean for the eight new or changed scripts including the installer.
- Node: release assets (15), draft release (4), and publication (9) suites pass.
- `goreleaser check` validates the configuration in the real checkout; the tool updater regenerates identical Make metadata from the manifest; every pinned checksum, before and after the bump, matches the upstream checksum file; the installer downloads and verifies v1.51.1.
- actionlint reports only the `queue` key it does not know yet; Prettier with the repository configuration passes for the workflows and the Node scripts; markdownlint and cspell pass for the runbook and these notes.
