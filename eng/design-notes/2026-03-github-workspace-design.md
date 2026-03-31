# GitHub Actions Workspace with Git-Backed State Persistence

* **Author**: Sylvain Niles (@sylvainsf)

## Overview

This feature introduces a new `github` workspace kind for Radius that enables ephemeral CI/CD environments in GitHub Actions workflows. Each workflow run spins up a full Radius control plane in a k3d cluster (UCP, Controller, Applications RP, Dynamic RP, Deployment Engine, and a single PostgreSQL instance) and persists state between runs by backing up the PostgreSQL databases to a git orphan branch (`radius-state`). A deploy lock mechanism prevents concurrent deploys from colliding in shared-runner scenarios.

The goal is to let teams use Radius in GitHub Actions with the familiar `rad init` and `rad deploy` commands, without requiring long-lived infrastructure. A new `rad shutdown` command handles state backup and cluster teardown. State survives workflow restarts, spot instance preemptions, and retry attempts.

## Terms and definitions

| Term | Definition |
|------|-----------|
| **Orphan branch** | A git branch with no shared history with `main`. Used here as a key-value store for state files (SQL backups, sentinel files). |
| **Sentinel file** | A marker file on the orphan branch that indicates lifecycle state: `.lock` (deploy in progress), `.backup-ok` (clean shutdown), `.deploy-lock` (concurrent deploy guard). |
| **Semaphore state** | The inferred lifecycle phase based on which sentinel files exist: `FirstRun`, `Clean`, or `Interrupted`. |
| **State worktree** | A `git worktree` checked out in `/tmp/` pointing at the `radius-state` orphan branch. Isolates state files from the application checkout. |
| **Deploy lock** | A JSON file (`.deploy-lock`) recording the GitHub Actions RunID, RunAttempt, JobName, and Repository to prevent concurrent deploys. |

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/pull/11457

### Goals

- Enable Radius usage in GitHub Actions workflows with no long-lived infrastructure.
- Persist Radius state (resource groups, environments, applications, recipes) across workflow runs using git as the storage backend.
- Handle spot instance preemptions and retry attempts gracefully via semaphore-based recovery.
- Prevent concurrent deploys from corrupting state via a deploy lock mechanism.
- Provide a clean lifecycle: `rad init --kind github` (one-time setup), then repeated cycles of spin up cluster, restore state, `rad deploy`, `rad shutdown` (backup + teardown).

### Non goals

- **Multi-cluster support**: Only k3d single-node clusters are supported initially. Separating the control plane k3d cluster from the target Kubernetes cluster for deployments is an immediate follow-on.
- **Non-GitHub CI systems**: This design is specific to GitHub Actions. GitLab CI, Azure DevOps, and similar systems are future work.
- **Partial restore**: If a backup is corrupt, the entire init falls back to a fresh start. Incremental restore is not planned.

### User scenarios

#### Scenario 1: First-time setup in a GitHub Actions workflow

A developer runs `rad init --kind github` once (locally or in a setup workflow). This creates the initial workspace configuration and performs the first deployment cycle: spin up k3d, install Radius with PostgreSQL, deploy the application, then `rad shutdown` to back up state and tear down the cluster.

#### Scenario 2: Subsequent deploys triggered by UI or workflow dispatch

After the initial setup, subsequent deploys are triggered via a UI element (e.g., a "Deploy" button in a GitHub Actions `workflow_dispatch`) or by push events. The deploy workflow spins up a fresh k3d cluster, restores state from the `radius-state` orphan branch, runs `rad deploy`, then `rad shutdown` to persist state and tear down. 

#### Scenario 3: Spot instance preemption recovery

A deploy workflow is interrupted mid-deploy (spot instance reclaimed). The `.lock` sentinel remains on the orphan branch. On the next deploy workflow run, the system detects the `Interrupted` state, logs a warning, and starts fresh without attempting to restore potentially inconsistent state.

#### Scenario 4: Concurrent deploy prevention

Two workflow runs attempt to `rad deploy` simultaneously. The first acquires the deploy lock (writes `.deploy-lock` with its RunID). The second sees a held lock with a different RunID and fails with `ErrDeployLockHeld`. If the second run is a retry of the first (same RunID, higher RunAttempt), it takes over the lock.

## User Experience

