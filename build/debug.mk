# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

##@ Debugging

.PHONY: dump
dump: ## Outputs the values of all variables in the makefile.
	$(foreach v, \
		$(shell echo "$(filter-out .VARIABLES,$(.VARIABLES))" | tr ' ' '\n' | sort), \
		$(info $(shell printf "%-20s" "$(v)")= $(value $(v))) \
	)

##@ Debug Development Automation
# This section provides automation for running Radius components as OS processes for advanced debugging

# Debug configuration
DEBUG_CONFIG_FILE ?= build/debug-config.yaml
DEBUG_DEV_ROOT ?= $(PWD)/debug_files

.PHONY: debug-setup debug-start debug-stop debug-status debug-help debug-build-all debug-build-ucpd debug-build-applications-rp debug-build-controller debug-build-dynamic-rp debug-build-rad debug-deployment-engine-pull debug-deployment-engine-start debug-deployment-engine-stop debug-deployment-engine-status debug-deployment-engine-logs debug-register-recipes debug-env-init debug-check-prereqs

debug-help: ## Show debug automation help
	@echo "Debug Development Automation Commands:"
	@echo ""
	@echo "Setup Commands:"
	@echo "  debug-check-prereqs  - Check if all required tools are installed"
	@echo "  debug-setup          - Complete one-time setup for OS process debugging"
	@echo "  debug-stop           - Stop all components and destroy k3d cluster"
	@echo ""
	@echo "Runtime Commands:"
	@echo "  debug-start          - Start all Radius components as OS processes"
	@echo "  debug-stop           - Stop all components, destroy cluster, and clean up completely"
	@echo "  debug-status         - Show status of all components"
	@echo "  debug-logs           - Tail all component logs"
	@echo ""
	@echo "Environment Commands:"
	@echo "  debug-env-init       - Create resource group, environment, and register recipes (first time only)"
	@echo "  debug-register-recipes - Register default recipes for common resource types"
	@echo ""
	@echo ""
	@echo "Deployment Engine Commands:"
	@echo "  debug-deployment-engine-pull   - Pull latest deployment engine image from ghcr.io"
	@echo "  debug-deployment-engine-start  - Start deployment engine in k3d cluster"
	@echo "  debug-deployment-engine-stop   - Stop deployment engine"
	@echo "  debug-deployment-engine-status - Check deployment engine status"
	@echo "  debug-deployment-engine-logs   - View deployment engine logs"
	@echo ""
	@echo "Development Commands:"
	@echo "  debug-build          - Build all components with debug symbols (incremental)"
	@echo "  debug-build-ucpd     - Build only UCP daemon (only compiles changed code)"
	@echo "  debug-build-applications-rp - Build only Applications RP (only compiles changed code)"
	@echo "  debug-build-controller - Build only Controller (only compiles changed code)"
	@echo "  debug-build-dynamic-rp - Build only Dynamic RP (only compiles changed code)"
	@echo "  debug-build-rad      - Build only rad CLI (only compiles changed code) + symlink"
	@echo "  debug-logs           - Tail all component logs"
	@echo ""
	@echo "💡 All builds are incremental - only changed code is recompiled"
	@echo "💡 Individual component builds are fastest when working on specific components"
	@echo "💡 drad CLI is created for debug configuration (preserves 'rad' for your installed version)"
	@echo ""
	@echo "Configuration:"
	@echo "  DEBUG_CONFIG_FILE    - Debug configuration file (default: build/debug-config.yaml)"
	@echo "  DEBUG_DEV_ROOT       - Debug development root (default: $(PWD)/debug_files)"
	@echo ""

