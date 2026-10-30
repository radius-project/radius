# Delegated provisioning demo

## What this proves

This is an experimental, source-built Radius prototype of the [Delegated Provisioning ADR](../../../eng/design-notes/extensibility/2026-10-delegated-provisioning-adr.md), not a production feature or an ACA deployment. Read the [implementation design](../../../eng/design-notes/extensibility/2026-10-delegated-provisioning-implementation-design.md) for the full implementation plan and the explicit prototype boundary.

The demo uses real `Radius.Compute/containers` and `Radius.Compute/routes`, a Radius.Core recipe pack, the UCP API, Dynamic RP async operations, shared recipe orchestration, and the normal Terraform Kubernetes provider. **No external watcher redeploys the target.** The script calls ordinary `rad resource create/delete`; core Radius code reruns the target recipe. The container owns a native Deployment and, while routes exist, one Service. Delegates own no native resources or Terraform state.

The single target recipe supports nginx images, three declared ports (80/8080/9090), unconditional TCP routes with one rule each, and unique ports. Its nginx configuration listens on all three ports so live HTTP-over-TCP probes verify traffic before and after route updates. This is intentionally a small demonstration recipe, not a general Kubernetes container/route recipe.

## Prerequisites and isolation

Use the preconfigured Windows host and WSL distro `radius`, Go/Make/Docker/kind/kubectl/Helm/Python 3, and a native LF source checkout. Host internet access is required to fetch public images and checksum-verified Terraform/provider releases. Azure credentials, cloud resources, role grants and global configuration changes are not needed.

These scripts are intentionally scoped to this machine and experiment:

| Item                                     | Exact identity                                                       |
|------------------------------------------|----------------------------------------------------------------------|
| Windows source worktree                  | `D:\cp\wt\radius\brooke-hamilton-studious-fortnight`                 |
| WSL native source                        | `/home/brhamilt/delegated-provisioning-deebec8c`                     |
| WSL runtime/evidence                     | `/home/brhamilt/delegated-provisioning-deebec8c-run`                 |
| kind cluster/context                     | `delegated-deebec8c` / `kind-delegated-deebec8c`                     |
| Owned Docker network                     | `delegated-net-deebec8c`                                             |
| Image registry                           | `delegated-registry-deebec8c`, loopback port `5568`                  |
| Module server                            | `delegated-recipes-deebec8c`, reachable only on owned Docker network |
| API proxy                                | Loopback port `18586`, started/stopped for each demo invocation      |
| kubeconfig / CLI HOME                    | Runtime directory's `kubeconfig` / `home`; never the user's defaults |
| Radius group / environment / application | `delegation-demo` / `demo` / `demo`                                  |
| Workload namespace                       | `delegated-demo`                                                     |

Do not substitute shared clusters, default kubeconfigs, `~/radius-mc`, or unrelated cleanup scripts. Setup creates its own Docker network because the existing shared `kind` bridge had missing forwarding rules; it does not repair or change that bridge.

## Build and run

Run from the native WSL checkout containing these changes. Before the first build, transfer the changed source files from the Windows worktree into that checkout and normalize CRLF to LF. Do not execute mounted Windows shell files. For this session, the private `files/sync.sh` helper copies exactly the changed/untracked paths received on stdin; it is not required by the product or demo.

```powershell
# Execute from the Windows source worktree.
@(git diff --name-only; git ls-files --others --exclude-standard) |
  wsl -d radius -- bash -lc 'bash ~/delegated-provisioning-deebec8c-run/sync.sh'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && bash test/experimental/delegated-provisioning/setup.sh'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning/demo.py'
```

`setup.sh` runs targeted tests, builds `rad` and applications-rp/dynamic-rp/ucpd from the supplied source, publishes only to the loopback-owned registry, verifies node image pulls, and installs the local chart into the owned cluster. Other Radius components are public upstream images, not claimed to contain the patch. It enables `RADIUS_EXPERIMENTAL_DELEGATION=true` only on the owned applications-rp and Dynamic RP deployments.

Public image layers are mirrored locally because the original bridge could not pull them. The exact Terraform `1.16.5` and Kubernetes provider `2.38.0` downloads are checked against upstream SHA256SUMS. The recipe archive includes a filesystem provider mirror and a local `terraform.rc`; a normal Radius.Core TerraformSettings resource selects it. No global `.terraformrc`, provider cache, cloud authentication or runtime credentials are copied. The archive's SHA256 is its filename; setup and demo persist its URL/hash in `evidence/recipe.json`.

**API setup shortcut:** the driver/context path is real, but recipe-pack setup uses direct UCP REST because this prototype has generated Go/Swagger registration fields, not regenerated/published Bicep packages. Group, environment, TerraformSettings and application setup use REST for deterministic isolation. Owner/route operations use the source-built `rad` CLI and normal registered schemas. Production must regenerate/publish Bicep types so the ADR's recipe-pack Bicep example compiles against the new contract.

## Expected sequence and visible evidence

The script resets only `cache`, `web`, `metrics`, and its two named negative-test routes (`invalid`/`repair`), then asserts each state and writes JSON snapshots. All updates are independent Radius CRUD operations. Recipe-pack names include the archive hash; if the fixture changes, reset drains the old target before attaching the new immutable pack.