**Workflow YAML:**
```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: write  # Required to push to radius-state branch
    steps:
      - uses: actions/checkout@v4
      - name: Init Radius
        run: rad init --kind github
      - name: Deploy
        run: rad deploy app.bicep
      - name: Shutdown
        if: always()
        run: rad shutdown
```

**Sample `rad init --kind github` output:**
```
Checking prerequisites...
✅ k3d is installed
Opening state worktree...
Creating k3d cluster 'radius-github'...
✅ Cluster created (context: k3d-radius-github)
Installing Radius with PostgreSQL...
✅ Radius installed
Waiting for database...
✅ PostgreSQL ready
Restoring state from previous run...
✅ State restored (3 databases)
```

**Sample `rad shutdown` output:**
```
Backing up PostgreSQL state...
✅ State backed up
Pushing state to radius-state branch...
✅ State persisted
Deleting k3d cluster...
✅ Cluster deleted
```

## Design

### High Level Design

The feature adds three capabilities layered on top of the existing Radius CLI:

1. **Lifecycle management** (`rad init --kind github` / `rad shutdown`): Creates and tears down ephemeral k3d clusters with PostgreSQL-backed Radius installations.
2. **State persistence** (`gitstate` package): Uses a git orphan branch as a durable store for SQL backup files and sentinel markers.
3. **Concurrency control** (`deploy lock`): Prevents concurrent `rad deploy` commands from corrupting shared state.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│  GitHub Actions Runner                                   │
│                                                          │
│  ┌──────────┐    ┌──────────┐    ┌───────────────────┐  │
│  │ rad init  │───▶│ gitstate │───▶│ radius-state      │  │
│  │ --kind    │    │ worktree │    │ (orphan branch)   │  │
│  │ github    │    │ /tmp/... │    │                   │  │
│  └────┬─────┘    └──────────┘    │ .lock             │  │
│       │                          │ .backup-ok        │  │
│       ▼                          │ .deploy-lock      │  │
│  ┌──────────┐                    │ ucp.sql           │  │
│  │   k3d    │                    │ applications_rp.sql│  │
│  │ cluster  │                    │ dynamic_rp.sql    │  │
│  │          │                    └───────────────────┘  │
│  │ ┌──────┐ │                           ▲               │
│  │ │ UCP  │ │    ┌──────────┐           │               │
│  │ │ RP   │ │◀──▶│PostgreSQL│──pg_dump──┘               │
│  │ │ Ctrl │ │    │ (in-k3d) │◀──psql restore            │
│  │ └──────┘ │    └──────────┘                           │
│  └──────────┘                                           │
│       ▲                                                  │
│       │                                                  │
│  ┌────┴─────┐                                           │
│  │rad deploy│                                           │
│  │rad shutdown                                          │
│  └──────────┘                                           │
└─────────────────────────────────────────────────────────┘
```

### Detailed Design

#### 1. Git State Management (`pkg/cli/gitstate`)

The `StateWorktree` struct manages all git operations against the `radius-state` orphan branch.

**Branch isolation**: The orphan branch has no common ancestor with `main`. State files are never visible in the application checkout. A `git worktree add` in `/tmp/radius-state-<nanotime>/` provides a separate working directory for state operations.

**Lifecycle operations**:
- `OpenOrCreate()`: Fetches the remote orphan branch or creates it. Checks out into a temp worktree.
- `CheckSemaphore() -> SemaphoreState`: Reads sentinel files to determine state: `FirstRun` | `Clean` | `Interrupted`.
- `WriteLock()`: Writes `.lock`, commits, and pushes. Done **before** cluster creation to mark intent (spot safety).
- `ClearLock()`: Writes `.backup-ok`, removes `.lock`, stages all files (including SQL dumps), commits, and pushes atomically.
- `Remove()`: Tears down the worktree.

**Why git?** GitHub Actions runners have git pre-installed with push credentials already configured via the `actions/checkout` step. No external storage (S3, Azure Blob) is needed. The orphan branch serves as a zero-dependency persistence layer.

#### 2. PostgreSQL Backup/Restore (`pkg/cli/pgbackup`)

A single PostgreSQL instance hosts three logical databases: `ucp`, `applications_rp`, and `dynamic_rp`. All three are owned by separate PostgreSQL users but served by the same pod.

- **Backup**: `kubectl exec` into the single PostgreSQL pod and runs `pg_dump --clean --if-exists` for each logical database, writing one SQL file per database to the state worktree directory.
- **Restore**: `kubectl exec` into the same pod and pipes each SQL file via `psql` into the corresponding database. The `--clean` flag in the dump ensures idempotent restores.
- **Readiness**: Polls `kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=database` before any operation.

#### 3. k3d Cluster Management (`pkg/cli/k3d`)

- Default cluster name: `radius-github`
- Created with `k3d cluster create radius-github --wait --timeout 120s`
- Kubeconfig context: `k3d-radius-github`
- Idempotent: checks if cluster exists before creating.
- `rad shutdown` always deletes the cluster after backing up state.

#### 4. Deploy Lock (`rad deploy` extension)

The deploy lock prevents concurrent `rad deploy` commands:

| Lock state | Same RunID, higher attempt | Same RunID, same attempt | Different RunID |
|------------|---------------------------|--------------------------|-----------------|
| No lock | Acquire | Acquire | Acquire |
| Lock held | **Take over** (retry) | **ErrDeployLockHeld** | **ErrDeployLockHeld** |

The lock is a JSON file (`.deploy-lock`) containing `{RunID, RunAttempt, JobName, Repository}`, sourced from GitHub Actions environment variables.

Lock acquisition and release are injected into the deploy runner via a function field, making it testable and no-op for non-GitHub workspaces.

#### 5. Helm Chart Changes

When `database.enabled=true`:
- A single PostgreSQL StatefulSet is deployed (image: `postgres:16-alpine`).
- An init-db ConfigMap creates three logical databases (`ucp`, `applications_rp`, `dynamic_rp`) and their owners within that single instance.
- UCP, Controller, Applications RP, and Dynamic RP configmaps switch from the `apiserver` storage provider to the `postgresql` provider with connection URLs pointing to their respective logical databases.
- The `POSTGRES_PASSWORD` env var is injected into control plane deployments from the database secret.

When `database.enabled=false` (default): No change to existing behavior.

#### 6. Semaphore State Machine

```
                     ┌─────────────┐
       first run     │  FirstRun   │ No sentinel files
       ─────────────▶│  (fresh)    │
                     └──────┬──────┘
                            │ rad init → WriteLock
                            ▼
                     ┌─────────────┐
                     │ Interrupted │ .lock present
                     │ (in-flight) │
                     └──────┬──────┘
                            │ rad shutdown → ClearLock
                            ▼
                     ┌─────────────┐
                     │   Clean     │ .backup-ok present
                     │ (restorable)│
                     └──────┬──────┘
                            │ rad init → WriteLock
                            ▼
                     ┌─────────────┐
                     │ Interrupted │ (cycle continues)
                     └─────────────┘
