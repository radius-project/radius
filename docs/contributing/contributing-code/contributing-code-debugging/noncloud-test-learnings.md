# Non-Cloud Functional Test Debug — Learnings

Working notes captured while stabilizing
`make test-functional-corerp-noncloud` against the local OS-process Radius
debug stack (see
[radius-os-processes-debugging.md](./radius-os-processes-debugging.md)).
They cover the bugs hit, why CI didn't catch them, and follow-ups worth
picking up. The fixes themselves landed in the PR these notes shipped with;
this file is the longer-form record so the next person doesn't have to
re-derive any of it.

## TL;DR — fixes applied

| # | Area | File | Change |
|---|---|---|---|
| 1 | Engine panic loop | [pkg/recipes/engine/engine.go](../../../../pkg/recipes/engine/engine.go) | Guard Execute/Delete double-call; treat env 404 in `deleteCore` as no-op |
| 2 | HTTP body re-read panic | [pkg/azure/clientv2/unfold.go](../../../../pkg/azure/clientv2/unfold.go) | `readResponseBody` now restores body via `io.NopCloser(bytes.NewReader(data))` so it is idempotent |
| 3 | Test cleanup index bug | [test/rp/rptest.go](../../../../test/rp/rptest.go) | Iterate cleanup in descending order to avoid skip-on-delete |
| 4 | Validation race | [test/validation/shared.go](../../../../test/validation/shared.go) | Replaced `ListEnvironments`/`ListApplications` + linear search with direct `GetEnvironment`/`GetApplication` |
| 5 | gotestsum overwrite | [build/test.mk](../../../../build/test.mk) | New `GOTESTSUM_JSONFILE_DIR` → per-target `--jsonfile=$DIR/<target>.jsonl` (uses recursive `=` so `$@` resolves per recipe) |
| 6 | Flux automation | [build/debug.mk](../../../../build/debug.mk) | `debug-install-flux` (idempotent, retried) wired into `debug-start` |
| 7 | Local Flux version | host | Removed brew 2.8.7, installed 2.6.4 binary to `~/bin/flux` to match CI ([.github/actions/install-flux/action.yaml](../../../../.github/actions/install-flux/action.yaml)) |
| 8 | Local OCI registry HTTP | [pkg/rp/util/registry.go](../../../../pkg/rp/util/registry.go), [build/scripts/start-radius.sh](../../../../build/scripts/start-radius.sh) | Recipe pulls force `PlainHTTP=true` when `RADIUS_INSECURE_LOOPBACK_REGISTRIES=true` and templatePath host is loopback. `start-radius.sh` exports the var so dlv-launched ARP/UCP/etc. inherit it. CI / unit tests / production are unaffected (env var unset → behaviour identical to before). |
| 9 | Postgres pagination TZ bug | [pkg/components/database/postgres/postgresclient.go](../../../../pkg/components/database/postgres/postgresclient.go) | `created_at > $5::TIMESTAMP` → `::TIMESTAMPTZ`. The pagination token round-trips a UTC RFC3339Nano string; casting to naive `TIMESTAMP` made postgres reinterpret it in the session's local timezone, shifting the boundary by the local UTC offset and dropping rows from page N+1. Symptom: `Applications.Core/containers` LIST returned 10/12 → `getGraph` saw empty resources → `Test_ApplicationGraph` failed. Regression test in [postgresclient_test.go](../../../../pkg/components/database/postgres/postgresclient_test.go) forces `timezone=America/Los_Angeles` on the pool. |

## Root causes

### 1. Engine panic loop (was the headline bug)

- `engine.Execute`/`Delete` re-entered the same code path on retry after a transient error, dereferencing a nil `definition` pointer at `engine.go:124`.
- Queue worker recovered the panic and re-dequeued the message → loop.
- Symptom externally: test packages taking 644s of "doing nothing" while ARP log filled with `recovering from panic`.
- Fix: short-circuit when `definition == nil` and convert env-not-found 404 in `deleteCore` to a successful no-op.

### 2. HTTP body re-read panic in Azure client

- `readResponseBody` consumed `resp.Body` but did not put it back. A downstream handler tried to read again → nil deref.
- Fix: after `io.ReadAll`, set `resp.Body = io.NopCloser(bytes.NewReader(data))`. Makes the helper idempotent.

