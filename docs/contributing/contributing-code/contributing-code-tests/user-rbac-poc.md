# Testing the user RBAC prototype

## Purpose

Build this checkout and test user authorization through Kubernetes API aggregation, UCP, and the real resource providers. This is an opt-in local prototype, not a production security release. It reuses the RBAC catalog, evaluator, recoverable policy store and lifecycle-managed cache. Internal authorization comes from committed snapshot `093010a90`; the source branch is unchanged.

The [runner](../../../../hack/user-rbac/poc.sh) creates only `radius-user-rbac-poc` (or a name with that prefix), loads locally built images without pushing a registry, and keeps its kubeconfig and evidence in ignored `dist/user-rbac-poc/`. It does not change your normal kube context. Do not use this kit on a shared cluster.

## Prerequisites

- Go, GNU Make, Git, kind, kubectl, Helm, jq, and a running Docker or Podman runtime.
- Enough runtime memory for an additional Kubernetes control plane and Radius. Existing clusters remain running.
- Internet access for the pinned build base images, cert-manager, Bicep, and deployment engine. The external engine and Bicep default to `latest`; set `RBAC_POC_ENGINE_TAG` to a fixed published engine tag for repeatable engine behavior. The kit records pod image IDs.
- Permission to create a dedicated local cluster. No cloud account is used.

## Steps

### Build, install and run

From the repository root:

```sh
bash hack/user-rbac/poc.sh up
bash hack/user-rbac/poc.sh test
bash hack/user-rbac/poc.sh logs
```

`up` cross-compiles the control plane for the node architecture and builds `rad` for your host. It installs internal authorization in `enforce`, independently enables the user RBAC prototype, and explicitly accepts incomplete rollout history. That waiver is logged as high impact when changing to user `Enforce`; it is not the chart default.

The kit sets the readiness hook's image pull policy to `IfNotPresent` so kind can use the loaded `pre-upgrade` image without a registry. Normal chart installations retain `Always`.

The test uses the cluster-admin credential only to request explicit Kubernetes impersonation of `poc-admin`, `poc-reader`, `poc-developer` and `poc-outsider`. These users receive the Kubernetes `radius-api-user` gate for the actual `planes` and `providers` routes, not Radius administrator access or the separate bootstrap SAR resource. Only `poc-admin` receives the Kubernetes `radius-rbac-bootstrap` permission. UCP verifies the aggregation proxy certificate before accepting the impersonated user and groups.

The scenario runner expects a fresh policy. A second initialization returns `409 Conflict`, never overwrites policy, and never revives revoked grants. For another full run, delete only this kit's cluster and recreate it:

```sh
bash hack/user-rbac/poc.sh down
bash hack/user-rbac/poc.sh up
bash hack/user-rbac/poc.sh test
```

To repeat only the workload scenarios on an already initialized kit, run `RADIUS_USER_RBAC_RESUME=true bash hack/user-rbac/poc.sh test`. This uses the permanent admin's normal authorized APIs to restore the known test role and reader/staging-use assignments, then repeats the workload checks and revocations. It does not reset policy, rerun initialization, or retest Disabled/DryRun. Do not use it on a cluster containing policy you want to preserve unchanged.

`RBAC_POC_SKIP_BUILD=true` reuses images already loaded into an existing kit cluster; do not use it after deleting the cluster or editing Go code.

To create another cluster without replacing the original kit's kubeconfig, set a dedicated name for every command:

```sh
export RBAC_POC_CLUSTER=radius-user-rbac-poc-20261026
bash hack/user-rbac/poc.sh up
bash hack/user-rbac/poc.sh test
```

Named clusters keep credentials, ownership markers and evidence in `dist/$RBAC_POC_CLUSTER/`; the default cluster retains `dist/user-rbac-poc/`. For the manual commands below, replace the default state path and context with your named cluster's path and `kind-$RBAC_POC_CLUSTER`.

Rerunning `up` upgrades an existing kit release without deleting its policy, then restarts only the kit's control-plane deployments. The installer uses a dedicated `HOME` because the current `rad` Kubernetes helper reads `HOME/.kube/config`; your normal home and kube context are unchanged.

### Manual API calls

After the automated run, start an admin proxy in a separate terminal:

```sh
kubectl --kubeconfig "$PWD/dist/user-rbac-poc/kubeconfig" \
  --context kind-radius-user-rbac-poc \
  --as poc-admin --as-group poc-admins --as-group system:authenticated \
  proxy --port=8001
```

