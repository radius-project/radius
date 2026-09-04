# Repo Radius

**Repo Radius** is a delivery model in which Radius runs *ephemerally inside a GitHub Actions runner* instead of as a persistent installation that a platform engineer operates. A run creates a throwaway control-plane cluster, restores the previous run's state, deploys the user's application to an *external* Kubernetes cluster, persists state again, and tears the control plane down.

Three things define the model:

- **The unit of deployment-target configuration** is a **GitHub Environment**.
- **The cloud credential model** is **OIDC federation**, with no long-lived cloud credentials stored anywhere.
- **The durable state** lives in a **GHCR package** linked to the user's repository.

This document has five parts. **Repo Radius Today** summarizes the system as shipped. **Vision** states what Repo Radius is for (beyond its first frontend in the GitHub Copilot App). **Architecture Direction** names what has to change for the vision to hold. **Roadmap** puts those changes in dependency order. The **Appendix** is the verified long-form account of the current system: what this repository contributes, where the boundary with `radius-project/ai-extensions` lies, and which parts are implemented. The first four sections are the argument; the appendix is the evidence behind it.

> **Status as of 2026-09-03.** The appendix distinguishes shipped behavior from work in review. Anything described as "in review", "proposed", or linked to an open pull request is *not* current behavior. Re-check the linked items before relying on them. The *Architecture Direction* and *Roadmap* sections describe intent, not implementation, and nothing in them should be read as a commitment.

## Repo Radius Today

