# Implementation Plan: Live Deployment Graph Progress

**Branch**: `live-graph` | **Date**: 2026-08-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/005-live-deploy-graph/spec.md`

## Summary

Add live per-resource graph updates without replacing the terminal artifact flow delivered by PR #12628. First, add `rad resource list --preview` for `Radius.Core/2025-08-01-preview` and make every Radius.Core preview graph omit the complete `properties.containers[*].env` subtree. Then the existing `run-rad-commands` action will wrap only application deploy commands with the polling sidecar proven in PR #12584, produce the existing `deploy-progress.json` contract from the preview list command, and call a bundled official `@actions/artifact` client to publish changed snapshots into an eight-slot artifact ring. The canvas will read and validate active-run snapshots, apply the highest sequence to modeled topology, and switch to the fixed-name terminal artifact when deployment ends.

This is a cross-repository contract with a same-repository safety prerequisite. The preview list and graph sanitization land first in `radius`, correcting empty terminal progress and preventing environment values from reaching any new artifact. The consumer lands next because it is backward compatible with final-only artifacts. The live producer lands only after both prerequisites are available. Each PR is split into reviewable commits, and the producer is not enabled until the consumer can safely ignore or consume its artifacts.

## Technical Context

**Languages/Versions**: Go 1.26.5 for the CLI and graph projection; Bash on `ubuntu-24.04`; Node.js 24 for the bundled artifact uploader; TypeScript/JavaScript in `radius-project/ai-extensions`
**Primary Dependencies**: Radius.Core `2025-08-01-preview` generated clients, `rad`, `jq`, official `@actions/artifact`, existing GitHub CLI/REST client in the canvas
**Storage**: GitHub Actions workflow artifacts; runner-local sequence checkpoint
**Testing**: Go unit and CLI tests, graph serialization regression tests, Bash action tests with stub binaries, Node unit tests, canvas Vitest tests, and a real Actions artifact integration workflow
**Target Platform**: GitHub.com-hosted Actions runner and Radius canvas
**Project Type**: Cross-repository workflow producer and canvas consumer
**Performance Goals**: Changed state visible within 15 seconds; no overlapping consumer polls; no more than eight live artifacts per run
**Constraints**: Reporting is best-effort; graph responses must never expose container environment maps; non-preview CLI behavior remains unchanged; artifact archives are immutable; application control plane is ephemeral; no runtime package installation; no handwritten private artifact protocol
**Scale/Scope**: One application deploy per wrapped command, normally fewer than 100 resources, eight live artifact slots, one terminal artifact

## Constitution Check

| Principle                                    | Verdict                | Notes                                                                                                                                          |
| -------------------------------------------- | ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| I. API-First Design                          | Pass                   | The preview CLI contract, safe graph projection, `deploy-progress.json`, artifact naming, identity validation, and sequence ordering are specified before implementation. |
| II. Idiomatic Code Standards                 | Pass                   | Shell lifecycle remains explicit; artifact protocol details stay inside the official TypeScript client.                                       |
| III. Multi-Cloud Neutrality                  | Pass                   | The shared action and payload are identical for Azure, AWS, and local Kubernetes deployment workflows.                                        |
| IV. Testing Pyramid Discipline               | Pass                   | Go CLI/graph tests, pure producer/consumer tests, workflow-structure tests, and one real artifact-service integration test are required.       |
| V. Collaboration-Centric Design              | Pass                   | Developers see live deployment state; platform engineers retain final diagnostics and authoritative terminal status.                          |
| VI. Open Source and Community-First          | Pass with coordination | The contract spans `radius` and `ai-extensions`; both PRs must link this spec and each other.                                                   |
| VII. Simplicity Over Cleverness              | Pass                   | Reuses existing polling, payload, terminal publisher, and canvas projection. The ring exists only to satisfy artifact immutability while bounding count. |
| VIII. Separation of Concerns                 | Pass                   | Shell owns deploy lifecycle, the Node helper owns artifact upload, and the canvas owns discovery and rendering.                               |
| IX. Incremental Adoption and Compatibility   | Pass                   | Consumer-first rollout supports legacy final-only artifacts; producer can be reverted without affecting deploys.                              |
| XVII. Polyglot Project Coherence             | Pass                   | One schema and one state vocabulary are shared across Bash, Node, and canvas code.                                                             |

## Proposed Repository Structure

### `radius-project/radius`

```text
.github/extension/actions/
├── deploy-progress/
│   ├── progress.sh
│   ├── progress_test.sh
│   └── artifact-uploader/
│       ├── package.json
│       ├── pnpm-lock.yaml
│       ├── src/upload.ts
│       ├── src/upload_test.ts
│       └── dist/index.js
├── publish-deploy-status/
│   ├── action.yml
│   └── publish-deploy-status_test.sh
└── run-rad-commands/
    ├── action.yml
    └── deploy-parameters_test.sh

