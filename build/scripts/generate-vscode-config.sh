#!/bin/bash
# generate-vscode-config.sh - Generates VS Code debugging configuration
# Usage: generate-vscode-config.sh <config-file> <workspace-root>

set -euo pipefail

CONFIG_FILE="${1:-build/debug-config.yaml}"
WORKSPACE_ROOT="${2:-$(pwd)}"
VSCODE_DIR="$WORKSPACE_ROOT/.vscode"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Create .vscode directory
mkdir -p "$VSCODE_DIR"

# Generate launch.json
log_info "Generating launch.json..."

cat > "$VSCODE_DIR/launch.json" <<EOF
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug UCP",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "\${workspaceFolder}/cmd/ucpd",
      "args": [
        "--config-file=\${workspaceFolder}/debug_files/configs/ucp.yaml"
      ],
      "env": {
        "DATABASE_CONNECTION_STRING": "postgresql://radius_user:radius_pass@localhost:5432/radius",
        "RADIUS_LOG_LEVEL": "debug"
      },
      "cwd": "\${workspaceFolder}/debug_files",
      "showLog": true,
      "logOutput": "console"
    },
    {
      "name": "Debug Applications RP",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "\${workspaceFolder}/cmd/applications-rp",
      "args": [
        "--config-file=\${workspaceFolder}/debug_files/configs/applications-rp.yaml"
      ],
      "env": {
        "RADIUS_ENV": "self-hosted",
        "K8S_CLUSTER": "true",
        "UCP_ENDPOINT": "http://localhost:9000",
        "KUBECONFIG": "\${env:HOME}/.kube/config"
      },
      "cwd": "\${workspaceFolder}/debug_files",
      "showLog": true,
      "logOutput": "console",
      "preLaunchTask": "Verify UCP Running"
    },
    {
      "name": "Debug Controller",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "\${workspaceFolder}/cmd/controller",
      "args": [
        "--config-file=\${workspaceFolder}/debug_files/configs/controller.yaml",
        "--cert-dir="
      ],
      "env": {
        "UCP_ENDPOINT": "http://localhost:9000",
        "KUBECONFIG": "\${env:HOME}/.kube/config"
      },
      "cwd": "\${workspaceFolder}/debug_files",
      "showLog": true,
      "logOutput": "console"
    },
    {
      "name": "Debug Dynamic RP",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "\${workspaceFolder}/cmd/dynamic-rp",
      "args": [
        "--config-file=\${workspaceFolder}/debug_files/configs/dynamic-rp.yaml"
      ],
      "env": {
        "UCP_ENDPOINT": "http://localhost:9000",
        "KUBECONFIG": "\${env:HOME}/.kube/config"
      },
      "cwd": "\${workspaceFolder}/debug_files",
      "showLog": true,
      "logOutput": "console"
    },
    {
      "name": "Debug Deployment Engine",
      "type": "coreclr",
      "request": "launch",
      "program": "\${workspaceFolder}/../deployment-engine/src/DeploymentEngine/bin/Debug/net8.0/DeploymentEngine.dll",
      "args": [],
      "env": {
        "ASPNETCORE_ENVIRONMENT": "Development",
        "RADIUSBACKENDURL": "http://localhost:9000/apis/api.ucp.dev/v1alpha3",
        "ASPNETCORE_URLS": "http://localhost:5017"
      },
      "cwd": "\${workspaceFolder}/../deployment-engine/src/DeploymentEngine",
      "console": "internalConsole",
      "stopAtEntry": false,
      "serverReadyAction": {
        "action": "openExternally",
        "pattern": "Now listening on: https?://[^:]+:([0-9]+)",
        "uriFormat": "http://localhost:%s"
      },
      "preLaunchTask": "Build Deployment Engine"
    },
    {
      "name": "Attach to Running Applications RP",
      "type": "go",
      "request": "attach",
      "mode": "local",
      "processId": "\${command:pickProcess}",
      "showLog": true
    }
  ],
  "compounds": [
    {
      "name": "Launch Control Plane (all)",
      "configurations": [
        "Debug UCP",
        "Debug Applications RP",
        "Debug Dynamic RP",
        "Debug Controller"
      ],
      "stopAll": true,
      "preLaunchTask": "Build All Components"
    },
    {
      "name": "Launch Control Plane (with Deployment Engine)",
      "configurations": [
        "Debug UCP",
        "Debug Applications RP", 
        "Debug Dynamic RP",
        "Debug Controller",
        "Debug Deployment Engine"
      ],
      "stopAll": true,
      "preLaunchTask": "Build All Components"
    }
  ]
}
EOF