```

#### Proposed Option

Git orphan branch was chosen over alternatives (see Alternatives section) because it requires zero additional infrastructure, works with GitHub Actions' existing git credentials, and provides atomic commit+push semantics.

#### 7. Resource Type Sync (`rad resource-type sync`)

After a PostgreSQL restore, the registered resource types reflect the saved state. If the upstream `resource-types-contrib` OCI artifact has been updated between workflow runs (e.g., new resource types added or API versions changed), the restored types will be stale. A sync command is required for clean state after restore.

**Command:**
```
rad resource-type sync --source oci://ghcr.io/radius-project/resource-types-contrib:latest [--force]
```

**Implementation approach:**

The command builds on existing infrastructure:
- **OCI pull**: The `pkg/rp/util/registry.go` package already uses the ORAS library to pull from OCI registries. The sync command adapts this pattern to pull resource type manifest files from an OCI artifact.
- **Manifest parsing**: `pkg/cli/manifest.ValidateManifest()` parses manifest files into `ResourceProvider` structs.
- **Registration**: `manifest.RegisterResourceProvider()` calls `ResourceTypesClient.BeginCreateOrUpdate()` against UCP to register or update types.
- **Listing existing types**: `ResourceTypesClient.NewListPager()` retrieves currently registered types for comparison.

**Sync algorithm:**
1. Pull the manifest bundle from the OCI source.
2. Parse each manifest into `ResourceProvider` structs.
3. List currently registered resource types from UCP.
4. For each type in the pulled manifests:
   - If the type does not exist in UCP, register it (new type added upstream).
   - If the type exists and the definition matches, skip it (no change).
   - If the type exists but the definition differs, this is a **conflict**.
5. **Conflict handling** depends on mode:
   - **Without `--force`** (standalone use): Log each conflict with details of what changed, sync all non-conflicting types, then exit non-zero. The CLI output includes remediation guidance:
     ```
     Conflict: Radius.Datastores/redisCaches
       Upstream changed capability set (added "manual")
     
     Non-conflicting types were synced successfully.
     To accept all upstream changes: rad resource-type sync --source <oci> --force
     ```
   - **With `--force`**: Overwrite all conflicting types with the upstream definition. Log a **WARNING** line for each overwritten type so the user can identify the cause if something breaks:
     ```
     WARNING: Overwriting resource type Radius.Datastores/redisCaches (upstream definition differs from restored state)
     WARNING: Overwriting resource type Radius.Messaging/rabbitMQQueues (upstream definition differs from restored state)
     Synced 14 resource types (2 overwritten, 4 new, 8 unchanged)
     ```

**Integration with restore flow:**

In `runGitHubPostInstall()`, after a successful restore (SemaphoreClean), sync runs automatically **with force semantics**. Each overwritten type is logged as a WARNING. This is necessary because in the GitHub workspace model the control plane cluster only exists during the workflow run. The user has no opportunity to manually resolve conflicts between runs, so upstream definitions must take precedence. If the force sync fails for a non-conflict reason (e.g., network error pulling the OCI artifact), log a warning and continue with the restored state.

**Package location**: `pkg/cli/cmd/resourcetype/sync/`

### API design

N/A. No REST API changes. All changes are CLI-side.

### CLI Design

**New commands:**
```
rad shutdown [--workspace <name>]
```
- Backs up PostgreSQL state, pushes to the orphan branch, and deletes the k3d cluster.
- Only supported for `github` workspace kind.

```
rad resource-type sync --source <oci-reference> [--force]
```
- Pulls resource type manifests from an OCI artifact and syncs them into UCP.
- Registers new types, skips unchanged types.
- Without `--force`: reports conflicts and exits non-zero with remediation guidance.
- With `--force`: overwrites conflicting types with upstream definitions.
- Called automatically with force semantics during post-restore in `rad init --kind github`. Also available for standalone use.

**Modified commands:**
```
rad init --kind github
```
- New `--kind` value: `github`. Creates k3d cluster, installs Radius with PostgreSQL, restores state if available.

```
rad deploy <template>
```
- Acquires deploy lock for `github` workspaces before deploying.
- Releases lock after deploy completes (success or failure).

**New workspace kind in `~/.rad/config.yaml`:**
```yaml
workspaces:
  items:
    my-github-ws:
      connection:
        kind: github
        context: k3d-radius-github
      scope: /planes/radius/local/resourceGroups/default
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
```

### Implementation Details

#### UCP
- PostgreSQL storage provider is now usable when `database.enabled=true` in the Helm chart. The provider was already implemented; this feature enables it via chart configuration.

#### Controller
- No storage changes. Controller uses UCP for state.

#### Core RP / Portable Resources RP
- Same PostgreSQL provider enablement as UCP.

#### Dynamic RP
- Adds PostgreSQL provider support to the Dynamic RP configmap and deployment. On `main`, Dynamic RP was hardcoded to the `apiserver` storage provider with no `database.enabled` conditional, meaning `database.enabled=true` did not work for Dynamic RP. This design fixes that by adding the same conditional template pattern used by UCP and Applications RP: when `database.enabled=true`, the configmap uses the `postgresql` provider with a connection URL, and the deployment injects `POSTGRES_PASSWORD` from the database secret.

#### Deployment Engine
- No changes.

#### Bicep
- No changes.

### Error Handling

| Error scenario | Behavior |
|----------------|----------|
| k3d not installed | `rad init --kind github` fails with clear error message and install instructions |
| PostgreSQL not ready within timeout | `rad init` fails; `.lock` remains on orphan branch; next run detects `Interrupted` state |
| Spot instance kills runner mid-deploy | `.lock` remains; next run starts fresh (no restore from potentially corrupt state) |
| Concurrent `rad deploy` | Second deploy fails with `ErrDeployLockHeld` and message identifying the holder |
| `git push` conflict on orphan branch | Currently not handled with retry; relies on GitHub Actions job isolation |
| Corrupt SQL backup | `psql` restore fails; `rad init` reports error; user must delete orphan branch and start fresh |

## Test plan

- **Unit tests**: All new packages (`gitstate`, `pgbackup`, `k3d`, `shutdown`, `recipepack`) have unit tests.
- **Functional test workflow**: `.github/workflows/functional-test-github-workspace.yaml` runs an end-to-end lifecycle:
  1. `rad init --kind github` (first run, `FirstRun` state)
  2. `rad shutdown` (backup + destroy)
  3. `rad init --kind github` (second run, `Clean` state, restore)
  4. Verify backup files exist on orphan branch
- **Helm chart tests**: Unit tests for conditional PostgreSQL template rendering.
- **Test isolation**: The functional test workflow sets `RADIUS_STATE_BRANCH=radius-state-test-<run_id>` so each run uses a unique branch name. The cleanup phase deletes both the local and remote branch (`git push origin --delete`) to prevent test artifacts from leaking into the repository. The `gitstate.BranchName()` function reads this env var, falling back to the default `radius-state` when unset. This ensures the test exercises the same code paths as production without leaving state behind.

## Security

- **Git credentials**: Uses the GitHub Actions `GITHUB_TOKEN` provided by `actions/checkout`. Requires `contents: write` permission to push the orphan branch.
- **Database credentials**: PostgreSQL passwords are stored in Kubernetes secrets within the ephemeral k3d cluster. Passwords are generated at install time and not persisted to git (only the SQL data is persisted, not the credentials).
- **SQL backup exposure**: SQL dumps are pushed to the `radius-state` branch in the repository. For public repos, this data would be publicly visible. Users should ensure their repo is private or that the backed-up state doesn't contain sensitive data.
- **Deploy lock spoofing**: The deploy lock trusts GitHub Actions environment variables (`GITHUB_RUN_ID`, etc.). These cannot be spoofed within GitHub Actions but could be set manually in local use.

## Compatibility

- **Backward compatible**: The `github` workspace kind is additive. Existing `kubernetes` workspaces are unaffected.
- **Helm chart**: `database.enabled` defaults to `false`, preserving existing behavior.
- **CLI**: `rad shutdown` only works with `github` workspaces; errors on other kinds.

## Monitoring and Logging

- All lifecycle steps log to stdout with prefixed emoji indicators.
- Component logs are available via `kubectl logs` within the k3d cluster.
- The orphan branch itself serves as an audit trail: each commit records a backup or lock transition with timestamps.

## Development plan

| Work item | Status |
|-----------|--------|
| `gitstate` package (orphan branch, sentinels, worktree) | Complete |
| `pgbackup` package (backup/restore via kubectl exec) | Complete |
| `k3d` package (cluster lifecycle) | Complete |
| `rad init --kind github` | Complete |
| `rad shutdown` command | Complete |
| Deploy lock in `rad deploy` | Complete |
| Helm chart PostgreSQL conditional | Complete |
| GitHub workspace connection type | Complete |
| Functional test workflow | Complete |
| `rad resource type sync` (post-restore reconciliation) | Not started (required for clean state, see Detailed Design section 7) |

## Open Questions

1. **Public repo security**: Should we warn users when backing up state to a public repository? The SQL dumps could contain resource names, connection strings, or other configuration data.

2. **Orphan branch cleanup**: Should there be a `rad state clear` or similar command to delete the orphan branch entirely for a fresh start?

3. **Auto-force sync on restore**: The post-restore sync currently uses force semantics (upstream wins on conflict) because the user cannot intervene during a workflow run. Is this the right default? An alternative would be to fail the workflow on conflict and require the user to update their workflow to pass `--force` explicitly, making the overwrite a conscious opt-in. The tradeoff is reliability (auto-force never blocks deploys) vs. safety (explicit force prevents silent schema changes that could break running applications).

## Alternatives considered

| Alternative | Why rejected |
|-------------|-------------|
| **S3/Azure Blob for state** | Requires cloud credentials and additional infrastructure. Git is already available with `actions/checkout`. |
| **GitHub Actions cache** | Cache eviction is unpredictable (7-day TTL, LRU). State loss would force full re-init frequently. |
| **GitHub Actions artifacts** | Artifacts are scoped to individual workflow runs. Cross-run persistence requires complex artifact download logic and has retention limits. |
| **Persistent k3d cluster** | GitHub Actions runners are ephemeral. No way to keep a cluster alive between runs. |
| **SQLite file backup** | Would require schema-aware backup tooling. PostgreSQL's `pg_dump`/`psql` are battle-tested and handle schema migrations natively. |

## Design Review Notes

<!-- Update this section with the decisions made during the design review meeting. -->
