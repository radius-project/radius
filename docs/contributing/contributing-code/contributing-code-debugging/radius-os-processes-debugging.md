# Running Radius as OS Processes for Advanced Debugging

This guide explains how to run Radius components as native OS processes instead of containers to enable advanced debugging capabilities, including using debuggers and debugging authentication issues.

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

The Radius build system provides complete automation for OS process debugging:

```bash
# Check prerequisites and setup everything
make debug-setup

# Generate VS Code debugging configuration
make debug-vscode

# Start all components as OS processes
make debug-start

# Check component status
make debug-status

# View logs
make debug-logs

# Stop all components
make debug-stop
```

**What the automation provides:**
- Environment directory structure at `debug_files/` (in project root)
- Component configuration files with correct schemas
- Controller configured to skip webhooks in local development (no TLS certs required)
- Database setup verification
- Management scripts (start/stop/status)
- VS Code launch and task configurations
- Deployment engine setup (Docker-based by default)
- Incremental builds for individual components
- Convenient `./rad` symlink in workspace root for easy CLI access
- Debug CLI wrapper `./rad` with automatic UCP endpoint configuration

## Prerequisites

The automation checks for all required tools. Install any missing prerequisites:

### Required Tools
- **Go 1.21+** - `go version`
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
# 2. Generates VS Code debugging configuration
# 3. Builds components with debug symbols
# 4. Starts all components as OS processes
# 5. Provides next steps for creating resources

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
make debug-vscode         # Generate VS Code debugging configuration
make debug-clean          # Clean up debug environment

# Development Workflow
make debug-start          # Start all components as OS processes
make debug-stop           # Stop all running components
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

And in your workspace:

```
.vscode/
├── launch.json            # Debug configurations
├── tasks.json             # VS Code tasks
├── settings.json          # Workspace settings
└── extensions.json        # Recommended extensions
```

## VS Code Debugging

> 💡 **Quick Setup**: Run `make debug-vscode` to automatically generate all VS Code configuration files.

### Generated Launch Configurations

The automation creates the following debug configurations in `.vscode/launch.json`:

- **"Debug UCP"** - Debug the Universal Control Plane
- **"Debug Applications RP"** - Debug the Applications Resource Provider
- **"Debug Controller"** - Debug the Kubernetes Controller
- **"Debug Dynamic RP"** - Debug the Dynamic Resource Provider
- **"Launch Control Plane (all)"** - Start all components for debugging

### Debugging Workflow in VS Code

1. **Setup** (one-time):
   ```bash
   make debug-setup
   make debug-vscode
   ```

2. **Start Debugging**:
   - Open VS Code in the radius repository
   - Open Debug panel (Ctrl+Shift+D / Cmd+Shift+D)
   - Select "Launch Control Plane (all)"
   - Press F5 or click Start Debugging

3. **Development Process**:
   - Set breakpoints in your code
   - Make changes and rebuild: Ctrl+Shift+P → "Tasks: Run Task" → "Build All Components"
   - Restart debugging: Ctrl+Shift+F5

4. **Component Status**:
   - All components start automatically in the correct order
   - UCP starts first (port 9000)
   - Applications RP, Dynamic RP, and Controller follow
   - Health checks verify successful startup

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
4. **Clean and restart**: Use `make debug-clean && make debug-setup`
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