Start a developer proxy in another terminal with the same command, replacing `poc-admin` with `poc-developer`, `poc-admins` with `poc-developers`, and port `8001` with `8002`. Use `poc-reader`, `poc-readers`, and port `8003` for the reader. Stop each foreground proxy with Ctrl-C.

Inspect the authenticated principal (`200`) and committed changes (`200` for admin):

```sh
API=http://127.0.0.1:8001/apis/api.ucp.dev/v1alpha3
curl -i "$API/providers/Radius.Core/whoami?api-version=2025-08-01-preview"
curl -i "$API/providers/Radius.Core/accessChanges?api-version=2025-08-01-preview"
curl -i "$API/providers/Radius.Core/roleDefinitions?api-version=2025-08-01-preview"
```

Create a custom installation role through the actual policy API (`200`):

```sh
curl -i -X PUT -H 'Content-Type: application/json' \
  "$API/providers/Radius.Core/roleDefinitions/demo-reader?api-version=2025-08-01-preview" \
  -d '{"properties":{"permissions":["Applications.Core/applications/read"],"assignableScopes":["/"]}}'
```

Create an assignment (`200`); its scope, creator and management provenance are server-owned. Obtain the issuer rather than guessing it:

```sh
ISSUER=$(cat dist/user-rbac-poc/issuer)
jq -n --arg issuer "$ISSUER" \
  '{properties:{roleDefinitionId:"/providers/Radius.Core/roleDefinitions/demo-reader",principal:{type:"user",issuer:$issuer,subject:"demo-viewer"}}}' \
  > dist/user-rbac-poc/assignment.json
curl -i -X PUT -H 'Content-Type: application/json' \
  "$API/providers/Radius.Core/roleAssignments/demo-viewer?api-version=2025-08-01-preview" \
  --data-binary @dist/user-rbac-poc/assignment.json
```

Check your own access with no side effects (`200`, with an `allowed` decision independent of mode):

```sh
curl -i -X POST -H 'Content-Type: application/json' \
  "$API/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview" \
  -d '{"checks":[{"permission":"Applications.Core/applications/write","target":"/planes/radius/local/resourceGroups/rbac-poc-team/providers/Applications.Core/applications/demo"}]}'
```

Revoke the manual grant (`204`):

```sh
curl -i -X DELETE \
  "$API/providers/Radius.Core/roleAssignments/demo-viewer?api-version=2025-08-01-preview"
```

### Manual workload checks after a successful scenario run

The suite leaves its resource groups, environments, developer role and permanent administrator in place, but revokes the reader and staging-use assignments. Start the three proxies above and an outsider proxy on port `8004` using `poc-outsider` and only group `system:authenticated`. Restore the two grants to repeat workload checks:

```sh
ADMIN=http://127.0.0.1:8001/apis/api.ucp.dev/v1alpha3
DEV=http://127.0.0.1:8002/apis/api.ucp.dev/v1alpha3
READER=http://127.0.0.1:8003/apis/api.ucp.dev/v1alpha3
OUTSIDER=http://127.0.0.1:8004/apis/api.ucp.dev/v1alpha3
TEAM=/planes/radius/local/resourceGroups/rbac-poc-team
PLATFORM=/planes/radius/local/resourceGroups/rbac-poc-platform
STAGING=$PLATFORM/providers/Applications.Core/environments/staging
PRODUCTION=$PLATFORM/providers/Applications.Core/environments/production
APP=$TEAM/providers/Applications.Core/applications/manual
ISSUER=$(cat dist/user-rbac-poc/issuer)
grant() {
  jq -n --arg issuer "$ISSUER" --arg role "$2" --arg group "$3" \
    '{properties:{roleDefinitionId:("/providers/Radius.Core/roleDefinitions/"+$role),principal:{type:"group",issuer:$issuer,subject:$group}}}' |
    curl -i -X PUT -H 'Content-Type: application/json' \
      "$ADMIN$1?api-version=2025-08-01-preview" --data-binary @-
}
grant "$TEAM/providers/Radius.Core/roleAssignments/readers" reader poc-readers
grant "$STAGING/providers/Radius.Core/roleAssignments/staging-deployers" environment-deployer poc-developers
```

Both grants return `200`. `GET $DEV/providers/Radius.Core/whoami?api-version=2025-08-01-preview` shows the verified developer group. Make the staging deployment (`200`, `201` or `202`), then poll the application until its state is `Succeeded` (`200`):

