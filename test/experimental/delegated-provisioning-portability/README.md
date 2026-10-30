# One application, Kubernetes and ACA

This follow-on demo preserves the [original Kubernetes demonstration](../delegated-provisioning/README.md) and deploys **one unchanged `app.bicep`** to two Radius environments. The application contains a server, one portable TCP route and a peer workload that verifies traffic. Only the workspace/environment selection changes; Azure IDs, native ingress fields and provider credentials do not appear in the application definition.

## Mapping and scope

| Surface                | Kubernetes                           | ACA using Bicep                     | Retained ACA using Terraform        |
|------------------------|--------------------------------------|-------------------------------------|-------------------------------------|
| Application definition | Same `app.bicep`                     | Same `app.bicep`                    | Same `app.bicep`                    |
| Target recipe          | `kubernetes/main.tf`                 | `aca-bicep/container-app.bicep`     | `aca/main.tf`                       |
| Native owner           | Deployment plus Service while routed | One native Container App per target | One native Container App per target |
| Delegate recipe        | `kind: delegated`                    | `kind: delegated`                   | `kind: delegated`                   |
| Listener               | Internal Service DNS                 | Internal ACA app name               | Internal ACA app name               |
| Peer                   | Portable Python workload             | Same portable peer                  | Same portable peer                  |

All target recipes receive `context.resource.incomingReferences` and return revision-matched `delegatedOutputs`. Only the target executes a driver and owns native inventory/state. The primary ACA example is an actual Bicep target recipe declaring `Microsoft.App/containerApps`; Radius automatically inventories that declaration. The retained Terraform example uses the standard AzureRM provider, not the previous custom field-ownership provider. No external process manually reapplies the target when routes change.

The ACA listener returns the native app name for internal TCP, as specified by [ACA ingress documentation](https://learn.microsoft.com/en-us/azure/container-apps/ingress-overview#tcp). The first FQDN-based attempt failed traffic checks and was corrected in the recipe, without changing the application definition.

The ACA slice supports **one unconditional internal TCP route** to a declared container port. It deliberately does not implement the ADR's full multi-port ACA mapping. The complete production design, distributed recovery, authorization/cycle checks and application-wide cascade remain outside the prototype. The shared Bicep definition uses no unsupported platform options.

## Prerequisites and authorization

Use the isolated runtime, native WSL checkout, source-built `rad`, local image/module servers and separate HOME/kubeconfig described by the original runbook. Do not change or use sibling runtimes. Bicep `0.42.1` was available on this machine; compilation uses `BICEP=/home/brhamilt/.local/bin/bicep`. The checked-in Bicep configuration uses the published Radius `latest` extension; the source-built edge CLI warns that full release compatibility is not verified. Successful compilation/deployment is evidence for this experiment, not a release compatibility guarantee.

The existing Azure execution principal was renewed under the user's explicit permission, with a four-hour deadline of **2026-10-28 00:28:03 UTC**, and registered only into this isolated Radius installation. The existing tenant, application and actual RG Contributor assignment were verified before renewal. No identity, role, grant or cloud scope was added, and no existing credential was removed. The renewal's secret was passed in memory to normal Radius registration, not saved in source or evidence.

ACA uses only the supplied environment `aca-proto-28500d6a`, subscription `2b04a171-9c7d-495f-9b99-96cefa291b05`, resource group `rg-radius-aca-proto-28500d6a`, East US2, with no VNet. Newly owned Terraform native names begin `delegated-deebec8c-` followed by a ten-character resource-ID hash; Bicep names use `delegated-deebec8c-b-` plus a ten-character unique suffix. Original todo/reference/custom-provider apps, the supplied environment, vaults, shared metadata and other principals/keys must not be changed.

Credential renewal and user sign-in are intentionally not performed automatically by the demo. After expiry, renew under appropriate user authorization using the private renewal helper, then repeat; never mine old secrets or copy shared runtime credentials. Read-only native inspection uses the Windows account's explicit target-tenant authentication through `aca-native.ps1`; cloud writes use Radius's registered execution principal.

## Prepare and run

Stage these source changes into `/home/brhamilt/delegated-provisioning-deebec8c` with LF endings, as in the original runbook. `prepare.sh` compiles the shared app, downloads/checks the public AzureRM `4.50.0` provider and loads the public Python image into the owned kind node. Recipe packages include their own public provider mirror; no global Terraform configuration is changed.

```powershell
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && bash test/experimental/delegated-provisioning-portability/prepare.sh'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py kubernetes'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py aca-bicep'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py aca'
```

The visible initial deployment commands are equivalent to:

```bash
rad deploy app.bicep --workspace portable-kubernetes
rad deploy app.bicep --workspace portable-aca-bicep
```

The workspaces use the same isolated Radius runtime but different Radius resource groups and environment recipe packs. `aca-bicep` publishes the compiled target recipe to the owned local OCI registry with `rad bicep publish --plain-http`, then pins its manifest digest. `aca` retains the Terraform variant. Pack/environment setup uses the explicitly documented REST registration shortcut because delegated registration Bicep types were not generated/published. **Application deployment itself uses normal Bicep and `rad deploy` on both targets, and the ACA Bicep environment also executes Bicep as its target recipe.**

Every full invocation resets only its own previous application resources first. It then deploys the shared source, independently updates the server image, independently changes the route from port 80 to 8080, checks fresh peer traffic, deletes the route, and recreates it for the retained live demo. No route command is issued during the owner-only update, and no target command is issued during the route-only update.

## Expected evidence

| Phase                 | Expected result on both platforms                                                         |
|-----------------------|-------------------------------------------------------------------------------------------|
| Shared app deployment | Server, route and peer succeed; listener/native TCP port 80; fresh peer traffic succeeds. |
| Owner-only update     | Image changes to `nginx:1.28-alpine`; port 80 and listener remain.                        |
| Route-only update     | Native port and computed listener become 8080 automatically; fresh peer traffic succeeds. |
| Route deletion        | Native ingress is removed while the same native server identity remains.                  |
| Route recreation      | Port 8080 and peer traffic return without replacing the native workload.                  |

The original `app.bicep` remains unchanged throughout. Each JSON snapshot records its SHA256; the Kubernetes and ACA values must match. Source reuse is not inferred merely from similar screenshots.

Evidence lives in `/home/brhamilt/delegated-provisioning-deebec8c-run/portability/`: per-platform deployment output, resource/native snapshots and fresh traffic logs. The final session handoff preserves copies alongside the original demo's separate immutable source/evidence archive. There are no credentials, kubeconfigs or Terraform state files in those evidence bundles.

## Reset and retention

```powershell
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py kubernetes --reset'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py aca-bicep --reset'
wsl -d radius -- bash -lc 'cd ~/delegated-provisioning-deebec8c && python3 test/experimental/delegated-provisioning-portability/demo.py aca --reset'
```

Reset deletes the route first, then the peer and server through normal Radius lifecycle, and confirms the owned native resources are absent. It leaves the experiment's environment, recipe packs and module archives for repetition. It does not delete the Azure resource group, managed environment or any protected app.

A completed full run retains server, peer and a working port-8080 route for a live demonstration. Retained ACA workloads use Consumption CPU/memory and can incur charges. Use reset when video work is finished; do not allow credential expiry to become an unreported cleanup blocker. A narrated local video, transcript and raw capture are preserved separately in session artifacts. Nothing was externally published.