| Step                                        | Visible result                                                                                                             |
|---------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| Create owner `cache`                        | One ready Deployment; no Service; target inventory contains Deployment.                                                    |
| Create routes `web` and `metrics`           | One Service with ports `[80, 9090]`; each route has its own computed listener and no native inventory.                     |
| Reject target deletion / invalid selections | Native workload remains; invalid operations cannot report success.                                                         |
| Correct a duplicate-port request            | Corrected route PUT reruns the target automatically, then deleting that extra route restores `[80, 9090]`.                 |
| Update owner image only                     | Native ports and both listeners remain intact.                                                                             |
| Change `web` route only                     | Native ports become `[8080, 9090]`; listener updates without an owner redeployment command; live traffic succeeds on 8080. |
| Delete `web`                                | Service retains port `[9090]`; workload UID remains unchanged.                                                             |
| Delete final route                          | Service disappears; original Deployment remains ready and target inventory contains only it.                               |

Each snapshot asserts desired/applied target hashes agree. The UID must remain unchanged within a run, not across the intentional reset/recreate between runs.

For a video, show the recipe-pack JSON, the target recipe's `incomingReferences`/`delegatedOutputs`, then the six main CRUD transitions with the corresponding Service ports, route listener and target inventory. Pause at the owner-only and route-only updates to make the single-writer behavior visible. Show the final live traffic probe and route deletion, then disclose the limitations below. No recording or external publication is part of this task.

## Re-run, inspect and reset

```powershell
# Repeating the demo resets its owned workload first.
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning/demo.py'

# Inspect retained workload, without changing global kubectl context.
wsl -d radius -- bash -lc 'KUBECONFIG=/home/brhamilt/delegated-provisioning-deebec8c-run/kubeconfig kubectl get deployments,services -n delegated-demo'

# Delete delegates first, then target, using normal Radius lifecycle.
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning/demo.py --reset'
```

`--reset` verifies both native Deployment and Service are absent. It keeps the owned control plane, recipe pack, environment, application and module artifacts for the next demo. After a successful complete run, the control plane and owner Deployment are retained; there are no routes or native ingress Service.

After the video work is finished, fully remove only this experiment:

```powershell
wsl -d radius -- bash -lc 'kind delete cluster --name delegated-deebec8c && docker rm -f delegated-recipes-deebec8c delegated-registry-deebec8c && docker network rm delegated-net-deebec8c'
```

Keep the source and evidence directories unless explicitly choosing to delete them. No broad directory deletion is needed. Stop the separately identified session keepalive, if one is retained, through its owning PowerShell session.

## Limits and failure handling

This is a one-process, sequential-CRUD prototype. Backend execution uses a process mutex, not cross-process fencing or transactional admission. Persisted bindings/tombstones and hashes are useful evidence, not a durable recovery scheduler. Do not run competing clients, scale Dynamic RP, kill workers during native applies, mutate environment/provider scope, change bound schemas or test application/scope/force-deletion bypasses and claim the ADR's production guarantees.

Recipe-pack PUT rejects definition changes once created in this prototype; identical PUTs are allowed. Pack PATCH and environment/provider/schema mutation are outside the supported demo subset, not protected migration paths. The same-pack consent/selector checks are implemented, but cross-pack registration, full schema-aware selector registration, graph-cycle detection and authorization are not. Target creation timestamps are only a weak prototype binding check, not production incarnation IDs.

The prototype has no server-owned application cascade, durable outbox/scanner, crash/uncertain-operation recovery, complete partial-inventory journal, generalized retry policy, or comprehensive error status. It supports and tests only the disclosed happy path and specified precondition correction; it does not promise rollback. A failed operation stays failed; do not infer applied state from older listeners. Preserve the error and snapshots rather than manually editing the native Service to make the demo pass.

The live demo validates Terraform/Kubernetes behavior only, not ACA, Azure deployment authentication or Bicep-driver end-to-end execution. Shared context/result additions are driver-independent, but Bicep and full production acceptance remain future work.

If a command fails, inspect the runtime directory's `setup.log`/`demo.log`, `proxy.log`, and the owned cluster's Dynamic RP logs. An interrupted Helm install may require explicitly uninstalling only the owned `radius` release before re-running setup; never repair it by touching another cluster. Do not silently skip failed assertions.

## Evidence and checks

`demo.py` persists raw resource/native snapshots and expected-error evidence under the WSL runtime directory's `evidence/`. The private session artifact directory contains copies of the final logs/snapshots and a source-provenance record. The final handoff identifies the exact successful runs and artifact location; these files contain no kubeconfig, Terraform state, credentials or login cache.

```bash
go test ./pkg/recipes/delegation ./pkg/dynamicrp/backend/controller \
  ./pkg/dynamicrp/frontend ./pkg/corerp/api/v20250801preview \
  ./pkg/corerp/frontend/controller/recipepacks \
  ./pkg/recipes/configloader ./pkg/recipes/recipecontext
```

The focused tests cover registration opt-in/consent/chains, selector grammar and cardinality, desired-input filtering/revision stability, exact delegated output identity/revision matching, shared-engine snapshot injection and read-only schema-valid output projection. They complement, not replace, the live end-to-end demonstration.
