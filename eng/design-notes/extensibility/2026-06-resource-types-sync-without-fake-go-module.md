# Syncing Default Resource Types Without a Fake Go Module

- **Author**: Dariusz Porowski (@DariuszPorowski)

## Overview

Radius ships a set of default resource type manifests (for example `Radius.Compute/containers`) that are registered at startup so they are available out of the box. The canonical definitions for these types live in a separate repository, [`radius-project/resource-types-contrib`](https://github.com/radius-project/resource-types-contrib), which contains only YAML manifests plus HCL/Bicep recipes - no executable Go code. The [2026-04 automated default registration design](2026-04-automated-default-resource-type-registration.md) established the current synchronization model: `deploy/manifest/defaults.yaml` lists which types ship as defaults, `make sync-resource-types` copies the chosen manifests into `deploy/manifest/built-in-providers/{dev,self-hosted}/`, the copies are committed, and a CI check fails the build if the copies drift from the pinned upstream version.

That model is sound. The problem is the **transport** it uses to fetch a pinned snapshot of upstream files: it turns `resource-types-contrib` into a **Go module** (a `go.mod` plus a placeholder `doc.go`) solely so Go's module cache can be used as a versioned file downloader, adds it to Radius's `go.mod`, and keeps it from being garbage-collected by `go mod tidy` with a blank import in `pkg/resourcetypescontrib/import.go`. The repository has no Go in it; the module exists only to game Go tooling. This is the "fake Go module" the rest of this document proposes to remove.

This design keeps everything good about the 2026-04 model - Radius-side ownership of the default set, full YAML diff visibility in Radius PRs, version pinning, and CI drift detection - and replaces only the fetch transport with a mechanism that pins and downloads files directly from the upstream repository. It proposes a pragmatic first phase (pin-by-git-ref fetch) that can be implemented today, and a strategic end state (a versioned GitHub Release asset, optionally hardened with a signed OCI artifact) that aligns with the radius core repo's [GoReleaser release-lifecycle refactor](https://github.com/radius-project/design-notes/blob/main/tools/2026-03-goreleaser-release-lifecycle.md). GoReleaser is adopted by the radius core repo only; `resource-types-contrib` keeps its own minimal release workflow. The two phases share the same on-disk skeleton, so moving from one to the other changes a single fetch step and nothing else. From the start the pin is bumped by a Dependabot-like bot that opens a reviewable PR - tracking the moving `main`/`edge`/`latest` channel now and release tags once upstream publishes them - so automated sync is part of Phase A, not a later add-on.

## Terms and definitions

| Term                    | Definition                                                                                                                                                                                               |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Fake Go module**      | A `go.mod`/`doc.go` added to a repository that contains no Go code, so that Go's module system can be used purely as a versioned download mechanism. The current transport for `resource-types-contrib`. |
| **Pseudo-version**      | A Go module version derived from a commit when no semantic tag exists, e.g. `v0.0.0-20260618174538-51ee446a8fc6`. Opaque to humans.                                                                      |
| **Pin / ref**           | An immutable pointer to a specific upstream revision: a commit SHA, a git tag, or an OCI digest (`@sha256:…`).                                                                                           |
| **Manifest bundle**     | The set of resource type YAML files selected by `defaults.yaml`, treated as one versioned, non-Go release artifact.                                                                                      |
| **Drift check**         | The CI step that re-runs the copy from the pinned source and fails if the committed copies differ.                                                                                                       |
| **OCI artifact**        | Arbitrary content (here, the manifest bundle) stored in a container registry and addressed by an immutable digest. The org already publishes recipe OCI packages to GHCR.                                |
| **`RegisterDirectory`** | The existing UCP startup path that loads every manifest under `built-in-providers/` unchanged. Out of scope for this design.                                                                             |

## Objectives

This design shares the goals of the [2026-04 automated default registration design](2026-04-automated-default-resource-type-registration.md#goals). It changes only how a pinned upstream snapshot is fetched; it does not change which types are defaults, how they are stored, or how they are loaded at runtime.

> **Issue Reference:** N/A

### Goals

1. **Remove the fake Go module.** Delete the `go.mod`/`doc.go` from `resource-types-contrib`, the `require` entry from Radius's `go.mod`/`go.sum`, and the blank-import shim `pkg/resourcetypescontrib/import.go`. Radius's Go dependency graph should contain only real Go dependencies.
2. **Preserve the 2026-04 outcome.** Keep Radius-side ownership of the default set (`defaults.yaml`), full YAML-diff visibility in Radius PRs, version pinning with an audit trail, and CI drift detection.
3. **Pin to an immutable, auditable revision** of upstream (commit SHA, tag, or OCI digest) with a readable record of what version is in use.
4. **Align with the radius GoReleaser refactor.** The mechanism must require no Go-module machinery and must model the manifest bundle as a non-Go release artifact that a thin, tag-driven workflow in `resource-types-contrib` can publish and that a small coordination PR on the radius side can bump - the shape the GoReleaser note prescribes for non-Go assets. GoReleaser itself runs only in the radius core repo, not in `resource-types-contrib`.
5. **Keep the on-disk skeleton stable across phases**, so the transport can evolve (git ref → OCI digest) without touching `defaults.yaml` semantics, the copy/prune logic, the drift check, or `RegisterDirectory`.
6. **Automate the bump as a reviewable PR from the start.** A Dependabot-like bot opens a PR that bumps `source.ref` and re-syncs - tracking the moving `main`/`edge`/`latest` channel continuously and release tags at release time - so `latest`/`edge` builds never silently lag upstream and every bump is auditable with a full YAML diff.

### Non goals

1. **Changing runtime registration.** `RegisterDirectory` and the `built-in-providers/` layout are unchanged.
2. **Changing the default set.** The list in `defaults.yaml` is unchanged by this design.
3. **Recipe / Bicep-extension publishing.** Tracked as follow-ups in the 2026-04 note; unaffected here.
4. **Mandating a Phase B artifact end state on day one.** Phase B (a release asset, or an OCI artifact) needs a minimal release workflow in `resource-types-contrib` and is sequenced with the radius core repo's GoReleaser work. Phase A delivers the core win (no fake module) immediately, with no upstream changes.

### User scenarios

The personas and scenarios are identical to the [2026-04 design](2026-04-automated-default-resource-type-registration.md#user-scenarios): a platform engineer adds a new default type, or updates an existing type's schema. The only change they observe is the command sequence in the Radius PR (no `go get`; a `ref` bump instead), described under [User Experience](#user-experience).

## User Experience

This is an internal contributor/release workflow. There is no change to the `rad` CLI, Bicep authoring, or any end-user surface.

**Sample Input - bump the pinned version and re-sync (Phase A):**

```bash
# Edit deploy/manifest/defaults.yaml: set source.ref to the new upstream commit/tag,
# and add/remove entries under defaultRegistration as needed.
make update-resource-types   # resolves latest (or uses the pinned ref) and copies files
git add deploy/manifest/
git commit -m "Update default resource types to resource-types-contrib <ref>"
```

**Sample Output:**

```text
Syncing default resource types from resource-types-contrib...
  Source: github.com/radius-project/resource-types-contrib @ v0.56.0 (sha 51ee446)
  Copied Radius.Compute/containers
  Copied Radius.Compute/persistentVolumes
  Copied Radius.Compute/routes
  Copied Radius.Data/mySqlDatabases
  Copied Radius.Security/secrets
Done. Review and commit the updated files.
```

The Radius PR diff shows the actual YAML changes inline (the visibility property the 2026-04 design intentionally chose), plus a one-line `source.ref` change - instead of an opaque `go.mod` pseudo-version bump.

In practice this command is usually run **by the sync bot, not by hand**: when `resource-types-contrib` updates, [`contrib-update-resource-types.yaml`](../../../.github/workflows/contrib-update-resource-types.yaml) opens a `bot/update-resource-types` PR that runs it and pushes the result, so the same one-line `source.ref` change and inline YAML diff arrive as a reviewable PR with CI already running. The manual command remains the fallback and the release-time path.

## Design

### High Level Design

The model keeps a fixed **skeleton** and makes the **transport** swappable:

- **Skeleton (unchanged across phases):** `defaults.yaml` declares the default set and records the upstream pin → `make sync-resource-types` copies the selected manifests into `built-in-providers/{dev,self-hosted}/` and prunes stale managed files → the copies are committed → a CI drift check re-runs the copy and fails on any diff → `RegisterDirectory` loads the committed files at startup.
- **Transport (the only thing that changes):** how `sync-resource-types` obtains the pinned snapshot of upstream files. Today: the Go module cache. Phase A: a pinned git ref fetched directly. Phase B: a versioned GitHub Release asset verified by checksum (optionally a signed OCI artifact pulled by digest).
- **Bump (automated from the start):** a Dependabot-like bot opens a reviewable PR that advances `source.ref` to the next revision on its channel - the moving `main`/`edge`/`latest` HEAD now, release tags later - and re-runs the copy. The bump is never silent: it lands as a PR with the full YAML diff, the drift check, and CI.

Because the pin is a plain string in `defaults.yaml` and the copy/prune/drift logic is transport-agnostic, swapping transports is a localized change to one Make recipe. The bump mechanism is likewise transport-agnostic - the bot rewrites the same `source.ref` string regardless of how `sync` fetches it.

### Architecture Diagram

```text
                         ┌──────────────────────────────────────────────┐
                         │  resource-types-contrib (YAML + recipes only) │
                         │  NO go.mod, NO doc.go                         │
                         └───────────────┬──────────────────────────────┘
                                         │
            Phase A: git fetch --depth 1 │ <ref>      Phase B: curl release asset + verify sha256
            (commit SHA or tag)          │            (or oras pull @digest + cosign verify)
                                         ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  radius                                                                        │
│                                                                                │
│  deploy/manifest/defaults.yaml                                                 │
│    source:                                                                     │
│      repo: github.com/radius-project/resource-types-contrib                    │
│      ref:  v0.56.0            # commit SHA (Phase A) or tag/digest (Phase B)    │
│    defaultRegistration: [ Radius.Compute/containers, ... ]                     │
│                          │                                                     │
│                          │  make sync-resource-types  (transport-agnostic)     │
│                          ▼                                                     │
│  deploy/manifest/built-in-providers/{dev,self-hosted}/                         │
│    radius_core.yaml          (manual, unchanged)                               │
│    microsoft_resources.yaml  (manual, unchanged)                               │
│    containers.yaml           (copied + committed)                              │
│    secrets.yaml              (copied + committed)   ...                        │
│                          │                                                     │
│                          │  startup                                            │
│                          ▼                                                     │
│  RegisterDirectory(...)   (existing path, unchanged)                           │
└──────────────────────────────────────────────────────────────────────────────┘
                          ▲
                          │  CI: make sync-resource-types && git diff --exit-code
                          │      (drift check - same as today, minus Go setup)
```

### Detailed Design

The following options were considered for the transport. Each lists its own advantages and disadvantages, followed by the recommended option.

#### Option 0 - Keep the fake Go module (status quo)

Add `go.mod`/`doc.go` to a non-Go repo; depend on it from Radius's `go.mod`; keep it with a blank import; fetch via the module cache.

##### Advantages

- Already implemented and working.
- `go mod download -json` gives a local cache path for free.
- A future `go.mod` bump could in principle be automated by Dependabot.

##### Disadvantages

- **Misleading repository shape.** A reader of `resource-types-contrib` sees Go scaffolding for a repo with zero Go (HCL/Bicep/YAML only).
- **Phantom Go dependency.** `go mod tidy`, SBOM generators, license scanners, vulnerability scanners, and Dependabot all treat a code-less module as a real dependency. It pollutes Radius's dependency graph and supply-chain reports without shipping any code.
- **Opaque versions.** With no upstream tags, the pin is a Go pseudo-version (`v0.0.0-20260618174538-51ee446a8fc6`) - not human-readable, no release notes, no tags.
- **A shim that exists only to defeat tooling.** `pkg/resourcetypescontrib/import.go` is a maintenance trap whose sole purpose is to stop `go mod tidy` from removing the dependency.
- **Couples unrelated supply chains.** Manifest syncing now depends on Go module resolution behavior.
- **Awkward for the GoReleaser future.** The Dependabot-bump story only works _because_ of the fake module; it is a hack riding on a hack, and it pulls a non-Go concern into the radius core repo's Go release surface that GoReleaser is meant to own.

Rejected - this is the mechanism being replaced.

#### Option 1 - Git submodule

Pin `resource-types-contrib` as a submodule at a commit; `sync` copies from the submodule working tree.

##### Advantages

- Native git, exact commit pin, full history, browseable.
- No Go involvement.

##### Disadvantages

- **Against the grain of a deliberate, recent decision.** Radius just _removed_ its `bicep-types` submodule and migrated to pnpm (see the v0.55 changelog entry "Removal of bicep-types submodule with migration to pnpm"). Reintroducing a submodule reverses that direction.
- **Contributor friction.** Detached HEAD states, `--recurse-submodules`, partial clones, and "forgot to update the submodule pointer" mistakes are exactly the pain that motivated the bicep-types removal.
- **Always-on coupling.** Every clone and CI checkout carries the submodule whether or not anyone is touching resource types.

Rejected - reverses a recent, intentional move away from submodules.

#### Option 2 - Git subtree

Vendor `resource-types-contrib` into the Radius tree with `git subtree` at a pinned commit; `sync` copies from the subtree directory.

##### Advantages

- Native git, exact commit pin, no Go involvement.
- Files are present in-tree, so there is no fetch at build time.

##### Disadvantages

- **Vendors the entire upstream repo** - every recipe and namespace, far more than the handful of manifests `defaults.yaml` selects.
- **Awkward, low-visibility updates.** `git subtree pull` produces a merge commit that folds upstream history into Radius; the manifest diff is buried rather than surfaced as a clean YAML change.
- **Same coupling problem as submodules.** It reintroduces the upstream-history entanglement the project has been moving away from.

Rejected - vendors far more than needed and buries the diff.

#### Option 3 - Pinned git-ref fetch (recommended for Phase A)

Record `repo` + `ref` (a commit SHA today, a tag later) in `defaults.yaml`. `sync-resource-types` performs a shallow, blobless fetch of that exact ref into a temp directory, copies the selected files, and prunes - then deletes the temp directory.

##### Advantages

- **No fake module, no submodule, no blank-import shim.** Removes all three smells at once.
- **Intrinsic integrity.** Git objects are content-addressed, so a SHA pin is tamper-evident without separate checksum bookkeeping.
- **Human-auditable pin.** The `ref` is visible in `defaults.yaml`; commit/tag history is browsable upstream.
- **Same tool the drift check already uses** (git), and the copy/prune/drift skeleton is reused verbatim.
- **Trivial release integration.** Bumping `ref` is a one-line edit a bot or release workflow can make.

##### Disadvantages

- **Requires `git` + network at sync time.** This is the same constraint as today's `go mod download` (which also needs the network or a warm cache); sync runs rarely (only on a bump) and CI always has both.
- **SHA pins are less readable than tags.** Mitigated by promoting to semver tags in Phase B and by annotating the SHA with a date/tag comment in the interim.
- **Shallow fetch of an arbitrary SHA depends on server support.** GitHub supports fetching reachable commit SHAs; pinning to a tag (or using the tarball fallback below) avoids the question entirely.

> **Fallback transport (no git):** download the pinned tarball `https://github.com/<org>/resource-types-contrib/archive/<ref>.tar.gz`, extract, copy, prune. Because the committed files plus the drift check are the authoritative integrity record, tarball byte-stability is irrelevant; if a fetch-time gate is desired, checksum the _extracted files being copied_, not the tarball envelope.

#### Option 4 - Pinned GitHub Release asset (recommended end state, Phase B)

When `resource-types-contrib` adopts tagged releases, its release workflow attaches the manifest bundle as a release asset (`resource-types-manifests-<tag>.tar.gz`) alongside a `checksums.txt`. Radius pins the release `tag` plus the asset `sha256` in `defaults.yaml`; `sync-resource-types` downloads the asset, verifies the checksum, extracts, copies, and prunes.

##### Advantages

- **Simple, standard release format.** A release asset plus a `checksums.txt` is a few lines of a GitHub Actions workflow (`gh release create` or `softprops/action-gh-release`), so `resource-types-contrib` needs only a minimal publish workflow - not GoReleaser. It is also the same artifact shape the radius core repo's GoReleaser emits, so the format is already familiar to maintainers.
- **Human-readable version with a checksum gate.** The pin reads as `tag v0.56.0 (sha256 abc123…)` - a real version plus a tamper-evident integrity check.
- **Tool-light.** Needs only `curl` + `sha256sum`, both already on CI runners - no registry client and no new binary.
- **No registry account or auth** for a public repo's release assets.

##### Disadvantages

- **Requires a real release process upstream** (tags + a publish workflow). Same prerequisite as the OCI option - a small, self-contained workflow in `resource-types-contrib` (the 2026-04 note's Follow-up #3), not GoReleaser.
- **Integrity without provenance.** The `sha256` proves the bytes match what was pinned, but not who produced them. Add cosign/SBOM (Option 5) when signature-level assurance is wanted.
- **Distribution is GitHub-bound.** Assets are served from GitHub Releases rather than a neutral registry - fine for this project, but a coupling to note.

#### Option 5 - Versioned, signed OCI artifact (Phase B upgrade: registry + signing)

A hardening of Option 4: instead of (or alongside) a release asset, the contrib release workflow packages the manifest bundle into an **OCI artifact** pushed to GHCR (the org already publishes recipe OCI packages there). Radius pins by immutable **digest** plus a friendly tag, pulls with `oras`/`crane`, verifies a **cosign** signature, then copies and prunes.

##### Advantages

- **Strongest supply chain.** Digest pinning, signature verification, SBOM, and provenance attestation - the same supply-chain capabilities the radius GoReleaser work is adding on the radius side - applied here by the `resource-types-contrib` publish workflow.
- **Human-readable versions.** Pin reads as `…/resource-types-manifests:v0.56.0@sha256:…`.
- **First-class non-Go artifact.** Mirrors how the radius GoReleaser model treats other non-Go assets - published by a thin workflow and pinned/coordinated separately, not built by GoReleaser itself.
- **Registry reuse.** GHCR is already used by the org for recipe packages.

##### Disadvantages

- **Requires a real release process upstream.** `resource-types-contrib` must tag releases and run a publish workflow - a called-for follow-up (2026-04 Follow-up #3). It is a small, self-contained workflow in `resource-types-contrib`, not GoReleaser, but not free.
- **New tooling on the sync path** (`oras`/`crane`, optionally `cosign`). Small, well-known binaries; still one more dependency than git.
- **More moving parts than Phase A** for the same on-disk result, justified only once signing/SBOM are actually wanted.

#### Option 6 - Automated, Dependabot-like PR sync (adopted in Phase A)

A bot opens (or refreshes) a Radius PR that runs `make update-resource-types` - resolving the upstream channel head and rewriting `source.ref` - so the pin is bumped continuously, not only by hand at release time. This already exists as [`contrib-update-resource-types.yaml`](../../../.github/workflows/contrib-update-resource-types.yaml): a `repository_dispatch` from `resource-types-contrib` on every push to its `main` opens a `bot/update-resource-types` PR. A GitHub App token makes that PR trigger CI (including the drift check) like a human PR. It tracks two channels - the moving `main`/`edge`/`latest` HEAD now, and release tags once upstream publishes them.

##### Advantages

- **Keeps `latest`/`edge` current.** Contrib `main` moves continuously; the bot keeps the lag to one reviewable PR instead of a silent gap closed only at release time.
- **Removes manual bump toil** while preserving every review gate: the bump is a PR with the full YAML diff, the drift check, and CI.
- **Reuses an existing capability** - the workflow is already in the repo.

##### Disadvantages

- **Not a transport** - it automates _when_ `update-resource-types` runs, not _how_ files are fetched. It composes with Option 3 (and later 4/5) rather than replacing them.
- **Cross-repo token/permission management** (a scoped GitHub App), plus a `release: published` trigger still to be added for the release channel.

Adopted in Phase A as the default bump mechanism - the hybrid of an immutable pin (Option 3) and automated PR sync - mirroring the GoReleaser note's "open a PR to update `versions.yaml`" coordination step.

#### Option 7 - `go:embed`

The originally proposed 2026-04 approach: embed manifests into the binary via `go:embed` from a `defaults.yaml` in the contrib repo.

##### Advantages

- No copied files in Radius.

##### Disadvantages

- **Still requires the fake Go module** (the contrib repo must be importable), so it does not meet this design's primary goal.
- **No YAML-diff visibility** in Radius PRs - the reason the 2026-04 design rejected it.

Rejected - doubly disqualified.

#### Option 8 - Runtime / install-time fetch

Skip vendoring entirely: have Helm (at install) or UCP (at startup) pull the manifest bundle from a pinned URL or OCI ref into the cluster.

##### Advantages

- No copied files in Radius at all.

##### Disadvantages

- **Violates a 2026-04 non-goal.** That design requires manifests to be committed files on disk with no network dependency at runtime. Install/startup fetch reintroduces a network dependency on the critical path and weakens air-gapped behavior.
- **No PR-time YAML visibility**, the property the copy-based model exists to provide.

Rejected - excluded by the 2026-04 "no runtime fetching" decision.

#### Option 9 - Republish to a foreign package registry (npm)

Publish the manifests as an npm package (the repo already uses pnpm for bicep-types) and pin via `package.json` + lockfile.

##### Advantages

- Reuses the Node toolchain already present for bicep-types.
- Real, human-readable semver versions.

##### Disadvantages

- **Same smell in a different ecosystem.** It makes `resource-types-contrib` a "fake npm package" of YAML, trading a phantom Go dependency for a phantom Node dependency plus a publish step.
- **Adds a Node dependency** to an otherwise tool-light sync path and to the contrib repo's release surface.

Rejected - relocates the "fake module" problem rather than removing it.

#### Proposed Option

Adopt **Option 3 (pinned git-ref fetch) now** - it needs no upstream changes and removes the fake module immediately. Adopt **Option 4 (pinned GitHub Release asset) as the end state** - published by a minimal `resource-types-contrib` release workflow (not GoReleaser) and consumed on the radius side in line with the radius GoReleaser refactor - with **Option 5 (signed OCI artifact)** as an optional registry-and-signing upgrade. Phase B is sequenced with the radius core repo's GoReleaser work and the new contrib release workflow. Adopt **Option 6 (automated, Dependabot-like PR sync) in Phase A from the start** - the hybrid of an immutable pin plus a bot that proposes each bump as a PR - tracking the moving `main`/`edge`/`latest` channel now and release tags once they exist (see [Automated sync](#automated-sync-dependabot-like-and-channels)). This supersedes only the _transport_ of the [2026-04 design](2026-04-automated-default-resource-type-registration.md); its copy-based outcome (files committed in Radius, `defaults.yaml` in Radius, drift check) is preserved exactly.

##### `defaults.yaml` schema change

Add a `source` block recording the upstream pin. The existing `defaultRegistration` list is unchanged.

```yaml
source:
  repo: github.com/radius-project/resource-types-contrib
  # Phase A: a commit SHA (immutable) or branch/tag.
  # Phase B: a release tag; a checksum/digest is recorded alongside for verification.
  ref: 51ee446a8fc6c0c0a1b2c3d4e5f6071829304152
  # Phase B only: release-asset sha256 (Option 4) or OCI digest (Option 5).
  # sha256: abc123...
defaultRegistration:
  - Radius.Compute/containers
  - Radius.Compute/persistentVolumes
  - Radius.Compute/routes
  - Radius.Data/mySqlDatabases
  - Radius.Security/secrets
```

Keeping the pin in `defaults.yaml` means a version bump touches that file and triggers the existing CI path filter. (An alternative is a separate `deploy/manifest/sources.yaml`; preferred only if multiple upstream sources appear later.)

##### Pin granularity: repository-wide vs. per-namespace versioning

The recommended `source` block above pins the **entire upstream repository** to one `ref`, so every default type is drawn from a single coherent snapshot. A finer alternative is to give each **namespace** its own pin, so namespaces advance independently. `resource-types-contrib` is laid out by namespace (`Radius.Compute/`, `Radius.Data/`, `Radius.Security/`), and these areas can evolve at different rates - so the namespace is the natural unit if independent versioning is ever wanted. (`Radius.Core` is a special case: today it ships from the manual `radius_core.yaml`, not from upstream sync, so it carries no upstream pin; were it ever sourced from the contrib repo it would slot into the same per-namespace scheme.)

**Schema (per-namespace variant).** Replace the single `source` block with a list of namespace-scoped sources; `defaultRegistration` is unchanged, and each entry resolves its files from the source whose `namespace` it matches.

```yaml
sources:
  - namespace: Radius.Compute
    repo: github.com/radius-project/resource-types-contrib
    ref: Radius.Compute/v0.56.0          # SHA (Phase A) or tag/digest (Phase B)
  - namespace: Radius.Data
    repo: github.com/radius-project/resource-types-contrib
    ref: Radius.Data/v0.42.1
  - namespace: Radius.Security
    repo: github.com/radius-project/resource-types-contrib
    ref: Radius.Security/v0.30.0
defaultRegistration:
  - Radius.Compute/containers
  - Radius.Compute/persistentVolumes
  - Radius.Compute/routes
  - Radius.Data/mySqlDatabases
  - Radius.Security/secrets
```

###### Advantages

- **Independent cadence.** A fix in one namespace ships without re-vendoring the others.
- **Smaller, clearer bumps.** Each PR moves one namespace; the diff and the `ref` change name the namespace explicitly.
- **Matches upstream ownership.** Aligns with per-namespace release ownership if `resource-types-contrib` adopts it.

###### Disadvantages

- **Needs per-namespace upstream versions to be meaningful.** It only pays off once upstream publishes per-namespace tags/assets (Phase B). In Phase A every SHA is repository-wide, so per-namespace SHAs just fetch several snapshots of one linear history for little gain.
- **More pins to track and bump.** N refs instead of one, widening the surface for a stale or mismatched pin.
- **Cross-namespace consistency risk.** If a type in one namespace depends on a type in another, independent versions can drift into an untested combination; a repository-wide pin guarantees a single coherent snapshot.
- **More fetches.** Sync must group entries by resolved ref and fetch once per distinct ref instead of once total (minor; CI has the network).

###### Recommendation

Keep the **repository-wide pin for Phase A** (this design): it is simplest, and a single linear upstream history makes per-namespace SHAs low-value. Revisit **per-namespace pinning in Phase B**, gated on `resource-types-contrib` publishing per-namespace versioned artifacts. The copy/prune/drift skeleton and `RegisterDirectory` are unaffected either way.

> **Open question: per-type pinning within a namespace.** The namespace is the finest unit proposed here, but the same mechanism can be pushed one level deeper: pin each individual _type_ rather than its namespace - for example by attaching a per-type `ref` to each `defaultRegistration` entry (`Radius.Compute/containers: v1.4.0`). It maximizes independence - a single type can be hotfixed without touching its siblings - but only pays off if upstream publishes per-type versioned artifacts (tags like `Radius.Compute/containers/v1.4.0`), turns one pin into one-per-type (potentially dozens), has no natural ownership boundary (ownership tracks the namespace directory, not the individual file), and widens the in-namespace consistency risk for types that share schemas and conventions. Recommended stance: treat the **namespace as the finest practical unit** and revisit per-type pinning only if upstream adopts independent per-type release ownership.

### How this folds into the GoReleaser release lifecycle

The [GoReleaser note](https://github.com/radius-project/design-notes/blob/main/tools/2026-03-goreleaser-release-lifecycle.md) makes releases tag-driven, demotes `versions.yaml` from automation trigger to metadata, and keeps non-Go artifacts (Helm chart, Bicep image, deployment-engine assets) as thin post-release coordination workflows. **GoReleaser is adopted by the radius core repo only - `resource-types-contrib` is not a GoReleaser repo and keeps its own minimal release workflow.** This design aligns the radius side with that lifecycle on every axis:

- **No Go-module entanglement.** Removing the fake module keeps the radius core repo's GoReleaser Go build/release graph clean; the manifest bundle is explicitly a _non-Go artifact_, the category the note keeps outside GoReleaser.
- **The pin is just a version string.** Bumping `source.ref` is the same shape as the note's "open a PR to update `versions.yaml`" coordination step - easy for a release workflow or a bot to perform.
- **Phase B reuses the note's supply-chain rails.** Digest pinning, cosign signing, SBOMs, and provenance attestation are exactly what the note wants to add; applying them to the manifest bundle shares tooling rather than inventing a parallel mechanism.
- **The end-state artifact is a standard release format.** A release asset plus a `checksums.txt` is what the radius core repo's GoReleaser already emits, so the shape is familiar - but `resource-types-contrib` produces it with a minimal publish workflow of its own, not GoReleaser. Publishing the manifest bundle as a release asset (Option 4) is the lowest-friction fit; an OCI artifact (Option 5) is the optional registry/signing upgrade. Either way, a small contrib release workflow satisfies the 2026-04 note's Follow-up #3 (tagged releases) without the Dependabot-on-a-fake-module hack.
- **`defaults.yaml` stays declarative metadata**, mirroring the note's treatment of `versions.yaml` as metadata rather than a tooling trigger.

Concretely, two separate workflows: (1) a minimal `resource-types-contrib` release workflow (not GoReleaser) attaches the manifest bundle (release asset + `checksums.txt`, optionally an OCI artifact + signature) on tag; (2) the radius core repo's GoReleaser-driven post-release coordination bumps `deploy/manifest/defaults.yaml` `source.ref`/`sha256` via PR. Only step (2) touches the radius GoReleaser plan.

### Automated sync (Dependabot-like) and channels

The pin in `defaults.yaml` is bumped by an **automated, Dependabot-like job that opens a reviewable PR**, not only by hand at release time. This is part of Phase A from the start: the immutable-pin transport (Option 3) and the automated PR sync (Option 6) are adopted together as a **hybrid** - an immutable `source.ref` plus a bot that proposes the next ref as a PR carrying the full YAML diff, the drift check, and CI.

The bot already exists as [`contrib-update-resource-types.yaml`](../../../.github/workflows/contrib-update-resource-types.yaml): a `repository_dispatch` from `resource-types-contrib` opens (or refreshes) a `bot/update-resource-types` PR that runs `make update-resource-types`, which resolves the channel head and rewrites `source.ref`. A scoped GitHub App token makes that PR trigger CI (including the drift check) exactly like a human PR.

**Channels.** `source.ref` follows one of two upstream channels, both expressed as the same immutable pin:

| Channel | `source.ref` points to | Trigger | Status |
| --- | --- | --- | --- |
| `main` / `edge` / `latest` | The upstream `main` HEAD, resolved to a commit SHA and recorded in the bump PR | `repository_dispatch` on every push to contrib `main` | In place (Phase A) |
| release | An upstream release **tag** (and, in Phase B, the asset `sha256` / OCI digest) | Contrib `release: published` | Trigger to be added; tag verification in Phase B |

The moving channel keeps Radius `latest`/`edge` builds current with contrib `main`; the release channel pins a stable upstream tag at Radius release time. The manual `make update-resource-types` in the [release guide](../../../docs/contributing/contributing-releases/README.md) is the human equivalent of the release-channel bump and remains the fallback. Both channels deliver the bump as a PR, so nothing is fetched at build or runtime without review.

**Why automate from the start.** Manual-only bumps drift: contrib `main` moves, Radius `latest` silently lags, and the gap is closed only at release time. A continuous bot PR keeps the lag to a single reviewable diff, makes every bump auditable, and runs the same drift check and CI a human bump would - so the automation is strictly the human path with the toil removed, not a new trust surface.

### Error Handling

| Scenario                                                  | Behavior                                                                            |
|-----------------------------------------------------------|-------------------------------------------------------------------------------------|
| `defaults.yaml` lists a type absent from the pinned ref   | `sync-resource-types` fails on the missing file, naming the entry. CI fails the PR. |
| `ref` is unreachable / fetch fails (network, deleted ref) | The fetch step fails with the underlying git error; the pin is not changed.         |
| Shallow SHA fetch unsupported by server                   | Pin to a tag or use the tarball fallback; documented in the recipe README.          |
| Committed copies drift from the pinned ref                | The CI drift check shows the diff and fails before merge (unchanged from today).    |
| (Phase B) signature/digest verification fails             | `cosign verify` / digest mismatch aborts the sync before any file is copied.        |
| Copied manifest is invalid or fails schema validation     | The existing startup parser/validator rejects the specific file (unchanged).        |

## Test plan

1. **`sync-resource-types` correctness:** on a clean checkout it copies exactly the files in `defaults.yaml` for the pinned `ref`; running it twice is idempotent (no diff); a bogus entry fails clearly.
2. **CI drift detection:** a PR that hand-edits a copied file without re-syncing fails; a PR that bumps `source.ref` without re-syncing fails; a clean `make update-resource-types` PR passes.
3. **Fake-module removal:** `go mod tidy` leaves no `resource-types-contrib` entry; the repo builds with `pkg/resourcetypescontrib/import.go` deleted; no reference to the module remains.
4. **Startup registration:** existing `Test_ResourceProvider_RegisterManifests` and the no-location registration test continue to pass against the copied files.
5. **(Phase B) integrity:** a tampered artifact (wrong digest or bad signature) aborts the sync.

## Security

- **Immutable pinning.** A commit SHA (Phase A) or OCI digest (Phase B) is tamper-evident; the upstream content for a pin cannot change underneath Radius.
- **Authoritative integrity record.** The committed YAML plus the CI drift check guarantee that what runs is what was reviewed - independent of the transport.
- **Phase B strengthens the chain.** cosign verification, SBOM, and provenance add signature-level assurance the fake module never provided.
- **Removes a false signal.** Today's fake module makes scanners report a Go dependency that ships no code; removing it makes Radius's supply-chain reports reflect reality.
- **Two review gates remain:** a PR in `resource-types-contrib` and a PR in `radius` with full YAML-diff visibility.

## Compatibility

- **No runtime change.** `RegisterDirectory`, the `built-in-providers/` layout, and the manual `radius_core.yaml`/`microsoft_resources.yaml` files are untouched.
- **No default-set change.** `defaultRegistration` is unchanged; only a `source` block is added.
- **Contributor workflow change.** Bumping the default set no longer involves `go get`; it is a `source.ref` edit plus `make update-resource-types`. This will be documented in the release process and the contrib README (see PR 3 below).
- **One-time cleanup.** Removing the module from `go.mod`/`go.sum` and deleting `import.go` is a mechanical, reviewable change.

## Development plan

1. **PR 1 (radius):** add the `source` block to `defaults.yaml`; rewrite `build/resource-types.mk` to fetch by pinned ref (Option 3); update `verify-resource-types.yaml` (drop Go setup and `go.mod`/`go.sum` path filters); remove the `require` line and run `go mod tidy`; delete `pkg/resourcetypescontrib/import.go`. Verify drift CI and startup tests pass.
2. **PR 2 (resource-types-contrib):** delete `go.mod` and `doc.go`.
3. **PR 3 (radius):** update the release process doc and the contrib README to describe the ref-based bump.
4. **Automated sync (Phase A, partly in place):** the [`contrib-update-resource-types.yaml`](../../../.github/workflows/contrib-update-resource-types.yaml) bot already opens a PR bumping `source.ref` on every `resource-types-contrib` `main` update (the `main`/`edge`/`latest` channel). Add a `release: published` trigger so a contrib release also opens a bump PR (the release channel). Both run `make update-resource-types`, surface the full YAML diff, and gate on the drift check + CI.
5. **Phase B (contrib release workflow + radius coordination):** add a minimal release workflow in `resource-types-contrib` (not GoReleaser) that, on tag, attaches the manifest bundle as a GitHub Release asset + `checksums.txt` (Option 4); switch the Radius fetch step to download-and-verify the asset; record `tag` + `sha256` in `defaults.yaml`, bumped by the radius post-release coordination step. Optionally harden to a signed OCI artifact (Option 5: `oras pull … @digest` + `cosign verify`/SBOM). Sequenced with the radius core repo's GoReleaser work.

## Open Questions

1. **Channel default:** track the moving `main`/`edge`/`latest` channel (the bot resolves HEAD to a commit SHA on each PR, so the committed pin stays an immutable, reproducible SHA) vs pinning a release tag. Recommended: track `main` now and switch the release channel to tags once `resource-types-contrib` publishes them.
2. **One file or two:** keep the pin in `defaults.yaml` (recommended) or split into `sources.yaml` (better if multiple sources ever appear).
3. **End-state transport:** GitHub Release asset (Option 4 - lighter, idiomatic GoReleaser) vs OCI artifact (Option 5 - registry + cosign). The Release asset is recommended as the default end state, with OCI as an opt-in upgrade.
4. **Phase B timing:** gate strictly behind `resource-types-contrib` adopting tagged releases, or stand up a minimal manifest-bundle release workflow sooner.
5. **Signing scope:** cosign keyless (OIDC) vs keyed, aligned with whatever the GoReleaser supply-chain work standardizes on.
6. **Versioning granularity:** one repository-wide pin (recommended for Phase A) vs per-namespace pins, gated on `resource-types-contrib` publishing per-namespace versioned artifacts in Phase B - and, finer still, optional per-type pinning within a namespace (left as a future option, see the note under [Pin granularity](#pin-granularity-repository-wide-vs-per-namespace-versioning)).

## Alternatives considered

| Option | Verdict | Why |
| --- | --- | --- |
| 0. Fake Go module (status quo) | Rejected | Phantom dependency, opaque pseudo-versions, shim to defeat `go mod tidy`, misleading repo shape. |
| 1. Git submodule | Rejected | Reverses the recent bicep-types submodule removal; contributor friction. |
| 2. Git subtree | Rejected | Vendors the whole upstream repo and buries the manifest diff in merge commits. |
| 3. Pinned git-ref fetch | **Phase A** | Removes all three smells; intrinsic integrity; reuses the existing skeleton; needs no upstream changes. |
| 4. Pinned GitHub Release asset | **Phase B (end state)** | Standard release format (archive + `checksums.txt`) from a minimal contrib workflow; human-readable tag + checksum; tool-light (`curl`/`sha256sum`). |
| 5. Versioned signed OCI artifact | Phase B upgrade | Adds registry distribution + cosign/SBOM over Option 4; needs `oras`/`cosign`. |
| 6. Automated, Dependabot-like PR sync | **Phase A (automation)** | Bumps `source.ref` as a reviewable PR from the start - `main`/`edge`/`latest` channel now, releases later; already implemented as `contrib-update-resource-types.yaml`. |
| 7. `go:embed` | Rejected | Still needs the fake module; no YAML-diff visibility. |
| 8. Runtime / install-time fetch | Rejected | Violates the 2026-04 "no runtime fetching" non-goal; no PR-time visibility. |
| 9. Foreign package registry (npm) | Rejected | Relocates the fake-module smell to another ecosystem. |
