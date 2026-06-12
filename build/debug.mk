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

# PostgreSQL connection - Try Docker first (postgres user), fallback to local user
POSTGRES_ADMIN_CONNECTION ?= postgresql://postgres:radius_pass@localhost:5432/postgres
POSTGRES_FALLBACK_CONNECTION ?= postgresql://$(shell whoami)@localhost:5432/postgres
POSTGRES_CONTAINER_NAME ?= radius-postgres

# Local OCI registry for test Bicep recipes. applications-rp runs as a host
# process in debug mode, so it pulls recipes from localhost directly.
DEBUG_REGISTRY_NAME ?= radius-debug-registry
DEBUG_REGISTRY_PORT ?= 5000
DEBUG_REGISTRY_HOST ?= localhost:$(DEBUG_REGISTRY_PORT)

# In-cluster Git HTTP backend used by the kubernetes-noncloud Flux tests.
# Values mirror .github/workflows/functional-test-noncloud.yaml so that local
# runs of `make test-functional-kubernetes-noncloud` Just Work after
# `make debug-start`.
DEBUG_GIT_HTTP_NAMESPACE ?= git-http-backend
DEBUG_GIT_HTTP_USERNAME ?= testuser
DEBUG_GIT_HTTP_PASSWORD ?= not-a-secret-password
DEBUG_GIT_HTTP_EMAIL ?= testuser@radapp.io
DEBUG_GIT_HTTP_LOCAL_PORT ?= 30080

# Flux source-controller is required by the kubernetes-noncloud Flux tests.
# Version must match the locally installed `flux` CLI to avoid the compatibility
# check failing during `flux install`.
DEBUG_FLUX_NAMESPACE ?= flux-system
DEBUG_FLUX_VERSION ?= $(shell flux --version 2>/dev/null | awk '{print $$NF}')

# Bicep extension types published to the local debug registry.
# Mirrors .github/workflows/functional-test-noncloud.yaml so dynamicrp-noncloud
# tests resolve `radius` / `testresources` extensions from localhost rather than
# the public ACR (which drifts from source `*.yaml`).
#
# Strategy: write a gitignored bicepconfig.json INSIDE the testdata/ dir. Bicep
# walks up from each .bicep template looking for the nearest bicepconfig.json,
# so this override wins over the tracked sibling one folder up — without
# mutating anything that's tracked.
DEBUG_BICEP_VERSION ?= latest
DEBUG_BICEP_TEST_RESOURCES_YAML ?= test/functional-portable/dynamicrp/noncloud/resources/testdata/testresourcetypes.yaml
DEBUG_BICEP_TEST_CONFIG_OVERRIDE ?= test/functional-portable/dynamicrp/noncloud/resources/testdata/bicepconfig.json
# Local file targets for the published bicep extensions. We deliberately avoid
# the local OCI registry here: the bicep CLI's `publish-extension` defaults to
# HTTPS even for `localhost:5000` and has no `--plain-http` flag, so a TLS
# handshake fails. bicepconfig.json's `extensions` block accepts absolute file
# paths to `.tgz` artifacts, which is faster and more deterministic anyway.
DEBUG_BICEP_EXT_DIR ?= $(DEBUG_DEV_ROOT)/bicep-extensions
DEBUG_BICEP_EXT_RADIUS ?= $(DEBUG_BICEP_EXT_DIR)/radius.tgz
DEBUG_BICEP_EXT_TESTRESOURCES ?= $(DEBUG_BICEP_EXT_DIR)/testresources.tgz

# Contour ingress controller is required by the Gateway functional tests
# (corerp-noncloud) which create `projectcontour.io/v1` HTTPProxy resources.
# Production `rad install kubernetes` installs it; our local-OS-process stack
# bypasses that path, so install it directly via helm.
DEBUG_CONTOUR_NAMESPACE ?= radius-system
DEBUG_CONTOUR_RELEASE ?= contour
DEBUG_CONTOUR_CHART_VERSION ?= 0.1.0
DEBUG_CONTOUR_HELM_REPO ?= https://projectcontour.github.io/helm-charts

# In-cluster Terraform module server used by the corerp-noncloud
# TerraformRecipe_* tests. Tests resolve the server via TF_RECIPE_MODULE_SERVER_URL
# (set by test/testutil); CI uses the in-cluster DNS name. For the local stack
# (ARP runs as an OS process), we port-forward the service to localhost:8999.
DEBUG_TF_MODULE_NAMESPACE ?= radius-test-tf-module-server
DEBUG_TF_MODULE_DEPLOYMENT ?= tf-module-server
DEBUG_TF_MODULE_LOCAL_PORT ?= 8999