```sh
jq -n --arg environment "$STAGING" \
  '{properties:{mode:"Incremental",template:{resources:[{type:"Applications.Core/applications",apiVersion:"2023-10-01-preview",name:"manual",location:"global",properties:{environment:$environment}}]}}}' \
  > dist/user-rbac-poc/manual-deployment.json
curl -i -X PUT -H 'Content-Type: application/json' \
  "$DEV$TEAM/providers/Microsoft.Resources/deployments/manual?api-version=2020-10-01" \
  --data-binary @dist/user-rbac-poc/manual-deployment.json
curl -i "$DEV$APP?api-version=2023-10-01-preview"
curl -i "$READER$APP?api-version=2023-10-01-preview"
curl -i -X PUT -H 'Content-Type: application/json' \
  "$READER$APP?api-version=2023-10-01-preview" -d '{"properties":{}}'
```

The reader GET is `200` through the group/resource-group grant; its PUT is `403 AuthorizationFailed`. An outsider GET of the same application is also `403`. Replacing staging with production must return `403` **before** either resource exists:

```sh
jq --arg environment "$PRODUCTION" \
  '.properties.template.resources[0].name="manual-denied" |
   .properties.template.resources[0].properties.environment=$environment' \
  dist/user-rbac-poc/manual-deployment.json > dist/user-rbac-poc/manual-denied.json
curl -i -X PUT -H 'Content-Type: application/json' \
  "$DEV$TEAM/providers/Microsoft.Resources/deployments/manual-denied?api-version=2020-10-01" \
  --data-binary @dist/user-rbac-poc/manual-denied.json
curl -i "$ADMIN$TEAM/providers/Microsoft.Resources/deployments/manual-denied?api-version=2020-10-01"
curl -i "$ADMIN$TEAM/providers/Applications.Core/applications/manual-denied?api-version=2023-10-01-preview"
```

The application GET must be `404`. The deployment engine returns an empty `204` for a missing deployment (a backend `404 NotFound` also means absent); no deployment document may be returned. Revoke staging-use (`204`), then submit a PATCH that omits the environment (`403`); the admin GET must show no `must-not-apply` tag:

```sh
curl -i -X DELETE "$ADMIN$STAGING/providers/Radius.Core/roleAssignments/staging-deployers?api-version=2025-08-01-preview"
curl -i -X PATCH -H 'Content-Type: application/json' \
  "$DEV$APP?api-version=2023-10-01-preview" -d '{"tags":{"changed":"must-not-apply"}}'
curl -i "$ADMIN$APP?api-version=2023-10-01-preview"
curl -i -X DELETE "$ADMIN$TEAM/providers/Radius.Core/roleAssignments/readers?api-version=2025-08-01-preview"
curl -i "$READER$APP?api-version=2023-10-01-preview"
curl -i -X DELETE "$ADMIN/providers/Radius.Core/roleAssignments/permanent-admins?api-version=2025-08-01-preview"
curl -i "$OUTSIDER$APP?api-version=2023-10-01-preview" -H 'X-Remote-User: poc-admin' -H 'X-Remote-Group: poc-admins'
```

Reader deletion returns `204`, its next GET `403`, last-administrator deletion `403 LastAdministratorProtected`, and forged outsider headers `403`. The `test` command separately port-forwards UCP and checks direct forged headers with no client certificate (`401` or `403`), then stops its own port-forward process.

### Bootstrap and modes

On a fresh installation, the test explicitly calls `POST /providers/Radius.Core/initializeAuthorization` (`201`) as the SAR-approved admin. Missing policy never silently initializes. Workloads fail `503 AuthorizationUnavailable` until that call succeeds.

Initialization creates only a temporary `Bootstrap` administrator and user mode `Disabled`. The test first adds a permanent installation `radius-administrator` assignment to group `poc-admins`. Bootstrap alone cannot satisfy the last-administrator safety rule. Enabling `Enforce` retires bootstrap assignments in the same policy generation; later rollback does not restore them.

The admin changes modes with `PUT /providers/Radius.Core/authorizationSettings/default`, body `{"properties":{"mode":"DryRun"}}` or `{"properties":{"mode":"Enforce"}}`. In `Disabled` and `DryRun`, each settings change requires a Kubernetes SAR for `update` on `api.ucp.dev/authorizationsettings`, name `default`. In `Enforce`, it requires the Radius settings permission. `Enforce` additionally requires internal component mode `enforce`, permanent administration, and the explicit POC history waiver. Returning to `Disabled` is rejected because recovery is not implemented.

For manual mode checks on a **fresh cluster instead of running the suite**, start the admin and outsider proxies. Define `ADMIN`, `OUTSIDER`, `ISSUER` and `grant` as above, but do not restore workload grants on this empty cluster. Initialize once and add permanent administration:

