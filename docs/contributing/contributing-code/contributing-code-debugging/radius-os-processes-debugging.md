# Debugging Radius with VS Code

Run Radius components as OS processes with full debugger support - set breakpoints, inspect variables, and step through code in real-time.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Prerequisites](#prerequisites)
3. [Debugging Workflow](#debugging-workflow)
4. [Troubleshooting](#troubleshooting)

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
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
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