.PHONY: debug-setup debug-start debug-stop debug-status debug-help debug-build-all debug-build-ucpd debug-build-applications-rp debug-build-controller debug-build-dynamic-rp debug-build-rad debug-deployment-engine-pull debug-deployment-engine-start debug-deployment-engine-deploy debug-deployment-engine-port-forward debug-deployment-engine-stop debug-deployment-engine-status debug-deployment-engine-logs debug-register-recipes debug-env-init debug-check-prereqs debug-install-crds debug-start-registry debug-publish-recipes debug-stop-registry debug-install-git-http-backend debug-stop-git-http-backend debug-install-flux debug-install-contour debug-install-tf-module-server debug-stop-tf-module-server debug-publish-bicep-types debug-remove-bicep-types-override

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
	@echo "    (alias of debug-build-all)"
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
	if ! command -v yq >/dev/null 2>&1; then \
		MISSING_TOOLS="$$MISSING_TOOLS yq"; \
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
		echo "  yq: go install github.com/mikefarah/yq/v4@$(YQ_VERSION)"; \
		exit 1; \
	fi; \
	echo "🔍 Checking PostgreSQL connectivity..."; \
	if command -v psql >/dev/null 2>&1; then \
		if psql "$(POSTGRES_ADMIN_CONNECTION)" -c "SELECT 1;" >/dev/null 2>&1; then \
			echo "✅ PostgreSQL accessible via local psql (Docker connection)"; \
		elif psql "$(POSTGRES_FALLBACK_CONNECTION)" -c "SELECT 1;" >/dev/null 2>&1; then \
			echo "✅ PostgreSQL accessible via local psql (local user)"; \
		elif docker exec $(POSTGRES_CONTAINER_NAME) psql -U postgres -c "SELECT 1;" >/dev/null 2>&1; then \
			echo "✅ PostgreSQL accessible via Docker container ($(POSTGRES_CONTAINER_NAME))"; \
		else \
			echo "❌ psql found but cannot connect to PostgreSQL"; \
			echo "   Quick start: docker run --name $(POSTGRES_CONTAINER_NAME) -e POSTGRES_PASSWORD=radius_pass -p 5432:5432 -d postgres:15"; \
			exit 1; \
		fi; \
	elif docker exec $(POSTGRES_CONTAINER_NAME) psql -U postgres -c "SELECT 1;" >/dev/null 2>&1; then \
		echo "✅ PostgreSQL accessible via Docker container ($(POSTGRES_CONTAINER_NAME))"; \
	else \
		echo "❌ Cannot connect to PostgreSQL"; \
		echo "   No local psql client found and Docker container '$(POSTGRES_CONTAINER_NAME)' is not running."; \
		echo ""; \
		echo "   Option 1 (recommended): Start PostgreSQL in Docker:"; \
		echo "     docker run --name $(POSTGRES_CONTAINER_NAME) -e POSTGRES_PASSWORD=radius_pass -p 5432:5432 -d postgres:15"; \
		echo ""; \
		echo "   Option 2: Install psql client and start a local PostgreSQL:"; \
		echo "     macOS (homebrew):  brew install postgresql && brew services start postgresql"; \
		echo "     Linux (systemd):   sudo systemctl start postgresql"; \
		echo ""; \
		echo "   After starting, re-run: make debug-check-prereqs"; \
		exit 1; \
	fi; \
	echo "✅ PostgreSQL is accessible"; \
	if ! command -v docker >/dev/null 2>&1; then \
		echo "⚠️ Docker not available - deployment engine will not be available"; \
	elif ! docker info >/dev/null 2>&1; then \
		echo "⚠️ Docker daemon not running - deployment engine will not be available"; \
	fi; \
	echo "✅ All required tools are available"