```sh
curl -i -X POST "$ADMIN/providers/Radius.Core/initializeAuthorization?api-version=2025-08-01-preview"
grant /providers/Radius.Core/roleAssignments/permanent-admins radius-administrator poc-admins
```

Initialization is `201`; repeating it is `409`. Before changing modes, run these in `Disabled`, then repeat after the admin's `DryRun` PUT:

```sh
curl -i -X PUT -H 'Content-Type: application/json' \
  "$OUTSIDER/providers/Radius.Core/roleDefinitions/attack?api-version=2025-08-01-preview" \
  -d '{"properties":{"permissions":["Applications.Core/applications/read"],"assignableScopes":["/"]}}'
curl -i -X PUT -H 'Content-Type: application/json' \
  "$OUTSIDER/planes/radius/local/resourceGroups/manual-mode?api-version=2023-10-01-preview" \
  -d '{"location":"global","properties":{}}'
curl -i -X PUT -H 'Content-Type: application/json' \
  "$ADMIN/providers/Radius.Core/authorizationSettings/default?api-version=2025-08-01-preview" \
  -d '{"properties":{"mode":"DryRun"}}'
```

The policy PUT is `403` in both modes; the ordinary resource-group PUT is `200` or `201`. DryRun logs `wouldDeny: true`. Changing the last body to `Enforce` as admin returns `200`; the outsider's same workload PUT is then `403`. Sending the settings PUT through `$OUTSIDER` is always `403`. Recreate the kit cluster before attempting the full fresh-policy suite.

Role and assignment writes always require Radius permissions and safety checks, including in `Disabled` and `DryRun`. Supplying `managedBy`, `createdBy`, or assignment `scope` fails with `ManagedByConflict`; a caller cannot manufacture bootstrap provenance.

## Verification

The [opt-in scenario suite](../../../../test/user-rbac/kind_test.go) makes real requests through the Kubernetes API server. Without `RADIUS_USER_RBAC_KUBECONFIG`, it skips; a skipped suite is not E2E evidence.

| Scenario                                                      | Expected result                                                                                    |
|---------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| Explicit trusted initialization                               | `201`; repeat is `409 Conflict`                                                                    |
| Permanent admin, custom role and assignments                  | `200`, with committed generations                                                                  |
| Unauthorized policy writes in all three modes                 | `403 AuthorizationFailed`                                                                          |
| Ordinary workload write without a grant in Disabled / DryRun  | Normal API success; DryRun emits `wouldDeny: true`                                                 |
| Reader group and resource-group inheritance                   | Application GET `200`; PUT `403`                                                                   |
| User without a grant reads an existing or missing application | Both `403`, not an existence leak                                                                  |
| Developer deployment referencing staging                      | Deployment accepted; actual application reaches `Succeeded`                                        |
| Deployment referencing production                             | `403` before creation; application GET `404`, absent deployment GET empty `204` or `404`           |
| PATCH after staging-use revocation, omitting environment      | `403`; tags remain unchanged                                                                       |
| Reader assignment revocation                                  | Next local request returns `403`                                                                   |
| Last permanent admin removal                                  | `403 LastAdministratorProtected`                                                                   |
| Forged remote-user headers through Kubernetes                 | No elevation; outsider receives `403`                                                              |
| Direct unverified remote headers                              | `401` or `403` at UCP; no client certificate or Kubernetes credentials are sent                    |
| Access history and audit                                      | Actor, target, generation and request ID; would-deny decisions are separate from committed changes |

PATCH uses JSON Merge Patch against a versioned GET after checking target write permission, then forwards the complete effective body to the RP's replacement-style PATCH handler. Every resulting reference is checked, including omitted ones. A returned resource ETag becomes `If-Match` to detect concurrent changes.

Deployment preflight supports **inline literal Incremental templates with 1–100 top-level Radius resources**. It authorizes every resource and reference, then converts the resource array into the real engine's Radius-import/symbolic-resource format. Missing `providerconfig` is filled with the deployment resource group's Radius and deployment scopes; supplied configuration must match exactly. Unsupported expressions, parameters, cloud resources, nested templates, copy loops, conditions, and template policy changes fail closed in `Enforce`. Authorization-sensitive JSON keys must use their canonical spelling, such as `properties.environment`, not `Properties.Environment`. Use the direct policy API for policy administration.

Execution records retain the verified originating identity and approved resource input hashes. Delegated writes must match an approved target/action/input and are reevaluated against current policy. Revocation is not bypassed by an older execution record.