This section summarizes the shipped system. [Appendix: Repo Radius v1](#appendix-repo-radius-v1) gives the full account, with a source citation behind each claim.

**What a run does.** A generated GitHub Actions workflow authenticates to the cloud provider over OIDC, creates a throwaway k3d cluster, installs Radius from the `edge` channel, and restores the previous run's state from a GHCR package. It then creates a resource group, registers a cloud credential, deploys a Radius environment, and deploys the application, whose workloads land on an external cluster the user already owns. Finally it publishes a deploy-status artifact, captures state back to the archive, and deletes the cluster.

**How the work is split.** This repository owns what must be true of the `rad` binary and the control plane for the model to be possible. [`radius-project/ai-extensions`](https://github.com/radius-project/ai-extensions) owns the orchestration: the workflow templates, the composite actions, and the GitHub Copilot canvas that generates and dispatches them. The templates and actions used to live here and were removed by [#12719](https://github.com/radius-project/radius/pull/12719). They should not come back.

**The three seams this repository owns.**

| Seam                       | Contract                                                                                   | Principal code                                                                                                                                   |
|----------------------------|--------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| External cluster targeting | The `RADIUS_TARGET_KUBECONFIG` environment variable. Unset means target the local cluster. | [pkg/kubeutil](../../pkg/kubeutil), [pkg/recipes/kubernetes/clusteraccess](../../pkg/recipes/kubernetes/clusteraccess)                           |
| Durable state              | `rad startup` and `rad shutdown` bracket a run, reading and writing a pluggable archive.   | [pkg/statearchive](../../pkg/statearchive), [pkg/cli/cmd/startup](../../pkg/cli/cmd/startup), [pkg/cli/cmd/shutdown](../../pkg/cli/cmd/shutdown) |
| Graph output               | `rad app graph`, whose modeled form commits to a graph archive when it detects a runner.   | [pkg/cli/cmd/app/graph](../../pkg/cli/cmd/app/graph), [pkg/graph/persistence](../../pkg/graph/persistence)                                       |

**What is implemented.** Four of the original specification's five investments are in place, two of them still hardening. The fifth, control-plane startup time, has not been started, so every run still builds a cluster and installs Radius from scratch.

**What surprises people.** Three properties of v1 catch readers out, and each is expanded in the appendix. The state restore is *destructive*, so anything the control plane must contain has to be created after `rad startup` or be present in the archive. The `--preview` flag is supplied by two unrelated mechanisms and applied unevenly, and omitting it produces a plausible wrong answer rather than an error. And the modeled-graph archive path this repository owns is not exercised by any generated workflow, because every generated graph call passes `--preview` and lands in a different implementation.

## Vision: One Backend, Many Interfaces

Repo Radius has one frontend today, the Radius plugin for the GitHub Copilot app. What deploys an application on the backend is a GitHub Actions workflow plus the `rad` behaviors this repository owns. The same backend should serve the Copilot CLI, the GitHub web UI, and any other surface that dispatches an agent (or agents) at a repository to deploy the application.

```text
  +---------------+ +---------------+ +---------------+ +---------------+
  | Copilot app   | | Copilot CLI   | | GitHub web UI | | anything else |
  | canvas        | |               | |               | |               |
  |  SHIPPED      | |  NOT BUILT    | |  NOT BUILT    | |  NOT BUILT    |
  +---------------+ +---------------+ +---------------+ +---------------+
          |                 |                 |                 |
          +-----------------+-----+-----------+-----------------+
                                  |
                                  v
  +---------------------------------------------------------------+
  | interface-independent capability                              |
  |   model an application     provision a deployment target      |
  |   generate workflows       dispatch a run and read it back    |
  +---------------------------------------------------------------+
                                  |
                                  v
  +---------------------------------------------------------------+
  | the run:  GitHub Actions workflow  +  rad  (this repo)        |
  +---------------------------------------------------------------+
```

Two goals follow from that picture, and they are independent of each other.

**Interface neutrality.** Everything a run needs should be reachable by any frontend.

**Agent-first primitives.** The caller is increasingly a program rather than a person. A person reading a rendered graph may notice an oddity. Building for agents means fewer primitives, uniform behavior across them, and output a parser can trust.

These two goals reinforce each other. A backend that is safe for an agent to drive is, by construction, a backend that a new frontend can adopt without inheriting undocumented behavior.

## Architecture Direction

Six themes. Each names a property the current system lacks, the evidence that it lacks it, and what would have to change. The evidence comes from [Appendix: Repo Radius v1](#appendix-repo-radius-v1), which carries the source citation behind every claim restated here. Read that appendix first if you want the current system described on its own terms, before it is argued with. None of this is scheduled work.

### 1. Move the Write Side Behind Ports

The neutral-backend intent is already written down in source. [`packages/core/src/ports/index.ts`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/core/src/ports/index.ts#L1-L19) opens by stating that the core never imports the Copilot SDK, opens an HTTP server, or touches the DOM, and that each UI adapter injects a concrete implementation. The package layout follows through: `core` holds modeling, graph, workflow generation, and platform logic, and `adapter-shared` holds the `rad` process invocation that any adapter would need.

The seam is real, and it is currently **read-only**. That file declares exactly one port, `GitHub`, with three read methods: `getContent`, `listNames`, and `treePaths`. Core decides and returns; it does not act. The mutations a run depends on live in `adapter-canvas`: creating the GitHub Environment, bootstrapping the GHCR package, provisioning the deploy-params secret, committing the generated files, dispatching the run, and, for Azure, creating the App Registration and federated credentials behind the `/api/azure-auto-setup` route.

Workflow generation shows the split exactly. `generateDeployWorkflow` in [`packages/core/src/workflows/deploy.ts`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/core/src/workflows/deploy.ts#L60-L86) returns the three files as values, and the adapter commits them. Azure OIDC shows it again: [`azure-oidc.ts`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/adapter-canvas/src/azure-oidc.ts#L1-L11) keeps the pure decisions and imports `buildOidcSubject` from core, while the route spawns `az` and `gh` to do the work.

The cost of drawing the line there shows up as provider asymmetry. Azure has an auto-setup route and a discovery route. AWS has neither, so its trust relationship is configured by hand. Capability parked in an adapter gets built once per provider, and then again per frontend.

A second frontend therefore inherits the modeling and none of the setup. The direction is to move each of those use cases into core behind a port, so that core owns the sequence and the adapter supplies only the transport that executes it. That is the arrangement the port comment already describes, extended to writes as well as reads.

**What this repository owes that goal.** The workflow contract and `rad`'s behavior are already interface-independent, and the work here is to keep them that way. Nothing in `rad` should assume a canvas, an interactive terminal, or a human reading the output.

### 2. Make the Graph a Data Contract

The graph is the most valuable thing Repo Radius produces and the least ready to be consumed by a program. Today `rad app graph` has three implementations reached through two unrelated dispatch mechanisms, the archive path this repository owns is not exercised by any generated workflow, and the published `deploy-graph.json` can carry a plain-text `Compiling <path>` line ahead of its JSON because progress logging and formatted output share one writer. A person looking at a rendered picture never notices any of that. A parser fails on the third item and silently misreads the first two.

The direction is one addressable graph artifact per run, with a versioned schema, stable identity for every node and edge, and a `--output json` path whose stdout carries the payload and nothing else.

Stability is only half of it. The graph should be **open to augmentation**, so that systems holding data Radius does not have, such as telemetry, inventory, or cost, can join against it without this repository learning about them. That argues for durable identifiers and a documented schema rather than a format tuned to one renderer.

### 3. Collapse the Command Surface

`--preview` is supplied by two unrelated mechanisms and applied unevenly inside a single run, and the resulting failure mode is the part that matters. Omitting the flag does not produce an error. It produces a plausible wrong answer: `rad app list` returns an empty list, and `rad app delete` reports success. A person debugging eventually distrusts the result. An agent records the empty list as a fact and acts on it.

The direction is to converge on one resource plane and retire the flag. Where two implementations have to coexist, the difference belongs in the output rather than in a flag the caller must remember. The general rule is to prefer failing loudly over answering plausibly, because only one of those is recoverable by a non-human caller.

### 4. Make a Run Addressable and Recoverable

An agent driving a deployment has to name what it deployed, observe the outcome, and recover from a partial failure. None of the three is reliable today. There is no `ref` input for deploying a revision other than the latest ([#12527](https://github.com/radius-project/radius/issues/12527)), no GitHub Deployment record per deploy ([#12528](https://github.com/radius-project/radius/issues/12528)), no mitigation for partial hydration, and reconciliation between the archive and reality is still in review ([#12871](https://github.com/radius-project/radius/pull/12871)).

These are prerequisites rather than enhancements. Unattended deployment means a caller that cannot ask a human what happened.

### 5. Reduce the Cost of a Run

Investment 5 remains untouched, and every run still builds a k3d cluster and installs Radius from scratch. This ranks low while a person deploys occasionally and high as soon as an agent iterates, because per-run cost sets the rate at which an agent can work. The specification's proposed direction, a pre-baked k3d node image and a composite rather than Docker action, is still the obvious starting point.

### 6. Widen the Target Surface

A backend that only reaches some clusters is not a general one. AWS is already a target: the AWS provider workflow is always committed, a run registers an IRSA credential, and EKS is supported. What is thin is the setup path around it. The canvas adapter has an Azure auto-setup route and an Azure discovery route and no AWS counterpart, so the AWS trust relationship is configured by hand. Beyond that, the known limits are clusters the runner cannot authenticate to ([#12550](https://github.com/radius-project/radius/issues/12550)) and clusters it cannot afford to run against ([#12857](https://github.com/radius-project/radius/issues/12857)).

## Roadmap

Ordered by dependency rather than by priority. The question each row answers is what has to be true before the theme can be finished, not when anyone intends to do it. "Hard dependency" means genuinely blocked, as distinct from merely cheaper to do later.

| Order | Theme                                        | Hard dependency          | Why it sits here                                                                                                    |
|-------|----------------------------------------------|--------------------------|---------------------------------------------------------------------------------------------------------------------|
| 1     | Trustworthy output (theme 3, and 2's stdout) | None                     | A backend that answers plausibly when it is wrong cannot be built on. Everything else assumes its output is true.   |
| 2     | Graph as a versioned, extensible contract    | 1                        | A schema is only worth publishing once the command that emits it is unambiguous.                                    |
| 3     | Capability behind ports                      | None                     | Sequenced after 1 by preference, not by necessity: a migration should carry settled contracts across the boundary.  |
| 4     | Addressable, recoverable runs                | 3, for the dispatch half | The `rad`-side reconciliation and run-record work is independent of everything else here and can start immediately. |
| 5     | Run cost                                     | None                     | Independent throughout. Becomes urgent exactly when agents start iterating.                                         |
| 6     | Target breadth                               | None                     | Independent throughout, and paced by demand rather than by architecture.                                            |

Three consequences are worth stating plainly.

**Output correctness comes first.** Theme 3, and the stdout half of theme 2, are cheap, already diagnosed, and inherited by everything else. Doing them last would mean building the rest on a foundation that reports success when it is wrong.

**Most of this is unblocked.** The table contains only two real constraints: the graph schema should follow the command surface, and the dispatch half of theme 4 should follow the ports. Themes 1, 3, 5, 6, and the `rad`-side half of 4 can all begin independently. The ordering above is mostly advice about what makes the rest cheaper.

**The port work is a migration, not a feature.** Moving the write side into core changes nothing a user can see. Its entire value is the second frontend, so it pays off only if the contracts it carries across the boundary are the settled ones. That is the reason to prefer doing it after theme 1 even though nothing forces the order.

## Appendix: Repo Radius v1

Everything in this section is verified against the source at the revisions cited. It is the long form of *Repo Radius Today* and the baseline the Vision, Architecture Direction, and Roadmap sections build from. Where this appendix and any summary of it disagree, trust the appendix.

### Why This Model Exists

Self-hosted Radius requires someone to own a long-lived control plane: install it, upgrade it, back up its database, and hold the cloud credentials it deploys with. That is a reasonable cost for a platform team and an unreasonable one for a developer who wants to deploy a single application from a repository they already own.

Repo Radius removes the persistent installation from the picture:

| Concern             | Self-hosted Radius                         | Repo Radius                                                                                  |
|---------------------|--------------------------------------------|----------------------------------------------------------------------------------------------|
| Control plane       | Long-lived, on a managed cluster           | Ephemeral k3d cluster, created and destroyed per run                                         |
| Where state lives   | The control plane's own database           | A durable archive in GHCR, restored and persisted per run                                    |
| Where workloads run | Usually the same cluster                   | Always an external cluster the user already owns                                             |
| Cloud credentials   | Stored in the control plane                | Minted per run via OIDC federation, never stored                                             |
| Environment         | A Radius resource an operator creates once | A GitHub Environment, plus a Radius environment resource restored or re-provisioned each run |
| Who upgrades it     | The platform team                          | Each run installs from the `edge` channel                                                    |

The trade is deliberate: startup cost is paid on every run in exchange for having no installation to own. Note that the Radius version is not pinned per workflow. `setup-control-plane` pins the *installer script* to a hardcoded commit in this repository and verifies its checksum, then runs `install-rad.sh edge`, so each run installs the current `edge` build. The k3d version is pinned as well.

**A note on the word "environment", which means two different things here.** A **GitHub Environment** is the external configuration and OIDC trust boundary: it holds the cloud client ID or role ARN and the identity the run federates as. A **`Radius.Core/environments` resource** still exists inside the control plane and is what recipes, applications, and the Kubernetes namespace binding attach to. The workflow deploys one from Bicep on every run, on top of whatever the state archive restored. Repo Radius does not eliminate the Radius environment resource; it moves the *configuration and credentials* out to GitHub.

### Repository Boundary

Repo Radius spans two repositories.

**This repository (`radius-project/radius`)** owns everything that must be true of the `rad` binary and the control plane for Repo Radius to be *possible*: the ability to target an external cluster, the ability to export and restore state, and the graph output the frontend renders.

**`radius-project/ai-extensions`** owns the *orchestration*: the GitHub Actions workflow templates and composite actions that sequence a run, plus the GitHub Copilot canvas and skills that generate and dispatch them.

> **Historical note.** The workflow templates and composite actions previously lived in this repository under `.github/extension/`. They were removed by [#12719](https://github.com/radius-project/radius/pull/12719) and now live solely in [`radius-project/ai-extensions`](https://github.com/radius-project/ai-extensions/tree/main/.github/extension). Do not re-add them here. Their contract with any frontend is documented in that repository's [`.github/extension/README.md`](https://github.com/radius-project/ai-extensions/blob/main/.github/extension/README.md).

```text
  radius-project/ai-extensions               the user's repository
  +-------------------------------+          +-----------------------------+
  | Copilot canvas + skills       |==(1)====>| .radius/app.bicep           |
  |                               |          |                             |
  | workflow templates            |==(2)====>| .github/workflows/*.yml     |
  |   .github/extension/*.yml     |          |                             |
  |                               |          +-----------------------------+
  | composite actions             |<-(3)-----  uses: ...@<commit sha>
  |   .github/extension/actions/  |
  +-------------------------------+          +-----------------------------+
                                             | GitHub Environment      (4) |
  radius-project/radius                      |   variables, OIDC trust     |
  +-------------------------------+          |                             |
  | rad CLI                       |          | GHCR package            (5) |
  | control-plane images          |          |   durable state archive     |
  |   (this repository)           |          +-----------------------------+
  +-------------------------------+
    |
    +--(6)--> installed into the runner on every run

  ==>  COPIED at generation time. The copy belongs to the user and changes
       only when the frontend regenerates it.
  -->  REFERENCED, never copied. The workflow names a commit and GitHub
       fetches the action at run time.

  (1) The canvas generates the application model.
  (2) The canvas commits the workflow files and can dispatch a run.
  (3) Those workflows pin each composite action to one commit SHA.
  (4) Created by the frontend. Holds variables, and an optional secret.
  (5) Bootstrapped by the frontend. Must be private or internal.
  (6) Each run fetches this repository's install.sh and runs it at edge.
```

The two boxes on the right differ in kind. The upper one holds files committed to the user's repository, which they can read and review in a diff. Three workflow files are always committed: the `run-rad-commands.yml` dispatcher plus the `run-rad-commands-azure.yml` and `run-rad-commands-aws.yml` provider workflows ([`deploy.ts:60-76`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/core/src/workflows/deploy.ts#L60-L76)). The lower box holds GitHub platform objects the repository is configured *against*, which exist only in GitHub's settings and API. The GitHub Environment is repository-scoped. The GHCR package is owned by the account and linked to the repository, and the frontend refuses to use one that is not private or internal ([`ghcr.ts:872-875`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/adapter-canvas/src/ghcr.ts#L872-L875)).

Arrows (1) through (3) all happen at *generation* time, when the frontend writes files. Arrow (6) happens at *run* time, on every workflow run: `setup-control-plane` downloads this repository's `deploy/install.sh`, verifies it against a pinned SHA-256, and runs `install-rad.sh edge` ([`setup-control-plane/action.yml:47-58`](https://github.com/radius-project/ai-extensions/blob/5712652/.github/extension/actions/setup-control-plane/action.yml#L47-L58)).

That arrow carries two different pins, and conflating them will mislead you. The *installer script* is pinned twice over: `RADIUS_INSTALL_REF` is a literal commit in this repository, currently `f4b44130e6cc`, and `RADIUS_INSTALL_SHA256` must match the bytes it fetches. The *product* is not pinned at all, because the script is then invoked as `install-rad.sh edge`. So a change to Radius itself reaches users on their next run through the `edge` channel, while a change to `deploy/install.sh` does not reach them until someone bumps that literal ref and checksum in `ai-extensions`. An `ai-extensions` change reaches nobody until they regenerate.

That copied-versus-referenced distinction shapes how changes reach users. Because templates are copied, a user's workflow only changes when the frontend regenerates it, so the run stays reviewable in their own repository. Composite actions are referenced rather than copied, which sounds like it should mean they update on their own. It does not. The reference names one specific commit, so a referenced action is as frozen as a copied file.

The ref those actions resolve at is worth knowing before you rely on either property. Generated workflows emit `uses: radius-project/ai-extensions/.github/extension/actions/<name>@{{RADIUS_REF}}`, and the generator substitutes `RADIUS_SOURCE_REF` ([`deploy.ts:5`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/core/src/workflows/deploy.ts#L5)). A **released** build stamps that value at bundle time with the exact `ai-extensions` commit it was built from, and rejects any explicit ref that is not a full 40-character SHA ([`build.mjs:114-141`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/adapter-canvas/build.mjs#L114-L141), [`build.mjs:553-554`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/adapter-canvas/build.mjs#L553-L554)). The `main` literal survives only when the build can resolve no commit at all, such as one made from a source archive rather than a checkout. Absent an explicit ref, the build stamps `git rev-parse HEAD`, so even an ordinary local build pins itself, and the source notes that a build with no resolvable commit stays unstamped and is rejected by every release validator.

The practical consequence is that a generated workflow pins first-party actions to one immutable commit, exactly as it pins the third-party actions in the same templates. Fixing a composite action does **not** reach already-generated workflows. Users pick it up by regenerating with a newer plugin version. Composite-action inputs are still a compatibility surface across plugin releases, but a merge to `ai-extensions` `main` cannot change what an existing user repository runs.

### The Five Investments

The original feature specification framed the work as five investments. Their status in this repository:

| # | Investment                          | Status                 | Where it lives                                                                                                                                   |
|---|-------------------------------------|------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| 1 | Deploy to an external cluster       | Implemented (v1)       | [pkg/kubeutil](../../pkg/kubeutil), [pkg/recipes/kubernetes/clusteraccess](../../pkg/recipes/kubernetes/clusteraccess)                           |
| 2 | Externalize the control-plane store | Implemented, hardening | [pkg/statearchive](../../pkg/statearchive), [pkg/cli/cmd/startup](../../pkg/cli/cmd/startup), [pkg/cli/cmd/shutdown](../../pkg/cli/cmd/shutdown) |
| 3 | Workflow with standardized I/O      | Implemented            | `ai-extensions`                                                                                                                                  |
| 4 | Cloud credential integration (OIDC) | Implemented, hardening | `ai-extensions`, plus credential registration in the CLI                                                                                         |
| 5 | Control-plane startup time          | **Not started**        | —                                                                                                                                                |

Investment 5 is the specification's own highest-priority non-functional requirement and remains unaddressed. Every run still builds a k3d cluster and installs Radius from scratch. Nobody has implemented the proposed direction, a pre-baked k3d node image and a composite rather than Docker action.

### Seams in This Repository

Three seams make a Radius build usable from an ephemeral runner. Each is a small, well-defined surface, and each is what you should look at first when changing Repo Radius behavior.

#### 1. External Cluster Targeting

The v1 contract is a single environment variable, **`RADIUS_TARGET_KUBECONFIG`**, defined in [pkg/kubeutil/config.go](../../pkg/kubeutil/config.go). The workflow builds a kubeconfig for the user's external cluster, stores it as a Secret in the `radius-system` namespace, and the Helm chart mounts it into the resource providers.

When the variable is unset, every component targets the cluster Radius itself runs on, which is the ordinary self-hosted behavior. When it is set, the *control plane* stays local and only *workloads* are redirected. Three components honor it:

- **dynamic-rp**, for Terraform-backed recipes, through the `ClusterAccessResolver` abstraction in [pkg/recipes/kubernetes/clusteraccess](../../pkg/recipes/kubernetes/clusteraccess). [`resolver.go`](../../pkg/recipes/kubernetes/clusteraccess/resolver.go) selects between the injected kubeconfig ([`injected.go`](../../pkg/recipes/kubernetes/clusteraccess/injected.go)) and in-cluster access ([`local.go`](../../pkg/recipes/kubernetes/clusteraccess/local.go)). [pkg/recipes/terraform/config/providers/kubernetes.go](../../pkg/recipes/terraform/config/providers/kubernetes.go) configures the Terraform Kubernetes provider to match. Terraform *state* deliberately stays on the control-plane cluster; only the provider target moves.
- **The async worker**, in [pkg/server/asyncworker.go](../../pkg/server/asyncworker.go), so directly-rendered Kubernetes resources also land on the external cluster.
- **The deployment engine**, for Bicep-backed recipes, which is a separate repository.

Radius neither creates nor owns the target cluster. It is supplied and must already exist.

**Target clients are resolved lazily and cached in the async worker.** [pkg/server/asyncworker.go](../../pkg/server/asyncworker.go) builds the target Kubernetes clients on the first deployment operation rather than at startup, so a not-yet-mounted or unreachable target fails a deployment instead of preventing the process from starting. A *successful* resolution is then cached for the process lifetime; only a failed resolution is retried. Rewriting the mounted kubeconfig Secret therefore does not affect an already-resolved async worker: kubelet does refresh the file in the pod, but the cached client never re-reads it. This cache is specific to the applications-rp async worker. The `ClusterAccessResolver` used by dynamic-rp has no equivalent. The chart deliberately leaves Secret refresh to the orchestration layer, so credential rotation is a workflow concern and may require restarting consumers.

#### 2. Durable State

Because the control plane is destroyed after every run, its state must be exported and restored. Two CLI commands bracket a run:

- **`rad startup`** ([pkg/cli/cmd/startup](../../pkg/cli/cmd/startup)) restores the previous run's control-plane PostgreSQL databases and the Terraform state Secrets.
- **`rad shutdown`** ([pkg/cli/cmd/shutdown](../../pkg/cli/cmd/shutdown)) captures both and commits them to the archive.

Both use [pkg/cli/pgbackup](../../pkg/cli/pgbackup) for the PostgreSQL databases (`ucp`, `applications_rp`, `dynamic_rp`) and [pkg/cli/tfstate](../../pkg/cli/tfstate) for the Terraform state Secrets, and both write through the pluggable **state archive** abstraction. [state-archive.md](state-archive.md) covers the archive itself: its two interfaces, its git and OCI backends, and its selection logic. Read that document before changing anything under `pkg/statearchive`.

Three behaviors matter specifically for Repo Radius:

- **The database restore is destructive, and the destruction is baked into the dump.** `rad shutdown` runs `pg_dump --format=plain --clean --if-exists` ([pkg/cli/pgbackup/pgbackup.go](../../pkg/cli/pgbackup/pgbackup.go)), so the captured SQL carries its own `DROP ... IF EXISTS` statements. `rad startup` pipes that SQL into `psql` and adds no clean-up logic of its own. The practical consequence is that the restore drops anything created in the control-plane databases *before* `rad startup` runs. Because the flags live on the backup side, they are frozen into each archive at capture time: changing them alters only archives captured afterwards, and every existing archive keeps its original restore behavior.
- **Terraform state restore is an upsert, not a replacement.** `tfstate` creates each Secret or updates it in place, and does not remove Secrets that are absent from the archive. Pruning happens on the *backup* side instead: `Backup` clears the target directory first, so a Secret deleted since the previous run does not linger and get restored. Do not assume the two halves of a restore behave alike.
- **Backend selection is asymmetric on purpose.** In [pkg/statearchive/factory/factory.go](../../pkg/statearchive/factory/factory.go), `NewStateArchive` defaults to OCI even when no registry is configured, so a missing `RADIUS_STATE_REGISTRY` surfaces as a clear configuration error instead of silently falling back to git and writing state somewhere unexpected. `NewGraphArchive` keeps the git fallback, because graph output has a working zero-configuration path that predates OCI.

Restore is also **not transactional**. [pkg/cli/cmd/startup/startup.go](../../pkg/cli/cmd/startup/startup.go) scales the resource providers down, restores the databases, restores Terraform state, then scales back up, and a deferred scale-up is *attempted* even when a restore step fails. That attempt is best-effort: it reuses the same context and only logs a warning if it fails. So a failure between the two restores leaves the control plane partially hydrated, and running only if the deferred scale-up succeeded.

#### 3. Graph Output

`rad app graph` has two *forms* but three *implementations*. The **modeled** form takes a Bicep file and computes the graph the application *would* produce. The **deployed** form takes an application name and queries the running control plane. Both live in [pkg/cli/cmd/app/graph/graph.go](../../pkg/cli/cmd/app/graph/graph.go), where `Validate` picks between them: a single positional argument whose extension is `.bicep` selects the modeled form, and anything else falls through to the deployed one. See [application-graph.md](application-graph.md) for how each graph is built.

The third implementation is the one Repo Radius actually runs. `--preview`, or `RADIUS_PREVIEW=true`, routes to a separate `Radius.Core` runner in [pkg/cli/cmd/app/graph/preview](../../pkg/cli/cmd/app/graph/preview). `wirePreviewSubcommand` in [cmd/rad/cmd/root.go](../../cmd/rad/cmd/root.go) swaps `RunE` outright, so the legacy runner above never executes. That runner accepts a Bicep path *and* `--application` together as a deliberate enriched mode: it compiles the file only to extract `dependsOn` edges, then queries the deployed graph and merges them.

Only the legacy modeled form changes behavior inside a runner. Its `runModeled` consults `inRepoRadiusMode()`, which tests the `GITHUB_ACTIONS` environment variable, and commits the graph to the graph archive instead of writing `./app-graph.json`, because the runner's filesystem is discarded. The graph is stored under a namespace derived from the source branch, so a pull request's graph does not overwrite the target branch's. Graph persistence lives in [pkg/graph/persistence](../../pkg/graph/persistence).

**That archive path is not exercised by the generated workflows today.** Every `rad app graph` the templates issue passes `--preview`, so every one of them lands in the preview runner, which has no archive logic at all. The deploy dispatch builds `app graph --application <name> --preview --include-icons`, and `publish-deploy-status` adds a Bicep path to the same preview call. Reaching the archive takes a custom `rad_commands` entry with a *positional Bicep path* and no `--preview`; merely dropping `--preview` from an `--application` call selects the legacy deployed form, which does not archive either. `RADIUS_GRAPH_REGISTRY` is still plumbed into the workflow environment and documented as optional, so the path is reachable rather than dead. Do not read the archive code as describing what a normal run does.

If you add a new graph form, decide explicitly whether it needs archiving, and check which of the three implementations your caller will actually reach. The mode check is per-form, not global.

Deployment *status*, as distinct from graph topology, is not produced by this repository at all. The `publish-deploy-status` composite action in `ai-extensions` uploads it as a workflow artifact ([`action.yml:216-222`](https://github.com/radius-project/ai-extensions/blob/5712652/.github/extension/actions/publish-deploy-status/action.yml#L216-L222)), and that repository documents the payload contract.

One detail of that action is worth knowing before you change `rad`'s output handling. It runs [`rad app graph "$APP_FILE" --application "$APP_NAME" --preview --include-icons --output json`](https://github.com/radius-project/ai-extensions/blob/5712652/.github/extension/actions/publish-deploy-status/action.yml#L93) and redirects stdout into `deploy-graph.json`. In the enriched shape the preview runner first logs `Compiling <path>`, and `LogInfo` and `WriteFormatted` share one writer, which is stdout. The redirected file therefore begins with a plain-text line ahead of its JSON. The action checks only the command's exit status, so that file is published as-is. Treat stdout in a `--output json` path as the payload, and send progress elsewhere.

### Representative Flow

A single deploy run, reduced to the steps that touch this repository. The workflow steps around them are owned by `ai-extensions` and are shown only for ordering. The phase names are this document's own grouping, not stages named anywhere in the code; indentation marks steps performed by the `rad` command above them.

```text
  WF  = workflow (ai-extensions)      ARC = state archive (GHCR)
  CLI = rad CLI                       TGT = external AKS / EKS cluster
  CP  = control plane (k3d 'radius-cp')

  PREPARE
    WF  -> WF     OIDC login, build target kubeconfig
    WF  -> CP     create k3d cluster 'radius-cp'
    WF  -> CP     rad install kubernetes
                    database.enabled=true
                    global.targetCluster.enabled=true
    WF  -> CP     project cloud OIDC token into RP / DE pods
                  [kubeconfig mounted as a Secret; RADIUS_TARGET_KUBECONFIG
                   points at it, so recipes and the async worker target TGT]

  HYDRATE
    WF  -> CLI    rad startup
    CLI -> ARC      Archive.Open
    CLI -> CP       scale providers to zero
    CLI -> CP       wait for the database to be ready
    CLI -> CP       restore databases (psql)
    CLI -> CP       restore Terraform state Secrets (upsert)
    CLI -> CP       scale providers up
    WF  -> CLI    rad group create / switch
                  [must follow startup, or the restore drops the group]

  DEPLOY
    WF  -> CLI    rad credential register azure wi | aws irsa
                  [must follow startup, so the credential lands in the
                   restored state; must precede the deploy that uses it]
    WF  -> CLI    rad deploy radius-env.bicep
    CLI -> CP       create Radius.Core/environments + recipe packs
    WF  -> CLI    rad deploy app.bicep
    CP  -> TGT      apply workloads via resolved target clients

  RECORD (publish-deploy-status; runs unless the job was cancelled,
          and only when the application name resolves)
    WF  -> CLI    rad app graph app.bicep --application <app> --preview
    CLI -> CP       query the deployed graph, enriched with bicep edges
    WF  -> WF     upload deploy-graph.json + status files as an artifact

  PERSIST
    WF  -> CLI    rad shutdown
    CLI -> ARC      Archive.Open
    CLI -> CP       back up databases (pg_dump --clean --if-exists)
    CLI -> CP       back up Terraform recipe-state Secrets
    CLI -> ARC      Session.Commit
                  [gated on a successful rad startup]
    WF  -> CP     delete cluster
```

Four `[...]` notes appear above. The first is descriptive: it records how `RADIUS_TARGET_KUBECONFIG` gets into the pods. The other three are correctness requirements rather than conveniences, and they do not share a cause.

Of those three, the first two follow from the restore being destructive: the resource group and the registered credential are both dropped if they are created before `rad startup` runs, so both must follow it. The credential additionally has to precede the environment deploy that consumes it.

The last has a different root cause and would survive a change to restore semantics. `rad shutdown` is gated on a successful `rad startup` because an un-restored control plane is empty regardless of *how* the restore works, and committing that empty snapshot destroys the archive it was supposed to extend. Do not remove that gate on the grounds that the restore was made non-destructive.

### Configuration Reference

#### What the User Configures

The GitHub Environment is the unit that names a deployment target and groups its settings, but it is not the only surface a user touches. The others are either files committed to their repository or objects that live outside GitHub entirely.

| Surface                     | Where it lives                                                      | Who creates it                                                  |
|-----------------------------|---------------------------------------------------------------------|-----------------------------------------------------------------|
| GitHub Environment          | Repository settings: Actions **variables**, plus an optional secret | Frontend, via a bodiless `PUT`, then variable writes            |
| Application model           | `.radius/app.bicep`, committed                                      | Frontend generates it; the user reviews and owns it             |
| Deploy workflows            | `.github/workflows/`, committed copies                              | Frontend generates them; updated only by regenerating           |
| Cloud-side OIDC trust       | Azure federated credential, AWS role trust policy                   | Frontend can create it for Azure; by hand for AWS               |
| Target cluster              | AKS, EKS, or any reachable cluster                                  | The user; Radius neither creates nor owns it                    |
| GHCR state package          | Account-owned, linked to the repository                             | Frontend bootstraps it; must be private or internal             |
| Deployment protection rules | On the GitHub Environment                                           | The user only; the frontend creates the environment unprotected |

The environment holds the cloud and cluster configuration as variables, and OIDC covers cloud authentication, so no long-lived cloud credential is ever stored. That is narrower than "no secrets". The environment can also carry `RADIUS_DEPLOY_PARAMS`, an *environment secret* the frontend provisions when the application model declares `@secure()` parameters, and workflows additionally use GitHub's built-in `GITHUB_TOKEN`. What OIDC removes is the stored cloud credential, not every secret. The frontend creates the environment with a bodiless `PUT` ([`github-environment.ts:515`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/adapter-canvas/src/server/services/github-environment.ts#L515)), which is what leaves protection rules unset, and writes the settings as variables afterwards. Those variables are the three state-archive settings from the table below plus the provider's identity and cluster coordinates — `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, and `AZURE_AKS_CLUSTER_NAME`, or `AWS_ROLE_ARN`, `AWS_REGION`, `AWS_ACCOUNT_ID`, `AWS_EKS_CLUSTER_NAME`, `RADIUS_VPC_ID`, and `RADIUS_SUBNET_IDS`.

The cloud-side trust is the one surface in that table that lives outside GitHub, and how it gets created depends on the provider. For Azure the canvas exposes an auto-setup route that shells out to `az` and `gh` to create the App Registration and its federated credentials. For AWS there is no equivalent, so the role trust policy is written by hand. Either way the object ends up in the cloud provider, where nothing in GitHub can keep it in sync.

That is why the environment's *name* is a coupling point rather than a label. In GitHub's default subject format the federated subject is `repo:<owner>/<repo>:environment:<environment-name>`. That default is not guaranteed: a repository or org can customize the claim, and GitHub's immutable-subject rollout changes the default to `repo:<owner>@<ownerId>/<repo>@<repoId>:environment:<environment-name>`, which is why [`oidc-subject.ts:3-17`](https://github.com/radius-project/ai-extensions/blob/5712652/packages/core/src/platforms/oidc-subject.ts#L3-L17) computes the subject rather than hardcoding it. Every form still carries the `environment:<environment-name>` suffix, so the coupling holds regardless: renaming or recreating an environment breaks authentication at the cloud provider, which has no way to learn about the change.

Note that `RADIUS_TARGET_KUBECONFIG`, the variable this repository actually reads to redirect workloads, is never set by the user. The workflow builds the kubeconfig from the environment's cluster coordinates, stores it as a Secret, and the chart mounts it.

#### Variables This Repository Reads

The orchestration variables consumed only by workflow templates are documented in `ai-extensions`.

| Variable                    | Read by                    | Purpose                                                                          |
|-----------------------------|----------------------------|----------------------------------------------------------------------------------|
| `RADIUS_TARGET_KUBECONFIG`  | dynamic-rp, async worker   | Path to the external cluster's kubeconfig. Unset means target the local cluster. |
| `RADIUS_STATE_BACKEND`      | CLI                        | `git` or `oci`. Unset selects per the asymmetry described above.                 |
| `RADIUS_STATE_REGISTRY`     | `rad startup` / `shutdown` | OCI repository holding the control-plane state archive.                          |
| `RADIUS_GRAPH_REGISTRY`     | `rad app graph`            | OCI repository holding modeled graph output.                                     |
| `RADIUS_STATE_ARCHIVE`      | `rad startup` / `shutdown` | Archive name. Supersedes the deprecated `RADIUS_STATE_BRANCH`.                   |
| `RADIUS_ARCHIVE_PLAIN_HTTP` | Archive factory            | Allows plain HTTP for a local registry. Testing only.                            |
| `GITHUB_ACTIONS`            | `rad app graph`            | Detects Repo Radius mode.                                                        |
| `GITHUB_HEAD_REF`           | `rad app graph`            | Source branch for the graph namespace. Falls back to `GITHUB_REF_NAME`.          |
| `RADIUS_PREVIEW`            | `rad` root command         | Routes preview-wired commands to their `Radius.Core` implementation.             |

### State Durability and Reconciliation

The five investments treat the state archive as a faithful snapshot. Hardening work has shown it is better modeled as a **fallible authoritative checkpoint that can diverge from external reality**. The distinction matters. This is not a cache: nothing else holds the data, and no more authoritative source can rebuild it. Losing the Terraform state orphans real cloud infrastructure. The archive is authoritative *and* it can be corrupted, raced, or left describing resources that no longer exist.

Hardening work has surfaced five distinct failure modes, each with a different fix at a different layer:

| Failure                  | Cause                                                                         | Mitigation                            | Layer                |
|--------------------------|-------------------------------------------------------------------------------|---------------------------------------|----------------------|
| Archive never self-heals | A resource created before `rad startup` is dropped by the destructive restore | Create it after startup instead       | Workflow ordering    |
| Good archive overwritten | A run that fails before startup still reaches `rad shutdown`                  | Gate shutdown on a successful restore | Workflow ordering    |
| Degenerate snapshot      | The control plane degrades *after* a successful startup                       | Refuse to commit an empty snapshot    | CLI command          |
| Concurrent runs race     | Two runs share one archive                                                    | Serialize per environment             | Workflow concurrency |
| Partial hydration        | A restore step fails between the databases and the Terraform state            | None today                            | —                    |

The first two are fixed ([#12840](https://github.com/radius-project/radius/pull/12840)). The third is in review ([#12849](https://github.com/radius-project/radius/pull/12849)) and is therefore **not current behavior**. The fourth is partly covered already: the current deploy and delete templates in `ai-extensions` set a repository-wide `concurrency` group, `radius-routes-gateway-${{ github.repository }}` with `cancel-in-progress: false` ([`run-rad-commands-azure.yml:37-39`](https://github.com/radius-project/ai-extensions/blob/5712652/.github/extension/run-rad-commands-azure.yml#L37-L39), [`delete-azure.yml:44-46`](https://github.com/radius-project/ai-extensions/blob/5712652/.github/extension/delete-azure.yml#L44-L46)), which serializes runs across the whole repository rather than per environment. [#12850](https://github.com/radius-project/radius/pull/12850) proposes environment-scoped state serialization on top of that. Workflows generated before those templates landed may have neither guard. The fifth is unmitigated.

The workflow-level guard and the proposed command-level guard are deliberately *both* present and are not redundant: the workflow guard establishes that `rad startup` ran, while the command guard catches a control plane that emptied out afterwards. The command-level check is narrow. As proposed, it tests only whether the `ucp` dump's `resources` table has rows. That blocks one specific, observed corruption mode. It does not validate the `applications_rp` or `dynamic_rp` databases, the Terraform state, or control-plane health generally.

One assumption behind the concurrency row does not hold on its own. Serializing runs bounds the race only if runs are short. A GitHub deployment protection rule can suspend a queued run indefinitely, so an environment with required reviewers or a wait timer can hold a run pending long after later runs have superseded the archive it will hydrate from. `concurrency:` prevents two runs executing at once. It does not make a long-delayed run notice that its view of the world is stale.

**Reconciliation** is the open half of this theme. A restored archive can preserve a resource in a non-terminal `provisioningState` for something that no longer exists on the target cluster, which makes `rad app delete` retry forever against a `409`. The proposed fix ([#12871](https://github.com/radius-project/radius/pull/12871), open and draft) adds a `reconcile` custom action on `Radius.Core/applications`, a `/reconcile` route on dynamic-rp types, and a best-effort reconciliation stage in `rad startup` that checks each hydrated resource's output resources against reality. Two of its design decisions are worth carrying forward:

- A Kubernetes `404` marks the resource `Failed` and **keeps the row** rather than deleting it, so an ordinary `rad app delete` cleans it up through the normal state machine.
- No `--force` flag was added to `rad app delete`, because force-updating the database breaks that state machine.

Cloud outputs backed by Terraform are explicitly out of scope for the first iteration and are reported as skipped. The divergence this section describes therefore remains unaddressed for exactly the resources whose loss is most expensive.

### Known Gaps

Grouped by why they are missing, which matters more than the individual items.

**Not started.** Control-plane startup time (Investment 5), the specification's leading non-functional requirement, with no implementation.

**Deferred by design.** What-if / preview deployment, and migration from Repo Radius to a self-hosted installation. Both were scoped out of the original specification as needing their own design.

**Specified but unbuilt.** A `ref` input allowing deployment of a revision other than the latest, which is the prerequisite for any promotion flow ([#12527](https://github.com/radius-project/radius/issues/12527)); and a GitHub Deployment record per deploy, deactivated on application delete ([#12528](https://github.com/radius-project/radius/issues/12528)).

**Known defects.** `rad app delete --preview` can report success while recipe-generated Kubernetes resources remain on the target cluster ([#12878](https://github.com/radius-project/radius/issues/12878)). This orphans workloads *and* destroys the Radius records needed to clean them up later, and it blocks shared Gateway teardown because the remaining Route objects correctly cause the Gateway to be retained. This is a *distinct* problem from the `409` delete loop that reconciliation addresses. There, delete fails and retries forever; here, delete succeeds while leaving resources behind. Do not treat a fix for one as a fix for the other.

**Runtime and environment constraints.** These are not code defects in the delete path but they block real deployments:

- Deploys cannot schedule the PostgreSQL-backed control plane on standard private-repository runners ([#12857](https://github.com/radius-project/radius/issues/12857)), because the default resource request exceeds what a 2-vCPU runner can satisfy.
- Deploying to an AKS Automatic cluster fails because the runner lacks `kubelogin` ([#12550](https://github.com/radius-project/radius/issues/12550)). Two independent mechanisms are easy to confuse here: *workload identity*, which lets a pod obtain a cloud token and behaves the same on both AKS variants, and *cluster access*, which lets a client reach the Kubernetes API server. Only the second is broken. AKS Automatic disables local admin accounts and uses Azure RBAC for Kubernetes, so it always requires `kubelogin convert-kubeconfig`, both on the runner and inside the resource-provider images.

### Change-Safety Guidance

- **Do not add workflow templates or composite actions to this repository.** They live in `ai-extensions`. Changing orchestration here will be silently ignored by users, whose workflows resolve actions from that repository. Note also what that does *not* buy you. Because a released plugin stamps composite-action references with the commit it was built from, a fix merged to `ai-extensions` reaches an existing user repository only when that user regenerates with a newer plugin. Treat composite-action inputs as a versioned public contract, and do not assume a fix propagates on its own.
- **`deploy/install.sh` does not reach Repo Radius users when you change it.** Everything else in this repository does, through the `edge` channel, but the installer script itself is fetched at a literal commit and checked against a recorded SHA-256 in `setup-control-plane`. Changing the script here has no effect until someone bumps both values in `ai-extensions`, and changing it without bumping the checksum breaks every run that pins the old bytes. This is the one place where "this repository ships on the next run" stops being true.
- **Treat `RADIUS_TARGET_KUBECONFIG` as a contract, not an implementation detail.** Any new component that creates Kubernetes resources on behalf of a user's application must honor it, or those resources will land on the ephemeral control-plane cluster and vanish. The existing three consumers are the pattern to follow.
- **Assume state restore is destructive.** Anything the control plane must contain has to be created *after* `rad startup`, or be present in the archive.
- **Guard writes to the archive.** A commit that persists degraded state is worse than no commit, because the corruption propagates to every subsequent run. Prefer failing loudly over persisting something questionable.
- **Preview-surface commands need `--preview`.** Repo Radius provisions `Radius.Core` resources throughout, so any command that must see them needs the flag, or `RADIUS_PREVIEW=true`. Two different mechanisms supply it, which is why the surface is easy to misread. Most commands are wired by `wirePreviewSubcommand` in [cmd/rad/cmd/root.go](../../cmd/rad/cmd/root.go), which swaps `RunE` to a separate `Radius.Core` runner: `rad app list`, `show`, `status`, `graph`, and `delete`; `rad resource list`; `rad env create`, `delete`, `list`, `show`, `update`, and `switch`; `rad init`; and `rad workspace create`. `rad deploy` is not wired that way. It registers its own `--preview` flag in [pkg/cli/cmd/deploy/deploy.go](../../pkg/cli/cmd/deploy/deploy.go), where the flag selects only which application resource type it creates. `rad run` embeds the same runner without registering the flag, so preview is unavailable there.
- **The generated deploy does not pass `--preview`, and that is a live inconsistency.** `delete-resource` passes it, and so does the graph call in `publish-deploy-status`, but the `rad deploy` that created those resources does not, in either the composite action or the command the canvas builds. Omitting it routes to the legacy `Applications.Core` implementation, which is a different plane rather than an error. The failure mode is therefore a *plausible wrong answer* rather than a diagnostic, and it varies by command: `rad app list` returns an empty list, `rad app delete` logs `Applications.Core/applications/<name> not found` and exits successfully, and `rad app show` returns a not-found error. Only the last is obviously wrong. When something "does not exist" while debugging a Repo Radius run, check the flag before believing the result.
- **Exercise the database-backed control plane.** Repo Radius installs with `database.enabled=true`, which is not the default. The `database-noncloud` functional test group exists for exactly this reason.

## Related Material

- [state-archive.md](state-archive.md) — the durable archive abstraction and its backends.
- [application-graph.md](application-graph.md) — how the graph is computed and rendered.
- [credentials.md](credentials.md) — cloud credential storage and client authentication.
- [rad-cli.md](rad-cli.md) — CLI wiring, including Windows child-process behavior relevant to frontends that spawn `rad`.
- [repo-radius-state-e2e.md](../contributing/contributing-code/contributing-code-tests/repo-radius-state-e2e.md) — the scheduled end-to-end test that exercises a full state round-trip.
- [contributing-deploy-environments.md](../contributing/contributing-deploy-environments.md) — setting up a deploy environment as a contributor.
- [`ai-extensions/.github/extension/README.md`](https://github.com/radius-project/ai-extensions/blob/main/.github/extension/README.md) — the normative workflow contract.
- Design notes under [eng/design-notes](../../eng/design-notes) record the original decisions: [multi-cluster](../../eng/design-notes/environments/2026-06-multi-cluster.md), [state storage](../../eng/design-notes/2026-06-repo-radius-state-storage.md), [the OCI archive](../../eng/design-notes/architecture/2026-07-oci-ghcr-state-archive.md), and [the deploy workflow](../../eng/design-notes/environments/2026-06-repo-radius-deploy-workflow.md).
