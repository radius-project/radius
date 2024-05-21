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

# Will be set by our build workflow, this is just a default
TEST_TIMEOUT ?=1h
RADIUS_CONTAINER_LOG_PATH ?=./dist/container_logs
REL_VERSION ?=latest
DOCKER_REGISTRY ?=ghcr.io/radius-project/dev
ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.23.*
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
# We need the double dash here to separate the 'gotestsum' options from the 'go test' options
GOTEST_TOOL ?= gotestsum $(GOTESTSUM_OPTS) --
endif

.PHONY: test
test: test-get-envtools ## Runs unit tests, excluding kubernetes controller tests
	KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" CGO_ENABLED=1 $(GOTEST_TOOL) -v ./pkg/... $(GOTEST_OPTS)

.PHONY: test-get-envtools
test-get-envtools:
	@echo "$(ARROW) Installing Kubebuilder test tools..."
	$(call go-install-tool,$(ENV_SETUP),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)
	@echo "$(ARROW) Instructions:"
	@echo "$(ARROW) Set environment variable KUBEBUILDER_ASSETS for tests."
	@echo "$(ARROW) KUBEBUILDER_ASSETS=\"$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)\""

.PHONY: test-validate-cli
test-validate-cli: ## Run cli integration tests
	CGO_ENABLED=1 $(GOTEST_TOOL) -coverpkg= ./pkg/cli/cmd/... ./cmd/rad/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-all: test-functional-ucp test-functional-kubernetes test-functional-shared test-functional-cli test-functional-msgrp test-functional-daprrp test-functional-datastoresrp test-functional-samples ## Runs all functional tests

test-functional-ucp: test-functional-ucp-noncloud test-functional-ucp-cloud ## Runs all UCP functional tests (both cloud and non-cloud)

test-functional-ucp-noncloud: ## Runs UCP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/ucp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-ucp-cloud: ## Runs UCP functional tests that require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/ucp/cloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-kubernetes: test-functional-kubernetes-noncloud test-functional-kubernetes-cloud ## Runs all Kubernetes functional tests
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/kubernetes/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-kubernetes-noncloud: ## Runs Kubernetes functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/kubernetes/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-kubernetes-cloud: ## Runs Kubernetes functional tests that require cloud resources

test-functional-shared: test-functional-corerp-noncloud test-functional-corerp-cloud ## Runs all Core RP functional tests (both cloud and non-cloud)

test-functional-corerp-noncloud: ## Runs corerp functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/corerp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

test-functional-corerp-cloud: ## Runs corerp functional tests that require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/corerp/cloud/... -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

test-functional-msgrp: test-functional-msgrp-noncloud test-functional-msgrp-cloud ## Runs all Messaging RP functional tests (both cloud and non-cloud)

test-functional-msgrp-noncloud: ## Runs Messaging RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/messagingrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 2 $(GOTEST_OPTS)

test-functional-msgrp-cloud: ## Runs msgrp functional tests that require cloud resources

test-functional-cli: test-functional-cli-noncloud test-functional-cli-cloud ## Runs all cli functional tests (both cloud and non-cloud)

test-functional-cli-noncloud: ## Runs cli functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/cli/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

test-functional-cli-cloud: ## Runs cli functional tests that require cloud resources

test-functional-daprrp: test-functional-daprrp-noncloud test-functional-daprrp-cloud ## Runs all Dapr RP functional tests (both cloud and non-cloud)

test-functional-daprrp-noncloud: ## Runs Dapr RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/daprrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 3 $(GOTEST_OPTS)

test-functional-daprrp-cloud: ## Runs daprrp functional tests that require cloud resources

test-functional-datastoresrp: test-functional-datastoresrp-noncloud test-functional-datastoresrp-cloud ## Runs all Datastores RP functional tests (non-cloud and cloud)

test-functional-datastoresrp-noncloud: ## Runs Datastores RP functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/datastoresrp/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 3 $(GOTEST_OPTS)

test-functional-datastoresrp-cloud: ## Runs Datastores RP functional tests that require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/datastoresrp/cloud/... -timeout ${TEST_TIMEOUT} -v -parallel 3 $(GOTEST_OPTS)

test-functional-samples: test-functional-samples-noncloud test-functional-samples-cloud ## Runs all Samples functional tests

test-functional-samples-noncloud: ## Runs Samples functional tests that do not require cloud resources
	CGO_ENABLED=1 $(GOTEST_TOOL) ./test/functional-portable/samples/noncloud/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-samples-cloud: ## Runs samples functional tests that require cloud resources

test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin/rad-bicep" ./build/validate-bicep.sh

.PHONY: oav-installed
oav-installed:
	@echo "$(ARROW) Detecting oav (https://github.com/Azure/oav)..."
	@which oav > /dev/null || { echo "run 'npm install -g oav' to install oav"; exit 1; }
	@echo "$(ARROW) OK"

# TODO re-enable https://github.com/radius-project/radius/issues/5091
.PHONY: test-ucp-spec-examples 
test-ucp-spec-examples: oav-installed ## Validates UCP examples conform to UCP OpenAPI Spec
	# @echo "$(ARROW) Testing x-ms-examples conform to ucp spec..."
	# oav validate-example swagger/specification/ucp/resource-manager/UCP/preview/2023-10-01-preview/openapi.json