## debug-build is maintained as an alias for debug-build-all to avoid confusion about which
## target to use. The legacy behavior depended on the top-level 'build' target with DEBUG flags;
## now we always explicitly build all debug binaries via the component-specific targets.
debug-build: debug-build-all ## Alias: build all components with debug symbols (see debug-build-all)
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
	@echo "Ensuring k3d cluster 'radius-debug' is running..."
	@if k3d cluster list --no-headers 2>/dev/null | awk '{print $$1}' | grep -qx "radius-debug"; then \
		servers=$$(k3d cluster list --no-headers 2>/dev/null | awk '$$1=="radius-debug"{print $$2}'); \
		running=$$(echo "$$servers" | cut -d/ -f1); \
		if [ "$$running" = "0" ]; then \
			echo "k3d cluster 'radius-debug' exists but is stopped — starting it..."; \
			k3d cluster start radius-debug; \
		else \
			echo "k3d cluster 'radius-debug' already running ($$servers servers)"; \
		fi; \
	else \
		echo "Creating k3d cluster 'radius-debug'..."; \
		k3d cluster create radius-debug --api-port 0.0.0.0:6443 --wait --timeout 60s; \
	fi
	@echo "Switching to k3d context..."
	@# If the kubeconfig context is missing (e.g. cluster was created against a
	@# different KUBECONFIG, or `kubectl config delete-context` was run), merge
	@# the k3d kubeconfig back in before switching. `k3d kubeconfig merge` is
	@# idempotent and writes/updates ~/.kube/config.
	@if ! kubectl config get-contexts -o name 2>/dev/null | grep -qx "k3d-radius-debug"; then \
		echo "  Context 'k3d-radius-debug' missing — merging k3d kubeconfig..."; \
		k3d kubeconfig merge radius-debug --kubeconfig-merge-default --kubeconfig-switch-context >/dev/null; \
	fi
	@kubectl config use-context k3d-radius-debug
	@echo "Ensuring radius-encryption-key secret exists in k3d cluster..."
	@chmod +x build/scripts/ensure-encryption-key.sh 2>/dev/null || true
	@build/scripts/ensure-encryption-key.sh
	@$(MAKE) debug-install-crds
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
	@echo "Publishing test Bicep recipes to the local debug registry..."
	@$(MAKE) debug-publish-recipes
	@echo "Installing in-cluster Git HTTP backend (for kubernetes-noncloud Flux tests)..."
	@$(MAKE) debug-install-git-http-backend
	@echo "Installing Flux source-controller (for kubernetes-noncloud Flux tests)..."
	@$(MAKE) debug-install-flux
	@echo "Installing Contour ingress controller (for Gateway tests)..."
	@$(MAKE) debug-install-contour
	@echo "Installing Terraform module server (for TerraformRecipe_* tests)..."
	@$(MAKE) debug-install-tf-module-server
	@echo "Publishing test bicep extensions to the local debug registry..."
	@$(MAKE) debug-publish-bicep-types
	@echo "🚀 All components started and environment initialized!"
	@echo "📊 Use 'make debug-status' to check component health"
	@echo "🚢 Use 'make debug-deployment-engine-status' to check deployment engine"

debug-install-crds: ## Apply Radius CRDs (radapp.io and ucp.dev) into the k3d-radius-debug cluster
	@echo "Applying Radius CRDs to k3d-radius-debug..."
	@kubectl --context k3d-radius-debug apply -f deploy/Chart/crds/radius -f deploy/Chart/crds/ucpd
	@echo "✅ Radius CRDs applied"

debug-start-registry: ## Start a local OCI registry on $(DEBUG_REGISTRY_HOST) for test recipes
	@if docker ps --format '{{.Names}}' | grep -qx "$(DEBUG_REGISTRY_NAME)"; then \
		echo "✅ Local registry '$(DEBUG_REGISTRY_NAME)' already running on $(DEBUG_REGISTRY_HOST)"; \
	elif docker ps -a --format '{{.Names}}' | grep -qx "$(DEBUG_REGISTRY_NAME)"; then \
		echo "Starting existing registry container '$(DEBUG_REGISTRY_NAME)'..."; \
		docker start $(DEBUG_REGISTRY_NAME) >/dev/null; \
		echo "✅ Local registry started on $(DEBUG_REGISTRY_HOST)"; \
	else \
		echo "Creating local registry on $(DEBUG_REGISTRY_HOST)..."; \
		docker run -d --restart=unless-stopped --name $(DEBUG_REGISTRY_NAME) \
			-p $(DEBUG_REGISTRY_PORT):5000 registry:2 >/dev/null; \
		echo "✅ Local registry created on $(DEBUG_REGISTRY_HOST)"; \
	fi

