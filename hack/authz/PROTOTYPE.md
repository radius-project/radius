# Internal component authorization prototype

This is a prototype of the design in
`eng/design-notes/security/2026-09-internal-component-authorization.md`. It lets
you install Radius on a local kind cluster with authorization on, and try each
check by hand. It is not production code. See [Shortcuts](#prototype-shortcuts)
and [Known gaps](#known-gaps).

## What the prototype adds

| Item  | What it does                                                                                                                                                                    |
|-------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A1–A5 | cert-manager issues each component a certificate. Every internal server requires a client certificate, and every client presents one.                                           |
| A6    | UCP trusts `X-Remote-User` only on requests that carry the API server's front-proxy certificate.                                                                                |
| B1–B6 | UCP creates an execution record for each user deploy. Writes from the deployment engine must fall inside the record. Providers check the assigned operation and its input hash. |
| C1–C3 | The controller uses its own certificate. It may only deploy from a Kubernetes namespace to the resource group mapped to it (`global.rbac.namespaceMappings`).                   |
| D1–D3 | Applications RP and Dynamic RP save and delete resources through UCP (`/authz/data/save` and `/delete`). UCP checks that the write belongs to an open operation.                |
| E1    | Environment namespaces get Pod Security `baseline` labels (`enforce` in enforce mode, `warn` and `audit` in dryRun).                                                            |
| E2    | A ValidatingAdmissionPolicy stops Radius identities from running workloads in the control-plane namespace or with control-plane service accounts or Secrets.                    |

Every check uses the mode in `global.rbac`: `off` (the default) skips checks,
`dryRun` logs `authorization check would deny request` and lets the request
through, and `enforce` rejects it with the design's error code.

## Prerequisites

- Docker, or Podman with `KIND_EXPERIMENTAL_PROVIDER=podman`
- kind 0.20 or later, kubectl, Helm 3, Go, make, and `curl`
- About 8 GB of memory for the kind node

## Setup (one command)

```bash
hack/authz/prototype-up.sh enforce   # or: hack/authz/prototype-up.sh dryRun
```

This builds `rad` and every image from this checkout for your CPU architecture.
It creates the kind cluster `radius-authz` and a local registry on
`localhost:5001`, then installs Radius with cert-manager. It maps the `default`
namespace to the `default` resource group for the controller. The script prints
each component's logged mode at the end. A full build takes 10–20 minutes.

Useful options:

- `AUTHZ_CLUSTER_NAME=<name>` uses another cluster name.
- `--skip-build --tag <tag>` reinstalls without rebuilding. The last tag is in
  `dist/authz-kit/image-tag`.
- Run it again with the other mode to switch modes on the same cluster.

Then set up a `rad` workspace that does not touch `~/.rad`:

```bash
export AUTHZ_CLUSTER_NAME=radius-authz          # match the setup command
rad() { "$PWD/dist/authz-kit/rad" --config "$PWD/dist/authz-kit/rad.yaml" "$@"; }
rad workspace create kubernetes proto --context "kind-${AUTHZ_CLUSTER_NAME}" --group default --force
rad group create default
```

The scenarios use these helpers in `hack/authz/`:

| Helper                                   | Use                                                                    |
|------------------------------------------|------------------------------------------------------------------------|
| `rogue.sh exec -- [flags] URL`           | Call a URL from a pod with no Radius certificate.                      |
| `as-component.sh NAME -- [flags] URL`    | Call a URL presenting component `NAME`'s certificate.                  |
| `records.sh [list\|get\|create\|revoke]` | Show or create execution records (`create` is a prototype test hook).  |
| `would-deny.sh`                          | Print every would-deny line in dryRun. Exits non-zero if it finds any. |

Each call prints the response body and ends with `radius-rogue-status:<code>`.

## Scenarios (enforce)

Run these with the cluster installed in `enforce`. Run all commands from the
repository root.

1. **A normal deploy succeeds.**

   ```bash
   (cd hack/authz/prototype && rad deploy app.bicep)
   hack/authz/records.sh list
   kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" get ns proto-env --show-labels
   ```

   Expected: `Deployment Complete` with `proto-env`, `proto-app` and `web`. The
   record list shows active records with submitter `ucp` and `bicep-de`. The
   `proto-env` namespace has `pod-security.kubernetes.io/enforce=baseline`.

2. **A workload with no certificate cannot call a provider.**

   ```bash
   hack/authz/rogue.sh exec -- -k \
     'https://applications-rp.radius-system.svc:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'
   ```

   Expected: `remote error: tls: certificate required`.

3. **A forged `X-Remote-User` is rejected.** UCP port 443 is the API server
   aggregation port. Only the API server's front-proxy certificate may set
   `X-Remote-*`.

   ```bash
   U='https://ucp.radius-system.svc/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/default?api-version=2023-10-01-preview'
   hack/authz/rogue.sh exec -- -k -H 'X-Remote-User: kubernetes-admin' -H 'X-Remote-Group: system:masters' "$U"
   hack/authz/as-component.sh dynamic-rp -- -k -H 'X-Remote-User: kubernetes-admin' "$U"
   ```

   Expected: `401` with `InvalidAuthenticationInfo` for both calls.

4. **A dev record cannot be reused against prod.** The record covers only the
   `dev` resource group.

   ```bash
   rad group create dev && rad group create prod
   hack/authz/records.sh create dev-rec hack/authz/prototype/dev-record.json
   B='{"location":"global","properties":{"compute":{"kind":"kubernetes","namespace":"devns"}}}'
   for rg in dev prod; do
     hack/authz/as-component.sh bicep-de -- -X PUT -H 'Content-Type: application/json' \
       -H 'X-Radius-Execution-Record: dev-rec' --data "$B" \
       "https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/$rg/providers/Applications.Core/environments/e1?api-version=2023-10-01-preview"
   done
   ```

   Expected: `dev` returns `200`. `prod` returns `403 GrantScopeExceeded`.

5. **A tampered operation body is rejected.** The record's operation assigns
   `tamper-app` to applications-rp with a fixed body. The call sends the same
   body plus a tag.

   ```bash
   OP=$(hack/authz/records.sh create tamper hack/authz/prototype/tamper-record.json |
     python3 -c 'import json,sys; print(json.load(sys.stdin)["operations"][0]["id"])')
   ENV=/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/proto-env
   RID=/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/tamper-app
   hack/authz/as-component.sh ucp -- -X PUT -H 'Content-Type: application/json' \
     -H 'X-Radius-Execution-Record: tamper' -H "X-Radius-Operation-Id: $OP" \
     --data "{\"location\":\"global\",\"tags\":{\"tampered\":\"true\"},\"properties\":{\"environment\":\"$ENV\"}}" \
     "https://applications-rp.radius-system.svc:5443$RID?api-version=2023-10-01-preview"
   ```

   Expected: `409 OperationInputMismatch`.

6. **A direct write that no operation assigns is denied.** First at the
   provider, then at UCP's data-access endpoint, which the providers now use
   for every save.

   ```bash
   hack/authz/as-component.sh ucp -- -X PUT -H 'Content-Type: application/json' \
     --data "{\"location\":\"global\",\"properties\":{\"environment\":\"$ENV\"}}" \
     "https://applications-rp.radius-system.svc:5443${RID%/*}/unassigned-app?api-version=2023-10-01-preview"
   hack/authz/as-component.sh applications-rp -- -X POST -H 'Content-Type: application/json' \
     --data '{"id":"/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/hijack-test","data":{"hijacked":true}}' \
     https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3/authz/data/save
   ```

   Expected: `403 OperationNotAssigned` for both calls.

7. **The controller is rejected outside its mapping.** Only `default` maps to
   resource group `default`.

   ```bash
   hack/authz/prototype/deployment-template.sh default default   # mapped
   hack/authz/prototype/deployment-template.sh default prod      # wrong group
   hack/authz/prototype/deployment-template.sh team-x default    # unmapped namespace
   sleep 20
   kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" get deploymenttemplates -A
   kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" get events -A --field-selector involvedObject.kind=DeploymentTemplate
   ```

   Expected: `default/dt-default` is `Ready`. `default/dt-prod` is `Failed` with
   `GrantScopeExceeded`. `team-x/dt-default` is `Failed` with `AuthorizationFailed`.
   Delete them when done: `kubectl delete deploymenttemplates -A --all`.

8. **A privileged pod is rejected in an environment namespace, and Radius
   identities cannot run workloads in the control plane.**

   ```bash
   kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" -n proto-env run priv --image=busybox --restart=Never --privileged -- sleep 1
   kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" -n radius-system create deployment evil --image=busybox \
     --as=system:serviceaccount:radius-system:applications-rp
   ```

   Expected: the first is forbidden with `violates PodSecurity "baseline:latest"`.
   The second is denied by `radius-control-plane-workloads` with
   `AdmissionPolicyDenied: application workloads may not run in the Radius control-plane namespace`.

## Scenario 9: dryRun logs instead of denying

```bash
hack/authz/prototype-up.sh dryRun --skip-build --tag "$(cat dist/authz-kit/image-tag)"
(cd hack/authz/prototype && rad deploy app.bicep)
hack/authz/would-deny.sh            # expect: no would-deny lines, exit 0
```

Switching from enforce removes the `enforce` Pod Security label from
`proto-env` on the next deploy. Then repeat scenarios 2–8. Run
`hack/authz/would-deny.sh` after each one, or watch the component's log. Each
scenario succeeds and logs exactly one `authorization check would deny request`
line with the code below. Scenario 8 gets a Kubernetes warning instead of a log
line.

| Scenario                     | dryRun result                                                                             | Logged by         | `authzCode`                 |
|------------------------------|-------------------------------------------------------------------------------------------|-------------------|-----------------------------|
| 2. No certificate            | `radius-rogue-status:200`                                                                 | `applications-rp` | `PeerCertificateInvalid`    |
| 3. Forged `X-Remote-User`    | `200` for both calls                                                                      | `ucp` (each call) | `InvalidAuthenticationInfo` |
| 4. Dev record against prod   | `dev`: `200`, no line. `prod`: `200`                                                      | `ucp`             | `GrantScopeExceeded`        |
| 5. Tampered body             | `200`                                                                                     | `applications-rp` | `OperationInputMismatch`    |
| 6. Unassigned write (RP)     | `200`                                                                                     | `applications-rp` | `OperationNotAssigned`      |
| 6. Unassigned write (UCP)    | `200`                                                                                     | `ucp`             | `OperationNotAssigned`      |
| 7. Controller, wrong group   | `dt-prod` is `Ready`                                                                      | `ucp`             | `GrantScopeExceeded`        |
| 7. Controller, unmapped ns   | `team-x/dt-default` is `Ready`                                                            | `ucp`             | `AuthorizationFailed`       |
| 8. Privileged pod            | `Warning: would violate PodSecurity "baseline:latest"` then `pod/priv created`            | Kubernetes        | none                        |
| 8. Control-plane deployment  | `Warning: ... AdmissionPolicyDenied: application workloads may not run ...` then created  | Kubernetes        | none                        |

`dt-default` in `default` and the dev call in scenario 4 log nothing. In
dryRun the write really happens, so clean up afterwards:

```bash
kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" -n proto-env delete pod priv
kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" -n radius-system delete deployment evil
kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" delete deploymenttemplates -A --all
```

To see one line per check, run this after each scenario:

```bash
kubectl --context "kind-${AUTHZ_CLUSTER_NAME}" -n radius-system logs deploy/ucp --since=1m | grep 'would deny'
```

Delete the cluster with `kind delete cluster --name "${AUTHZ_CLUSTER_NAME}"`.

## Prototype shortcuts

These are deliberate and marked `PROTOTYPE` in the code.

- **Deployment engine has no certificate.** The engine cannot present a
  certificate until A7 lands in the engine repository. UCP lets a request with
  no certificate on port 443 through when it comes from the engine pod's IP,
  and logs `authzPrototypeExemption=engine-no-cert`.
- **Engine writes bind to a record by deployment.** The engine does not send
  record IDs (B7). UCP binds an engine write with no record header to the newest
  active `bicep-de` record whose targets cover the write
  (`engine-record-binding`).
- **Test hook for records.** UCP serves
  `/planes/radius/local/providers/System.Authorization/executionRecords` so
  `records.sh` can list, create and revoke records, including operations.
- **UCP's own async work skips the operation claim.** UCP's background work for
  tracked resources has no operation.
- **dryRun writes continue unbound.** In dryRun a write that fails the record
  or operation check continues with an unbound record, so later checks for the
  same request do not log a second line.
- **In dryRun the controller does not check its mapping.** UCP checks the same
  mapping on the deployment write and logs the would-deny line.

## Known gaps

- Records are not closed when a deployment finishes. They stay active until
  their TTL.
- POST actions, operation status reads, UCP-native resources and
  `createResourceGroupIfNotExists` are not checked against records.
- Providers still claim and finish their own operations in the
  `System.Authorization` store directly, and keep their own database
  credentials. Data access through UCP relies on a shared store. Per-service
  Postgres databases are not done.
- The admission policy matches Radius identities only. Pod Security labels are
  set when Radius creates or patches a namespace, not on existing namespaces.
- Not built: A7–A11 (engine certificate, other certificate modes, external
  mTLS), B7, D4–D8 (cloud broker, remote targets), E3 and the rollout stack.