.github/extension/README.md
build/test.mk
pkg/cli/cmd/resource/list/
├── list.go
├── list_test.go
└── preview/
    ├── list.go
    └── list_test.go
pkg/corerp/frontend/controller/applications/v20250801preview/
├── graph_util.go
└── graph_util_test.go
pkg/cli/graph/
├── modeled.go
└── modeled_test.go
pkg/graph/sanitize/
├── container_properties.go
└── container_properties_test.go
specs/005-live-deploy-graph/
├── plan.md
└── spec.md
```

The preview command owns Radius.Core application lookup and app-scoped resource filtering. `pkg/graph/sanitize` owns a pure cloned-property projection that removes `properties.containers[*].env`; both modeled and runtime Radius.Core preview graph paths call it before serialization or persistence. `progress.sh` owns app-name resolution, artifact base-name sanitization, state normalization, snapshot comparison, sequence checkpointing, and sidecar lifecycle functions. The uploader owns only artifact delete/upload operations. Existing composite actions source the shell helper rather than duplicating the cross-repository contract.

The exact test-file placement may be adjusted to match the existing action test harness, but deploy logic must remain locally executable.

### `radius-project/ai-extensions`

```text
adapters/canvas/src/
├── deploy.mjs
├── deploy_test.mjs
├── server.mjs
├── server_test.mjs
├── pages.mjs
└── pages_test.mjs

radius-core/src/graph/
├── deployed.ts
└── deployed_test.ts

