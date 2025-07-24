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
# One-time setup (uses your current kubectl context)
make debug-setup

# Start all components as OS processes with debugging
make debug-start

# First time only: initialize environment resources
make debug-env-init

# Check that everything is running
make debug-status

# Use the debug CLI (doesn't conflict with installed rad)
source debug_files/env-setup.sh
drad version  # Should show "Connected" status
```

**VS Code Debugging:**
- Debugger attach configurations are pre-configured in `.vscode/launch.json`
- Set breakpoints in your code, then use F5 to attach to any component
- Debug ports: UCP (40001), Controller (40002), Applications RP (40003), Dynamic RP (40004)
5. Debug with full breakpoint support!

**For code changes:**
1. Use "Rebuild and Restart [Component]" task
2. Re-attach debugger to new process

**What the automation provides:**
- Environment directory structure at `debug_files/` (in project root)
- Component configuration files with correct schemas
- Controller configured to skip webhooks in local development (no TLS certs required)
- Database setup verification
- Management scripts (start/stop/status)
- Incremental builds for individual components
- Convenient `./rad` symlink in workspace root for easy CLI access
- Debug CLI wrapper `./rad` with automatic UCP endpoint configuration

## Prerequisites

The automation checks for all required tools. Install any missing prerequisites:

### Required Tools
- **Go 1.21+** - `go version`
- **Delve debugger** - `dlv version` (Go debugger for VS Code integration)
- **kubectl** - Kubernetes cluster access
- **psql** - PostgreSQL client for database verification
- **terraform** - Terraform CLI for recipe execution

### Optional Tools
- **docker** - For deployment engine (recommended)
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
make debug-dev-start

# This single command:
# 1. Sets up directory structure and configuration files
# 2. Builds components with debug symbols
# 3. Starts all components as OS processes
# 4. Provides next steps for creating resources

# Create Radius resources (after components are running)
# Use ./rad for automatic UCP connection and workspace setup
./rad group create default
./rad env create default

# Or use regular rad CLI (requires workspace override configuration)
./rad group create default
./rad env create default

# Stop development environment when done
make debug-dev-stop
```

### Daily Development

```bash
# Start components (uses existing configuration)
make debug-start

# Check component health
make debug-status

# Monitor logs
make debug-logs

# Build only changed components (incremental builds)
make debug-build-ucpd              # Build only UCP daemon
make debug-build-applications-rp   # Build only Applications RP
make debug-build-controller        # Build only Controller
make debug-build-dynamic-rp        # Build only Dynamic RP
make debug-build-rad              # Build only rad CLI

# Stop components
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

All configurations use the "attach" mode - they connect to already running processes started via `make debug-start`.

### Debugging Workflow in VS Code

This workflow separates process management (via make) from debugging (via VS Code), making it much cleaner and more reliable.

#### Initial Setup

1. **Complete Setup and Start** (first time):
   ```bash
   make debug-setup              # Setup environment
   make debug-start              # Start all components as processes
   make debug-env-init           # Initialize database (first time only)
   ```

2. **Daily Development Start**:
   ```bash
   make debug-start              # Start all components
   # Components run as regular OS processes, ready for debugger attachment
   ```

#### Debugging Workflow

1. **Set Breakpoints**: Add breakpoints in your code in VS Code

2. **Attach Debugger** (Choose one method):

   **Method A: Automatic PID Resolution (Recommended)**
   - Run VS Code task: "Update Launch.json PIDs" (Ctrl+Shift+P → "Tasks: Run Task")
   - Open Debug panel (Ctrl+Shift+D / Cmd+Shift+D)
   - Select "Quick Attach to [Component]" (e.g., "Quick Attach to UCP (Update PID)")
   - Press F5 - debugger attaches immediately with current PID
   
   **Method B: Manual PID Entry**
   - Run VS Code task: "Show PIDs for Debugging" to see current process IDs
   - Open Debug panel and select "Attach to [Component]"
   - Press F5 and enter the PID when prompted
   
   **Method C: VS Code Process Picker**
   - Open Debug panel and select "Attach to [Component]"
   - Press F5 - VS Code will show a process picker
   - Select the component process (e.g., "ucpd")

   **Method D: Attach to All Components**
   - Use compound configuration "Attach to All Components" for multi-component debugging

3. **Code Changes**: 
   - Make your code changes
   - Use rebuild/restart tasks: Ctrl+Shift+P → "Tasks: Run Task" → "Rebuild and Restart [Component]"
   - Re-run "Update Launch.json PIDs" task if using Method A
   - Re-attach debugger to the new process

4. **Individual Component Development**:
   - **UCP**: "Rebuild and Restart UCP" → "Update Launch.json PIDs" → "Quick Attach to UCP"  
   - **Applications RP**: "Rebuild and Restart Applications RP" → "Update Launch.json PIDs" → "Quick Attach to Applications RP"
   - **Controller**: "Rebuild and Restart Controller" → "Update Launch.json PIDs" → "Quick Attach to Controller"
   - **Dynamic RP**: "Rebuild and Restart Dynamic RP" → "Update Launch.json PIDs" → "Quick Attach to Dynamic RP"

> **Note**: Currently, the rebuild/restart tasks restart all components because the underlying make system doesn't support individual component restart. This ensures all inter-component dependencies are properly refreshed, but means a slight delay when you only want to restart one component.
> 
> **💡 Contribution Opportunity**: Adding individual component start/stop make targets (e.g., `debug-start-ucpd`, `debug-stop-ucpd`) would enable truly granular rebuild/restart tasks. This would be a great contribution for anyone wanting to improve the developer experience!

#### Available VS Code Tasks

**Debug Management:**
- **"Show PIDs for Debugging"** - Display current process IDs for all components
- **"Update Launch.json PIDs"** - Automatically update Quick Attach configurations with current PIDs

**Build and Restart:**
- **"Rebuild and Restart UCP"** - Rebuild UCP binary and restart all components
- **"Rebuild and Restart Applications RP"** - Rebuild Applications RP and restart all components
- **"Rebuild and Restart Controller"** - Rebuild Controller and restart all components
- **"Rebuild and Restart Dynamic RP"** - Rebuild Dynamic RP and restart all components

**General:**
- **"Build All Components"** - Build all components (without restart)
- **"Component Status"** - Check which components are running
- **"View All Logs"** - Tail all component logs

> **Technical Note**: The rebuild/restart tasks currently restart all components due to make system limitations. Individual component builds are supported (`debug-build-ucpd`, etc.), but individual start/stop is not yet implemented.

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
make debug-build
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

The automation handles database setup verification. If you see database errors:

```bash
# Check if PostgreSQL is running
# macOS:
brew services start postgresql

