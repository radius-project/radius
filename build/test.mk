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

##@ Test

CLI_DOWNLOAD_TEST_SCRIPT := ./build/test-cli-download.sh

# Default values for CLI download test
CLI_DOWNLOAD_OS ?= linux
CLI_DOWNLOAD_ARCH ?= amd64
CLI_DOWNLOAD_FILE ?= rad
CLI_DOWNLOAD_EXT ?=

.PHONY: test-cli-download
test-cli-download: ## Test CLI download for specified OS and ARCH (defaults to linux/amd64). Usage: make test-cli-download [CLI_DOWNLOAD_OS=linux] [CLI_DOWNLOAD_ARCH=amd64] [CLI_DOWNLOAD_FILE=rad] [CLI_DOWNLOAD_EXT=]
	@bash $(CLI_DOWNLOAD_TEST_SCRIPT) $(CLI_DOWNLOAD_OS) $(CLI_DOWNLOAD_ARCH) $(CLI_DOWNLOAD_FILE) $(CLI_DOWNLOAD_EXT)

# Will be set by our build workflow, this is just a default
TEST_TIMEOUT ?=1h
RADIUS_CONTAINER_LOG_PATH ?=./dist/container_logs
REL_VERSION ?=latest
DOCKER_REGISTRY ?=ghcr.io/radius-project/dev

# Auto-detect the local debug OCI registry started by `make debug-publish-recipes`.
# When it is running and the user has not explicitly set BICEP_RECIPE_REGISTRY,
# point the functional tests at it so the locally-published test recipes are used
# instead of the (private) ghcr.io fallback.
ifeq ($(origin BICEP_RECIPE_REGISTRY), undefined)
ifneq ($(shell docker ps --format '{{.Names}}' 2>/dev/null | grep -x radius-debug-registry),)
export BICEP_RECIPE_REGISTRY := localhost:5000
export BICEP_RECIPE_TAG_VERSION ?= latest
$(info Using local debug recipe registry: BICEP_RECIPE_REGISTRY=$(BICEP_RECIPE_REGISTRY) BICEP_RECIPE_TAG_VERSION=$(BICEP_RECIPE_TAG_VERSION))
# When the debug-built rad CLI is available, point the functional tests at it
# (via RAD_PATH, honored by test/radcli/cli.go) so they exercise the HEAD CLI
# matching the locally-running control plane, not any system-installed rad.
ifneq ($(wildcard $(CURDIR)/debug_files/bin/rad),)
export RAD_PATH := $(CURDIR)/debug_files/bin
$(info Using debug-built rad CLI: RAD_PATH=$(RAD_PATH))
endif
endif
endif

# Auto-detect the in-cluster Git HTTP backend started by `make debug-install-git-http-backend`.
# When its port-forward PID file exists and the process is alive, export the
# GIT_HTTP_* variables that the kubernetes-noncloud Flux tests expect.
ifeq ($(origin GIT_HTTP_SERVER_URL), undefined)
ifneq ($(wildcard $(CURDIR)/debug_files/logs/git-http-port-forward.pid),)
ifneq ($(shell pid=$$(cat $(CURDIR)/debug_files/logs/git-http-port-forward.pid 2>/dev/null); kill -0 $$pid 2>/dev/null && echo up),)
export GIT_HTTP_SERVER_URL := http://localhost:30080
export GIT_HTTP_USERNAME ?= testuser
export GIT_HTTP_PASSWORD ?= not-a-secret-password
export GIT_HTTP_EMAIL ?= testuser@radapp.io
$(info Using local git-http-backend: GIT_HTTP_SERVER_URL=$(GIT_HTTP_SERVER_URL))
endif
endif
endif
ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.30.*
ENV_SETUP=$(GOBIN)/setup-envtest$(BINARY_EXT)

# Use gotestsum if available, otherwise use go test. We want to enable testing with just 'make test'
# without external dependencies, but want to use gotestsum in our CI pipelines for the improved
# reporting.
#
# See: https://github.com/gotestyourself/gotestsum
#
# Gotestsum is a drop-in replacement for go test, but it provides a much nicer formatted output
# and it can also generate JUnit XML reports.
ifeq (, $(shell which gotestsum))
GOTEST_TOOL ?= go test
else
# Use these options by default but allow an override via env-var
GOTEST_OPTS ?=
# When set, a per-target JSON timing file is emitted as $(GOTESTSUM_JSONFILE_DIR)/<target>.jsonl.
# This avoids the file being overwritten by each sub-target in test-functional-all-*.
# Example: GOTESTSUM_JSONFILE_DIR=/tmp/timings make test-functional-all-noncloud
GOTESTSUM_JSONFILE_DIR ?=
# Recursive '=' so $@ resolves in each recipe's context.
# We need the double dash here to separate the 'gotestsum' options from the 'go test' options.
GOTEST_TOOL = gotestsum $(GOTESTSUM_OPTS)$(if $(GOTESTSUM_JSONFILE_DIR), --jsonfile=$(GOTESTSUM_JSONFILE_DIR)/$@.jsonl) --
endif

