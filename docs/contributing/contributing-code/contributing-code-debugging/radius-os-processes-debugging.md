# Running Radius as OS Processes for Advanced Debugging

This guide details how to leverage the fully-integrated VS Code debugging experience for Radius development. By running core components as native OS processes instead of in containers, you can take advantage of pre-configured launch configurations and tasks to enable a seamless "inner-loop" workflow. This setup allows for advanced debugging capabilities, such as setting breakpoints, inspecting variables, and stepping through code in real-time—all directly within the VS Code editor—significantly accelerating development and troubleshooting.

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Prerequisites](#prerequisites)
4. [Development Workflow](#development-workflow)
5. [VS Code Debugging](#vs-code-debugging)
6. [Troubleshooting](#troubleshooting)

## Overview

Radius consists of several key components that normally run in Kubernetes containers:
- **Applications Resource Provider (applications-rp)** - Manages Applications.Core resources
- **UCP Daemon (ucpd)** - Universal Control Plane for resource management
- **Controller** - Kubernetes controller for managing Radius resources
- **Dynamic Resource Provider (dynamic-rp)** - Handles dynamic resource types

Running these as OS processes enables:
- Full debugger support with breakpoints and variable inspection
- Real-time configuration changes
- Performance profiling and analysis
- Network traffic inspection

## Quick Start

The simplest way to get debugging working:

### Prerequisites
- Local Kubernetes cluster accessible via kubectl (Docker Desktop, k3d, minikube, etc.)
- Current kubectl context must point to your **local** cluster
- PostgreSQL database running locally

### Setup Commands
```bash
# Start all components as OS processes with debugging (Checks prereqs and creates necessary folders)
make debug-start

# Check that everything is running
make debug-status
```

**VS Code Debugging:**
- Debugger attach configurations are pre-configured in `.vscode/launch.json`
- Set breakpoints in your code, then use F5 to attach to any component
- Debug ports: UCP (40001), Controller (40002), Applications RP (40003), Dynamic RP (40004)

**CLI Debugging Options:**
- **Use `./drad` for convenience**: When you only need to test CLI commands against the debug environment without debugging the CLI code itself
- **Use "Debug drad CLI (debug environment)" in VS Code**: When you need to debug CLI code with breakpoints, variable inspection, and step-through debugging

**For code changes:**
1. Use "Rebuild and Restart [Component]" task
2. Re-attach debugger to new process

The project could take on Air as a depdendency and allow for golang hot-reload, this would be a huge value if someone wants to contribute.

**What the automation provides:**
- Environment directory structure at `debug_files/` (in project root)
- Component configuration files with correct schemas
- Controller configured to skip webhooks in local development (no TLS certs required)
- Database setup verification
- Management scripts (start/stop/status)
- Incremental builds for individual components
- Convenient `./drad` symlink in workspace root for easy CLI access
- Debug CLI wrapper `./drad` with automatic UCP endpoint configuration

## Prerequisites

The automation checks for all required tools. Install any missing prerequisites:

### Required Tools
- **Go 1.21+** - `go version`
- **Delve debugger** - `dlv version` (Go debugger for VS Code integration)
- **kubectl** - Kubernetes cluster access
- **psql** - PostgreSQL client for database verification
- **terraform** - Terraform CLI for recipe execution
- **docker** - To host k3d

#### Quick PostgreSQL Setup (if you don't already have one)

The automation automatically detects and works with different PostgreSQL setups:

**Option 1: Docker PostgreSQL (Recommended for Development)**
If you need a throwaway local PostgreSQL instance for debugging Radius:

```bash
docker run --name radius-postgres \
   -e POSTGRES_PASSWORD=radius_pass \
   -p 5432:5432 \
   -d postgres:15
```

**Option 2: Local PostgreSQL Installation (Homebrew/System)**
If you already have PostgreSQL installed locally (via Homebrew, apt, etc.), the automation will detect and use it automatically.

**Automatic Database Setup**
The automation handles all database setup automatically when you run `make debug-start`:
- Creates required users (`applications_rp`, `ucp`) with proper passwords
- Creates databases with correct ownership
- Sets up proper permissions and table structures
- Works with both Docker and local PostgreSQL installations

The automation will verify connectivity during `make debug-check-prereqs` and create all required users/databases during `make debug-start`. No manual database setup is required.

### Optional Tools
- **VS Code** - For integrated debugging experience

### Installation Commands

**macOS:**
```bash
# Core tools
brew install go kubectl postgresql

# Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest

# Add Go binaries to PATH (required for delve)
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# Terraform (HashiCorp official method)
brew tap hashicorp/tap
brew install hashicorp/tap/terraform

# Optional tools
brew install --cask docker
brew install --cask visual-studio-code
```

**Ubuntu/Debian:**
```bash
# Core tools
sudo apt update
sudo apt install golang-go kubectl postgresql-client

# Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest

# Add Go binaries to PATH (required for delve)
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc

# Terraform (HashiCorp official method)
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install terraform

# Optional tools
sudo apt install docker.io
```

**Verification:**
```bash
# Check all prerequisites
make debug-check-prereqs
```

## Development Workflow

### Automated Workflow (Recommended)

The automation handles all setup, configuration, and management tasks:

```bash
# Complete setup and start development environment
make debug-start

# This single command:
# 1. Sets up directory structure and configuration files
# 2. Builds components with debug symbols
# 3. Starts all components as OS processes
# 4. Initializes a clean dev environment with default recipes

# Stop development environment when done
make debug-stop
```

### Available Make Targets

```bash
# Setup and Configuration
make debug-help           # Show all available debug commands
make debug-check-prereqs  # Verify all prerequisites are installed
make debug-setup          # Complete one-time environment setup

# Development Workflow
make debug-start          # Start all components as OS processes
make debug-stop           # Stop all running components and clean up database
make debug-status         # Show component health status
make debug-build          # Build all components with debug symbols (incremental)

# Individual Component Builds (incremental - only changed code is recompiled)
make debug-build-ucpd             # Build only UCP daemon
make debug-build-applications-rp  # Build only Applications RP
make debug-build-controller       # Build only Controller
make debug-build-dynamic-rp       # Build only Dynamic RP
make debug-build-rad             # Build only rad CLI

# Monitoring and Troubleshooting
make debug-logs                   # Tail all component logs

# Complete Development Setup
make debug-dev-start             # Setup + VS Code config + start components
make debug-dev-stop              # Stop all components
```

### What the Automation Creates

When you run `make debug-setup`, the following structure is created in `debug_files/`:

```
debug_files/
├── bin/                    # Built Radius binaries with debug symbols
│   ├── rad                 # rad CLI binary
│   └── rad-wrapper         # Debug wrapper that auto-configures UCP
├── configs/                # Component configuration files
│   ├── ucp.yaml
│   ├── applications-rp.yaml
│   ├── controller.yaml
│   ├── dynamic-rp.yaml
│   ├── rad-debug-config.yaml  # CLI config with UCP override (used by wrapper)
│   └── terraformrc
├── logs/                   # Component logs
├── scripts/                # Management scripts
│   ├── start-radius.sh
│   ├── stop-radius.sh
│   ├── status-radius.sh
│   ├── start-deployment-engine.sh
│   └── stop-deployment-engine.sh
├── terraform-cache/        # Terraform provider cache
└── env-setup.sh           # Environment variables
```

And VS Code configuration files are already included in the repository:

```
.vscode/
├── launch.json            # Debug configurations
├── tasks.json             # VS Code tasks
├── settings.json          # Workspace settings
└── extensions.json        # Recommended extensions
```

## VS Code Debugging

> 💡 **Quick Setup**: VS Code configuration files are included in the repository and ready to use.

### Available Debug Configurations

The following attach-based debug configurations are available in `.vscode/launch.json`:

- **"Attach to UCP"** - Attach debugger to running UCP process
- **"Attach to Applications RP"** - Attach debugger to running Applications RP process  
- **"Attach to Controller"** - Attach debugger to running Controller process
- **"Attach to Dynamic RP"** - Attach debugger to running Dynamic RP process

**CLI Debug Configurations:**
- **"Debug rad CLI"** - Basic CLI debugging with hardcoded 'version' command
- **"Debug rad CLI (prompt for args)"** - CLI debugging with argument prompts (uses default rad config)
- **"Debug drad CLI (debug environment)"** - CLI debugging with debug environment configuration (equivalent to `./drad` but debuggable)

All server configurations use "attach" mode - they connect to already running processes started via `make debug-start`.

**When to use each CLI option:**
- **`./drad` command**: Use for quick CLI testing when you don't need to debug the CLI code itself. Perfect for testing server-side functionality while working on UCP, RP, or Controller code.
- **"Debug drad CLI (debug environment)"**: Use when you need to debug CLI code with breakpoints and variable inspection while connected to your debug environment.

### Debugging Workflow in VS Code

This workflow separates process management (via make) from debugging (via VS Code), making it much cleaner and more reliable.

#### Debugging Workflow

1. **Set Breakpoints**: Add breakpoints in your code in VS Code

2. **Attach Debugger** (Choose one method):
   
   **Method A: VS Code Process Picker**
   - Open Debug panel and select "Attach to [Component]"
   - Press F5 - VS Code will show a process picker
   - Select the component process (e.g., "ucpd")

   **Method B: CLI Debugging**
   - For CLI testing without debugging: Use `./drad <command>` 
   - For CLI debugging with breakpoints: Select "Debug drad CLI (debug environment)" and press F5
   - Enter your CLI command when prompted (e.g., `env list`, `app deploy app.bicep`)

   **Method C: Attach to All Components**
   - Use compound configuration "Attach to All Components" for multi-component debugging

3. **Code Changes**: 
   - Make your code changes
   - Use rebuild/restart tasks: Ctrl+Shift+P → "Tasks: Run Task" → "Rebuild and Restart [Component]"
   - Re-run "Update Launch.json PIDs" task if using Method A
   - Re-attach debugger to the new process

#### Advantages of This Approach

- **Clean Separation**: Make handles processes, VS Code handles debugging
- **Reliable**: No complex launch configurations that can break
- **Flexible**: Debug any combination of components
- **Fast Iteration**: Rebuild/restart only the component you're working on
- **Preserved State**: Other components keep running while you restart one

### Debugging Specific Issues

**Authentication Problems:**
- Set breakpoints in authentication-related code
- Monitor environment variables in the Debug Console

**Database Issues:**
- UCP database connections are pre-configured
- PostgreSQL setup is verified during `make debug-setup`

**Kubernetes Integration:**
- Controller uses your current kubectl context
- RBAC permissions are checked during setup

## Troubleshooting

### Common Issues and Solutions

**1. Component Startup Failures**

```bash
# Check component status
make debug-status

# View logs for specific components
cat debug_files/logs/ucp.log
cat debug_files/logs/applications-rp.log
cat debug_files/logs/controller.log
cat debug_files/logs/dynamic-rp.log

# Restart specific components in VS Code debugger
# Or rebuild and restart all components
make debug-stop
make debug-start
```

**2. Missing Prerequisites**

```bash
# Check what's missing
make debug-check-prereqs

# Install missing tools (example for macOS)
brew install go kubectl postgresql terraform docker

# Verify installation
make debug-check-prereqs
```

**3. Database Connection Issues**

The automation handles database setup automatically and works with your current user. If you see database errors:

```bash
# Check if PostgreSQL is running
# macOS:
brew services start postgresql

# Linux:
sudo systemctl start postgresql

# Docker:
docker start radius-postgres  # If using Docker PostgreSQL

# The automation will automatically:
# - Detect your PostgreSQL installation (Docker vs local)
# - Create required users and databases
# - Set up proper permissions and table structures

# If you need to test connections manually:
# For Docker PostgreSQL:
psql "postgresql://postgres:radius_pass@localhost:5432/postgres" -c "SELECT 1;"

# For local PostgreSQL (Homebrew/system):
psql postgres -c "SELECT 1;"

# Test the actual databases created by automation:
psql "postgresql://applications_rp:radius_pass@localhost:5432/applications_rp" -c "SELECT 1;"
psql "postgresql://ucp:radius_pass@localhost:5432/ucp" -c "SELECT 1;"
```

**4. Port Conflicts**

```bash
# Check for port conflicts
lsof -i :9000  # UCP
lsof -i :8080  # Applications RP
lsof -i :8082  # Dynamic RP
lsof -i :7073  # Controller health
lsof -i :5017  # Deployment Engine
```

**5. Kubernetes Permission Issues**

```bash
# Test current permissions
kubectl auth can-i "*" "*" --all-namespaces

# Verify kubectl context
kubectl config current-context

# Check if radius-system namespace exists
kubectl get namespace radius-system || kubectl create namespace radius-system

# Check if radius-testing namespace exists  
kubectl get namespace radius-testing || kubectl create namespace radius-testing
```

### Getting Help

If you encounter issues not covered here:

1. **Check the rad CLI configuration**: The `./drad` wrapper is automatically configured for local debugging
2. **Check component logs**: Use `make debug-logs` to see all component output
3. **Verify prerequisites**: Run `make debug-check-prereqs` 
4. **Clean and restart**: Use `make debug-stop && make debug-start`
5. **Use VS Code debugging**: Set breakpoints and step through problematic code paths

The automation handles ~90% of the setup complexity, but understanding the underlying components helps with advanced debugging scenarios.

## Summary

The Radius debug automation provides:

✅ **Fully Automated:**
- Directory structure creation
- Configuration file generation with correct schemas
- VS Code integration (launch.json, tasks.json, settings.json)
- Build process with debug symbols
- Component management (start/stop/status)
- Controller webhook configuration (automatic TLS certificate handling)
- Environment variables and path setup
- Deployment engine configuration
- Log aggregation and monitoring
- Health checking and verification
- Incremental builds for individual components
- Database setup

🔶 **Partially Automated:**
- Kubernetes prerequisites (namespace creation, permission verification)
- Prerequisites validation (automated checking with installation guidance)

❌ **Manual Steps Required:**
- Tool installation (Go, Docker, kubectl, PostgreSQL, Terraform) - one-time setup
- Cloud credentials configuration (Azure/AWS) - as needed for your development

The automation eliminates the complexity of manual configuration while preserving the flexibility needed for advanced debugging scenarios.