# Generate tasks.json
log_info "Generating tasks.json..."

cat > "$VSCODE_DIR/tasks.json" <<EOF
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Build All Components",
      "type": "shell",
      "command": "make",
      "args": ["build"],
      "group": {
        "kind": "build",
        "isDefault": true
      },
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": false,
        "panel": "shared"
      },
      "problemMatcher": ["\$go"]
    },
    {
      "label": "Build Debug Components",
      "type": "shell",
      "command": "make",
      "args": ["debug-build"],
      "group": "build",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": false,
        "panel": "shared"
      },
      "problemMatcher": ["\$go"]
    },
    {
      "label": "Build Deployment Engine",
      "type": "shell",
      "command": "dotnet",
      "args": ["build"],
      "options": {
        "cwd": "\${workspaceFolder}/../deployment-engine/src/DeploymentEngine"
      },
      "group": "build",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": false,
        "panel": "shared"
      },
      "problemMatcher": "\$msCompile"
    },
    {
      "label": "Start All Components",
      "type": "shell",
      "command": "make",
      "args": ["debug-start"],
      "group": "build",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    },
    {
      "label": "Stop All Components",
      "type": "shell",
      "command": "make",
      "args": ["debug-stop"],
      "group": "build",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    },
    {
      "label": "Component Status",
      "type": "shell",
      "command": "make",
      "args": ["debug-status"],
      "group": "test",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    },
    {
      "label": "Verify UCP Running",
      "type": "shell",
      "command": "curl",
      "args": ["-s", "http://localhost:9000/healthz"],
      "group": "test",
      "presentation": {
        "echo": false,
        "reveal": "never"
      }
    },
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    },
    {
      "label": "View All Logs",
      "type": "shell",
      "command": "make",
      "args": ["debug-logs"],
      "group": "test",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    },
    {
      "label": "Clean Debug Environment",
      "type": "shell",
      "command": "make",
      "args": ["debug-clean"],
      "group": "build",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "focus": true,
        "panel": "new"
      }
    }
  ]
}
EOF

# Generate settings.json with Go debugging configurations
log_info "Generating settings.json..."

cat > "$VSCODE_DIR/settings.json" <<EOF
{
  "go.gopath": "\${env:GOPATH}",
  "go.useLanguageServer": true,
  "go.delveConfig": {
    "dlvLoadConfig": {
      "followPointers": true,
      "maxVariableRecurse": 1,
      "maxStringLen": 64,
      "maxArrayValues": 64,
      "maxStructFields": -1
    },
    "apiVersion": 2,
    "showGlobalVariables": true
  },
  "files.associations": {
    "*.yaml": "yaml",
    "*.yml": "yaml"
  },
  "terminal.integrated.env.linux": {
    "RADIUS_DEV_ROOT": "\${workspaceFolder}/debug_files"
  },
  "terminal.integrated.env.osx": {
    "RADIUS_DEV_ROOT": "\${workspaceFolder}/debug_files"
  },
  "terminal.integrated.env.windows": {
    "RADIUS_DEV_ROOT": "\${workspaceFolder}/debug_files"
  }
}
EOF

# Generate extensions.json with recommended extensions
log_info "Generating extensions.json..."

cat > "$VSCODE_DIR/extensions.json" <<EOF
{
  "recommendations": [
    "golang.go",
    "ms-vscode.vscode-json",
    "redhat.vscode-yaml",
    "ms-kubernetes-tools.vscode-kubernetes-tools",
    "ms-dotnettools.csharp",
    "ms-vscode.makefile-tools"
  ]
}
EOF

log_success "VS Code configuration generated successfully!"
log_info "Configuration files created in $VSCODE_DIR:"
echo "  - launch.json (debug configurations)"
echo "  - tasks.json (build and automation tasks)"
echo "  - settings.json (workspace settings)"
echo "  - extensions.json (recommended extensions)"
echo ""
log_info "Usage:"
echo "1. Open VS Code: code ."
echo "2. Install recommended extensions when prompted"
echo "3. Use F5 to start debugging with 'Launch Control Plane (all)'"
echo "4. Use Ctrl+Shift+P -> 'Tasks: Run Task' for build and management tasks"
