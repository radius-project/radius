# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Radius is a cloud-native application platform that enables developers and platform engineers to collaborate on delivering and managing cloud-native applications. It's a CNCF sandbox project that supports deploying applications across private cloud, Microsoft Azure, and Amazon Web Services.

## Key Commands

### Building and Testing

```bash
# Build all packages and executables
make build

# Run unit tests (excluding Kubernetes controller tests)
make test

# Run linters
make lint

# Check code formatting
make format-check

# Fix formatting issues (for TS, JS, MJS, and JSON files)
make format-write

# Complete verification (build, test, lint, format check)
make build test lint format-check

# Generate code (mocks, CRDs, SDK clients)
make generate

# Run functional tests
make test-functional-all         # All functional tests
make test-functional-all-noncloud  # Tests not requiring cloud resources
make test-functional-corerp       # Core RP tests
make test-functional-cli          # CLI tests

# Run Helm chart unit tests
make test-helm

# Validate Bicep files
make test-validate-bicep
```

### Docker and Deployment

```bash
# Build Docker images
make docker-build

# Push Docker images (set DOCKER_REGISTRY and DOCKER_TAG_VERSION)
DOCKER_REGISTRY=ghcr.io/my-registry DOCKER_TAG_VERSION=latest make docker-build docker-push

# Install local build for development
make install

# Publish test recipes
BICEP_RECIPE_REGISTRY=<registry> make publish-test-bicep-recipes
make publish-test-terraform-recipes
```

### Database Management

```bash
# Initialize local PostgreSQL database
make db-init

# Stop database
make db-stop

# Open database shell
make db-shell

# Reset database
make db-reset
```

### Debug and Development

```bash
# See all available make targets
make help

# Dump all makefile variables (for debugging)
make dump

# Run single test example (from test directory)
go test -v -run TestSpecificTest ./test/functional-portable/corerp/noncloud/
```

## High-Level Architecture

### Core Components

1. **Universal Control Plane (UCP)** (`pkg/ucp/`)
   - Acts as a proxy between services
   - Manages deployments of cloud resources (AWS, Azure)
   - Handles resource planes and resource groups
   - Port: 9000 (local development)

2. **Applications.Core RP** (`pkg/corerp/`)
   - Handles core resources (containers, environments, applications)
   - Manages recipe execution
   - Port: 8080 (local development)

3. **Deployment Engine** (`pkg/bicep/`, `de/`)
   - Handles Bicep deployment orchestration
   - Converts and deploys Bicep templates
   - Port: 5017 (local development)

4. **Kubernetes Controller** (`pkg/controller/`)
   - Manages Radius CRDs
   - Handles deployment reconciliation
   - Integrates with Flux for GitOps

5. **Dynamic RP** (`pkg/dynamicrp/`)
   - Manages resources without dedicated resource providers
   - Handles generic resource lifecycle

### Resource Providers

- **Applications.Core** (`pkg/corerp/`): Containers, environments, gateways, volumes
- **Applications.Dapr** (`pkg/daprrp/`): Dapr configuration stores, pub/sub, state stores
- **Applications.Datastores** (`pkg/datastoresrp/`): MongoDB, Redis, SQL databases
- **Applications.Messaging** (`pkg/messagingrp/`): RabbitMQ queues

### CLI (`pkg/cli/`)

The `rad` CLI is the primary interface for users:
- Manages workspaces and environments
- Deploys applications
- Handles recipes
- Integrates with cloud providers (Azure, AWS)

### Key Patterns

1. **ARM RPC Pattern**: All resource providers follow Azure Resource Manager patterns
   - Frontend handlers for HTTP requests
   - Backend controllers for business logic
   - DataModel for resource state
   - Conversions between API versions

2. **Recipe System** (`pkg/recipes/`)
   - Supports Terraform and Bicep recipes
   - Environment-based configuration
   - Secret management integration

3. **Portable Resources** (`pkg/portableresources/`)
   - Abstraction over cloud-specific resources
   - Recipe-based provisioning
   - Multi-cloud support

## Important Development Notes

1. **Namespace Usage**: Debug setup uses `radius-testing` namespace, separate from installed Radius

2. **Code Generation**: Always run `make generate` after modifying:
   - API schemas
   - Go interfaces with mocks
   - TypeSpec definitions
   - CRD definitions

3. **Testing Cloud Resources**: Set appropriate environment variables for cloud provider credentials

4. **Fast Cleanup Mode**: Use `RADIUS_TEST_FAST_CLEANUP=true` for faster test execution in CI

5. **Local Development**: Create a `dev` workspace in `~/.rad/config.yaml` with UCP override to `http://localhost:9000`

## TypeSpec and API Definitions

- API definitions are in `typespec/` using TypeSpec language
- Generates OpenAPI specs in `swagger/specification/`
- Each resource provider has its own TypeSpec module

## Helm Chart Deployment

- Chart located in `deploy/Chart/`
- Supports Azure Workload Identity and AWS IRSA
- Configurable resource limits and prometheus metrics
- Pre-upgrade jobs for version compatibility checks

## Common Workflows

1. **Making API Changes**:
   - Update TypeSpec definitions
   - Run `make generate`
   - Update tests
   - Commit generated files

2. **Adding New Resource Type**:
   - Create TypeSpec definition
   - Implement frontend/backend/datamodel
   - Add conversion logic
   - Write unit and functional tests

3. **Debugging Locally**:
   - Run `rad init`
   - Configure dev workspace
   - Create `radius-testing` namespace
   - Use VS Code launch configurations

4. **Testing Recipe Changes**:
   - Update recipe in `test/testrecipes/`
   - Republish with `make publish-test-*-recipes`
   - Run relevant functional tests