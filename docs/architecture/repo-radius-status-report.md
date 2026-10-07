# Repo Radius: implementation and remaining work

Status as of October 6, 2026.


This document assesses what work is complete for Repo Radius today and what work (according to open GitHub issues) is still outstanding.

For the work that is still outstanding, we (eng + product) must decide:
* Is this something that we need for Repo Radius?
* If so, is this something we need now or in the near future?
* If so, what architecture decisions must be made for that work to be implemented?


## Repo Radius Today

The following features have already been implemented for Repo Radius:

- **Temporary control-plane lifecycle:** Start Radius, restore state, run commands, save state, and tear down. The foundation landed in [radius-project/radius#12214](https://github.com/radius-project/radius/pull/12214).
- **External-cluster deployment:** Radius manages workloads outside its own cluster. The foundation landed in [radius-project/radius#12106](https://github.com/radius-project/radius/pull/12106); provider workflows connect to AKS/EKS. See the [Azure workflow](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/run-rad-commands-azure.yml). This does not establish support for every cluster configuration.
- **Durable OCI state:** A later control plane can restore state saved by an earlier run. [radius-project/radius#12364](https://github.com/radius-project/radius/pull/12364) merged July 17. The [archive factory](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/pkg/statearchive/factory/factory.go) and [archive documentation](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/docs/architecture/state-archive.md) describe the current implementation.
- **Cloud verification and OIDC authentication:** Provider workflows check cloud access and configure deployment credentials. Verification work landed in [radius-project/radius#12170](https://github.com/radius-project/radius/pull/12170). Provisioning cloud-side identities is separate.
- **Custom types and recipe packs:** Support landed in [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367), merged July 25. [radius-project/radius#12742](https://github.com/radius-project/radius/pull/12742), merged August 24, fixed recipe-pack reconciliation on repeated deployments.
- **Application and environment deletion:** Delete workflows landed in [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367) and moved with the other workflow assets.
- **Command results:** The [command action](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/actions/run-rad-commands/action.yml) emits versioned results in command order under `rad-commands-result`. Some requirements in [radius-project/radius#12526](https://github.com/radius-project/radius/issues/12526) remain unfinished.
- **Graph and status artifacts:** The [status action](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/actions/publish-deploy-status/action.yml) publishes graph, progress, and diagnostic files. These are workflow artifacts, not native GitHub Deployment records.
- **Pinned workflow assets:** Plugin releases bundle workflows, and generated first-party action references pin a source commit. [radius-project/ai-extensions#657](https://github.com/radius-project/ai-extensions/pull/657) merged August 31. GitHub Marketplace publication is still outstanding.
- **State lifecycle testing:** A scheduled test restores state from private GHCR after replacing the control plane, then updates an existing workload on a persistent target cluster. See the [test documentation](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/docs/contributing/contributing-code/contributing-code-tests/repo-radius-state-e2e.md).

## What is left to implement or resolve

The following issues were open at the October 6 assessment. The decisions below are questions to resolve, not approved designs.

### Document the supported interface ([#12997](https://github.com/radius-project/radius/issues/12997))

Define and test the supported GitHub Actions interface, including installation, inputs, outputs, permissions, and upgrade rules.

**Architectural decisions:**

- Which actions, workflows, configuration settings, and result files are public contracts, and which remain internal?
- Which action, CLI, and plugin versions must work together, and how are breaking changes introduced?
- Which cloud configurations and deployment/failure scenarios must pass contract tests before the interface is supported?

### Publish the GitHub Marketplace actions ([#12524](https://github.com/radius-project/radius/issues/12524))

Package cloud verification as a standalone action and publish it alongside the existing command action under the two requested GitHub Marketplace identities.

**Architectural decisions:**

- Where will `radius-project/verify-cloud-auth` and `radius-project/run-rad-commands` be published, and how will their releases come from ai-extensions?
- Will the actions release independently or together with the CLI and plugin?
- Which setup and authentication steps belong in each action rather than in its calling workflow?

### Update the generated wrappers ([#12525](https://github.com/radius-project/radius/issues/12525))

Switch generated workflows from current commit-pinned assets to the published actions.

**Architectural decisions:**

- What remains in the wrappers, and what moves into the published actions?
- How will generated workflows pin action releases and receive upgrades?
- How will the plugin update existing workflows without overwriting user changes?

### Finish result reporting ([#12526](https://github.com/radius-project/radius/issues/12526))

Standardize command and verification artifacts, and distinguish command failure from state-save failure.

**Architectural decisions:**

- How will `rad-commands-result` migrate to `run-rad-commands-result`, and how will both result schemas be versioned?
- Who assembles the final outcome after teardown attempts to save state: the workflow or each frontend?
- How will results represent partial execution, failed state saves, and interrupted runs with no artifact?

### Add an application-source ref ([#12527](https://github.com/radius-project/radius/issues/12527))

Add an explicit application revision input for promotion and redeployment, separate from the workflow dispatch ref.

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

Add the authentication setup needed for both the GitHub runner and Radius pods to access AAD-enabled AKS.

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

- Radius source and documentation: `ec90522af3e8ca3d4aa348848a16a15e9e1988e9`, matching this worktree's HEAD and remote main when assessed.
- ai-extensions source: `a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1`.
- Issue discussions, PR states, and workflow runs: checked October 6, 2026.

This snapshot does not cover every frontend/cloud combination or assign owners and delivery dates.
