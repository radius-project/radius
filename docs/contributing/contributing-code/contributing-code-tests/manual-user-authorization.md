# Manually verify user authorization

## Purpose

Verify the four user-facing authorization successor PRs, [#13604](https://github.com/radius-project/radius/pull/13604) (verified caller), [#13605](https://github.com/radius-project/radius/pull/13605) (policy lifecycle), [#13606](https://github.com/radius-project/radius/pull/13606) (target-only request observations), and [#13607](https://github.com/radius-project/radius/pull/13607) (inspection), using an interactive, isolated Kubernetes aggregation lab. The [launcher](../../../../hack/authz/user-lab.sh) reuses the [real aggregation fixture](../../../../pkg/ucp/frontend/api/inspection_aggregation_test.go); it does not merely run a test and exit. It leaves a real API server, etcd, and TLS UCP frontend available for commands from a second terminal.

The inherited foundation, [#13346](https://github.com/radius-project/radius/pull/13346), [#13349](https://github.com/radius-project/radius/pull/13349), and [#13354](https://github.com/radius-project/radius/pull/13354), supplies the contracts, evaluator/catalog, and safe policy storage. This lab seeds an **in-memory test policy** through an explicitly test-only verifier. It does not initialize production authorization, expose a policy writer, or install Radius. It is separate from the [internal-component authorization kit](../../../../hack/authz/README.md), which changes a cluster.

## Prerequisites

- Linux or macOS, two Bash terminals, the repository's Go toolchain, `kubectl`, and `curl`. No Docker, existing Kubernetes cluster, Helm, cloud credentials, or shared kubeconfig changes are needed. `jq` is optional for formatting JSON.
- A checkout containing all four successor layers and their inherited foundation. Use the inspection-layer checkout to verify the combined stack; do not cherry-pick inspection onto `main` without its parents.
- A source-built CLI from this checkout, using the [CLI build instructions](../contributing-code-building/). Run from the repository root:

  ```bash
  make build-rad
  export RAD_BIN="$PWD/dist/$(go env GOOS)_$(go env GOARCH)/release/rad"
  export KUBEBUILDER_ASSETS="$(go tool setup-envtest use -p path '1.30.*')"
  ```

  `setup-envtest` may download platform-specific binaries. The launcher checks for executable `kube-apiserver` and `etcd` in this directory and does not install missing prerequisites.

Only generated test identities are used: Alice is an installation administrator; Bob belongs to `developers`; `system:serviceaccount:default:inspection` receives a real Kubernetes-issued token and belongs to `system:serviceaccounts:default`. Both groups have installation-wide Reader grants. Kubernetes RBAC permits the lab's authenticated users to reach the aggregated API; **that Kubernetes permission is not a Radius administrator grant**.

## Steps

### 1. Start the isolated lab

In the first terminal, from the repository root:

```bash
bash hack/authz/user-lab.sh DryRun
```

Wait for `LAB READY mode=DryRun` and `state=Fresh running=true generation=2 lastKnownMode=DryRun`. Copy the exact `source '.../env.sh'` command printed by the launcher into the second Bash terminal. Then run:

```bash
lab_rad bob auth whoami -o json
kubectl get --raw "$API"
```

The first response names Bob, includes `developers` and `system:authenticated`, reports issuer `kubernetes:test-cluster`, mode `DryRun`, and `isRadiusAdministrator: false`. The second returns UCP discovery through the real Kubernetes API server. `lab_rad` selects the named identity's private kubeconfig and `rad` config for each invocation, without changing your normal configuration.

All listeners bind to loopback: API server and UCP use `127.0.0.1`; envtest starts loopback-only etcd. A real `APIService` uses a verified serving CA and an `ExternalName: localhost` Service, without kube-proxy. `USE_EXISTING_CLUSTER` is explicitly ignored. Resources named `ucp` and `radius-inspection` exist only inside this disposable control plane.

The launcher creates a unique `radius-user-authz.*` directory with permissions `0700`; config, control, certificate, and key files are private (`0600`). It contains **test-only front-proxy credentials capable of asserting arbitrary lab identities**: never copy them to a real cluster, print their contents, enable shell tracing, or collect the entire directory as evidence. Certificates and service-account credentials expire within about one hour; restart the lab for longer sessions.

### 2. Verify caller identity and trust (#13604)

In the second terminal:

```bash
lab_rad alice auth whoami -o json
lab_rad bob auth whoami
lab_rad serviceaccount auth whoami -o json
```

Expect Alice to be a user and administrator; Bob to be a non-administrator user; the service account to be a non-administrator workload with subject `system:serviceaccount:default:inspection` and the trusted service-account group. Table output includes TYPE, SUBJECT, ISSUER, and GROUPS.

Send forged forwarded headers **to Kubernetes**, using Bob's real client certificate:

```bash
lab_api bob -H 'X-Remote-User: alice' -H 'X-Remote-Group: admins' \
  "$KUBE_API$API/providers/Radius.Core/whoAmI?api-version=$VERSION"
```

Expect Bob and his authentic groups, not Alice or `admins`. This request exercises genuine Kubernetes authentication and front-proxy header replacement, not direct UCP header injection.

Check direct UCP rejection without a proxy certificate, with no asserted identity, and with an untrusted proxy certificate:

```bash
curl -sS --max-time 10 --cacert "$LAB/ucp-ca.crt" \
  -H 'X-Remote-User: alice' -o /dev/null -w '%{http_code}\n' \
  "$UCP$API/providers/Radius.Core/whoAmI?api-version=$VERSION"
curl -sS --max-time 10 --cacert "$LAB/ucp-ca.crt" \
  -o /dev/null -w '%{http_code}\n' \
  "$UCP$API/providers/Radius.Core/whoAmI?api-version=$VERSION"
curl -sS --max-time 10 --cacert "$LAB/ucp-ca.crt" \
  --cert "$LAB/untrusted.crt" --key "$LAB/untrusted.key" \
  -H 'X-Remote-User: alice' -o /dev/null -w '%{http_code}\n' \
  "$UCP$API/providers/Radius.Core/whoAmI?api-version=$VERSION"
```

Each returns `401`. Repeat these inspection probes after restarting in Disabled and Enforce: inspection always requires verified identity. Headerless workload compatibility is not an authenticated identity fallback.

**Automated-only:** missing request-header trust, allowed-name semantics, stripping headers before downstream routing, immutable trust snapshots, and startup validation are covered by `TestServiceRequestHeader` and the request-header unit tests. Production CA rotation requires restarting every UCP replica; this one-process lab is not a multi-replica rotation test.

### 3. Observe DryRun and protected writes (#13606)

Create a plane as Alice, then write a resource group as Bob, through the **real aggregated API**:

```bash
lab_api alice -H 'Content-Type: application/json' -X PUT \
  --data '{"location":"global","properties":{"resourceProviders":{}}}' \
  "$KUBE_API$API/planes/radius/local?api-version=2023-10-01-preview"
lab_api bob -H 'Content-Type: application/json' -X PUT \
  --data '{"location":"global"}' \
  "$KUBE_API$API/planes/radius/local/resourcegroups/test?api-version=2023-10-01-preview"
```

Both succeed and return resources with IDs `/planes/radius/local` and `/planes/radius/local/resourcegroups/test`. In the first terminal, type `events` and press Enter. Expect Alice's write to have `result: "allow"` and Bob's to have `result: "wouldDeny"`, with `coverage: "targetOnly"`, mode `DryRun`, and the current generation. Bob's denied policy check **does not block** the actual group controller. The `events` control prints only bounded observer metadata, never request bodies, tokens, keys, or arbitrary frontend logs.

Verify the intentional headerless workload compatibility path separately:

```bash
curl -sS --max-time 10 --cacert "$LAB/ucp-ca.crt" \
  -H 'Content-Type: application/json' -X PUT --data '{"location":"global"}' \
  "$UCP$API/planes/radius/local/resourcegroups/test?api-version=2023-10-01-preview"
```

In DryRun this returns the group successfully, but `events` reports `authenticated: false` and `result: "indeterminate"`. The same headerless caller still receives `401` from inspection and protected mutations. This compatibility behavior is not authentication or supported workload enforcement.

Probe an unmapped resource:

```bash
lab_api bob "$KUBE_API$API/planes/radius/local/resourcegroups/test/providers/Example.Test/widgets/a?api-version=2025-01-01"
```

Expect the ordinary routing error, not authorization enforcement. Type `events`; expect `indeterminate`, not `allow`.

All protected policy writes are unavailable, including for Alice. In the second terminal:

```bash
for method in PUT PATCH DELETE POST; do
  for target in Radius.Core/roleDefinitions/test Radius.Core/roleAssignments/test \
    Radius.Core/authorizationSettings/default Radius.Core/accessChanges/test \
    System.Authorization/records/test; do
    lab_curl -X "$method" -H 'X-Remote-User: alice' \
      -o /dev/null -w "$method $target %{http_code}\n" \
      "$UCP$API/providers/$target?api-version=$VERSION"
  done
done
```

Every response is `503`. Repeat with Bob: also `503`. Omit the `X-Remote-User` header: `401`. These are **direct boundary probes with the test proxy certificate**, separate from the real aggregated user requests above. They prove rejection, not availability of a public management API. Repeat this matrix after restarting in Disabled, Enforce, and `Unconfigured`; policy writes must never reach proxy routing, regardless of runtime availability.

**Automated-only:** unchanged deployment PUT/validate bodies, responses, and headers; simulated deployment steps returning through UCP; nil/stale runtime mutation rejection; every method/path encoding; and committed-policy preservation are covered by `TestServiceDryRunRequests`, `TestServiceProtectedMutationsEveryMode`, and `TestObserveRequests`. The lab runs real plane/group controllers, not a deployment engine. Target-only observations do not prove reference checks, PATCH merge authorization, parent-list filtering, delegation, or atomic mixed-deployment preflight.

### 4. Inspect grants and errors (#13607)

In the second terminal:

```bash
lab_rad bob auth status
lab_rad bob auth status -o json
lab_rad bob auth access check --action Radius.Core/applications/read \
  --action Radius.Core/applications/write -o json
lab_rad bob auth access check --action Radius.Core/applications/read \
  --principal user:bob -o json
lab_rad alice auth access check --action Radius.Core/applications/read \
  --principal group:developers -o json
```

Status reports `DryRun`, non-administrator status for Bob, the target-only/nonblocking warning, and recovery-management unavailability. It is read-only inspection, not recovery management. The ordered batch returns a single top-level `generation: 2` and checks `[true, false]`. A denied grant is a **successful command** with `allowed: false`. Explicit `user:bob` preserves Bob's trusted current groups, so the self read is allowed. Alice can inspect `group:developers` using that group's direct grants.

Check permission gating before another identity's membership resolution:

```bash
lab_rad bob auth access check --action Radius.Core/applications/read \
  --principal group:developers
lab_rad bob auth access check --action Radius.Core/applications/read \
  --principal user:someone-else
lab_rad alice auth access check --action Radius.Core/applications/read \
  --principal user:someone-else
```

The first two exit nonzero with `AuthorizationFailed`. The third exits nonzero with `AuthorizationIndeterminate`: Alice has `Radius.Core/checkAccess/action`, but there is no trusted resolver for another user's or workload's memberships. Unauthorized callers must not receive resolver information. The API accepts no asserted groups; group inspection does not claim nested membership expansion.

Check strict input validation:

```bash
lab_rad bob auth access check --action Radius.Core/applications/read --scope relative
lab_rad bob auth access check --action Not.A.Permission
lab_rad bob auth access check --action Radius.Core/applications/read \
  --target /planes/radius/elsewhere
lab_rad bob auth access check --action Radius.Core/applications/read \
  --target / --target /
lab_curl -H 'X-Remote-User: bob' -H 'Content-Type: application/json' \
  --data '{"checks":[{"permission":"Radius.Core/applications/read","target":"/"}],"groups":[]}' \
  "$UCP$API/providers/Radius.Core/checkAccess?api-version=$VERSION"
```

The CLI commands exit nonzero for invalid scope, unknown permission, out-of-scope target, and mismatched action/target counts. The API returns `InvalidRequestContent` for spoofed groups. `curl` does not itself fail on HTTP errors here; read the error body or add `-w '%{http_code}\n'`.

In the first terminal type `bump`. This test-only local control adds an unrelated fixture assignment, advancing generation to 3 without changing Bob's access. Then:

```bash
lab_rad bob auth access check --action rAdIuS.cOrE/aPpLiCaTiOnS/rEaD \
  --action Radius.Core/applications/write -o json
```

Expect generation 3, case-insensitive permission matching, preserved request order, and `[true, false]`. Principal type/issuer/subject tuples remain case-sensitive. **Automated-only:** concurrent refresh/mutation versus batch consistency, exact JSON serialization, malformed/nested payloads, and warning edge cases are covered by inspection and CLI unit tests; a sequential manual batch cannot prove absence of races.

### 5. Exercise policy lifecycle (#13605)

The first-terminal commands below control **only this test process**, through a private local command file populated by the launcher's stdin reader. They are not HTTP management, production configuration, or public recovery APIs.

| First-terminal control                                           | Second-terminal observation                                                                                                                                                                   |
|------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `state`                                                          | Fresh, running, current generation and last-known mode. This is diagnostic fixture state, not permission evidence.                                                                            |
| `fail-reads`, then wait at least 31 seconds and type `state`     | State becomes Unavailable; last-known mode/generation remain diagnostic. `lab_rad bob auth status -o json` exits nonzero with `AuthorizationUnavailable`, never a fabricated Disabled result. |
| While unavailable, repeat the Bob resource-group PUT from step 3 | The ordinary DryRun workload still succeeds; `events` shows an indeterminate observation, not a stale allow/wouldDeny. Protected writes remain rejected.                                      |
| `resume-reads`, wait at least 6 seconds, then `state`            | Fresh again; `lab_rad bob auth status -o json` succeeds with the original mode.                                                                                                               |
| `cancel`, then `state`                                           | Running becomes false; inspection immediately returns `AuthorizationUnavailable`. `resume-reads` cannot restart this runtime; restart the lab.                                                |

For missing-anchor startup, type `quit`, wait for cleanup, then start:

```bash
bash hack/authz/user-lab.sh DryRun --missing-anchor
```

Source the **new** printed environment command in terminal two. Expect state Unavailable, generation -1, empty last-known mode, and inspection failure. There is no invented Disabled policy or administrator. In terminal one type `initialize`: the test-only verifier seeds the fixture anchor, the real runtime retries, and state becomes Fresh at generation 2; inspection now succeeds. This is recovery of a missing **test fixture**, not a production bootstrap procedure.

**Automated-only:** the exclusive freshness boundary at exactly 30 seconds, refresh scheduling, cancellation races, startup transaction recovery/corruption, host opt-in wiring, catalog changes, and storage CAS/crash/capacity guarantees require the deterministic/runtime and foundation suites below. Wall-clock waits in this lab are observations, not exact-time proofs. Its database-failure switch affects policy reads only; it is not an etcd outage or persistent-storage recovery test.

### 6. Repeat modes and shut down safely

Stop the current lab before each restart:

```bash
bash hack/authz/user-lab.sh Disabled
bash hack/authz/user-lab.sh Enforce
bash hack/authz/user-lab.sh Unconfigured
```

Run **one command at a time in terminal one**, source its new environment in terminal two, and repeat the identity, self-check, and protected-write probes.

| Startup mode   | Expected inspection/workload behavior                                                                                                                                                                                                                        |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Disabled       | Inspection still authenticates and returns real grants: Bob's write check is false. Ordinary workload requests are not checked; no target-only workload observations are claimed.                                                                            |
| Enforce        | Inspection still works and reports Enforce with an unsupported-rollout warning. Ordinary workload requests return `503`; `kubectl get --raw "$API"` discovery stays available. This is not a supported Enforce rollout.                                      |
| `Unconfigured` | Request-header authentication remains configured, but the policy runtime is absent. Inspection returns `AuthorizationUnavailable`; fixture state is NotConfigured, generation -1. Ordinary requests retain compatibility, and policy writes remain rejected. |

Use `quit`, EOF (Ctrl-D), Ctrl-C, or SIGTERM to stop. The launcher joins its input reader and test process; test cleanup stops UCP, the runtime, API server, and etcd, then the launcher removes its private directory. Normal exits return 0; Ctrl-C returns 130 and SIGTERM returns 143. Do not use SIGKILL: no process can guarantee cleanup after it. After shutdown, `test ! -d "$LAB"` must succeed; old loopback URLs must no longer respond. Unset the lab variables or close terminal two to avoid reusing expired credentials.

## Verification

Record the checkout SHA (`git rev-parse HEAD`), mode, test identities, selected JSON/check output, bounded `events` records, lifecycle transitions, and successful cleanup. Do **not** retain credential files or dump kubeconfigs. Manual coverage is the observed matrix above, not a replacement for foundation safety or concurrency tests.

Run the matching automated suites from the repository root:

```bash
make test-user-authz-lab
go test -race ./pkg/ucp/authorization ./pkg/components/database/inmemory \
  ./pkg/ucp ./pkg/ucp/server ./pkg/ucp/frontend/api \
  ./pkg/cli/cmd/auth ./cmd/rad/cmd -short -count=1
go test -race ./pkg/ucp/frontend/api \
  -run '^TestInspectionKubernetesAggregation$' -count=1 -v
go test -race ./pkg/ucp/authorization \
  -run '^(TestPolicyStoreManifestCapacity|TestPolicyCacheRateLimitedCapacity|TestPolicyStoreAPIServer)$' \
  -count=1 -v
```

The launcher contract suite uses stubs to verify arguments, prerequisites, compile/start failure, private permissions, and quit/EOF/INT/TERM cleanup without infrastructure. The genuine aggregation test uses real envtest binaries. The interactive lab is an explicitly opted-in `*_test.go` entry point and is skipped in ordinary suites; its fixture controls cannot appear in production UCP binaries. See the [test index](./README.md#rbac-policy-storage) for backend-specific Postgres prerequisites and limits.

## Troubleshooting

- **Missing tool or binary:** follow the named prerequisite error. Build `rad` from this checkout, and set `RAD_BIN` and `KUBEBUILDER_ASSETS` explicitly. The launcher never falls back to your current cluster or a released CLI.
- **Lab not ready:** inspect the first terminal for startup errors. Do not run client probes until `LAB READY`. Local loopback TLS/port access is required; no Service endpoint on the LAN is needed.
- **Expired token/certificate, stale config, or connection refused:** restart and source the new printed environment command. Each launch uses new credentials and random ports. `lab_rad` selects the requested identity independently of the exported default Bob kubeconfig.
- **Unexpected workload error:** use the documented `lab_api` JSON requests with explicit `Content-Type`, not a generic Kubernetes object/apply command. This frontend is an aggregated ARM-style API, not a CRD schema. Unknown resource providers are not installed.
- **Unavailable after `fail-reads`:** this is intentional after freshness expires. Resume policy reads and wait for refresh; after `cancel`, restart instead. Do not interpret last-known mode as current authorization state.
- **Stopped with SIGKILL or host crash:** identify only this launcher's private path and recorded owned process IDs before any cleanup. Never use name-based process killing or delete a shared temp root. Normal/error/interrupt exits are the supported cleanup paths.