debug-setup: debug-check-prereqs ## Complete one-time setup for OS process debugging
	@echo "Setting up Radius debug environment..."
	@mkdir -p $(DEBUG_DEV_ROOT)/{logs,bin,terraform-cache}
	@echo "Making scripts executable..."
	@chmod +x build/scripts/*.sh 2>/dev/null || true
	@chmod +x build/scripts/rad-wrapper 2>/dev/null || true
	@chmod +x drad 2>/dev/null || true
	@echo "✅ Debug environment setup complete at $(DEBUG_DEV_ROOT)"
	@echo "💡 Use './drad' command from project root for debug environment"
	@echo "📖 See docs/contributing/contributing-code/contributing-code-debugging/radius-os-processes-debugging.md for usage instructions"

debug-check-prereqs: ## Check if all required tools are installed for debugging
	@echo "🔍 Checking debug prerequisites..."
	@MISSING_TOOLS=""; \
	if ! command -v go >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS go"; \
	fi; \
	if ! command -v dlv >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS dlv"; \
	fi; \
	if ! command -v k3d >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS k3d"; \
	fi; \
	if ! command -v kubectl >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS kubectl"; \
	fi; \
	if ! command -v terraform >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS terraform"; \
	fi; \
	if [ -n "$$MISSING_TOOLS" ]; then \
		echo "❌ Missing required tools:$$MISSING_TOOLS"; \
		echo ""; \
		echo "Installation instructions:"; \
		echo "  go: https://golang.org/doc/install"; \
		echo "  dlv: go install github.com/go-delve/delve/cmd/dlv@latest"; \
		echo "  k3d: https://k3d.io/v5.6.0/#installation"; \
		echo "  kubectl: https://kubernetes.io/docs/tasks/tools/"; \
		echo "  terraform: https://learn.hashicorp.com/tutorials/terraform/install-cli"; \
		exit 1; \
	fi; \
	if ! command -v psql >/dev/null 2>&1; then \
		echo "⚠️  psql not available - database may not be properly initialized"; \
	fi; \
	if ! command -v docker >/dev/null 2>&1; then \
		echo "⚠️  docker not available - deployment engine will not be available"; \
	elif ! docker info >/dev/null 2>&1; then \
		echo "⚠️  Docker daemon not running - deployment engine will not be available"; \
	fi; \
	echo "✅ All required tools are available"

debug-build: build ## Build components with debug symbols for debugging
	@echo "Building Radius components with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin

debug-build-all: debug-build-ucpd debug-build-applications-rp debug-build-controller debug-build-dynamic-rp debug-build-rad ## Build all debug components
	@echo "✅ All debug binaries built in $(DEBUG_DEV_ROOT)/bin/"

debug-build-ucpd: ## Build UCP daemon with debug symbols
	@echo "Building ucpd with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin
	@go build -gcflags="all=-N -l" -o $(DEBUG_DEV_ROOT)/bin/ucpd ./cmd/ucpd
	@echo "✅ ucpd built"

debug-build-applications-rp: ## Build Applications RP with debug symbols
	@echo "Building applications-rp with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin
	@go build -gcflags="all=-N -l" -o $(DEBUG_DEV_ROOT)/bin/applications-rp ./cmd/applications-rp
	@echo "✅ applications-rp built"

debug-build-controller: ## Build Controller with debug symbols
	@echo "Building controller with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin
	@go build -gcflags="all=-N -l" -o $(DEBUG_DEV_ROOT)/bin/controller ./cmd/controller
	@echo "✅ controller built"

debug-build-dynamic-rp: ## Build Dynamic RP with debug symbols
	@echo "Building dynamic-rp with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin
	@go build -gcflags="all=-N -l" -o $(DEBUG_DEV_ROOT)/bin/dynamic-rp ./cmd/dynamic-rp
	@echo "✅ dynamic-rp built"

debug-build-rad: ## Build rad CLI with debug symbols + create drad alias
	@echo "Building rad CLI with debug symbols..."
	@mkdir -p $(DEBUG_DEV_ROOT)/bin
	@go build -gcflags="all=-N -l" -o $(DEBUG_DEV_ROOT)/bin/rad ./cmd/rad
	@echo "✅ rad CLI built"
	@echo "Creating drad alias for debug CLI..."
	@if [ -f build/scripts/rad-wrapper ]; then \
		ln -sf build/scripts/rad-wrapper ./drad; \
		echo "✅ drad alias (debug wrapper) created"; \
	else \
		ln -sf $(DEBUG_DEV_ROOT)/bin/rad ./drad; \
		echo "✅ drad alias (binary) created"; \
	fi
	@echo "💡 Use './drad' for debug-configured CLI (preserves 'rad' for your installed version)"

debug-start: debug-setup debug-build-all ## Start k3d cluster and all Radius components as OS processes
	@echo "Creating k3d cluster..."
	@if k3d cluster list | grep -q "radius-debug"; then \
		echo "k3d cluster 'radius-debug' already exists"; \
	else \
		k3d cluster create radius-debug --api-port 6443 --wait --timeout 60s; \
	fi
	@echo "Switching to k3d context..."
	@kubectl config use-context k3d-radius-debug
	@echo "Starting Radius components as OS processes..."
	@build/scripts/start-radius.sh
	@echo "Waiting for components to be ready..."
	@max_attempts=30; \
	attempt=0; \
	while [ $$attempt -lt $$max_attempts ]; do \
		if curl -s "http://localhost:9000/healthz" > /dev/null 2>&1; then \
			echo "✅ UCP is ready"; \
			break; \
		fi; \
		echo "Waiting for UCP... (attempt $$((attempt + 1))/$$max_attempts)"; \
		sleep 2; \
		attempt=$$((attempt + 1)); \
	done; \
	if [ $$attempt -eq $$max_attempts ]; then \
		echo "❌ UCP not ready after $$max_attempts attempts"; \
		echo "💡 Check component logs with 'make debug-logs'"; \
		exit 1; \
	fi
	@echo "Initializing environment resources..."
	@$(MAKE) debug-env-init
	@echo "🚀 All components started and environment initialized!"
	@echo "📊 Use 'make debug-status' to check component health"
	@echo "🚢 Use 'make debug-deployment-engine-status' to check deployment engine"

debug-stop: ## Stop all running Radius components, destroy k3d cluster, and clean up
	@echo "Stopping Radius components..."
	@if [ -f build/scripts/stop-radius.sh ]; then \
		build/scripts/stop-radius.sh; \
	else \
		echo "❌ Stop script not found at build/scripts/stop-radius.sh"; \
		exit 1; \
	fi
	@echo "Stopping deployment engine..."
	@$(MAKE) debug-deployment-engine-stop
	@echo "Destroying k3d cluster..."
	@k3d cluster delete radius-debug 2>/dev/null || echo "k3d cluster was not running"
	@echo "Cleaning up PostgreSQL databases..."
	@psql "postgresql://$(shell whoami)@localhost:5432/postgres" -c "DROP DATABASE IF EXISTS applications_rp; DROP DATABASE IF EXISTS ucp; DROP DATABASE IF EXISTS radius;" 2>/dev/null || echo "Database cleanup completed or PostgreSQL not accessible"
	@psql "postgresql://$(shell whoami)@localhost:5432/postgres" -c "DROP USER IF EXISTS applications_rp; DROP USER IF EXISTS ucp; DROP USER IF EXISTS radius_user;" 2>/dev/null || echo "User cleanup completed or PostgreSQL not accessible"
	@echo "Cleaning up debug files and symlinks..."
	@rm -rf $(DEBUG_DEV_ROOT)/logs
	@rm -f ./drad
	@echo "✅ Debug environment completely stopped and cleaned up"

debug-status: ## Show status of all components
	@if [ -f build/scripts/status-radius.sh ]; then \
		build/scripts/status-radius.sh; \
	else \
		echo "❌ Status script not found at build/scripts/status-radius.sh"; \
		exit 1; \
	fi

debug-logs: ## Tail all component logs
	@echo "Tailing all component logs (Ctrl+C to stop)..."
	@if [ -d $(DEBUG_DEV_ROOT)/logs ]; then \
		tail -f $(DEBUG_DEV_ROOT)/logs/*.log; \
	else \
		echo "❌ Logs directory not found. Start components first with 'make debug-start'"; \
		exit 1; \
	fi

# Deployment Engine Management
debug-deployment-engine-pull: ## Pull latest deployment engine image from ghcr.io
	@echo "Pulling Deployment Engine image from ghcr.io..."
	@command -v docker >/dev/null 2>&1 || { echo "❌ Docker not found. Please install Docker to use Deployment Engine"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "❌ Docker daemon not running. Please start Docker"; exit 1; }
	@docker pull ghcr.io/radius-project/deployment-engine:latest \
		&& echo "✅ Deployment Engine image pulled successfully" \
		|| echo "❌ Failed to pull Deployment Engine image"

debug-deployment-engine-start: ## Start deployment engine in k3d cluster
	@echo "Installing ONLY deployment engine to k3d cluster..."
	@echo "Applying deployment engine manifest to k3d cluster..."
	@kubectl --context k3d-radius-debug apply -f build/configs/deployment-engine.yaml
	@echo "Waiting for deployment engine to be ready..."
	@kubectl --context k3d-radius-debug wait --for=condition=available deployment/deployment-engine --timeout=60s
	@echo "Setting up port forwarding for deployment engine..."
	@pkill -f "port-forward.*deployment-engine" 2>/dev/null || true
	@kubectl --context k3d-radius-debug port-forward -n default service/deployment-engine 5017:5445 > $(DEBUG_DEV_ROOT)/logs/de-port-forward.log 2>&1 &
	@echo "Waiting for deployment engine health check..."
	@max_attempts=30; \
	attempt=0; \
	while [ $$attempt -lt $$max_attempts ]; do \
		if curl -s "http://localhost:5017/metrics" > /dev/null 2>&1; then \
			echo "✅ Deployment Engine is ready"; \
			break; \
		fi; \
		echo "Waiting for Deployment Engine... (attempt $$((attempt + 1))/$$max_attempts)"; \
		sleep 2; \
		attempt=$$((attempt + 1)); \
	done; \
	if [ $$attempt -eq $$max_attempts ]; then \
		echo "❌ Deployment Engine not ready after $$max_attempts attempts"; \
		echo "💡 Check component logs with 'make debug-logs' and 'make debug-deployment-engine-logs'"; \
		exit 1; \
	fi
	@echo "✅ Deployment engine installed and ready in k3d cluster"

debug-deployment-engine-stop: ## Stop deployment engine in k3d cluster
	@echo "Removing deployment engine from k3d cluster..."
	@pkill -f "port-forward.*deployment-engine" 2>/dev/null || true
	@kubectl --context k3d-radius-debug delete deployment deployment-engine 2>/dev/null || echo "Deployment engine deployment not found"
	@kubectl --context k3d-radius-debug delete service deployment-engine 2>/dev/null || echo "Deployment engine service not found"
	@echo "✅ Deployment engine removed from k3d cluster"

debug-deployment-engine-status: ## Check deployment engine status
	@echo "🚀 Deployment Engine Status:"
	@if kubectl --context k3d-radius-debug get deployment deployment-engine >/dev/null 2>&1; then \
		replicas=$$(kubectl --context k3d-radius-debug get deployment deployment-engine -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0"); \
		if [ "$$replicas" = "1" ]; then \
			echo "✅ Deployment Engine (k3d) - Running and ready"; \
		else \
			echo "⚠️  Deployment Engine (k3d) - Deployment exists but not ready"; \
		fi; \
	else \
		echo "❌ Deployment Engine - Not deployed to k3d cluster"; \
		echo "💡 Start with: make debug-deployment-engine-start"; \
	fi

debug-deployment-engine-logs: ## View deployment engine logs
	@echo "Showing deployment engine logs from k3d cluster..."
	@kubectl --context k3d-radius-debug logs -l app=deployment-engine --tail=100 -f



# Recipe registration
debug-register-recipes: ## Register default recipes in the debug environment
	@echo "Registering default recipes..."
	@if [ ! -f build/scripts/rad-wrapper ]; then \
		echo "❌ rad-wrapper script not found. This should not happen."; \
		exit 1; \
	fi
	@build/scripts/register-recipes.sh

debug-env-init: ## Create default resource group, environment, and register recipes
	@echo "Initializing debug environment resources..."
	@if [ ! -f build/scripts/rad-wrapper ]; then \
		echo "❌ rad-wrapper script not found. This should not happen."; \
		exit 1; \
	fi
	@echo "Creating resource group 'default'..."
	@build/scripts/rad-wrapper group create default || echo "Resource group may already exist"
	@echo "Creating environment 'default' with Kubernetes compute configuration..."
	@build/scripts/rad-wrapper env create default --namespace default || echo "Environment may already exist"
	@echo "Starting deployment engine in k3d cluster..."
	@$(MAKE) debug-deployment-engine-start
	@echo "Registering default recipes..."
	@$(MAKE) debug-register-recipes
	@echo "✅ Debug environment ready for application deployment!"

# Integration with existing build system
build-debug: debug-build ## Alias for debug-build

# Validate debug configuration
debug-validate:
	@if [ ! -f $(DEBUG_CONFIG_FILE) ]; then \
		echo "❌ Debug configuration file not found: $(DEBUG_CONFIG_FILE)"; \
		echo "💡 This file should be created automatically during setup"; \
		exit 1; \
	fi
	@echo "✅ Debug configuration valid"

# Development workflow targets
debug-dev-start: debug-setup debug-start ## Complete development setup and start
	@echo "🎉 Debug development environment ready!"

debug-dev-stop: debug-stop ## Stop development environment
	@echo "🛑 Debug development environment stopped"

# Prerequisite checks
debug-check-prereqs:
	@echo "Checking prerequisites for debug development..."
	@command -v go >/dev/null 2>&1 || { echo "❌ Go not found. Please install Go 1.21+"; exit 1; }
	@command -v dlv >/dev/null 2>&1 || { echo "❌ Delve debugger not found. Please install: go install github.com/go-delve/delve/cmd/dlv@latest"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "❌ kubectl not found. Please install kubectl"; exit 1; }
	@command -v psql >/dev/null 2>&1 || { echo "❌ PostgreSQL client not found. Please install PostgreSQL"; exit 1; }
	@command -v terraform >/dev/null 2>&1 || { echo "❌ Terraform not found. Please install Terraform"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "⚠️  Docker not found. Deployment Engine will not be available"; }
	@if command -v docker >/dev/null 2>&1; then \
		docker info >/dev/null 2>&1 || { echo "⚠️  Docker daemon not running. Start Docker to use Deployment Engine"; }; \
	fi
	@echo "✅ Core prerequisites found"

.PHONY: debug-check-prereqs debug-validate debug-dev-start debug-dev-stop build-debug