debug-publish-recipes: debug-start-registry ## Publish test Bicep recipes to the local debug registry
	@$(MAKE) publish-test-bicep-recipes BICEP_RECIPE_REGISTRY=$(DEBUG_REGISTRY_HOST) BICEP_RECIPE_TAG_VERSION=latest BICEP_RECIPE_PLAIN_HTTP=true
	@echo "✅ Recipes published to $(DEBUG_REGISTRY_HOST)"
	@echo "💡 Functional tests will auto-detect this registry — no env vars needed."

debug-stop-registry: ## Stop and remove the local debug OCI registry
	@if docker ps -a --format '{{.Names}}' | grep -qx "$(DEBUG_REGISTRY_NAME)"; then \
		docker rm -f $(DEBUG_REGISTRY_NAME) >/dev/null; \
		echo "✅ Local registry '$(DEBUG_REGISTRY_NAME)' removed"; \
	else \
		echo "Local registry '$(DEBUG_REGISTRY_NAME)' not present"; \
	fi

debug-install-git-http-backend: ## Deploy in-cluster Git HTTP backend and port-forward $(DEBUG_GIT_HTTP_LOCAL_PORT)->3000 (for Flux tests)
	@echo "Deploying git-http-backend into namespace '$(DEBUG_GIT_HTTP_NAMESPACE)'..."
	@KUBECONFIG_CTX=k3d-radius-debug; \
	kubectl config use-context $$KUBECONFIG_CTX >/dev/null
	@.github/actions/install-git-http-backend/install-git-http-backend.sh \
		"$(DEBUG_GIT_HTTP_USERNAME)" "$(DEBUG_GIT_HTTP_PASSWORD)" \
		"$(DEBUG_GIT_HTTP_NAMESPACE)"
	@mkdir -p $(DEBUG_DEV_ROOT)/logs
	@if [ -f $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid ]; then \
		old=$$(cat $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid 2>/dev/null); \
		if [ -n "$$old" ] && kill -0 "$$old" 2>/dev/null; then kill "$$old" 2>/dev/null || true; fi; \
		rm -f $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid; \
	fi
	@pkill -f "port-forward.*git-http-backend.*git-http" 2>/dev/null || true
	@echo "Waiting for git-http-backend pod to be Ready..."
	@kubectl --context k3d-radius-debug wait --for=condition=Available deployment/git-http-backend \
		-n $(DEBUG_GIT_HTTP_NAMESPACE) --timeout=120s >/dev/null
	@echo "Starting port-forward localhost:$(DEBUG_GIT_HTTP_LOCAL_PORT) -> deploy/git-http-backend:3000..."
	@# Port-forward to the deployment (auto-selects a Ready pod) rather than the
	@# service: a service endpoint round-robin can land on a still-terminating pod
	@# from the previous rollout and the forward dies with "network namespace ...
	@# is closed". We also retry the whole forward+curl probe so a flaky pod
	@# transition can't leave us with a stale, dead listener.
	@attempt=0; max=6; ok=0; \
	while [ $$attempt -lt $$max ]; do \
		attempt=$$((attempt+1)); \
		pkill -f "port-forward.*git-http-backend.*git-http" 2>/dev/null || true; \
		sleep 1; \
		nohup kubectl --context k3d-radius-debug port-forward \
			-n $(DEBUG_GIT_HTTP_NAMESPACE) deploy/git-http-backend \
			$(DEBUG_GIT_HTTP_LOCAL_PORT):3000 \
			> $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.log 2>&1 & \
		pf_pid=$$!; \
		echo $$pf_pid > $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid; \
		probe=0; \
		while [ $$probe -lt 15 ]; do \
			probe=$$((probe+1)); \
			if kill -0 $$pf_pid 2>/dev/null && \
				curl -s -o /dev/null -m 2 "http://localhost:$(DEBUG_GIT_HTTP_LOCAL_PORT)"; then \
				ok=1; break; \
			fi; \
			sleep 2; \
		done; \
		if [ $$ok -eq 1 ]; then break; fi; \
		echo "git-http-backend port-forward attempt $$attempt/$$max failed, retrying..."; \
	done; \
	if [ $$ok -ne 1 ]; then \
		echo "❌ git-http-backend not reachable after $$max port-forward attempts"; \
		echo "--- port-forward log ---"; \
		tail -20 $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.log 2>/dev/null || true; \
		exit 1; \
	fi
	@echo "✅ git-http-backend reachable at http://localhost:$(DEBUG_GIT_HTTP_LOCAL_PORT)"
	@echo "💡 Functional tests auto-detect this — no env vars needed."