# Linux:
sudo systemctl start postgresql

# Verify connection manually
psql "postgresql://radius_user:radius_pass@localhost:5432/radius" -c "SELECT 1;"

# If database doesn't exist, create it manually:
sudo -u postgres psql <<EOF
CREATE DATABASE radius;
CREATE USER radius_user WITH PASSWORD 'radius_pass';
GRANT ALL PRIVILEGES ON DATABASE radius TO radius_user;
GRANT CREATE ON SCHEMA public TO radius_user;
\q
EOF
```

**4. Port Conflicts**

```bash
# Check for port conflicts
lsof -i :9000  # UCP
lsof -i :8080  # Applications RP
lsof -i :8082  # Dynamic RP
lsof -i :7073  # Controller health
lsof -i :5017  # Deployment Engine

# Kill conflicting processes
sudo kill -9 $(lsof -t -i:9000)
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

**6. Controller TLS Certificate Issues**

The controller component uses webhooks for validation, which require TLS certificates in production. For local development, the automation automatically configures the controller to skip webhook setup when TLS certificates are not available.

If you see TLS-related errors in the controller logs:

```bash
# Check if controller is configured without TLS certificates (expected for local dev)
grep "Webhooks will be skipped" debug_files/logs/controller.log

# The controller should show this message for local development:
# "Webhooks will be skipped. TLS certificates not present."
```

The automation handles this automatically by:
- Setting `--cert-dir=""` in the start script
- Configuring VS Code launch configurations without TLS requirements
- The controller service detects empty cert directory and skips webhook registration

**7. rad CLI 503 "Service Unavailable" Errors**

When running Radius components as OS processes, the rad CLI may fail with a 503 error because it's configured to connect to Kubernetes instead of the local UCP endpoint. The CLI needs to be configured to connect directly to the local UCP at `http://localhost:9000`.

```bash
# Check current workspace configuration
rad workspace show

# If you see connection kind "kubernetes", you need to add UCP override
# Edit your workspace configuration file (~/.rad/config.yaml)
```

**Solution 1: Use the debug CLI wrapper (Recommended)**

The automation creates a debug CLI wrapper that automatically configures the UCP endpoint:

```bash
# Use the debug wrapper (no configuration needed)
./rad workspace show
./rad group create default
./rad env create default

# The ./rad symlink automatically uses debug configuration for local development
```

**Solution 2: Configure workspace UCP override**

Add a UCP override to your workspace configuration in `~/.rad/config.yaml`:

```yaml
workspaces:
  default: default
  items:
    default:
      connection:
        context: k3d-k3s-default  # Your current Kubernetes context
        kind: kubernetes
        overrides:
          ucp: http://localhost:9000  # Add this override
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
```

After adding the override:

```bash
# Test the connection
rad workspace show

# You should see output indicating direct UCP connection
# "Kubernetes (context=k3d-k3s-default, ucp=http://localhost:9000)"

# Create resources using local UCP
rad group create default
rad env create default
```

The automation handles this automatically by:
- Setting `--cert-dir=""` in the start script
- Configuring VS Code launch configurations without TLS requirements
- The controller service detects empty cert directory and skips webhook registration

### Getting Help

If you encounter issues not covered here:

1. **Check the rad CLI configuration**: The `./rad` wrapper is automatically configured for local debugging
2. **Check component logs**: Use `make debug-logs` to see all component output
3. **Verify prerequisites**: Run `make debug-check-prereqs` 
4. **Clean and restart**: Use `make debug-stop && make debug-setup`
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

🔶 **Partially Automated:**
- Database setup (automated checks, manual creation if needed)
- Kubernetes prerequisites (namespace creation, permission verification)
- Prerequisites validation (automated checking with installation guidance)

❌ **Manual Steps Required:**
- Tool installation (Go, kubectl, PostgreSQL, Terraform) - one-time setup
- Cloud credentials configuration (Azure/AWS) - as needed for your development
- Kubernetes cluster setup or access - one-time setup

The automation eliminates the complexity of manual configuration while preserving the flexibility needed for advanced debugging scenarios.
