# Debugging Radius with VS Code

Run Radius components as OS processes with full debugger support - set breakpoints, inspect variables, and step through code in real-time.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Prerequisites](#prerequisites)
3. [Debugging Workflow](#debugging-workflow)
4. [Using a Local Deployment Engine](#using-a-local-deployment-engine)
5. [Troubleshooting](#troubleshooting)

## Overview

**What you get:**

- Radius components (UCP, Applications RP, Controller, Dynamic RP) running as debuggable OS processes
- Pre-configured VS Code launch configurations for one-click debugging
- Full breakpoint support, variable inspection, and step-through debugging
- Fast iteration cycle - modify code, restart, and re-attach debugger

## Quick Start

### 1. Start Components

**Option A: Using VS Code (Recommended)**

1. Open Run and Debug panel (`Cmd+Shift+D` or `Ctrl+Shift+D`)
2. Select "Start Control Plane" from dropdown
3. Press F5

**Option B: Using Make**

```bash
make debug-start
```

Both methods:

- Check prerequisites automatically
- Build all components
- Start UCP, Controller, Applications RP, and Dynamic RP
- Initialize default environment and recipes

### 2. Attach Debugger

1. Set breakpoints in your code
2. Open Run and Debug panel (`Cmd+Shift+D`)
3. Select a debug configuration:
   - "Attach UCP (dlv 40001)" - Debug UCP
   - "Attach Controller (dlv 40002)" - Debug Controller
   - "Attach Applications RP (dlv 40003)" - Debug Applications RP
   - "Attach Dynamic RP (dlv 40004)" - Debug Dynamic RP
   - "Attach Radius (all)" - Debug all components at once
4. Press F5

**Your debugger is now attached** - breakpoints will trigger when code is executed.

### 3. Test with rad CLI

**Quick testing (no CLI debugging):**

```bash
./drad env list
./drad app deploy my-app.bicep
```

**Debug CLI code:**

1. Run and Debug panel → "Debug rad CLI (prompt for args)"
2. Press F5
3. Enter command arguments when prompted (e.g., `env list`)

### 4. Make Code Changes

1. Stop components: Run and Debug → "Stop Control Plane" → F5
2. Edit your code
3. Restart: Run and Debug → "Start Control Plane" → F5 (rebuilds automatically)
4. Re-attach debugger: Select attach configuration → F5

## Prerequisites

### Required Tools

- **Go 1.25+** - `go version`
- **Delve debugger** - `dlv version`
- **kubectl** - Kubernetes CLI
- **PostgreSQL** - Database (Docker or local install)
- **terraform** - For recipe execution
- **docker** - To host k3d cluster

### Quick Install

**macOS:**

```bash
brew install go kubectl postgresql terraform docker
go install github.com/go-delve/delve/cmd/dlv@latest
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

**Linux:**

```bash
# Install Go 1.25+
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

# Other tools
sudo apt update && sudo apt install kubectl postgresql-client docker.io
go install github.com/go-delve/delve/cmd/dlv@latest

# Terraform
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install terraform
```

### PostgreSQL Setup

**Option 1: Docker (Recommended)**

```bash
docker run --name radius-postgres \
  -e POSTGRES_PASSWORD=radius_pass \
  -p 5432:5432 -d postgres:15
```

**Option 2: Use existing local PostgreSQL**
The automation detects and works with Homebrew or system PostgreSQL installations automatically.

**Database setup is automatic** - `make debug-start` creates required users, databases, and tables.

## Debugging Workflow

### Understanding the Attach Model

Components run as independent processes started by `make debug-start` or the "Start Control Plane" launch configuration. Each component runs under Delve (dlv) listening on a specific port:

- **UCP**: Port 40001
- **Controller**: Port 40002
- **Applications RP**: Port 40003
- **Dynamic RP**: Port 40004

**To debug, you attach VS Code to the already-running process.** You don't launch the process from VS Code.

### Step-by-Step Debugging

**1. Start Components (one time)**

```bash
make debug-start
# OR use VS Code: Run and Debug → "Start Control Plane" → F5
```

**2. Set Breakpoints**

- Open the source file you want to debug
- Click in the gutter to add breakpoints

**3. Attach Debugger**

- Run and Debug panel (`Cmd+Shift+D`)
- Select component to debug (e.g., "Attach UCP (dlv 40001)")
- Press F5
- Status bar turns orange when attached

**4. Trigger Your Code**

- Use `./drad` commands or deploy Bicep files
- Breakpoints will pause execution
- Inspect variables, step through code, etc.

**5. Detach When Done**

- Click the disconnect button or press `Shift+F5`
- Components keep running - you can re-attach anytime

### Code Change Workflow

```bash
# 1. Stop components
make debug-stop
# OR VS Code: "Stop Control Plane" → F5

# 2. Edit code

# 3. Restart (rebuilds changed components automatically)
make debug-start
# OR VS Code: "Start Control Plane" → F5

# 4. Re-attach debugger
# Run and Debug → Select attach configuration → F5
```

### Available VS Code Configurations

**Process Management:**

- **Start Control Plane** - Start all components
- **Stop Control Plane** - Stop all components
- **Control Plane Status** - Check health

**Attach Debuggers:**

- **Attach UCP (dlv 40001)** - Attach to UCP
- **Attach Controller (dlv 40002)** - Attach to Controller
- **Attach Applications RP (dlv 40003)** - Attach to Applications RP
- **Attach Dynamic RP (dlv 40004)** - Attach to Dynamic RP
- **Attach Radius (all)** - Attach to all components simultaneously

**CLI Debugging:**

- **Debug rad CLI (prompt for args)** - Debug the rad CLI itself (launches a new process)

### Useful Make Commands

```bash
# Start/Stop
make debug-start          # Start all components
make debug-stop           # Stop all components
make debug-status         # Show component status

# Build Individual Components (incremental - faster for iterating on one component)
make debug-build-ucpd             # Build only UCP
make debug-build-applications-rp  # Build only Applications RP
make debug-build-controller       # Build only Controller
make debug-build-dynamic-rp       # Build only Dynamic RP
make debug-build-rad              # Build only rad CLI

# Logs
make debug-logs           # Tail all component logs

# Help
make debug-help           # Show all debug commands
```

## Using a Local Deployment Engine

By default `make debug-start` runs the Deployment Engine (DE) inside the k3d
cluster from the published `ghcr.io/radius-project/deployment-engine:latest`
image. If you are working on DE itself you can run it as a local OS process
instead and have the debug stack pick it up automatically.

### Auto-detection

`make debug-start` checks whether something is already listening on TCP
**port 5017** (`lsof -nP -iTCP:5017 -sTCP:LISTEN`). If it is, the in-cluster DE
deployment is skipped and a marker file is written to
`debug_files/logs/de-external.marker`. `make debug-stop` honours the marker and
leaves your local DE process alone.

There is nothing else to configure on the Radius side — UCP/Applications RP
will reach DE at `http://localhost:5017` and DE will reach UCP at
`http://localhost:9000/apis/api.ucp.dev/v1alpha3`.

### Running DE locally

From your `deployment-engine` checkout, in a shell where you want DE attached
to a debugger or just running with hot-reload:

```bash
# UCP endpoint exposed by `make debug-start`
export RADIUSBACKENDURL=http://localhost:9000/apis/api.ucp.dev/v1alpha3
export ASPNETCORE_URLS=http://+:5017

# Provider toggles that the in-cluster DE config sets by default
export AZURE_ENABLED=true
export AWS_ENABLED=false
export KUBERNETES_ENABLED=true

# IMPORTANT: do NOT set ARM_AUTH_METHOD when you want DE to use ambient
# Azure credentials (az CLI / DefaultAzureCredential). Setting it to
# UCPCredential forces DE to fetch credentials from UCP, which is the
# correct value when DE runs in-cluster but defeats the local path.
unset ARM_AUTH_METHOD SKIP_ARM

dotnet run --project src/DeploymentEngine
```

Then start (or restart) the rest of the stack:

```bash
make debug-start
# Output will include:
#   ℹ Detected Deployment Engine listening on localhost:5017 — using external instance
```

### Switching back to the in-cluster DE

Stop your local DE process so port 5017 is free, then:

```bash
rm -f debug_files/logs/de-external.marker
make debug-stop
make debug-start
```

### Running Azure functional tests against the local stack

When DE is running locally with ambient `az login` credentials, you can run
the Azure subset of the cloud functional tests without registering any Azure
service principal in UCP. The test helper
`AssertCredentialExists` honours the `RADIUS_TEST_USE_LOCAL_CLOUD_CREDS`
environment variable as a local-dev escape hatch (set to `azure`, `aws`,
`azure,aws`, or `1` for all clouds). The container-DE / CI path is unchanged
and still requires `rad credential register azure …`.

A make target orchestrates an ephemeral Azure resource group, deploys the
test fixtures (Cosmos Mongo for `Test_AzureConnections`), runs the entire
`corerp/cloud/...` suite (AWS-required tests skip automatically because
`RADIUS_TEST_USE_LOCAL_CLOUD_CREDS=azure` only covers Azure), and tears
everything down even if the tests fail:

```bash
az login
az account set --subscription <your-sub-id>

make debug-start                       # OS-process Radius, picks up local DE
make test-functional-azure-local       # setup → run → teardown
```

Sub-targets if you want manual control:

```bash
make test-functional-azure-local-setup     # create RG, deploy fixtures
make test-functional-azure-local-run       # run corerp/cloud tests against the stack
make test-functional-azure-local-teardown  # delete RG, clear state
```

For post-mortem debugging of failing tests, use the `-keep` variant. It runs
setup → run → teardown as normal, but **skips the teardown step if any test
fails** so you can inspect the RG and the running stack:

```bash
make test-functional-azure-local-keep
# On failure, RG is preserved. When done:
make test-functional-azure-local-teardown
```

The setup step creates a resource group named
`radlocal-${USER}-$(date +%s)` (tagged `creator`/`creationTime`/`purpose=radius-local-test`)
and writes state to `debug_files/logs/azure-local.env`. To reuse a long-lived
resource group instead of paying the ~3-5 minute Cosmos provisioning cost on
every run:

```bash
AZURE_LOCAL_PREPROVISIONED_RG=<your-rg> make test-functional-azure-local-setup
```

The teardown step refuses to delete a pre-provisioned RG.

#### Re-running individual tests

`run` accepts arbitrary `go test` flags after the sub-command, so you can
quickly re-run one failing test without re-doing setup or teardown:

```bash
./build/scripts/azure-local-testenv.sh run -run '^Test_TerraformRecipe_AzureResourceGroup$' -v
```

If `debug_files/logs/azure-local.env` was wiped (e.g. by `make debug-stop`),
`run` auto-recovers state by listing `radlocal-${USER}-*` resource groups in
the current subscription and picking the newest. It re-applies the Azure
scope on the `default` rad environment too — `make debug-start` resets the
embedded Postgres DB which clears the env's provider config.

#### Cleaning up orphaned resource groups

If a previous run left RGs behind (cancelled tests, lost state file, multiple
attempts), garbage-collect everything you own with one command:

```bash
./build/scripts/azure-local-testenv.sh teardown --all-orphans
```

This deletes every `radlocal-${USER}-*` RG in the current subscription
(`--no-wait`), stops the `tf-module-server` port-forward, and removes the
state file. Pre-provisioned RGs (`AZURE_LOCAL_PREPROVISIONED_RG`) are not
touched.

#### Terraform module server bootstrap

`Test_TerraformRecipe_AzureResourceGroup` consumes a recipe served from
`http://localhost:8999`. Both `setup` and `run` call `ensure_tf_module_server`
which:

1. Probes `http://localhost:8999/azure-rg.zip` and short-circuits if reachable.
2. Otherwise runs `make publish-test-terraform-recipes` (deploys the nginx
   `tf-module-server` Deployment + Service into the
   `radius-test-tf-module-server` namespace).
3. Starts a `kubectl port-forward svc/tf-module-server 8999:80` in the
   background (PID stored under `debug_files/logs/tf-module-server-pf.pid`).
4. Waits for `/azure-rg.zip` to return 200.

Teardown (and `--all-orphans`) stop the port-forward.

#### Terraform recipes and Azure CLI credentials

The Azure terraform provider configuration in
[`pkg/recipes/terraform/config/providers/azure.go`](../../../../pkg/recipes/terraform/config/providers/azure.go)
falls back to **`use_cli = true`** when no credential is registered with UCP
(404 from `/planes/azure/azurecloud/providers/System.Azure/credentials/default`).
This makes terraform pick up the same `az login` session the host RP process
already uses. No `rad credential register azure …` is required for local dev.

In CI a workload-identity credential is registered as before; that path is
unchanged.

> **Note:** AWS local-credentials and Azure MSSQL fixtures are intentionally
> out of scope for this flow. Tests that require `AZURE_MSSQL_*` env vars
> auto-skip when those vars are absent.

> **Known limitation — Flux tests:** `Test_Flux_Basic` and `Test_Flux_Complex`
> in `test/functional-portable/kubernetes/noncloud` fail under this
> OS-process flow. The Radius `FluxController` runs on the host and fetches
> artifacts from the in-cluster Flux source-controller using the cluster-DNS
> URL `http://source-controller.flux-system.svc.cluster.local./...`, which
> the host cannot resolve. These tests require the controller to run inside
> the cluster (the standard `rad install kubernetes` path or CI).

## Troubleshooting

### Components Won't Start

```bash
# Check status
make debug-status

# View logs
cat debug_files/logs/ucp.log
cat debug_files/logs/applications-rp.log
cat debug_files/logs/controller.log
cat debug_files/logs/dynamic-rp.log

# Clean restart
make debug-stop
make debug-start
```

### Port Conflicts

```bash
# Check what's using ports
lsof -i :9000   # UCP
lsof -i :8080   # Applications RP
lsof -i :8082   # Dynamic RP
lsof -i :7073   # Controller
lsof -i :40001  # UCP debug port
lsof -i :40002  # Controller debug port
lsof -i :40003  # Applications RP debug port
lsof -i :40004  # Dynamic RP debug port

# Kill process using a port
kill -9 <PID>
```

### PostgreSQL Connection Issues

```bash
# Check PostgreSQL is running
docker ps | grep postgres          # If using Docker
brew services list | grep postgres # If using Homebrew

# Start PostgreSQL
docker start radius-postgres       # Docker
brew services start postgresql     # Homebrew

# Test connection
psql "postgresql://postgres:radius_pass@localhost:5432/postgres" -c "SELECT 1;"
```

### Debugger Won't Attach

**Issue:** "Failed to attach to dlv"

**Solutions:**

1. Verify component is running: `make debug-status`
2. Check debug port is listening: `lsof -i :40001`
3. Restart component: `make debug-stop && make debug-start`
4. Verify dlv is installed: `dlv version`

### Breakpoints Not Hitting

1. **Verify debugger is attached** - VS Code status bar should be orange
2. **Check the code path is executed** - Try logging to confirm
3. **Rebuild the component** - Old binaries won't match source

   ```bash
   make debug-stop
   make debug-start  # Rebuilds automatically
   ```

### `./drad` Command Not Found

```bash
# Verify symlink exists
ls -la ./drad

# Recreate if missing
make debug-build-rad
```

### Getting Help

**Check component logs:**

```bash
make debug-logs
```

**Verify setup:**

```bash
make debug-check-prereqs
make debug-status
```

**Clean restart:**

```bash
make debug-stop
make debug-start
```
