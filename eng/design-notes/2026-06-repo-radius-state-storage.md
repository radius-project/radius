# Repo Radius — State Storage (Technical Design)

* **Author**: Sylvain Niles (@sylvainsf)
* **Status**: Draft
* **Feature spec**: Repo Radius (Zach Casper) — [PR #12078](https://github.com/radius-project/radius/pull/12078)
* **Related issues**: [#8096 External data store for Radius](https://github.com/radius-project/radius/issues/8096), [#8398 Postgres DB initialization](https://github.com/radius-project/radius/issues/8398)
* **Supersedes prototype**: [PR #11457](https://github.com/radius-project/radius/pull/11457) (closed unmerged)

## Scope

This document covers **Investment 2 of the Repo Radius feature spec: externalization of the
Radius data store** — and *only* the state-storage aspects of it. It addresses how Radius
control-plane state and Terraform recipe state survive across ephemeral GitHub Actions runs.

Explicitly **out of scope** for this document:

* Application graph storage / serialization (a separate workstream owns the serialized graph format).
* The Repo Radius workflow contract, OIDC/cloud credential integration, and external-cluster deployment (Investments 1, 3, 4).
* Replacing the persistent control-plane deployment model.

## Problem

Repo Radius runs the Radius control plane on an **ephemeral k3d cluster** inside a GitHub
Actions runner. The cluster — and everything stored in it — is destroyed when the workflow
ends. For Radius to be usable across runs, all durable state must be exported before teardown
and restored on the next run.

"Durable state" in the current control plane is **two physically separate stores**:

| State | Where it lives today | Survives teardown without action? |
|-------|----------------------|-----------------------------------|
| Control-plane resource data + deployment history | PostgreSQL — three logical databases (`ucp`, `applications_rp`, `dynamic_rp`) behind [`database.Client`](../../pkg/components/database/client.go) | No |
| Terraform recipe state | Kubernetes `Secret` objects named `tfstate-default-<sha1>` in the `radius-system` namespace, written by the [`kubernetes` Terraform backend](../../pkg/recipes/terraform/config/backends/kubernetes.go) | No |

## Gaps in the prototype / demo

The prototype ([PR #11457](https://github.com/radius-project/radius/pull/11457), branch
`filesystem-state`) demonstrated state persistence by dumping the three PostgreSQL databases to
a git orphan branch and restoring them on startup. Reviewing it against the requirements above
surfaced the following gaps. This design addresses them.

### Gap 1 — Terraform state is never persisted

The prototype's backup covers only the three PostgreSQL databases. **Terraform state is stored
in Kubernetes Secrets, not in PostgreSQL**, so it is destroyed on every shutdown and never
restored.

**Why the demo still worked:** the demo exercised paths that do not depend on Terraform state
surviving:

* **Bicep recipes have no local state.** The Deployment Engine reconciles declaratively against
  ARM/cloud, which is idempotent. Restoring PostgreSQL is sufficient.
* **First-time Terraform deploys** start from an empty backend and succeed; the cluster is alive
  for the whole run, so state exists *within* a run.

The gap only manifests on a **second deploy of the same Terraform-backed resource across two
runs** (feature-spec Scenario 4, "update a deployed application"). After a PostgreSQL restore,
Radius believes the resource exists, but the fresh cluster's Terraform backend is empty. The
next `terraform apply` plans from scratch, producing "already exists" errors for
stable-named resources or **orphaned duplicate** cloud resources for generated-named ones.

This is the primary capability gap closed by this design.

### Gap 2 — PostgreSQL was not actually usable for all RPs

Independent of Repo Radius, enabling `database.enabled=true` in the Helm chart did not produce a
working PostgreSQL-backed control plane:

* The UCP, Applications RP, and Dynamic RP configmaps/deployments were **hardcoded to the
  `apiserver` provider** with no `database.enabled` conditional, so no resource provider used
  PostgreSQL even when requested.
* The `POSTGRES_DB` secret value was the literal string `"POSTGRES_DB"`.
* No init-db scripts ran, so the per-RP databases, users, and tables were never created.
* The `databaseProvider` URL env-var substitution in
  [`factory.go`](../../pkg/components/database/databaseprovider/factory.go) had a regex bug that
  replaced the entire URL with the first captured variable name instead of expanding `${VAR}`.

These are pre-existing defects on the path that [#8398](https://github.com/radius-project/radius/issues/8398)
("Postgres DB initialization") already chose to fix via an init-db configmap. This design
includes those fixes as a prerequisite.

### Gap 3 — Backup push failures were silent

The prototype's `Push` treated a failed `git push` as a non-fatal warning. That is acceptable
for advisory sentinel files but means a failed **state backup** push is silent data loss. The
durability requirement is that a backup either succeeds or fails the command loudly.

### Gap 4 — The deploy lock was advisory, not a real mutex

The prototype's `.deploy-lock` was read from the local worktree and then pushed, so two runners
could both observe "no lock" and both proceed. A correct lock must use an atomic
compare-and-swap.

## Design decisions

### Decision 1 — Storage backend: git orphan branch for v1, OCI/GHCR deferred to v2

State is persisted to a **git orphan branch** (`radius-state`) in the same repository, as in the
prototype.

The feature spec raises OCI/GHCR as an alternative with better security and size properties. We
evaluated leading with OCI for v1 and rejected it:

* **Speed**: backup payloads are small (PostgreSQL dumps of control-plane metadata are
  hundreds of KB). At that size both `git push` and `oras push` are dominated by TLS + auth
  round-trips, not payload, so there is no meaningful speed advantage.
* **Simplicity**: the orphan-branch implementation already exists and ran in the demo. OCI
  requires new push/pull plumbing, a tag compare-and-swap lock, and an artifact media-type
  decision — strictly more code to ship first.

OCI/GHCR's genuine advantages are **security** (separate registry RBAC versus repository-read
exposure of the orphan branch) and **bounded storage growth** (content-addressed layers versus
unbounded git history). Both are real but neither is a v1 blocker. They are recorded as the
**v2 direction** below.

### Decision 2 — Control-plane state: physical `pg_dump`, not logical export

Control-plane state is captured with physical `pg_dump` / `psql` against the in-cluster
PostgreSQL, as in the prototype.

A logical export through `database.Client` was considered (it would be storage-engine
independent and could feed an offline graph reader). It is rejected because:

* Radius control-plane state is a graph of linked key/value records with ETags and
  cross-references. A logical re-`Save` mints new ETags and forces a dependency-ordered restore —
  strictly more fragile than a physical dump that preserves rows, ETags, and timestamps exactly.
* Its main upside, offline graph readability, is **out of scope** here and owned by a separate
  serialized-format workstream.
* The control plane has a fixed, known set of resource providers; we are not adding new RPs, so
  the fixed database list (`ucp`, `applications_rp`, `dynamic_rp`) is acceptable.

### Decision 3 — Terraform state: back up the backend Secrets alongside the PostgreSQL dumps

For v1, Terraform state is persisted by exporting the `tfstate-default-*` Kubernetes Secrets from
the `radius-system` namespace into the same state worktree as the PostgreSQL dumps, committed and
pushed in the same atomic operation. On startup, after the cluster is ready and **before any
deploy**, the Secrets are restored into the namespace.

Secrets are selected by the `tfstate=true` label that the Terraform Kubernetes backend applies to
every state Secret, rather than by name. This automatically captures the additional
`tfstate-{workspace}-{suffix}-{index}` Secrets that the backend creates when chunking large
state. The Lease resources the backend uses for locking are intentionally **not** backed up; they
are ephemeral and irrelevant across runs.

This mirrors the existing PostgreSQL backup flow exactly (same worktree, same commit/push, same
semaphore) and is the minimal change that closes Gap 1.

A v2 alternative — switching the Terraform backend from `kubernetes` to the `pg` backend pointed
at the same PostgreSQL instance, so a single `pg_dump` captures both stores — is recorded below.
It is deferred because it requires backend credential injection and per-recipe state isolation
work beyond v1 scope.

### Decision 4 — Durability and locking hardening

* The **final state-backup push must fail the command** on error (Gap 3). Advisory sentinel
  pushes remain best-effort.
* The deploy lock uses git's own compare-and-swap: acquisition commits the lock file and pushes;
  a non-fast-forward **push rejection is a failed acquisition** (Gap 4). On rejection, fetch and
  apply the existing `RunID`/`RunAttempt` takeover logic for same-run retries. The lock is keyed
  by **GitHub Environment name** so deploys to different environments do not serialize against
  each other.
* A small checksum manifest accompanies the dumps so a corrupt restore **fails closed** rather
  than silently starting from an empty state.

## v2 direction (not in this delivery)

* **OCI/GHCR backend**: a pluggable storage backend that pushes an encrypted, content-addressed
  state artifact to a private GHCR repo, with a tag compare-and-swap lock. Resolves the
  orphan-branch security exposure and unbounded git-history growth.
* **Unified Terraform backend on PostgreSQL**: move the Terraform backend to `pg` so control-plane
  and Terraform state collapse into a single dump and a single restore.
* **Client-side envelope encryption** (age/sops) of state artifacts before they leave the cluster,
  since both PostgreSQL data and Terraform state can contain secrets.

## Delivery plan

| PR | Contents | Closes |
|----|----------|--------|
| PR 1 | PostgreSQL enablement fixes: Dynamic RP `database.enabled` conditional, init-db configmap, `POSTGRES_DB` value, `factory.go` env-var substitution, Helm chart tests | Gap 2 |
| PR 2 | Terraform-state Secret backup/restore wired into the existing state-worktree flow | Gap 1 |
| PR 3 | Durability hardening: loud backup-push failure, push-rejection CAS lock, checksum manifest | Gaps 3, 4 |

## Test plan

* **Unit**: Helm chart conditional rendering (PR 1); Terraform-state export/import round-trip with
  a fake Kubernetes client (PR 2); push-failure and lock-contention paths (PR 3).
* **Functional**: an end-to-end lifecycle that deploys a **Terraform-backed** resource, shuts
  down, restarts, and deploys an **update** to the same resource — the path that exposes Gap 1.

## Security

* State (PostgreSQL dumps and Terraform Secrets, which may contain secret values) is pushed to a
  branch in the repository. For v1 the repository **must be private**; this constraint is removed
  in v2 by the encrypted OCI backend.
* Git credentials use the GitHub Actions token; pushing the state branch requires
  `contents: write`.
