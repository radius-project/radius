# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

### Building the Project
```bash
# Build all packages, binaries, and bicep templates
make build

# Build specific components
make build-packages    # Build all Go packages
make build-binaries    # Build all Go binaries
make build-rad        # Build the rad CLI
make build-controller # Build the Radius controller

# Build for specific platforms
make build-rad-darwin-arm64   # Build rad CLI for macOS ARM64
make build-rad-linux-amd64    # Build rad CLI for Linux AMD64

# Clean build artifacts
make clean
```

### Code Generation
```bash
# Generate all code (clients, CRDs, OpenAPI specs)
make generate

# Generate specific components
make generate-openapi-spec  # Generate OpenAPI specs from TypeSpec
make generate-controller    # Generate CRDs for Radius controller
make generate-ucp-crd       # Generate CRDs for UCP APIServer
```

## Testing

### Unit Tests
```bash
# Run unit tests
make test

# Run CLI integration tests
make test-validate-cli
```

### Functional Tests
```bash
# Run all functional tests (requires Kubernetes cluster)
make test-functional-all

# Run specific functional test suites
make test-functional-corerp    # Core RP tests
make test-functional-cli       # CLI tests
make test-functional-ucp       # UCP tests
make test-functional-daprrp    # Dapr RP tests

# Run tests that don't require cloud resources
make test-functional-all-noncloud
```

### Single Test Execution
```bash
# Run a single test file
go test -v ./pkg/cli/cmd/rollback/...

# Run a specific test
go test -v ./pkg/cli/cmd/rollback/... -run TestRollbackKubernetes

# Run tests with coverage
go test -v -cover ./pkg/cli/cmd/rollback/...

# Run tests with race detection
go test -v -race ./pkg/cli/cmd/rollback/...
```

## Linting and Formatting
```bash
# Format Go code
go fmt ./...

# Install golangci-lint if needed
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run golangci-lint (ensure it's installed first)
golangci-lint run

# Run specific linters
golangci-lint run --disable-all --enable=gofmt,govet,errcheck,staticcheck

# Auto-fix linting issues where possible
golangci-lint run --fix
```

## High-Level Architecture

### Repository Structure
The Radius project is a cloud-native application platform with the following key components:

1. **Universal Control Plane (UCP)** (`pkg/ucp/`)
   - Front-door proxy for all Radius operations
   - Handles integration with cloud resources (Azure, AWS)
   - Manages resource providers and planes
   - Located in `pkg/ucp/` with server entry point in `cmd/ucpd/`

2. **Resource Providers**
   - **Core RP** (`pkg/corerp/`): Manages applications, containers, environments
   - **Dapr RP** (`pkg/daprrp/`): Dapr integration (state stores, pub/sub, etc.)
   - **Datastores RP** (`pkg/datastoresrp/`): Database resources and recipes
   - **Messaging RP** (`pkg/messagingrp/`): Messaging resources (RabbitMQ, etc.)
   - **Dynamic RP** (`pkg/dynamicrp/`): Dynamic resource handling

3. **Radius CLI** (`pkg/cli/`, `cmd/rad/`)
   - Command-line interface for Radius operations
   - Commands organized in `pkg/cli/cmd/` with subcommands for each feature
   - Main entry point in `cmd/rad/main.go`

4. **Controller** (`pkg/controller/`, `cmd/controller/`)
   - Kubernetes controller for managing Radius resources
   - Handles reconciliation of deployments, recipes, and resources
   - Uses Kubernetes CRDs defined in `pkg/controller/api/`

5. **Recipe System** (`pkg/recipes/`)
   - Infrastructure-as-Code recipe engine
   - Supports Terraform and Bicep recipes
   - Recipe drivers for different IaC technologies

### Key Architectural Patterns

1. **ARM-RPC Pattern**: All resource providers follow Azure Resource Manager RPC patterns
   - Frontend handlers in `*/frontend/`
   - Backend processors in `*/backend/`
   - Data models in `*/datamodel/`
   - OpenAPI definitions generated from TypeSpec

2. **Resource ID System**: Uses ARM-style resource IDs
   - Parsed and handled via `pkg/ucp/resources/`
   - Scoped resources follow `/planes/.../providers/.../` pattern

3. **Async Operations**: Long-running operations use async patterns
   - Operations tracked in database
   - Status polling via operation endpoints

4. **Multi-Cloud Support**: Abstracted cloud operations
   - AWS support via `pkg/aws/`
   - Azure support via `pkg/azure/`
   - Cloud-agnostic interfaces in resource providers

## Development Tips

### Common Development Tasks

```bash
# Install Radius CLI locally after building
make build-rad
sudo cp ./dist/darwin_arm64/release/rad /usr/local/bin/rad

# Quick iteration on CLI changes
make build-rad && ./dist/darwin_arm64/release/rad [command]

# Debug a specific resource provider
go run ./cmd/applications-rp/main.go

# Check generated code is up to date
make generate && git diff

# Run local Kubernetes cluster for testing
kind create cluster --name radius-test
make test-functional-cli
```

### Working with Helm Charts

The project includes Helm charts for deployment located in `deploy/Chart/`:
- Main chart configuration in `deploy/Chart/values.yaml`
- CRD definitions in `deploy/Chart/crds/`
- Templates for all Radius components in `deploy/Chart/templates/`

```bash
# Package Helm chart
helm package deploy/Chart/

# Install Radius using Helm
helm install radius ./deploy/Chart/ --namespace radius-system --create-namespace

# Upgrade existing installation
helm upgrade radius ./deploy/Chart/ --namespace radius-system
```

### API Development

When modifying APIs:
1. Update TypeSpec definitions in `typespec/`
2. Run `make generate-openapi-spec` to regenerate OpenAPI specs
3. Run `make generate` to update Go clients and models
4. Update corresponding frontend/backend handlers in the resource provider

### Database Operations

The project uses MongoDB or In-Memory database for state storage:
- Database interfaces in `pkg/ucp/store/`
- MongoDB implementation in `pkg/components/database/mongo/`
- In-memory implementation for testing in `pkg/components/database/inmemory/`

## Important Notes

- The project uses Go 1.24.5 with modules
- All code generation requires TypeSpec and Autorest tools installed
- Kubernetes integration tests require a running cluster (use `kind` for local testing)
- The project follows CNCF standards and is a CNCF sandbox project
- Main documentation site: https://docs.radapp.io/
- Contributing guidelines: See CONTRIBUTING.md for detailed contribution process