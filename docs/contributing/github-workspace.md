# GitHub Workspace Type

This document describes the `github` workspace kind for Radius, designed for use inside
GitHub Actions workflows. It automates the lifecycle of a local Kubernetes cluster (via
[k3d](https://k3d.io)) with Radius installed and a PostgreSQL backend, with state persisting
across workflow runs via a git orphan branch.

## Overview

A workspace of kind `github`:

1. On `rad init --kind github`:
   - Opens (or creates) a `radius-state` git orphan branch in a **temporary worktree** — state
     files never appear in the application working tree.
   - Reads the semaphore state to determine whether previous state can be trusted.
   - Writes a `.lock` sentinel and pushes it so a spot-instance eviction is detectable.
   - Creates a k3d Kubernetes cluster, installs Radius with PostgreSQL.
   - If the previous run completed cleanly, restores the PostgreSQL state from the worktree.
2. On `rad shutdown`:
   - Dumps each PostgreSQL database to a SQL file in the state worktree directory.
   - Writes `.backup-ok`, removes `.lock`, stages all files, commits, and pushes to origin in
     one atomic `ClearLock` call.
   - Optionally deletes the k3d cluster (`--cleanup`).

## Prerequisites

- [k3d](https://k3d.io) v5+ installed and on `$PATH`.
- `kubectl` installed and on `$PATH`.
- A git repository (required for state management via orphan branch).

## Quick Start

### Initialize

```bash
rad init --kind github
```

This will:

1. Ensure k3d is available.
2. Open (or create) the `radius-state` orphan branch in an isolated temp worktree.
3. Check the semaphore state (first run / clean / interrupted — see [Spot Instance Safety](#spot-instance-safety)).
4. Write `.lock` and push to origin to mark the deploy as in-progress.
5. Create a k3d cluster named `radius-github`.
6. Install Radius with PostgreSQL using Helm (`database.enabled=true`).
7. Wait for PostgreSQL to be ready.
8. If the previous run was clean, restore the database state from the worktree.

`rad init` is idempotent — running it on an already-initialised cluster is safe. `CreateOrUpdateEnvironment`
is a PUT that converges, so re-running will not create duplicate environments.

### Shut down

```bash
rad shutdown          # back up state and push
rad shutdown --cleanup  # also delete the k3d cluster
```

This will:

1. Open the `radius-state` worktree in a temp directory.
2. Dump each PostgreSQL database to a SQL file in the worktree directory.
3. Call `ClearLock`: writes `.backup-ok`, removes `.lock`, stages all files (SQL + sentinels),
   commits, and pushes to `origin radius-state`.

The worktree is a temporary directory — unrelated to `$GITHUB_WORKSPACE` — so SQL backup files
are never staged or shown in `git status` on your application branch.

## State Persistence Across Runs

State lives entirely on the `radius-state` orphan branch in your repository. There is no
separate artifact upload step. `rad init` opens the branch via `git worktree` and `rad shutdown`
pushes to it directly.

The required workflow permissions are:

```yaml
permissions:
  contents: write  # needed for git push to radius-state
```

A minimal cross-run workflow:

```yaml
jobs:
  demo:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0           # needed so the orphan branch history is available

      - name: Install k3d
        run: curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

      - name: Initialize Radius
        run: rad init --kind github

      - name: Deploy / test application
        run: # ...

      - name: Shut down Radius
        run: rad shutdown --cleanup
        if: always()
```

`rad init` reads the `radius-state` branch (fetched by `fetch-depth: 0`). `rad shutdown`
pushes the updated state back. No explicit `git push` step is required in the workflow.

## Spot Instance Safety

GitHub-hosted runners may be evicted mid-deploy, especially when using larger runner types or
spot-backed infrastructure. The semaphore system provides best-effort detection of interrupted
runs. Note that failure scenarios during spot-instance eviction have not been fully evaluated
in production; the semaphore provides detection, not automatic recovery.

### Sentinel files

Two sentinel files live on the `radius-state` branch inside the worktree:

| File | Written by | Means |
|------|-----------|-------|
| `.lock` | `rad init` (before cluster creation) | A deploy is in progress |
| `.backup-ok` | `rad shutdown` (after successful backup) | Previous run completed cleanly |

### `rad init` semaphore logic

| State detected | Condition | Action |
|---------------|-----------|--------|
| `SemaphoreFirstRun` | No `.lock`, no `.backup-ok` | First run — no restore |
| `SemaphoreClean` | `.backup-ok` present, no `.lock` | Restore PostgreSQL state |
| `SemaphoreInterrupted` | `.lock` present | Last runner was evicted mid-deploy — **skip restore**, log warning |

When `SemaphoreInterrupted` is detected, `rad init` logs a warning and continues with a
fresh Radius installation. The potentially-incomplete state from the interrupted run is
left on the branch for manual inspection; it is **not** applied automatically.

### Lock lifecycle

```
rad init start:
  CheckSemaphore() → SemaphoreFirstRun / SemaphoreClean / SemaphoreInterrupted
  WriteLock()      → write .lock, remove .backup-ok, commit, push
  ... cluster create, helm install, postgres restore ...

rad shutdown end:
  pgbackup.Backup() → write *.sql to worktree
  ClearLock()       → write .backup-ok, remove .lock, git add -A, commit, push
```

## Resource Type Sync

After a successful PostgreSQL restore, Radius should sync resource types from
`resource-types-contrib` so the restored type definitions stay current:

> **TODO:** `rad resource type sync --source oci://ghcr.io/radius-project/resource-types-contrib:latest`  
> This command does not exist yet. When implemented, it must run **after** the restore so the
> restored types form the baseline. If a sync detects a conflict (an attribute set altered
> upstream while your saved state has a local version), it must return an error and the caller
> should log a warning and proceed with saved state only — never silently overwrite. The user
> must resolve the conflict manually before re-running.

## Architecture

```
rad init --kind github
    │
    ├── k3d.EnsureInstalled()
    ├── gitstate.OpenOrCreate()       → git worktree add /tmp/radius-state-NNN radius-state
    ├── worktree.CheckSemaphore()     → read .lock / .backup-ok from worktree
    ├── worktree.WriteLock()          → write .lock, commit, push
    ├── k3d.CreateCluster()           → k3d cluster create radius-github --wait
    ├── helm.InstallRadius()          → Helm install with database.enabled=true
    ├── pgbackup.WaitForReady()       → kubectl wait pod -l app=database
    │
    └── if SemaphoreClean && HasBackup:
        ├── pgbackup.Restore()        → kubectl exec psql < worktree/*.sql
        └── TODO: rad resource type sync

rad shutdown
    │
    ├── gitstate.OpenOrCreate()       → git worktree add /tmp/radius-state-NNN radius-state
    ├── pgbackup.Backup()             → kubectl exec pg_dump > worktree/*.sql
    ├── worktree.ClearLock()          → write .backup-ok, remove .lock, git add -A, commit, push
    └── k3d.DeleteCluster()           → k3d cluster delete radius-github  (only with --cleanup)
```

## Integration Test Workflow

A dedicated integration test workflow is provided at
[.github/workflows/functional-test-github-workspace.yaml](.github/workflows/functional-test-github-workspace.yaml).

It exercises the full lifecycle on a GitHub Actions runner within a single job:

1. **Phase 1 — Init (first run):** `rad init --kind github` creates the cluster, pushes `.lock`.
2. **Phase 2 — Deploy demo content:** Placeholder (`echo "No demo application yet"`).
3. **Phase 3 — Shutdown:** `rad shutdown --cleanup` backs up state, pushes `.backup-ok`, deletes
   the cluster.
4. **Phase 4 — Restore:** `rad init --kind github` again — detects `SemaphoreClean` and restores.
   `make workflow-github-workspace-verify-restore` asserts SQL files exist on the branch.
5. **Phase 5 — Cleanup.**

### Make Targets

| Target | Purpose |
|--------|---------|
| `make workflow-github-workspace-init` | `rad init --kind github` |
| `make workflow-github-workspace-shutdown` | `rad shutdown --cleanup` |
| `make workflow-github-workspace-verify-restore` | Assert SQL backup files exist on `radius-state` |

## Package Reference

| Package | Path | Purpose |
|---------|------|---------|
| `k3d` | `pkg/cli/k3d` | k3d cluster lifecycle |
| `pgbackup` | `pkg/cli/pgbackup` | PostgreSQL backup/restore via `kubectl exec` |
| `gitstate` | `pkg/cli/gitstate` | Orphan branch state via `git worktree` + semaphores |
