# Repo Radius: implementation and remaining work

**Assessment date:** October 6, 2026.

**Bottom line:** Repo Radius has an implemented on-demand execution path, durable state storage, cloud-specific deployment workflows, and frontend integration. Its public integration contract and intended Marketplace distribution remain incomplete. The evidence supports "implemented, with productization and compatibility work outstanding," not a declaration that the entire feature is finished or generally available.

This report distinguishes implementation evidence from issue status. A closed issue is not proof that every original requirement shipped; an open issue can contain substantial completed work. No completion percentage, release commitment, or delivery date is inferred.

## What Repo Radius is

Repo Radius runs the Radius control plane temporarily inside a GitHub Actions runner. A frontend, such as the Radius Canvas, configures and dispatches repository workflows. Those workflows authenticate to the cloud, create an ephemeral Kubernetes control plane, restore durable Radius state, run deployment or management commands, save state, and remove the temporary control plane.

The application workloads deploy to a separate, external AKS or EKS cluster. Repo Radius removes the need for a permanently running **Radius control plane**; it does not remove the need for a workload cluster, cloud permissions, configured credentials, or durable storage.

The original feature proposal described a more accessible, repository-driven Radius experience, including deployment, environment setup, promotion, and management. Its feature-spec PR, [radius-project/radius#12078](https://github.com/radius-project/radius/pull/12078), was closed without merging. Implementation landed through separate changes, so that proposal should not be treated as an approved, fully delivered specification.

```mermaid
flowchart LR
    Frontend["Frontend / Radius Canvas"] -->|dispatch| Workflow["GitHub Actions workflow"]
    Workflow --> CP["Temporary Radius control plane"]
    Archive["Private OCI state archive"] -->|rad startup: restore| CP
    CP -->|deploy / update / delete| Target["External AKS / EKS cluster"]
    CP -->|rad shutdown: save| Archive
    Workflow --> Results["Command results and deployment artifacts"]
```

## Key components and ownership

| Component | Responsibility | Current home |
| --- | --- | --- |
| Radius CLI and control plane | Run commands, manage resources, reach the external target cluster, and save/restore control-plane state. | `radius-project/radius` |
| Workflow templates and composite actions | Coordinate verification, authentication, temporary control-plane setup, command execution, result reporting, and teardown. | `radius-project/ai-extensions`, under `.github/extension/` |
| Frontend/plugin integration | Generate repository workflows and drive the verification/deployment experience. | `radius-project/ai-extensions` |
| Durable state and modeled-graph archives | Preserve Radius state between temporary control-plane instances; the same archive abstraction also supports durable modeled-graph output. | OCI implementation in Radius; separately configured state and graph repositories supply storage. |
| External workload cluster | Continue running application workloads after the temporary Radius control plane is removed. | User's AKS or EKS cluster. |

The workflow implementation moved out of the Radius repository. [radius-project/radius#12719](https://github.com/radius-project/radius/pull/12719), merged August 31, removed the duplicated extension assets after the port in [radius-project/ai-extensions#424](https://github.com/radius-project/ai-extensions/pull/424). The absence of `.github/extension/` in Radius is therefore an ownership change, not removal of Repo Radius.

## How it works

1. A frontend writes verification and deployment workflows into the application repository and configures a GitHub Environment.
2. The deployment dispatcher selects the Azure or AWS provider workflow using environment configuration.
3. The provider workflow authenticates using GitHub OIDC and obtains access to the external workload cluster.
4. Shared actions create a temporary control plane and configure Radius to target the external cluster.
5. `rad startup` restores previously archived state before commands execute.
6. The workflow configures cloud credentials and recipe packs, then runs requested `rad` commands or the default application deployment.
7. Command results and deployment graph/status information are published as workflow artifacts.
8. When startup reported successful state restoration, teardown attempts `rad shutdown` to persist state and removes the temporary cluster.

Teardown is arranged to run even after earlier workflow failures. It deliberately skips `rad shutdown` if startup did not report successful state restoration, so an uninitialized control plane cannot overwrite the existing archive. After a later failure, such as a partially applied deployment, it still attempts to save state. This is a recovery mechanism, not a guarantee that state can always be saved: a lost runner or abrupt termination can prevent cleanup or artifact publication. See the [teardown action](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/actions/teardown/action.yml).

## What has been implemented

| Capability | Implemented behavior | Evidence and qualification |
| --- | --- | --- |
| Ephemeral Radius lifecycle | Start a temporary control plane, restore state, run commands, save state, and tear down. | Foundational state externalization merged in [radius-project/radius#12214](https://github.com/radius-project/radius/pull/12214). Maintained provider workflows and shared actions implement the orchestration. |
| External-cluster deployment | Radius can manage workloads outside its own temporary Kubernetes cluster; provider workflows connect to AKS/EKS. | Multi-cluster foundation [radius-project/radius#12106](https://github.com/radius-project/radius/pull/12106) and the maintained [Azure workflow](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/run-rad-commands-azure.yml). This is not evidence that every AKS/EKS configuration is supported. |
| Durable OCI state | Radius state can be archived outside the running control plane and restored by a later run. | [radius-project/radius#12364](https://github.com/radius-project/radius/pull/12364), merged July 17; current [archive factory](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/pkg/statearchive/factory/factory.go) and [architecture documentation](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/docs/architecture/state-archive.md). |
| Cloud verification and OIDC deployment integration | Provider-specific workflows verify or consume cloud access and configure deployment credentials. | Verification work merged in [radius-project/radius#12170](https://github.com/radius-project/radius/pull/12170); maintained templates/actions are now in ai-extensions. Cloud-side identity provisioning is distinct from the deployment workflow itself. |
| Custom resource types and recipe packs | Repo Radius workflows support registering custom types and applying custom recipe packs. | [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367), merged July 25. Repeat-deploy reconciliation was fixed in [radius-project/radius#12742](https://github.com/radius-project/radius/pull/12742), merged August 24. |
| Application/environment deletion | Supporting delete workflows were added. | [radius-project/radius#12367](https://github.com/radius-project/radius/pull/12367) and the migrated assets described in [radius-project/radius#12719](https://github.com/radius-project/radius/pull/12719). Native GitHub Deployment deactivation is a separate requirement. |
| Structured command results | The command action emits versioned, ordered command results under the artifact name `rad-commands-result`. | The current [run-rad-commands action](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/actions/run-rad-commands/action.yml) confirms the older partial-completion audit on [radius-project/radius#12526](https://github.com/radius-project/radius/issues/12526). The requested end-to-end result contract is not fully implemented. |
| Deployment graph/status reporting | The workflow publishes deployed graph, progress, and diagnostic/status files for frontend consumption. | Current [publish-deploy-status action](https://github.com/radius-project/ai-extensions/blob/a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1/.github/extension/actions/publish-deploy-status/action.yml). Workflow artifacts are not native GitHub Deployment records. |
| Release-aligned workflow distribution | Plugin artifacts include workflow assets, and generated first-party action references pin the source commit. | [radius-project/ai-extensions#657](https://github.com/radius-project/ai-extensions/pull/657), merged August 31. This addresses mutable-source drift, not Marketplace publication. |
| State lifecycle end-to-end coverage | A scheduled test replaces the control plane, restores state from private GHCR, and updates the existing workload on a persistent target cluster. | [Test documentation](https://github.com/radius-project/radius/blob/ec90522af3e8ca3d4aa348848a16a15e9e1988e9/docs/contributing/contributing-code/contributing-code-tests/repo-radius-state-e2e.md) and recent runs described below. |

## What is left to implement or resolve

| Workstream | Completed portion | Remaining requirement or decision | Tracking |
| --- | --- | --- | --- |
| Supported deployment contract | Working deployment workflows and structured command artifacts exist. | Finish a documented installation/generation mechanism, stable inputs/outputs/permissions, a published result schema, compatibility and migration rules, examples, and contract-level success/failure/invalid-command coverage. Individual building blocks do not prove the aggregate acceptance criteria are complete. | [radius-project/radius#12997](https://github.com/radius-project/radius/issues/12997), open. |
| Marketplace actions | A canonical, commit-pinned `run-rad-commands` composite action exists. Cloud-auth verification exists as `verify-azure.yml` and `verify-aws.yml`, not as a standalone `verify-cloud-auth` action in the inspected tree. | Package verification as a reusable action, then publish and verify the requested `radius-project/verify-cloud-auth` and `radius-project/run-rad-commands` Marketplace identities and their release scheme. | [radius-project/radius#12524](https://github.com/radius-project/radius/issues/12524), open. Its September 29 update confirms publication is outstanding but overstates the standalone verification-action implementation; the current source tree takes precedence. |
| Thin workflows using Marketplace identities | Ready-to-commit, commit-pinned workflows exist. | Switch wrappers to the two requested published action identities after publication. This depends on the preceding workstream. | [radius-project/radius#12525](https://github.com/radius-project/radius/issues/12525), open; September 29 maintainer update. |
| Complete result-artifact contract | Versioned, ordered command results exist. | Resolve the `rad-commands-result` versus requested `run-rad-commands-result` naming contract; provide the required versioned `verify-cloud-auth-result` envelope; represent state-save failure distinctly. State saving currently happens separately during teardown. | [radius-project/radius#12526](https://github.com/radius-project/radius/issues/12526), open and partially complete. |
| Source revision selection and promotion | Generated workflow implementation is pinned to an immutable action-source commit. GitHub dispatch itself can target a branch or tag, which default checkout can follow. | Add the requested explicit application-source `ref` input and checkout contract for promotion or redeployment. The absence of that input does not mean branch/tag selection through dispatch is impossible; this report has not verified how the Canvas selects its dispatch ref. Pinning action code is not the same as selecting the application's revision. | [radius-project/radius#12527](https://github.com/radius-project/radius/issues/12527), open; the observed dispatcher exposes no `ref` input. |
| Native GitHub Deployment history | Deployment graph/status artifacts exist. | Record a GitHub Deployment at the deployed commit/environment and mark it inactive on application deletion. The observed status publisher uploads artifacts rather than creating Deployment API records. | [radius-project/radius#12528](https://github.com/radius-project/radius/issues/12528), open. |
| AKS Automatic / AAD compatibility | The Azure verification workflow installs `kubelogin` and converts its kubeconfig. The deployment workflow connects to AKS but lacks equivalent `kubelogin` setup. | Fix the deployment path and the different authentication requirements of runner commands and Radius pods, then verify supported target configurations. Successful verification does not establish that AAD-enabled deployment will work. | [radius-project/radius#12550](https://github.com/radius-project/radius/issues/12550), open. A July 28 maintainer comment quotes the docs stating AKS-managed AAD is unsupported; the inspected deploy template still lacks the setup. |
| Cloud-storage scope | OCI archival supplies durable storage outside the control plane. | Decide whether OCI replaces the original request for credential-driven cloud blob storage. If not, provision Azure Blob/S3 storage before control-plane startup using Repo Radius cloud credentials. The current archive factory has no such backend. | [radius-project/radius#11605](https://github.com/radius-project/radius/issues/11605), open; September 11 audit identifies this decision. |
| Historical state-test failure | Eleven consecutive scheduled runs are passing since the September 25 failure. | Investigate or document disposition of the earlier failure before declaring it fixed. | [radius-project/radius#13112](https://github.com/radius-project/radius/issues/13112), open. |

## Notable details and assessment

**Storage has changed since the early proposal.** The current factory accepts OCI or an unset backend and explicitly rejects `git`. The closed Git-storage issue, [radius-project/radius#11604](https://github.com/radius-project/radius/issues/11604), does not imply the Git backend remains supported. Some older design paragraphs still mention Git storage and pre-migration action locations; current implementation and explicit migration notes take precedence.

**Contract support is different from working integration.** A frontend can drive the existing workflows today, while the supported public interface, Marketplace identities, and full result contract remain unfinished. There is no verified evidence here of a formal general-availability declaration.

**Recommended implementation sequence, not a maintainer commitment:** settle the result schema and failure semantics, finish the supported contract and action-publication strategy, complete source-revision selection and native deployment history, then close compatibility and storage-scope gaps with explicit acceptance tests. Investigate the historical state failure independently rather than treating a green streak as a root-cause fix.

## Evidence baseline

Radius code and documents were examined at `ec90522af3e8ca3d4aa348848a16a15e9e1988e9`, which matched both this worktree's HEAD and remote main when checked. The ai-extensions implementation baseline was `a77ba988a4fb0e42c9cc96e70b2b0bf6f694a3d1`. Issue states, maintainer comments, PR merge states, and workflow runs were retrieved live on October 6, 2026.

This is an evidence-based status snapshot, not a complete requirements audit of every frontend and cloud combination. It does not assign owners, promise dates, or infer that all requirements of an open issue are unimplemented.

## Review and corrections

Claude Opus 5.5 independently reviewed this report against source code and live GitHub evidence. The corrected report distinguishes verification workflows from a standalone verification action, records the full observed 11-run passing streak and its tested commit, explains teardown's archive-protection guard, separates the AKS verification and deployment authentication paths, clarifies dispatch-ref selection versus an explicit application-source input, and cites current result-action code alongside the older issue audit. The review found no omission that changes the overall assessment.