debug-stop-git-http-backend: ## Tear down the in-cluster Git HTTP backend and port-forward
	@if [ -f $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid ]; then \
		pid=$$(cat $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid 2>/dev/null); \
		if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then kill "$$pid" 2>/dev/null || true; fi; \
		rm -f $(DEBUG_DEV_ROOT)/logs/git-http-port-forward.pid; \
	fi
	@pkill -f "port-forward.*git-http-backend.*git-http" 2>/dev/null || true
	@kubectl --context k3d-radius-debug delete namespace $(DEBUG_GIT_HTTP_NAMESPACE) --wait=false >/dev/null 2>&1 || true
	@echo "✅ git-http-backend stopped"

debug-install-flux: ## Install Flux source-controller into the k3d-radius-debug cluster (for Flux tests)
	@if ! command -v flux >/dev/null 2>&1; then \
		echo "⚠️  flux CLI not found on PATH; skipping Flux source-controller install."; \
		echo "   Install via: brew install fluxcd/tap/flux"; \
		echo "   Then re-run: make debug-install-flux"; \
		exit 0; \
	fi
	@if kubectl --context k3d-radius-debug get deployment -n $(DEBUG_FLUX_NAMESPACE) source-controller >/dev/null 2>&1; then \
		echo "✅ Flux source-controller already installed in namespace '$(DEBUG_FLUX_NAMESPACE)'"; \
		exit 0; \
	fi
	@if [ -z "$(DEBUG_FLUX_VERSION)" ]; then \
		echo "❌ Could not determine flux CLI version"; \
		exit 1; \
	fi
	@echo "Installing Flux source-controller v$(DEBUG_FLUX_VERSION) (matches local flux CLI)..."
	@kubectl config use-context k3d-radius-debug >/dev/null
	@for i in 1 2 3; do \
		flux install --namespace=$(DEBUG_FLUX_NAMESPACE) --version=v$(DEBUG_FLUX_VERSION) \
			--components=source-controller --network-policy=false && \
		kubectl wait --for=condition=available deployment \
			-l app.kubernetes.io/component=source-controller \
			-n $(DEBUG_FLUX_NAMESPACE) --timeout=120s && break; \
		echo "Attempt $$i failed, retrying in 10 seconds..."; \
		sleep 10; \
	done
	@echo "✅ Flux source-controller ready in namespace '$(DEBUG_FLUX_NAMESPACE)'"

debug-install-contour: ## Install the Contour ingress controller (required by Gateway tests)
	@command -v helm >/dev/null 2>&1 || { echo "❌ helm CLI not found on PATH (install via: brew install helm)"; exit 1; }
	@kubectl config use-context k3d-radius-debug >/dev/null
	@if helm --kube-context k3d-radius-debug status $(DEBUG_CONTOUR_RELEASE) -n $(DEBUG_CONTOUR_NAMESPACE) -o json 2>/dev/null | grep -q '"status": *"deployed"'; then \
		echo "✅ Contour already installed (release '$(DEBUG_CONTOUR_RELEASE)' in namespace '$(DEBUG_CONTOUR_NAMESPACE)')"; \
		exit 0; \
	fi
	@echo "Installing Contour $(DEBUG_CONTOUR_CHART_VERSION) into namespace '$(DEBUG_CONTOUR_NAMESPACE)'..."
	@kubectl create namespace $(DEBUG_CONTOUR_NAMESPACE) --dry-run=client -o yaml | \
		kubectl --context k3d-radius-debug apply -f - >/dev/null
	@helm --kube-context k3d-radius-debug repo add contour $(DEBUG_CONTOUR_HELM_REPO) >/dev/null 2>&1 || true
	@helm --kube-context k3d-radius-debug repo update contour >/dev/null
	@# Do NOT pass `--wait`: on k3d, contour-envoy is a LoadBalancer service whose
	@# EXTERNAL-IP allocation by klipper-lb can take several minutes, causing the
	@# helm release to be marked `failed` even though the controller + envoy pods
	@# are Running. We wait explicitly on the things tests actually need below.
	@helm --kube-context k3d-radius-debug upgrade --install $(DEBUG_CONTOUR_RELEASE) contour/contour \
		--namespace $(DEBUG_CONTOUR_NAMESPACE) --version $(DEBUG_CONTOUR_CHART_VERSION) --timeout 5m
	@echo "Waiting for Contour controller + envoy to become Ready..."
	@kubectl --context k3d-radius-debug wait --for=condition=Available \
		deployment/$(DEBUG_CONTOUR_RELEASE)-contour -n $(DEBUG_CONTOUR_NAMESPACE) --timeout=180s
	@kubectl --context k3d-radius-debug rollout status \
		daemonset/$(DEBUG_CONTOUR_RELEASE)-envoy -n $(DEBUG_CONTOUR_NAMESPACE) --timeout=180s
	@# Sanity check: the Gateway tests reference projectcontour.io/v1 HTTPProxy.
	@kubectl --context k3d-radius-debug get crd httpproxies.projectcontour.io >/dev/null \
		|| { echo "❌ HTTPProxy CRD missing after Contour install"; exit 1; }
	@echo "✅ Contour ingress controller ready (HTTPProxy CRD present)"