docs/design/
└── 2026-08-live-deployment-graph-artifacts.md
```

The consumer should reuse the modeled deployed-graph projection introduced by PR #200 where practical, but replace `fetchJobLog`/`parseRadDeployProgress` as the primary transport with artifact discovery and `deploy-progress.json` validation.

## Contract Detail

### Live Artifact Name

```text
<terminal-artifact-name>-live-<github-run-id>-slot-<0..7>
```

The terminal artifact-name derivation remains unchanged. Run ID and slot are ASCII decimal values. The payload, not the name, is authoritative for sequence and identity.

### Ordering

For payloads matching the selected application, environment, and active run:

1. Reject unsupported `schemaVersion` values and malformed resources.
2. Reject a payload whose `runId` differs from the active run.
3. Select the numerically greatest `sequence`.
4. Do not apply a sequence less than or equal to the last rendered sequence.
5. Treat a valid fixed-name terminal payload as terminal only when the run identity matches; its higher sequence wins naturally.

### State Mapping

| Raw `provisioningState`            | Normalized `status` |
| ---------------------------------- | ------------------- |
| `Succeeded`                        | `success`           |
| `Failed`, `Canceled`, `Cancelled`  | `failed`            |
| Empty or any other value           | `in_progress`       |

The live run-level state is always `in_progress`. The terminal publisher retains the PR #12628 outcome mapping.

### Preview Resource Listing

```text
rad resource list --preview --application <application> --output json
```

The preview command resolves `<application>` as `<workspace-scope>/providers/Radius.Core/applications/<application>`, lists resources through the Radius.Core-compatible generic resource path, and retains resources whose `properties.application` canonically matches that ID. JSON remains a bare array so the terminal publisher and sidecar share one normalizer. The existing command without `--preview` continues to use `Applications.Core` behavior.

### Safe Graph Projection

Before a Radius.Core preview graph resource is serialized, clone its property bag and remove `env` from every entry under `properties.containers`. Apply the same pure helper to the runtime graph response and CLI modeled graph. Do not mutate stored resources or compiled template values, and do not weaken existing schema-sensitive, secure-parameter, secret-resource, or status redaction.

## Pull Request and Commit Plan

### PR 1 - `radius`: Add safe Radius.Core graph and preview resource listing

**Merge first.** This PR is independently useful: it fixes empty terminal progress for Radius.Core applications and closes the observed graph artifact credential exposure before live publication increases artifact frequency.

#### Commit 1 - Remove container environment maps from runtime graphs

- Add a pure cloned-property helper that omits `properties.containers[*].env` while preserving all other container and resource properties.
- Call it from the `Radius.Core/2025-08-01-preview` runtime graph projection before response serialization.
- Add table-driven tests for plaintext, secret-derived, null, empty, and unexpectedly shaped environment values, plus nil containers and preservation of image, ports, volumes, code references, and connections.
- Assert the input property bag is unchanged.

#### Commit 2 - Apply the same safety rule to modeled graphs

- Use the shared helper in the Radius.Core modeled graph path before local-file output or graph-archive persistence.
- Extend modeled graph tests with literal, secure-parameter, and secret-reference environment values.
- Add serialized-response assertions that recursively reject every `properties.containers[*].env` key.
- Keep `Applications.Core` graph output unchanged.

#### Commit 3 - Add `rad resource list --preview`

- Register a preview implementation following the existing `rad app graph/list/show --preview` command pattern.
- Use the Radius.Core `2025-08-01-preview` client and canonical `Radius.Core/applications` ID.
- Implement app-scoped all-resource listing required by live progress, preserving bare-array JSON output and existing output formatting conventions.
- Compare application IDs case-insensitively and do not fall back to `Applications.Core` on preview errors.
- Add tests for lookup, filtering, no resources, mixed applications, case variants, control-plane errors, JSON shape, and isolation of existing non-preview behavior.

#### Commit 4 - Correct terminal progress publication

- Change `publish-deploy-status` to call `rad resource list --preview -a "$APP_NAME" -o json`.
- Capture stderr outside the artifact directory, emit one escaped/sanitized warning on failure, and retain best-effort `resources: []` fallback behavior.
- Add a structural assertion that the publisher cannot regress to non-preview listing.
- Add an artifact safety assertion that generated `deploy-graph.json` contains no container `env` map.
- Reproduce the observed successful Radius.Core deployment fixture and assert its terminal progress contains all graph resources instead of an empty array.

#### Commit 5 - Document and validate the prerequisite contract

- Update the extension README to explain that Radius.Core deployments require preview resource listing and that graph container environment maps are intentionally omitted.
- Add focused CLI, graph, and publisher test targets to the documented validation flow.
- Record the security rationale without including any observed credential value.

**PR 1 validation**:

```bash
go test ./pkg/cli/cmd/resource/list/... ./pkg/graph/sanitize/... ./pkg/cli/graph/... ./pkg/corerp/frontend/controller/applications/v20250801preview/...
make test-publish-deploy-status
```

Run `go diagnostics` on every edited Go file, `gofmt`, the focused tests above, and shellcheck for any changed shell extracted from the composite action.

### PR 2 - `ai-extensions`: Consume sequenced live artifacts

**Merge after or in parallel with PR 1, but release before PR 3.** This PR is backward compatible and continues to render the final-only artifact from PR #12628.

#### Commit 1 - Extract and test deployed-graph status projection

- Reconcile PR #200's `projectDeployedGraph` helper with the current branch.
- Match statuses by canonical resource ID, then name plus type only when needed.
- Keep output resources excluded from the live projection.
- Add tests for duplicate names, missing IDs, unknown resources, and unchanged modeled/planned views.

#### Commit 2 - Add artifact discovery and payload validation

- Find the active workflow run for the selected application/environment using the existing deployment monitor state.
- List artifacts for that run and filter the live-name prefix plus fixed terminal name.
- Download and validate `deploy-progress.json` fixtures.
- Select the highest valid sequence independent of REST list order or ring slot.
- Preserve the existing repo-wide fixed-name lookup when there is no active run.

#### Commit 3 - Wire live polling into the Deployed graph

- Poll artifact metadata without overlapping requests.
- Download only artifact IDs not previously inspected; render only increasing sequences.
- Update the existing graph controller in place so layout does not reset on every status change.
- Stop polling on terminal artifact or terminal workflow conclusion.
- Keep modeled topology when terminal graph generation is unavailable.

#### Commit 4 - Compatibility, errors, and documentation

- Cover final-only sequence 1 artifacts, malformed live payloads, expired artifacts, API failures, run mismatches, and out-of-order responses.
- Remove job-log parsing only if no other canvas surface depends on it; otherwise mark it as a diagnostic fallback, not an authority.
- Document the consumer contract and update the changeset.
- Regenerate the shipped canvas extension bundle.

**PR 2 validation**:

```bash
pnpm -r test
pnpm -r lint
```

Use repository-specific build commands to regenerate and verify the plugin bundle.

### PR 3 - `radius`: Publish live progress from the deploy sidecar

**Merge after PR 1 and after PR 2 is released or pinned by the consuming extension.** This PR keeps the corrected preview terminal publisher and command-result behavior.

#### Commit 1 - Add the bundled artifact uploader

- Pin `@actions/artifact` and bundle the minimal upload entry point for Node 24.
- Accept artifact name, progress-file path, retention days, and replace-existing-slot flag.
- Delete only an exact slot name in the current run before replacement.
- Return structured success/failure without printing tokens or service URLs.
- Add mocked unit tests and a reproducible bundle verification command.

#### Commit 2 - Add pure progress generation and sidecar tests

- Extract app-name and artifact-name derivation shared with `publish-deploy-status`.
- Normalize bare-array `rad resource list --preview` output to the PR #12628 resource contract.
- Compare canonical JSON so key order does not create false changes.
- Add sequence checkpoint and eight-slot selection.
- Test transient poll errors, malformed JSON, unknown states, unchanged snapshots, ring wrap, and interrupted cleanup.

#### Commit 3 - Wrap only application deploy commands

- Start the sidecar immediately before the default application deploy or a validated custom deploy targeting `APP_FILE`.
- Stop and wait immediately after that deploy returns, before another command starts.
- Preserve the existing `write_result` EXIT behavior and deploy exit code.
- Do not start for unrelated commands or deploy targets.
- Extend structural tests for command arrays and success/failure paths.

#### Commit 4 - Hand off sequence to the terminal publisher

- Read the last successful live sequence from runner temp.
- Set the final payload sequence to the next value, or 1 when no live upload succeeded.
- Keep `actions/upload-artifact@v7` for the fixed-name terminal artifact.
- Extend publisher tests for no-live, live, malformed-checkpoint, failed-deploy, and final-graph-failure cases.

#### Commit 5 - Documentation and CI wiring

- Update the extension README with live artifact names, retention, ring semantics, failure behavior, and terminal handoff.
- Add focused Make targets for shell and uploader tests and include them in the existing test aggregate.
- Add dependency license output required for the checked-in Node bundle.
- Link prerequisite PR 1 and consumer PR 2, and record merge/release ordering.

**PR 3 validation**:

```bash
make test-run-rad-commands-action
make test-publish-deploy-status
make test-deploy-progress
pnpm exec prettier --check ".github/extension/actions/deploy-progress/artifact-uploader/**/*.{ts,json}"
```

Run shellcheck on all new or changed shell sources. The exact Make target name for uploader tests will be finalized during implementation.

### PR 4 - Cross-repository end-to-end proof and cleanup

This PR is optional if the proof can run safely on PR 3's branch. It must complete before declaring the feature shipped.

#### Commit 1 - Add a manual integration workflow

- Use a controlled stub or small real Radius deployment with at least two observable state transitions.
- Assert `rad resource list --preview` returns the deployed Radius.Core application resources while the equivalent non-preview lookup does not define the live progress contract.
- Assert every modeled and terminal `deploy-graph.json` payload omits `properties.containers[*].env` before uploading it.
- Assert via GitHub REST that two increasing live sequences are readable before the deploy step exits.
- Assert that the terminal sequence is higher and the fixed-name artifact remains discoverable after completion.
- Exercise one successful and one failed deployment.

#### Commit 2 - Record results and remove temporary workflow hooks

- Add measured poll-to-visibility timing and artifact count to the design notes.
- Remove any temporary `workflow_dispatch`, debug logging, or test credentials not intended for production.
- Change this spec's status from Draft to Approved only after the end-to-end result and user approval.

## Merge and Release Order

1. Approve this specification and its six explicit approval questions.
2. Merge PR 1 (`radius` preview-list and graph-safety prerequisite).
3. Merge and release PR 2 (`ai-extensions` consumer). It must continue to support the existing final-only producer.
4. Merge PR 3 (`radius` live producer) after the extension version containing PR 2 is the deployed reader.
5. Run the end-to-end proof with all exact revisions pinned.
6. Enable or retain live publishing by default after the proof meets SC-001 through SC-008.

Rollback is asymmetric by design: the producer can be reverted independently and the consumer falls back to final-only artifacts; the consumer must not be rolled back while the producer remains enabled unless the extra live artifacts are accepted as harmless unused output.

## Risks and Mitigations

| Risk                                               | Mitigation                                                                                                                            |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Artifact service internals change                  | Use pinned official `@actions/artifact`; do not hand-code Twirp or blob-upload calls.                                                 |
| Slot replacement creates a brief missing artifact  | Eight-slot ring leaves seven previous snapshots readable; sequence chooses the newest available.                                     |
| Poller changes deploy outcome                      | Disable errexit in the sidecar, isolate failures, kill and wait during cleanup, and assert deploy exit codes under fault injection.    |
| Resource identity mismatch                         | Match by canonical ID and validate app/environment/run; name plus type is compatibility-only.                                         |
| Preview command accidentally queries legacy API    | Give preview its own Radius.Core runner and tests; forbid silent fallback and structurally assert `--preview` in both publishers.       |
| Container environment value reaches a graph        | Remove the entire `env` subtree in a shared pre-serialization projection and recursively assert serialized modeled/runtime fixtures.   |
| Canvas repeatedly downloads unchanged artifacts    | Cache inspected artifact IDs and rendered sequence; list metadata first.                                                              |
| Live artifacts clutter the workflow UI             | Bound names to eight slots and retain them for one day.                                                                               |
| Terminal state races a final live upload            | Stop and wait for the sidecar before reading its checkpoint and running the terminal publisher.                                       |
| Cross-repository skew                              | Consumer-first rollout, linked PRs, legacy fixture, and exact-revision end-to-end test.                                               |

## Deferred Work

- A first-class Radius deployment-events API or CLI streaming command.
- Graph status for output resources and recipe sub-operations.
- GitHub Enterprise Server support.
- Persisted event history beyond the latest eight snapshots.
- Cancellation-specific terminal publishing and a distinct `cancelled` run state.
- Selective allow-listing of non-sensitive container environment variables in graph responses; the initial safety contract excludes the entire map.

## Complexity Tracking

| Added Complexity              | Why Required                                                                                                                     | Simpler Alternative Rejected Because                                                                                         |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Bundled Node artifact client  | A `uses:` action cannot execute mid-step, while the official client safely handles runtime authentication and upload protocol.  | Handwritten REST is brittle and runtime package installation adds latency and supply-chain variability.                       |
| Eight-slot ring               | Artifacts are immutable, repeated names conflict, and unbounded unique snapshots can exceed per-job limits and clutter runs.     | Repeated overwrite introduces deletion gaps and unstable IDs; one final artifact is not live.                                 |
| Cross-repository rollout      | Producer and renderer are separate repositories with an explicit shared payload.                                                | Merging producer and consumer independently without compatibility ordering recreates the empty-tab failure noted in PR #12628. |
| Shared graph safety projection | Runtime and modeled graph paths must enforce exactly the same unconditional container environment exclusion.                    | Duplicating deletion logic risks one graph producer or persistence path continuing to expose values.                           |
