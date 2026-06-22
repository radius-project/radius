# Debugging Radius with VS Code

## Purpose

This guide is the single authoritative workflow for running and debugging the Radius control plane locally. It runs each Radius component (UCP, Controller, Applications RP, Dynamic RP) as an OS process under the [Delve](https://github.com/go-delve/delve) debugger so you can set breakpoints, inspect variables, and step through code in real time. Automation in [`build/debug.mk`](../../../../build/debug.mk) and [`.vscode/launch.json`](../../../../.vscode/launch.json) builds the components, provisions a disposable [k3d](https://k3d.io) cluster, initializes the database, and attaches the debugger with one command or one click.

This is intended for contributors who are changing control-plane code and want a fast edit → debug → re-run loop. The debug environment uses its own k3d cluster and database, so it never shares state with an installed copy of Radius.

## Prerequisites

Set up the base development environment first by following [Repository Prerequisites](../contributing-code-prerequisites/README.md). On Windows, run everything from a [WSL](https://learn.microsoft.com/windows/wsl/install) shell.

In addition to the base environment, debugging requires these tools. `make debug-start` runs `make debug-check-prereqs` and fails early if any are missing:

- **Go 1.26+** — `go version` (the module targets the version pinned in [`go.mod`](../../../../go.mod))
- **Delve** — `dlv version`
- **k3d** — `k3d version` (creates the disposable debug cluster; requires Docker)
- **kubectl** — `kubectl version --client`
- **Terraform** — `terraform version` (used by recipe execution)
- **Docker** — `docker version` (hosts the k3d cluster and, optionally, PostgreSQL)
- **PostgreSQL** — a reachable server on `localhost:5432` (Docker or a local install)

### Quick install

**macOS (Homebrew):**

```bash
brew install go kubectl k3d terraform postgresql
brew install --cask docker
go install github.com/go-delve/delve/cmd/dlv@latest
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

**Linux:**

```bash
# Go 1.26+
wget https://go.dev/dl/go1.26.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.4.linux-amd64.tar.gz
echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

# kubectl, Docker, and the PostgreSQL client
sudo apt update && sudo apt install -y kubectl postgresql-client docker.io

# Delve and k3d
go install github.com/go-delve/delve/cmd/dlv@latest
wget -q -O - https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Terraform
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install -y terraform
```

### PostgreSQL setup

The prerequisite check needs a PostgreSQL server it can reach before components start.

**Option 1: Docker (recommended)**

```bash
docker run --name radius-postgres \
  -e POSTGRES_PASSWORD=radius_pass \
  -p 5432:5432 -d postgres:15
```

**Option 2: existing local PostgreSQL** — the automation also detects a Homebrew or system PostgreSQL listening on `localhost:5432`.

You do not need to create databases by hand. `make debug-start` is idempotent and creates the `applications_rp` and `ucp` users and databases on first run.

## Steps

### 1. Start the control plane

**Option A: VS Code (recommended)**

1. Open the Run and Debug panel (`Ctrl+Shift+D` / `Cmd+Shift+D`).
2. Select **Start Control Plane** from the dropdown.
3. Press F5.

**Option B: Make**

```bash
make debug-start
```

Either path runs the same automation. It:

- Checks prerequisites and builds all components with debug symbols (`-gcflags="all=-N -l"`).
- Creates a disposable k3d cluster named `radius-debug` and switches your kubectl context to `k3d-radius-debug`.
- Starts UCP, Controller, Applications RP, and Dynamic RP as OS processes, each under `dlv`.
- Initializes the database, creates the `default` resource group and environment, starts the Deployment Engine in the cluster, and registers default recipes.
- Creates a `./drad` wrapper that runs your debug build of `rad` against the local endpoints.

### 2. Set breakpoints

Open the source file you want to debug and click in the gutter to add breakpoints.

### 3. Attach the debugger

Components are already running under `dlv`; you attach VS Code to a running process rather than launching it.

1. Open the Run and Debug panel.
2. Select an attach configuration:
   - **Attach UCP (dlv 40001)**
   - **Attach Controller (dlv 40002)**
   - **Attach Applications RP (dlv 40003)**
   - **Attach Dynamic RP (dlv 40004)**
   - **Attach Radius (all)** — attaches to all four at once
3. Press F5. The VS Code status bar turns orange when attached.

### 4. Exercise the code with `drad`

Use the `./drad` wrapper (not your installed `rad`) so commands target the local debug endpoints. Breakpoints trigger when the corresponding code runs:

```bash
./drad env list
./drad app deploy my-app.bicep
```

Detach the debugger with the disconnect button or `Shift+F5` when you are done. The components keep running, so you can re-attach at any time without restarting them.

### 5. Debug the `rad` CLI itself

To step through CLI code (rather than attach to a server):

1. Run and Debug panel → **Debug rad CLI (prompt for args)**.
2. Press F5.
3. Enter the command arguments when prompted (for example, `env list`).

### 6. Iterate on code changes

```bash
# 1. Stop and tear down (destroys the k3d cluster — see step 7)
make debug-stop

# 2. Edit your code

# 3. Rebuild and restart
make debug-start

# 4. Re-attach the debugger (Run and Debug → attach configuration → F5)
```

For a faster loop while iterating on a single component, rebuild just that component and restart, instead of rebuilding everything:

```bash
make debug-build-dynamic-rp   # or debug-build-ucpd, debug-build-applications-rp, debug-build-controller, debug-build-rad
```

### 7. Stop the control plane

```bash
make debug-stop
# OR VS Code: Run and Debug → "Stop Control Plane" → F5
```

`debug-stop` is destructive: it stops every component, **destroys the `radius-debug` k3d cluster**, removes the log directory, and deletes the `./drad` symlink. Run it when you are finished — not between debugger sessions. To pause debugging while leaving the components up, detach with `Shift+F5` (step 4) instead.

### Endpoints and ports

After `make debug-start`, the services are reachable locally:

| Component         | Local endpoint                                    | dlv attach port |
|-------------------|---------------------------------------------------|-----------------|
| UCP               | `http://localhost:9000/apis/api.ucp.dev/v1alpha3` | 40001           |
| Controller        | `http://localhost:7073/healthz`                   | 40002           |
| Applications RP   | `http://localhost:8080/healthz`                   | 40003           |
| Dynamic RP        | `http://localhost:8082/healthz`                   | 40004           |
| Deployment Engine | `http://localhost:5017`                           | n/a             |

The Deployment Engine runs inside the k3d cluster, so you attach to it through the cluster rather than a local Delve port.

### Useful Make commands

```bash
# Lifecycle
make debug-start          # Build, start the cluster, and start all components
make debug-stop           # Stop components, destroy the cluster, and clean up
make debug-status         # Show component status
make debug-logs           # Tail all component logs
make debug-help           # List all debug commands

# Incremental builds (faster when iterating on one component)
make debug-build-ucpd
make debug-build-applications-rp
make debug-build-controller
make debug-build-dynamic-rp
make debug-build-rad

# Environment and Deployment Engine
make debug-env-init                  # Re-create resource group, environment, and recipes
make debug-deployment-engine-status  # Check the Deployment Engine running in k3d
make debug-deployment-engine-logs    # Tail Deployment Engine logs
```

## Verification

The environment is ready when all of the following succeed:

- `make debug-status` reports every component as running.
- The control-plane health endpoints respond:

  ```bash
  curl -s http://localhost:9000/apis/api.ucp.dev/v1alpha3   # UCP
  curl -s http://localhost:7073/healthz   # Controller
  curl -s http://localhost:8080/healthz   # Applications RP
  curl -s http://localhost:8082/healthz   # Dynamic RP
  ```

- `make debug-deployment-engine-status` reports the Deployment Engine as running and ready.
- `./drad env list` returns the `default` environment.
- After attaching a debugger, the VS Code status bar is orange and breakpoints pause execution when their code runs.

## Troubleshooting

### Components won't start

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

### Port conflicts

```bash
# Service ports
lsof -i :9000   # UCP
lsof -i :8080   # Applications RP
lsof -i :8082   # Dynamic RP
lsof -i :7073   # Controller

# Debug (dlv) ports
lsof -i :40001  # UCP
lsof -i :40002  # Controller
lsof -i :40003  # Applications RP
lsof -i :40004  # Dynamic RP

# Stop the process holding a port
kill -9 <PID>
```

### PostgreSQL connection issues

```bash
# Check PostgreSQL is running
docker ps | grep postgres          # If using Docker
brew services list | grep postgres # If using Homebrew

# Start PostgreSQL
docker start radius-postgres       # Docker
brew services start postgresql     # Homebrew

# Test the connection
psql "postgresql://postgres:radius_pass@localhost:5432/postgres" -c "SELECT 1;"
```

### k3d cluster problems

```bash
# Confirm the debug cluster exists and your context points at it
k3d cluster list | grep radius-debug
kubectl config current-context        # expect k3d-radius-debug

# Recreate from scratch
make debug-stop
make debug-start
```

### Debugger won't attach

"Failed to attach to dlv" usually means the component isn't running or the port is blocked:

1. Verify the component is running: `make debug-status`.
2. Check the debug port is listening: `lsof -i :40001`.
3. Restart: `make debug-stop && make debug-start`.
4. Verify Delve is installed: `dlv version`.

### Breakpoints not hitting

1. Confirm the debugger is attached — the VS Code status bar should be orange.
2. Confirm the code path actually runs (add a log line to check).
3. Rebuild the component so the binary matches your source:

   ```bash
   make debug-stop
   make debug-start
   ```

### `./drad` command not found

```bash
# Verify the symlink exists
ls -la ./drad

# Recreate it (debug-stop removes it; debug-build-rad re-creates it)
make debug-build-rad
```

### Getting help

```bash
make debug-check-prereqs   # Verify required tools and PostgreSQL connectivity
make debug-status          # Show component health
make debug-logs            # Tail all component logs
```