debug-install-tf-module-server: ## Deploy in-cluster Terraform module server and port-forward localhost:$(DEBUG_TF_MODULE_LOCAL_PORT) (for TerraformRecipe_* tests)
	@echo "Publishing test Terraform recipes and deploying $(DEBUG_TF_MODULE_DEPLOYMENT) into namespace '$(DEBUG_TF_MODULE_NAMESPACE)'..."
	@kubectl config use-context k3d-radius-debug >/dev/null
	@$(MAKE) publish-test-terraform-recipes
	@echo "Waiting for $(DEBUG_TF_MODULE_DEPLOYMENT) to become Available..."
	@kubectl --context k3d-radius-debug wait --for=condition=Available \
		deployment/$(DEBUG_TF_MODULE_DEPLOYMENT) -n $(DEBUG_TF_MODULE_NAMESPACE) --timeout=120s
	@# Kill any stale port-forward for this port so we can rebind cleanly.
	@pkill -f "kubectl.*port-forward.*$(DEBUG_TF_MODULE_DEPLOYMENT).*$(DEBUG_TF_MODULE_LOCAL_PORT):" >/dev/null 2>&1 || true
	@mkdir -p $(DEBUG_DEV_ROOT)/logs
	@echo "Starting port-forward localhost:$(DEBUG_TF_MODULE_LOCAL_PORT) -> svc/$(DEBUG_TF_MODULE_DEPLOYMENT):80..."
	@nohup kubectl --context k3d-radius-debug port-forward \
		svc/$(DEBUG_TF_MODULE_DEPLOYMENT) $(DEBUG_TF_MODULE_LOCAL_PORT):80 \
		-n $(DEBUG_TF_MODULE_NAMESPACE) \
		> $(DEBUG_DEV_ROOT)/logs/tf-module-server-pf.log 2>&1 & disown
	@# Wait for the port-forward to actually accept connections before returning.
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		if curl -m 2 -fsS -o /dev/null "http://localhost:$(DEBUG_TF_MODULE_LOCAL_PORT)/" 2>/dev/null \
		   || curl -m 2 -sS -o /dev/null -w "%{http_code}" "http://localhost:$(DEBUG_TF_MODULE_LOCAL_PORT)/" 2>/dev/null | grep -qE "^(2|3|4)"; then \
			echo "✅ tf-module-server reachable at http://localhost:$(DEBUG_TF_MODULE_LOCAL_PORT)"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "❌ tf-module-server port-forward not reachable on localhost:$(DEBUG_TF_MODULE_LOCAL_PORT)"; \
	tail -20 $(DEBUG_DEV_ROOT)/logs/tf-module-server-pf.log 2>/dev/null || true; \
	exit 1

debug-stop-tf-module-server: ## Tear down the Terraform module server port-forward
	@pkill -f "kubectl.*port-forward.*$(DEBUG_TF_MODULE_DEPLOYMENT).*$(DEBUG_TF_MODULE_LOCAL_PORT):" >/dev/null 2>&1 || true
	@echo "✅ tf-module-server port-forward stopped"

