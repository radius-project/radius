# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Radius is a cloud-native application platform that enables developers and platform engineers to collaborate on delivering and managing cloud-native applications. It's a CNCF sandbox project supporting deployments across private cloud, Microsoft Azure, and Amazon Web Services.

## Development Commands

### Building
```bash
make build               # Build all packages and binaries
make build-packages     # Build Go packages only  
make build-binaries     # Build executables only
make clean              # Clean output directory
```

### Testing
```bash
make test                        # Unit tests
make test-functional-all         # All functional tests  
make test-functional-*-noncloud  # Non-cloud functional tests
make test-validate-cli           # CLI integration tests
make test-validate-bicep         # Bicep validation
```

### Code Quality
```bash
make lint                # Run golangci-lint
make format-check        # Check formatting
make format-write        # Fix formatting
make generate            # Generate code (APIs, mocks, CRDs)
```

### Development Database
```bash
make db-init             # Initialize PostgreSQL database
make db-stop             # Stop database
make db-shell            # Open database shell  
make db-reset            # Reset database
```

### Docker Operations
```bash
make docker-build        # Build Docker images
make docker-push         # Push Docker images
make install            # Install local build
```

## Architecture Overview

### Core Components
- **Universal Control Plane (UCP)** (`pkg/ucp/`) - Resource lifecycle management across clouds
- **Resource Providers** (`pkg/corerp/`, `pkg/daprrp/`, etc.) - Domain-specific resource management
- **ARM RPC Framework** (`pkg/armrpc/`) - ARM-compatible API framework
- **Recipes System** (`pkg/recipes/`) - Infrastructure component recipes (Bicep/Terraform)
- **Kubernetes Controller** (`pkg/controller/`) - Kubernetes-native operations
- **CLI** (`pkg/cli/`) - User-facing command-line interface

### Key Executables
- `rad` - Main CLI tool
- `ucpd` - Universal Control Plane daemon
- `applications-rp` - Applications Resource Provider
- `controller` - Kubernetes controller
- `dynamic-rp` - Dynamic Resource Provider

### Multi-Cloud Architecture
The system uses cloud-agnostic abstractions with provider-specific implementations for Azure (Azure SDK for Go) and AWS (AWS SDK v2), unified through the Universal Control Plane.

## Development Workflow

1. **Code Generation**: Run `make generate` after API changes or when adding mocks
2. **Build & Test**: Always run `make build test lint format-check` before commits
3. **Format**: Use `make format-write` to fix formatting issues
4. **Functional Testing**: Use `make test-functional-*-noncloud` for local testing

## Technology Stack

- **Go 1.24.2+** - Primary implementation language
- **TypeSpec** - API schema definitions in `typespec/`
- **Kubernetes** - Container orchestration and custom resources
- **PostgreSQL** - Data persistence layer
- **Bicep/Terraform** - Infrastructure recipes
- **OpenTelemetry** - Observability and distributed tracing

## Testing Strategy

The project follows a test pyramid approach:
- **Unit tests** - Located alongside source code
- **Integration tests** - In `test/` directory  
- **Functional tests** - End-to-end scenarios with cloud resources
- **CLI validation** - User workflow testing

## Code Organization

- `cmd/` - Entry points for all executables
- `pkg/` - Core implementation organized by functionality
- `deploy/` - Helm charts, Docker images, and deployment manifests
- `typespec/` - API schema definitions
- `test/` - Integration and end-to-end tests
- `hack/` - Code generation and development utilities

## Contributing Requirements

- **Developer Certificate of Origin** - All commits must be signed-off (`git commit -s`)
- **Issue-first approach** - Start with an existing issue or create one before coding
- **Code quality** - Unit tests required for new functionality
- **ARM compatibility** - APIs must follow ARM RPC patterns

## Prerequisites

- Go 1.24.2+
- Node.js (for TypeSpec)
- Docker
- PostgreSQL
- Kubernetes cluster (for functional tests)
- Bicep CLI (for infrastructure templates)