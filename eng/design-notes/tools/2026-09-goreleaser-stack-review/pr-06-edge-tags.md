# Review note: PR 6 - Edge tags and the `latest` deprecation

- **Pull request**: [#12750](https://github.com/radius-project/radius/pull/12750)
- **Plan phase**: [PR 6](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-6-introduce-edge-tags-and-deprecate-latest-as-edge)
- **Stack index**: [README](./README.md)

## Verdict

The layer meets the PR 6 exit criteria without code changes. The `edge` tags are manifest-only aliases of what `main` already publishes: `oras tag` for the seven CLI OCI artifacts and `docker buildx imagetools create` for every image in the Make image map, so `edge` and `latest` resolve to the same per-platform digests by construction and nothing is rebuilt. The chart's edge mapping moved from `latest` to `edge` for Radius-owned images while the Deployment Engine and dashboard keep their independently published `latest`, with explicit `de.tag`, `dashboard.tag`, and `global.imageTag` values still honored. The installers pull the edge CLI from `:edge`, the installer test drives that path through a fake `oras` instead of skipping when the real one is absent, and the chart README carries the deprecation notice the plan asks for. No other consumer of a Radius-owned `latest` tag remains in the repository; the remaining `latest` references are the Bicep types registry, mirrored third-party images, recipes, and samples, which this layer does not own.

## Changes made in this review

### 1. Pull request description

- **What changed**: the summary names the stack and the layer it depends on, and the file table lists this note.
- **Why**: consistency with the other layers.
- **Value**: accuracy only.
- **Impact**: description only; no code change.

## Findings left as-is

- **First `edge` tags appear only after the first `main` build**: the installers switch to `:edge` in the same change that starts publishing it, so `install.sh --version edge` fetched from `main` fails for the few minutes between the merge and the end of that build. Splitting the switch into a follow-up would avoid it at the cost of one more layer; not worth it for a one-time window.
- **Index digests**: `imagetools create` writes a new manifest list for the alias. The per-platform image digests are identical, which is what the parity collector compares; the list digest itself is not guaranteed to be byte-identical to `latest`. Nothing in the stack depends on it.
- **shfmt drift in `deploy/test-install.sh`**: the differences sit outside this layer's hunk and predate it.

## Verification

- `helm lint` passes and the 134 chart unit tests pass with the pinned helm-unittest plugin; the rendered default chart shows `:edge` for every Radius-owned image and `:latest` for the Deployment Engine and dashboard.
- `deploy/test-install.sh` passes all 22 tests, including the new deterministic edge test; ShellCheck is clean for both installer scripts.
- actionlint and Prettier pass for the two changed workflows; markdownlint passes for the chart README.