debug-publish-bicep-types: ## Build radius+testresources bicep extensions as local .tgz files and drop a closer-wins bicepconfig override next to the test templates
	@command -v bicep >/dev/null 2>&1 || { echo "❌ bicep CLI not found on PATH (install via: az bicep install / brew install bicep)"; exit 1; }
	@RAD_BIN="$(DEBUG_DEV_ROOT)/bin/rad"; \
	if [ ! -x "$$RAD_BIN" ]; then RAD_BIN="$$(command -v rad)" || true; fi; \
	if [ -z "$$RAD_BIN" ] || [ ! -x "$$RAD_BIN" ]; then \
		echo "❌ rad binary not found at $(DEBUG_DEV_ROOT)/bin/rad or on PATH (run 'make debug-build-rad' first)"; \
		exit 1; \
	fi; \
	mkdir -p $(DEBUG_BICEP_EXT_DIR); \
	echo "Generating bicep types (VERSION=$(DEBUG_BICEP_VERSION))..."; \
	$(MAKE) generate-bicep-types VERSION=$(DEBUG_BICEP_VERSION) || exit $$?; \
	echo "Publishing radius extension -> $(DEBUG_BICEP_EXT_RADIUS)..."; \
	bicep publish-extension ./hack/bicep-types-radius/generated/index.json \
		--target $(DEBUG_BICEP_EXT_RADIUS) --force || exit $$?; \
	echo "Publishing testresources extension -> $(DEBUG_BICEP_EXT_TESTRESOURCES)..."; \
	"$$RAD_BIN" bicep publish-extension \
		-f $(DEBUG_BICEP_TEST_RESOURCES_YAML) \
		--target $(DEBUG_BICEP_EXT_TESTRESOURCES) --force || exit $$?
	@# Bicep resolves the nearest bicepconfig.json by walking up from each .bicep
	@# file. Writing the override in testdata/ (gitignored) means it wins over the
	@# tracked sibling in resources/ without mutating any tracked file. Absolute
	@# file paths in `extensions` skip the OCI registry entirely (the bicep CLI's
	@# publish-extension forces HTTPS even for localhost, which the local debug
	@# registry does not support).
	@radius_abs=$$(cd $$(dirname $(DEBUG_BICEP_EXT_RADIUS)) && pwd)/$$(basename $(DEBUG_BICEP_EXT_RADIUS)); \
	tr_abs=$$(cd $$(dirname $(DEBUG_BICEP_EXT_TESTRESOURCES)) && pwd)/$$(basename $(DEBUG_BICEP_EXT_TESTRESOURCES)); \
	{ \
		echo '{'; \
		echo '	"experimentalFeaturesEnabled": {'; \
		echo '		"extensibility": true'; \
		echo '	},'; \
		echo '	"extensions": {'; \
		echo "		\"radius\": \"$$radius_abs\","; \
		echo '		"aws": "br:biceptypes.azurecr.io/aws:latest",'; \
		echo "		\"testresources\": \"$$tr_abs\""; \
		echo '	}'; \
		echo '}'; \
	} > $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE)
	@echo "✅ Bicep extensions built to $(DEBUG_BICEP_EXT_DIR); override written to $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE)"
	@echo "💡 The tracked resources/bicepconfig.json is untouched; 'make debug-stop' removes the override."

debug-remove-bicep-types-override: ## Delete the gitignored bicepconfig override next to the dynamicrp test templates
	@if [ -f $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE) ]; then \
		rm -f $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE); \
		echo "✅ Removed $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE)"; \
	else \
		echo "ℹ️  No bicepconfig override at $(DEBUG_BICEP_TEST_CONFIG_OVERRIDE); nothing to remove"; \
	fi

debug-stop: ## Stop all running Radius components, destroy k3d cluster, and clean up
	@echo "Stopping Radius components..."
	@if [ -f build/scripts/stop-radius.sh ]; then \
		build/scripts/stop-radius.sh; \
	else \
		echo "❌ Stop script not found at build/scripts/stop-radius.sh"; \
		exit 1; \
	fi
	@echo "Cleaning up debug files and symlinks..."
	@rm -rf $(DEBUG_DEV_ROOT)/logs
	@rm -f ./drad
	@$(MAKE) debug-remove-bicep-types-override
	@$(MAKE) debug-stop-tf-module-server
	@$(MAKE) debug-stop-git-http-backend
	@$(MAKE) debug-stop-registry
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

