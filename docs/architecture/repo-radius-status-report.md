# Repo Radius: implementation and remaining work

Status as of October 14, 2026.


This document assesses what work is complete for Repo Radius today and what work (according to open GitHub issues) is still outstanding. This document is intended to help us (eng + pm) discuss whether that work is necessary at this time. If yes, then product will own the prioritization.


## Repo Radius Today

The following features have already been implemented for Repo Radius:

- **Temporary control-plane lifecycle:** Start Radius, restore state, run commands, save state, and tear down. The foundation landed in [radius-project/radius#12214](https://github.com/radius-project/radius/pull/12214).
- **External-cluster deployment:** Radius manages workloads outside its own cluster. The foundation landed in [radius-project/radius#12106](https://github.com/radius-project/radius/pull/12106); provider workflows connect to AKS/EKS. See the [Azure workflow](https://github.com/radius-project/ai-extensions/blob/f48eba0d02fdaa7147acefe209e4ac84fc015023/.github/extension/run-rad-commands-azure.yml). This does not establish support for every cluster configuration.
- **Durable OCI state:** A later control plane can restore state saved by an earlier run. [radius-project/radius#12364](https://github.com/radius-project/radius/pull/12364) merged July 17. The [archive factory](https://github.com/radius-project/radius/blob/5e65916a3a6eac2a6a25d0014db05aa2ec2157bf/pkg/statearchive/factory/factory.go) still supports only OCI, not Git, Azure Blob, or S3 archives. See the [archive documentation](https://github.com/radius-project/radius/blob/5e65916a3a6eac2a6a25d0014db05aa2ec2157bf/docs/architecture/state-archive.md).
- **Cloud verification and OIDC authentication:** Provider workflows check cloud access and configure deployment credentials. Verification work landed in [radius-project/radius#12170](https://github.com/radius-project/radius/pull/12170). Provisioning cloud-side identities is separate.
- **Custom types and recipe packs:** Support landed in [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367), merged July 25. [radius-project/radius#12742](https://github.com/radius-project/radius/pull/12742), merged August 24, fixed recipe-pack reconciliation on repeated deployments.
- **Application and environment deletion:** Delete workflows landed in [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367) and moved with the other workflow assets.
- **Command results:** The [command action](https://github.com/radius-project/ai-extensions/blob/f48eba0d02fdaa7147acefe209e4ac84fc015023/.github/extension/actions/run-rad-commands/action.yml) emits versioned results in command order under `rad-commands-result`. The verification result artifact and distinct state-save failure outcome requested in [radius-project/radius#12526](https://github.com/radius-project/radius/issues/12526) remain unfinished.
- **Graph and status artifacts:** The [status action](https://github.com/radius-project/ai-extensions/blob/f48eba0d02fdaa7147acefe209e4ac84fc015023/.github/extension/actions/publish-deploy-status/action.yml) publishes graph, progress, and diagnostic files. These are workflow artifacts, not native GitHub Deployment records.
- **Pinned workflow assets:** Plugin releases bundle workflows, and generated first-party action references pin a source commit. [radius-project/ai-extensions#657](https://github.com/radius-project/ai-extensions/pull/657) merged August 31. GitHub Marketplace publication is still outstanding.
- **Deployment and setup reliability:** Bounded retries for newly written workflows landed October 9 in [radius-project/ai-extensions#992](https://github.com/radius-project/ai-extensions/pull/992). Azure setup diagnostics, retries, and cleanup improved October 12 in [radius-project/ai-extensions#1005](https://github.com/radius-project/ai-extensions/pull/1005); federated-credential retries landed October 14 in [radius-project/ai-extensions#1014](https://github.com/radius-project/ai-extensions/pull/1014). These are merged changes, not proof that every cloud setup path succeeds.
- **State lifecycle testing:** The eight scheduled runs from October 7 through October 14 passed. The [latest run](https://github.com/radius-project/radius/actions/runs/38512079121) saved and restored private-GHCR state after replacing the control plane, then updated an existing workload. It tested a persistent k3d target cluster, not the full AKS/EKS authentication matrix. See the [test documentation](https://github.com/radius-project/radius/blob/5e65916a3a6eac2a6a25d0014db05aa2ec2157bf/docs/contributing/contributing-code/contributing-code-tests/repo-radius-state-e2e.md).

## What is left to implement or resolve

All nine issues below remain open as of October 14. For each of these issues we must decide:
* Is this something that we need for Repo Radius?
* If so, is this something we need now or in the near future?

### Define an ephemeral control plane with persistent state ([#13292](https://github.com/radius-project/radius/issues/13292))

Design an optional architecture that keeps state persistent while control-plane instances and the CLI are ephemeral, reducing startup overhead and allowing safe parallel operations. This issue covers design and implementation planning, not implementation.

**Architectural decisions:**

- Where will live state persist, where will control-plane instances run, and who will operate them? This is more than replacing the snapshot-storage backend.
- How will clients access and authenticate to the control plane, and how will ephemeral instances access persistent state?
- Which operations can run in parallel, and how will conflicting operations be coordinated without losing state?
- How will existing GHCR snapshots migrate, which workflows must change, and how will latency improvements be measured against the current lifecycle?

### Document the supported interface ([#12997](https://github.com/radius-project/radius/issues/12997))

Define and test the supported GitHub Actions interface, including installation, inputs, outputs, permissions, and upgrade rules.

**Architectural decisions:**

- Which actions, workflows, configuration settings, and result files are public contracts, and which remain internal?
- Which action, CLI, and plugin versions must work together, and how are breaking changes introduced?
- Which cloud configurations and deployment/failure scenarios must pass contract tests before the interface is supported?

### Publish the GitHub Marketplace actions ([#12524](https://github.com/radius-project/radius/issues/12524))

Package cloud verification, which still lives in provider workflows, as a standalone action and publish it alongside the existing command action under the two requested GitHub Marketplace identities.

**Architectural decisions:**

- Where will `radius-project/verify-cloud-auth` and `radius-project/run-rad-commands` be published, and how will their releases come from ai-extensions?
- Will the actions release independently or together with the CLI and plugin?
- Which setup and authentication steps belong in each action rather than in its calling workflow?

### Update the generated wrappers ([#12525](https://github.com/radius-project/radius/issues/12525))

Switch generated workflows from current commit-pinned assets to the published actions.

**Architectural decisions:**

- What remains in the wrappers, and what moves into the published actions?
- How will generated workflows receive upgrades while keeping every action reference pinned to an exact commit SHA?
- How will the plugin update existing workflows without overwriting user changes?

### Finish result reporting ([#12526](https://github.com/radius-project/radius/issues/12526))

Standardize command and verification artifacts, and distinguish command failure from state-save failure.

**Architectural decisions:**

- Which command artifact name is the supported contract: `rad-commands-result` in #12997 or `run-rad-commands-result` in this issue? How will command and verification schemas be versioned, and how will consumers migrate?
- Who assembles the final outcome after teardown attempts to save state: the workflow or each frontend?
- How will results represent partial execution, failed state saves, and interrupted runs with no artifact?

### Add an application-source ref ([#12527](https://github.com/radius-project/radius/issues/12527))

Add an explicit application revision input for promotion and redeployment. The [Canvas already dispatches the selected branch](https://github.com/radius-project/ai-extensions/blob/f48eba0d02fdaa7147acefe209e4ac84fc015023/packages/adapter-canvas/src/server/services/deploy-dispatch.ts), but the workflow and action still lack a separate application-source ref.

**Architectural decisions:**

- How will application-source ref and workflow dispatch ref interact, including the requested default of the latest default-branch commit?
- When will a branch or tag resolve to a commit SHA so checkout, results, and deployment history identify the same revision?
- How will invalid or inaccessible refs fail before deployment begins?

### Record native GitHub Deployments ([#12528](https://github.com/radius-project/radius/issues/12528))

Record the deployed commit and environment in GitHub Deployments, and mark the matching deployment inactive on deletion.

**Architectural decisions:**

- Which workflow step creates and updates deployment records, and what token permissions does it need?
- Does deployment success require both successful commands and a successful state save?
- How will records identify the application and environment so deletion finds the right deployment when several applications share an environment?

### Fix AAD-enabled AKS deployment ([#12550](https://github.com/radius-project/radius/issues/12550))

Add authentication setup for both the GitHub runner and Radius pods: verification installs and configures `kubelogin`, but deployment still lacks that setup.

**Architectural decisions:**

- Which AKS configurations and non-interactive authentication modes will be supported?
- How will runner and pod kubeconfigs, credential-plugin binaries, and token refresh be configured separately?
- Which identities and permissions will each use? The runner's Azure CLI session is not available inside Radius pods.

### Settle cloud-storage scope ([#11605](https://github.com/radius-project/radius/issues/11605))

Decide whether OCI satisfies the storage requirement or whether Azure Blob/S3 archive support is still needed.

**Architectural decisions:**

- Is OCI the supported archive backend, or must Radius also implement Blob/S3 backends?
- If Blob/S3 is required, who provisions storage and configures access before control-plane startup?
- How will backend selection, archive migration, retention, and recovery work while keeping state and graph policies separate?

Terraform state in Blob/S3 is separate from the Radius control-plane archive and needs its own protection.

## Evidence baseline

- Radius remote main: `5e65916a3a6eac2a6a25d0014db05aa2ec2157bf`. The latest state lifecycle run tested the earlier commit `10e59be314d9bd6782f8b7689a76704155218564`.
- ai-extensions remote main: `f48eba0d02fdaa7147acefe209e4ac84fc015023`. Merged source is not necessarily included in an installed plugin release.
- Issue discussions, PR states, source changes, and workflow runs: checked October 14, 2026.

This snapshot does not cover every frontend/cloud combination or assign owners and delivery dates.