.PHONY: test
test: test-get-envtools test-helm ## Runs unit tests, excluding kubernetes controller tests
	KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" CGO_ENABLED=1 $(GOTEST_TOOL) ./pkg/... $(GOTEST_OPTS)

.PHONY: test-compile
test-compile: test-get-envtools ## Compiles all tests without running them
	@echo "$(ARROW) Compiling unit tests..."
	@KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" CGO_ENABLED=1 go test -c ./pkg/... -o /dev/null

.PHONY: test-get-envtools
test-get-envtools:
	@echo "$(ARROW) Installing Kubebuilder test tools..."
	$(call go-install-tool,$(ENV_SETUP),sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.20)
	@echo "$(ARROW) Instructions:"
	@echo "$(ARROW) Set environment variable KUBEBUILDER_ASSETS for tests."
	@echo "$(ARROW) KUBEBUILDER_ASSETS=\"$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)\""

.PHONY: test-validate-cli
test-validate-cli: ## Run cli integration tests
	CGO_ENABLED=1 $(GOTEST_TOOL) ./pkg/cli/cmd/... ./cmd/rad/... -timeout ${TEST_TIMEOUT} $(GOTEST_OPTS)

.PHONY: test-functional-all
test-functional-all: test-functional-ucp test-functional-kubernetes test-functional-corerp test-functional-cli test-functional-msgrp test-functional-daprrp test-functional-datastoresrp test-functional-samples test-functional-dynamicrp-noncloud ## Runs all functional tests

.PHONY: test-functional-all-noncloud
# Run all functional tests that do not require cloud resources
test-functional-all-noncloud: test-functional-ucp-noncloud test-functional-kubernetes-noncloud test-functional-corerp-noncloud test-functional-cli-noncloud test-functional-msgrp-noncloud test-functional-daprrp-noncloud test-functional-datastoresrp-noncloud test-functional-samples-noncloud test-functional-dynamicrp-noncloud ## Runs all functional tests that do not require cloud resources

.PHONY: test-functional-all-cloud
# Run all functional tests that require cloud resources
test-functional-all-cloud: test-functional-ucp-cloud test-functional-corerp-cloud

.PHONY: test-functional-ucp
test-functional-ucp: test-functional-ucp-noncloud test-functional-ucp-cloud ## Runs all UCP functional tests (both cloud and non-cloud)

.PHONY: test-functional-ucp-noncloud
test-functional-ucp-noncloud: ## Runs UCP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/ucp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

.PHONY: test-functional-ucp-cloud
test-functional-ucp-cloud: ## Runs UCP functional tests that require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/ucp/cloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

.PHONY: test-functional-kubernetes
test-functional-kubernetes: test-functional-kubernetes-noncloud ## Runs all Kubernetes functional tests
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/kubernetes/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

.PHONY: test-functional-kubernetes-noncloud
test-functional-kubernetes-noncloud: ## Runs Kubernetes functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/kubernetes/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

.PHONY: test-functional-corerp
test-functional-corerp: test-functional-corerp-noncloud test-functional-corerp-cloud ## Runs all Core RP functional tests (both cloud and non-cloud)

.PHONY: test-functional-corerp-noncloud
test-functional-corerp-noncloud: ## Runs corerp functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/corerp/noncloud/... -timeout ${TEST_TIMEOUT} -v -json -parallel 10 $(GOTEST_OPTS)

.PHONY: test-functional-corerp-cloud
test-functional-corerp-cloud: ## Runs corerp functional tests that require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/corerp/cloud/... -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

.PHONY: test-functional-msgrp
test-functional-msgrp: test-functional-msgrp-noncloud ## Runs all Messaging RP functional tests (both cloud and non-cloud)

.PHONY: test-functional-msgrp-noncloud
test-functional-msgrp-noncloud: ## Runs Messaging RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/messagingrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 2 $(GOTEST_OPTS)

.PHONY: test-functional-cli
test-functional-cli: test-functional-cli-noncloud ## Runs all cli functional tests (both cloud and non-cloud)

.PHONY: test-functional-cli-noncloud
test-functional-cli-noncloud: ## Runs cli functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/cli/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

.PHONY: test-functional-daprrp
test-functional-daprrp: test-functional-daprrp-noncloud ## Runs all Dapr RP functional tests (both cloud and non-cloud)

