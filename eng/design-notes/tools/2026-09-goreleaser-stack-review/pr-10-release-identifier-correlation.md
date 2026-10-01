# Review note: PR 10 - Release-identifier correlation for remote dispatch

- **Pull request**: [#12783](https://github.com/radius-project/radius/pull/12783)
- **Plan phase**: [PR 10](../2026-03-goreleaser-release-lifecycle-implementation-plan.md#pr-10-release-identifier-correlation-for-remote-dispatch)
- **Stack index**: [README](./README.md)

## Verdict

The layer does what the plan asks and implements the design's idempotency rule for remote publishers. Each caller hands the publisher a stable identifier, and the monitor looks for the exact run name before it dispatches anything: an existing successful run is reused, an active run is monitored to completion, a failed run is retried by a new dispatch under the same identifier, and only an absent run leads to a dispatch. The companion publisher change ([azure-octo/radius-publisher#25](https://github.com/azure-octo/radius-publisher/pull/25)) renders `run-name` as `<event type> / <identifier>` and keys its concurrency groups by the same identifier without cancellation, which is exactly the string and the behavior the monitor expects; dispatches without an identifier fall back to the publisher run id, so they can never collide with a release. The identifier choices fit each caller: version plus source commit for the release and Bicep dispatches, so a rerun at the same commit reuses the publication, and tag plus run id for the Deployment Engine bridge, whose dispatches must publish every time.

Discovery walks the publisher's whole `repository_dispatch` history under the monitor's deadline instead of a page cap, which is the right trade: the earlier five-page bound assumed that one identifier's runs are contiguous, and a retry days later is not. Today that costs seven requests for the Bicep publisher (607 retained runs) and two for the Deployment Engine publisher (186), well inside the budget of an App token. Payload values are emitted with `toJSON`, so an externally supplied image name, tag, or ref cannot break out of the JSON it is placed in. Transient API failures are retried with randomized exponential backoff inside one total budget, an uncertain dispatch is reconciled before any retry, and the job timeouts (18 and 25 minutes) sit above the 12-minute monitor budget. The hermetic scenarios cover reuse, retry, races, pagination, timeouts, and the caller wiring; Prettier and actionlint are clean.

## Changes made in this review

### 1. Failures name the call that failed

- **What changed**: an error that ends the monitor now says which operation failed and after how many attempts. A dispatch that exhausts its retries also says that no run with the expected name appeared and that a rerun reconciles before dispatching again, and the lookup-timeout message says that the monitor budget ran out before every history page was read. Two scenarios pin the exhausted-lookup and exhausted-dispatch messages, and the non-retryable case asserts the full text.
- **Why**: the raw GitHub error ("Not Found", "Bad Gateway") was the whole job annotation. It did not say whether the run lookup, the dispatch, or a status poll had failed, nor whether the dispatch may have been accepted, while the design asks every error summary to name the stage, the observed state, and the recovery action.
- **Value**: a release engineer reads the annotation and knows what to check and that rerunning the job is safe.
- **Impact**: messages only, no control-flow change; 19 scenarios pass.

## Findings left as-is

- **Retry by dispatch**: the design's idempotency contract says the controller "reruns a failed retryable run"; this layer dispatches a new run under the same identifier instead of calling the rerun API. The outcome is the same, it needs no `actions: write` on the publisher, which the App token does not hold, and a rerun would pin the publisher's old workflow definition. The design text needs no change.
- **Full-history discovery**: bounded by the monitor deadline, and measured above at seven and two requests per dispatch. A `created` window would cut this but would make a late resume miss its run; not worth it at this size.
- **`Capture release metadata` step in `__build-bicep-types.yaml`**: the values it copies are also available through the `env` context, so the step could go, but PR 18 edits the surrounding lines and the step keeps the identifier expression explicit in one place.
- **Node version in the unit-test job**: `make test` runs `node --test` with the runner image's Node 22 rather than the Node 24 pinned in `.node-version`; `node:test` behaves the same on both, and the lint job pins the version for Prettier.
- **Companion ordering**: the publisher pull request must be deployed before this layer merges, as the pull request body says; the publisher's current run titles are still the plain event names.

## Verification

- 19 scenarios pass; Prettier with the repository configuration passes for the script, the test, and the three workflows; actionlint passes for the three workflows.
- The companion pull request's head was read for `run-name`, the dispatch `types`, and `concurrency`; the history sizes come from the workflow-runs API of the publisher repository.