Evidence stays in ignored `dist/user-rbac-poc/scenarios.log`, `ucp-audit.jsonl`, and `pods.json`. Save exact output; do not infer a deployment success from unit tests or an accepted HTTP status.

### Recorded local execution status

On 2026-10-26, a fresh installation on `radius-user-rbac-poc-20261026` passed the complete scenario suite through the real Kubernetes-aggregated API, without resume mode. All five Radius service pods are ready, and the APIService is available. Staging deployment created an application that reached `Succeeded`; production preflight returned `403` before creating either resource. Complete effective PATCH, group inheritance, reader revocation, last-admin protection, and both spoofing checks passed. Policy writes were denied in all user modes; ordinary workloads passed in Disabled and DryRun. Direct forged headers returned `401`.

Focused race tests passed across the five authorization/frontend packages. Chart validation passed 195 tests; Helm lint and shellcheck passed. This run fixed the readiness hook's registry-only pull behavior, separated the actual API gate from bootstrap SAR authority, handled writes with no reference checks, and corrected the deployment API version.

Evidence is in ignored `dist/radius-user-rbac-poc-20261026/full-e2e.log`, `final-race.log`, `chart-tests.log`, `ucp-audit.jsonl`, `engine.jsonl`, `pods.json`, and `binary-sha256.txt`. The running UCP image is `sha256:10ce0571095bbab886df5833b9f665fe57beee0f48fb8e95ccdbbf50f09b702d`. The deployment engine resolved to `ghcr.io/radius-project/deployment-engine@sha256:317fd7e5aac2783b74c27cc2919eb395c3b6f1d3172e3938976edd81526f7d2f`.

The cluster is left in user `Enforce` with permanent admin group `poc-admins`, the developer role/group assignment, staging and production environments, and the deployed `demo` application. The suite intentionally removed reader and staging-use assignments; restore them with the manual commands above, or repeat workload scenarios with:

```sh
export RBAC_POC_CLUSTER=radius-user-rbac-poc-20261026
export KUBECONFIG="$PWD/dist/$RBAC_POC_CLUSTER/kubeconfig"
RADIUS_USER_RBAC_RESUME=true bash hack/user-rbac/poc.sh test
```

Use a recreated kit cluster for the complete initialization/Disabled/DryRun flow. This build retains the authorized safe-storage foundation and internal snapshot; it is not rebased onto unrelated newer `main` changes. No commits, pushes, source-branch changes, machine-wide settings, or other clusters were modified, and the normal kube context is unchanged.

### Prototype limits

There is no `rad auth` command yet; use the exact APIs above. The POC's Kubernetes API gate lists its canonical installation routes and the `local` plane explicitly, rather than granting `*` and accidentally permitting the separate bootstrap SAR resource; other plane names need their own reviewed gate rules. Dynamic registration catalog/schema reference adaptation, durable dry-run summaries and reports, 24-hour history checks, recovery Secrets/admission, graph redaction, operation-status authorization, controller workload delegation, GitOps provenance, and full template policy transactions are not implemented. Unsupported graph/status operations fail closed in user `Enforce`; poll the actual resource instead.

The reused internal prototype still recognizes the deployment engine by pod IP instead of a client certificate and selects a recent active execution record when the engine cannot forward its ID. User preflight/input binding narrows those records but does **not** make this a production component protocol. Component read isolation and shared-store isolation remain incomplete; verified component GETs retain the prototype behavior. Records expire by TTL rather than deployment-completion closure. Do not install this prototype in an untrusted multi-tenant cluster.

## Troubleshooting

If kind times out, inspect only the dedicated node:

```sh
kubectl --kubeconfig "$PWD/dist/user-rbac-poc/kubeconfig" get nodes
podman exec radius-user-rbac-poc-control-plane \
  journalctl -u containerd -u kubelet --no-pager -n 60
```

Do not stop other clusters or change machine-wide Podman/Docker settings. A node reporting `container runtime is down` is an infrastructure blocker, not RBAC evidence.

For `503` after installation, inspect `bash hack/user-rbac/poc.sh logs`. Missing anchors require explicit initialization; stale caches require successful database refresh. Do not delete anchors to reset policy. For `403` at the Kubernetes layer, check the test user's `radius-api-user` binding and authorized impersonation separately from Radius grants.

Teardown is `bash hack/user-rbac/poc.sh down`. It checks its ownership marker and targets only its named kind cluster. It retains local logs, kubeconfig and built images; remove only individually identified artifacts if you no longer need them.