.PHONY: test-functional-daprrp-noncloud
test-functional-daprrp-noncloud: ## Runs Dapr RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/daprrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 3 $(GOTEST_OPTS)

.PHONY: test-functional-datastoresrp
test-functional-datastoresrp: test-functional-datastoresrp-noncloud ## Runs all Datastores RP functional tests (non-cloud)

.PHONY: test-functional-datastoresrp-noncloud
test-functional-datastoresrp-noncloud: ## Runs Datastores RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/datastoresrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 3 $(GOTEST_OPTS)

.PHONY: test-functional-dynamicrp-noncloud
test-functional-dynamicrp-noncloud: ## Runs Dynamic RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/dynamicrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 1 $(GOTEST_OPTS)

.PHONY: test-functional-upgrade
test-functional-upgrade: test-functional-upgrade-noncloud ## Runs all Upgrade functional tests

.PHONY: test-functional-upgrade-noncloud
test-functional-upgrade-noncloud: ## Runs Upgrade functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/upgrade/... -timeout ${TEST_TIMEOUT} -v -parallel 1 $(GOTEST_OPTS)
	
.PHONY: test-functional-samples
test-functional-samples: test-functional-samples-noncloud ## Runs all Samples functional tests

.PHONY: test-functional-samples-noncloud
test-functional-samples-noncloud: ## Runs Samples functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/samples/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

# ----------------------------------------------------------------------------
# Local Azure functional tests
#
# These targets orchestrate an ephemeral Azure resource group, deploy the test
# fixtures (Cosmos Mongo for Test_AzureConnections), run the Test_Azure* subset
# of corerp-cloud tests against your locally-running Radius stack (make
# debug-start) using ambient `az login` credentials, and tear everything down.
#
# Prerequisites:
#   - `az login` succeeded for the target subscription.
#   - `make debug-start` is running (OS-process Radius).
#   - Deployment Engine is running locally on :5017 (NOT in a container) so it
#     can use the az CLI fallback. See debug_files/logs/de-external.marker.
#
# NOTE: AWS is intentionally out of scope for this iteration.
# ----------------------------------------------------------------------------
AZURE_LOCAL_TESTENV := ./build/scripts/azure-local-testenv.sh

.PHONY: test-functional-azure-local-setup
test-functional-azure-local-setup: ## Provision an ephemeral Azure RG and fixtures for local Azure functional tests.
	@$(AZURE_LOCAL_TESTENV) setup

.PHONY: test-functional-azure-local-run
test-functional-azure-local-run: ## Run Test_Azure* against the locally-running Radius stack using the env from setup.
	@$(AZURE_LOCAL_TESTENV) run

.PHONY: test-functional-azure-local-teardown
test-functional-azure-local-teardown: ## Delete the ephemeral Azure RG and clear local Azure test state.
	@$(AZURE_LOCAL_TESTENV) teardown

.PHONY: test-functional-azure-local
test-functional-azure-local: ## Setup -> run Test_Azure* -> teardown (teardown runs even on test failure).
	@$(AZURE_LOCAL_TESTENV) all

.PHONY: test-functional-azure-local-keep
test-functional-azure-local-keep: ## Same as test-functional-azure-local but skips teardown on failure (post-mortem).
	@AZURE_LOCAL_KEEP_ON_FAILURE=1 $(AZURE_LOCAL_TESTENV) all

.PHONY: test-validate-bicep
test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin/bicep" ./build/validate-bicep.sh

.PHONY: test-helm
test-helm: ## Runs Helm chart unit tests
	@echo "$(ARROW) Installing helm-unittest plugin if not already installed..."
	@helm plugin list | grep -q unittest || helm plugin install https://github.com/helm-unittest/helm-unittest.git --version 1.0.2
	@echo "$(ARROW) Running Helm unit tests..."
	cd deploy/Chart && helm unittest .

# TODO re-enable https://github.com/radius-project/radius/issues/5091
.PHONY: test-ucp-spec-examples 
test-ucp-spec-examples: generate-tsp-installed ## Validates UCP examples conform to UCP OpenAPI Spec
	# @echo "$(ARROW) Testing x-ms-examples conform to ucp spec..."
	# pnpm -C typespec exec oav validate-example ../swagger/specification/ucp/resource-manager/UCP/preview/2023-10-01-preview/openapi.json

.PHONY: test-deploy-lrt-cluster
test-deploy-aks-cluster: ## Deploys an AKS cluster to Azure for the long-running tests. Optional parameters: [TEST_AKS_AZURE_LOCATION=<location>] [TEST_AKS_RG=<resource group name>]
	@bash ./build/test.sh