### 3. Validation race (read-after-write)

- `ValidateRPResources` previously paginated `ListByScope` and linear-searched for the just-deployed resource.
- `rad deploy` returns "Deployment Complete" as soon as the LRO terminal status is written, but `ListByScope` reads can lag by a few ms on a long-lived local debug env (UCP store eventual visibility).
- One-shot assertion → flaky `application X was not found`.
- Why CI doesn't see it: each CI job uses a freshly-initialized env, so the list page is small/cold and consistency wins the race.
- Fix: direct `GetEnvironment`/`GetApplication` by name — same UCP node, read-after-write consistent on the resource ID.

### 4. Cleanup descending-index

- Cleanup was `for i := range resources { delete(resources[i]) }`, then mutating the slice on success → skipped neighbors.
- Fix: iterate `for i := len(resources)-1; i >= 0; i--`.

### 5. gotestsum `--jsonfile` overwrite

- `GOTESTSUM_OPTS` was a single shared string; every sub-target wrote to the same path → only the last package's JSON survived.
- Fix in [build/test.mk](../../../../build/test.mk):
  ```make
  GOTESTSUM_JSONFILE_DIR ?=
  GOTEST_TOOL = gotestsum $(GOTESTSUM_OPTS)$(if $(GOTESTSUM_JSONFILE_DIR), --jsonfile=$(GOTESTSUM_JSONFILE_DIR)/$@.jsonl) --
  ```
  Recursive `=` (not `?=`) is **required** so `$@` is expanded in each recipe's context.
- Usage: `GOTESTSUM_JSONFILE_DIR=/tmp/timings make test-functional-all-noncloud`.

### 6. Flux version mismatch

- `brew install fluxcd/tap/flux` pulls 2.8.7; CI pins 2.6.4 via `.github/actions/install-flux/install-flux.sh`.
- 2.8.7's `flux install` shipped different CRD manifests that broke `Test_Flux_Basic` against source-controller.
- Fix: uninstalled brew flux; downloaded `flux_2.6.4_darwin_arm64.tar.gz` to `~/bin/flux`; reinstalled `source-controller` in `flux-system` with the matching version.

### 7. Reproducibility: `debug-install-flux`

- `make debug-start` now installs the right Flux version into the k3d cluster automatically with retries (`DEBUG_FLUX_VERSION ?= 2.6.4`, `DEBUG_FLUX_NAMESPACE ?= flux-system`).

## Remaining known failures (not fixed; tracked separately)

| Test(s) | Failure mode | Likely cause |
|---|---|---|
| `Test_CommunicationCycle`, `Test_RedeployWith*` (mechanics) | Hang ≥1h on `rad deploy` | net/http `persistConn` waiting; suspected ARP/recipe deadlock after a previous resource is left in `Updating` state |
| `Test_Run_Logger` | Hang | `rad run --application` is long-lived by design; test lacks a timeout |
| `Test_DynamicRP_Recipe` family | `BCP053: type "userTypeAlphaProperties" does not contain property "port"` | Local `debug-env-init` registers user-defined types differently than CI's `resource-types-contrib` loader |

## Make / build lessons

- **`=` vs `?=` for `$@`**: a variable used inside a recipe that needs the current target must be `=` (recursive) or `:=` (simple, evaluated at definition time — wrong here). `?=` only sets a default but is still recursive — works too, just confusing.
- **Lazy expansion**: `$(if $(VAR), ...)` lets the same `GOTEST_TOOL` line behave correctly whether or not `GOTESTSUM_JSONFILE_DIR` is set, without duplicating the rule.

## Go / runtime lessons

- Any helper that consumes an `io.ReadCloser` from an HTTP response **must** restore it if the caller may read it again. Pattern:
  ```go
  data, err := io.ReadAll(resp.Body)
  if err != nil { return nil, err }
  _ = resp.Body.Close()
  resp.Body = io.NopCloser(bytes.NewReader(data))
  ```
- Panic recovery in a queue worker is dangerous without a poison-message break-out. The same message will be redelivered forever; pair recovery with a `dequeueCount` cap.
- `require.True(t, found, ...)` against a list result is a flake magnet; prefer direct `Get` with a 404-tolerant `require.NoErrorf` for membership checks.

## Test-suite triage tips