debug-deployment-engine-start: ## Start deployment engine in k3d cluster (or reuse a local OS process on port 5017)
	@echo "Checking for an existing Deployment Engine on localhost:5017..."
	@listener_cmd=""; \
	if command -v lsof >/dev/null 2>&1; then \
		listener_cmd=$$(lsof -nP -iTCP:5017 -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $$1}'); \
	fi; \
	if [ -n "$$listener_cmd" ] && [ "$$listener_cmd" != "kubectl" ] && curl -s "http://localhost:5017/metrics" > /dev/null 2>&1; then \
		echo "✅ Detected local Deployment Engine process ($$listener_cmd) on port 5017 — reusing it"; \
		echo "💡 Skipping k3d deployment-engine install and port-forward"; \
		mkdir -p $(DEBUG_DEV_ROOT)/logs; \
		echo "external" > $(DEBUG_DEV_ROOT)/logs/de-external.marker; \
	else \
		rm -f $(DEBUG_DEV_ROOT)/logs/de-external.marker 2>/dev/null || true; \
		echo "Installing ONLY deployment engine to k3d cluster..."; \
		if kubectl --context k3d-radius-debug get deployment deployment-engine >/dev/null 2>&1 && \
			kubectl --context k3d-radius-debug get deployment deployment-engine -o jsonpath='{.status.readyReplicas}' 2>/dev/null | grep -q "1" && \
			curl -s "http://localhost:5017/metrics" > /dev/null 2>&1; then \
			echo "✅ Deployment engine already running and healthy in k3d"; \
		else \
			$(MAKE) debug-deployment-engine-deploy; \
			$(MAKE) debug-deployment-engine-port-forward; \
		fi; \
		echo "✅ Deployment engine installed and ready in k3d cluster"; \
	fi

debug-deployment-engine-deploy: ## Deploy deployment engine to k3d cluster
	@echo "Applying deployment engine manifest to k3d cluster..."
	@kubectl --context k3d-radius-debug apply -f build/configs/deployment-engine.yaml
	@echo "Waiting for deployment engine to be ready..."
	@kubectl --context k3d-radius-debug wait --for=condition=available deployment/deployment-engine --timeout=60s

debug-deployment-engine-port-forward: ## Set up port forwarding for deployment engine
	@build/scripts/setup-deployment-engine-port-forward.sh

debug-deployment-engine-stop: ## Stop deployment engine in k3d cluster (leaves external local DE process alone)
	@if [ -f $(DEBUG_DEV_ROOT)/logs/de-external.marker ]; then \
		echo "ℹ️ External local Deployment Engine was in use — leaving it running"; \
		rm -f $(DEBUG_DEV_ROOT)/logs/de-external.marker; \
		exit 0; \
	fi
	@echo "Removing deployment engine from k3d cluster..."
	@if [ -f $(DEBUG_DEV_ROOT)/logs/de-port-forward.pid ]; then \
		kill $$(cat $(DEBUG_DEV_ROOT)/logs/de-port-forward.pid) 2>/dev/null || true; \
		rm -f $(DEBUG_DEV_ROOT)/logs/de-port-forward.pid; \
	fi
	@pkill -f "port-forward.*deployment-engine" 2>/dev/null || true
	@kubectl --context k3d-radius-debug delete deployment deployment-engine 2>/dev/null || echo "Deployment engine deployment not found"
	@kubectl --context k3d-radius-debug delete service deployment-engine 2>/dev/null || echo "Deployment engine service not found"
	@echo "✅ Deployment engine removed from k3d cluster"

debug-deployment-engine-status: ## Check deployment engine status
	@echo "🚀 Deployment Engine Status:"
	@if [ -f $(DEBUG_DEV_ROOT)/logs/de-external.marker ] && curl -s "http://localhost:5017/metrics" > /dev/null 2>&1; then \
		echo "✅ Deployment Engine (external local process on :5017) - Running"; \
	elif kubectl --context k3d-radius-debug get deployment deployment-engine >/dev/null 2>&1; then \
		replicas=$$(kubectl --context k3d-radius-debug get deployment deployment-engine -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0"); \
		if [ "$$replicas" = "1" ]; then \
			echo "✅ Deployment Engine (k3d) - Running and ready"; \
		else \
			echo "⚠️ Deployment Engine (k3d) - Deployment exists but not ready"; \
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

# Prerequisite checks are handled by the main debug-check-prereqs target above

.PHONY: debug-check-prereqs debug-validate debug-dev-start debug-dev-stop build-debug