- Run with `GOTESTSUM_JSONFILE_DIR=/tmp/timings` then:
  ```bash
  jq -r 'select(.Action=="fail" and .Test and (.Test|contains("/")|not)) | "\(.Package|split("/")|last)\t\(.Test)\t\(.Elapsed)s"' /tmp/timings/*.jsonl
  ```
- ARP log noise check after a run:
  ```bash
  grep -c 'recovering from panic' debug_files/logs/applications-rp.log
  ```
  After fixes 1–2 this should be **0**.

## Open todos / nice-to-haves

- Add a `dequeueCount` poison-pill threshold in the ARP async worker.
- Add a `--timeout` to `rad run` in the test harness for `Test_Run_Logger`.
- Investigate the mechanics-test deadlock with a goroutine dump capture step.
- Reconcile local `debug-env-init` with CI's resource-types-contrib so dynamicrp Bicep compiles locally.
- **Make namespaces explicit in every functional test.** Today many tests rely on the *implicit* default — ARP composes `<envNamespace>-<appName>` and several tests deploy into the shared `default` env / let the workspace's default namespace win. This causes cross-test bleed in long-lived local stacks (etcd/postgres persist across `debug-start`) and silently joins names >63 chars in some combinations. Audit + fix: every `Applications.Core/environments@*` block in `test/**/testdata/*.bicep` must declare a unique `compute.namespace`, and every `Applications.Core/applications@*` block must declare a `kubernetesNamespace` extension. Add a guardrail in `test/rp/rptest.go` (or extend `testbicep_scan_test.go`) that refuses test fixtures without both.
- ~~Investigate `Test_ApplicationGraph` returning an empty graph despite a successful deploy~~ — **resolved** by fix #9 (postgres TIMESTAMPTZ pagination). Was unrelated to the implicit-namespace issue.

## Why the postgres pagination bug never tripped CI

Two conditions had to coincide:

1. **Non-UTC postgres session.** CI runs the postgres container with default `TZ=UTC`; local macOS host postgres inherits `America/Los_Angeles`. In UTC the naive `::TIMESTAMP` cast lands at the same instant as `::TIMESTAMPTZ` — drift is zero.
2. **List crosses page boundary.** The handler default is `top=10`. CI uses fresh k3d clusters per job and rarely accumulates >10 of any one resource type. Local debug stacks persist data across `debug-start` runs, so the container count grew to 33 and every list paginated.

Reproducer (live session): `SET TIME ZONE 'America/Los_Angeles'; SELECT count(*) ... created_at > '<token>'::TIMESTAMP` → 0 rows; same query with `::TIMESTAMPTZ` → 23 rows.

## Functional test timing observations (fast-follow)

From `GOTESTSUM_JSONFILE_DIR=/tmp/timings make test-functional-corerp-noncloud` after all fixes (48 pass, 2 skip, 0 fail, **wall = 2m 24s**, sum-elapsed = 21m 38s → ~9× compression from parallelism):

- **`resources` package is the long pole.** 41/48 tests, 133s wall (mechanics finishes in 56s). Splitting it into two binaries (recipes vs containers/gateways) would let a second parallel package soak up half the load and meaningfully cut wall-clock on local *and* CI.
- **Container/gateway tests cluster at ~25s.** A dozen sit in the 25–26s band — looks like a flat floor from synchronous pod-readiness polling rather than test work. Profiling one of these (e.g. shorter readiness backoff or earlier-exit on `Ready` condition) would chop seconds off many tests at once.
- **`Test_ContainerVersioning` is the single slowest at 62.7s** — 2× the median. Worth a focused look; likely sequential deploys that could overlap.
- **Three `Test_Redeploy*` mechanics tests all hit 54.5s and finish within 3ms of each other** — confirming `t.Parallel()` is working; wall-clock cost is one redeploy, not three. Don't "optimize" by serializing.
- The recently-fixed `Test_ApplicationGraph` ran in 26.9s — back to the herd, no retry/backoff cost.
- One-liner for future runs:
  ```bash
  jq -r 'select(.Action=="pass" and .Test and (.Test|contains("/")|not)) | "\(.Elapsed)\t\(.Package|split("/")|last)\t\(.Test)"' /tmp/timings/*.jsonl | sort -rn | head -20
  ```
