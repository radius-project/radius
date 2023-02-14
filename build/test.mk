# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

# Will be set by our build workflow, this is just a default
TEST_TIMEOUT ?=1h
RADIUS_CONTAINER_LOG_PATH ?=./dist/container_logs
REL_VERSION ?=latest
DOCKER_REGISTRY ?=radiusdev.azurecr.io
ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.23.*
ENV_SETUP=$(GOBIN)/setup-envtest$(BINARY_EXT)

.PHONY: test
test: test-get-envtools ## Runs unit tests, excluding kubernetes controller tests
	KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" CGO_ENABLED=1 go test -v ./pkg/... $(GOTEST_OPTS)

.PHONY: test-get-envtools
test-get-envtools:
	$(call go-install-tool,$(ENV_SETUP),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

.PHONY: test-validate-cli
test-validate-cli: ## Run cli integration tests
	CGO_ENABLED=1 go test -coverpkg= ./pkg/cli/cmd/... ./cmd/rad/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-kubernetes: ## Runs Kubernetes functional tests
	CGO_ENABLED=1 go test ./test/functional/kubernetes/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-corerp: ## Runs Applications.Core functional tests
	CGO_ENABLED=1 go test ./test/functional/corerp/resources/dapr_statestore_test.go -timeout ${TEST_TIMEOUT} -v -parallel 10 $(GOTEST_OPTS)

test-functional-samples: ## Runs Samples functional tests
	CGO_ENABLED=1 go test ./test/functional/samples/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-ucp: ## Runs UCP functional tests
	CGO_ENABLED=1 go test ./test/functional/ucp/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin" ./build/validate-bicep.sh

.PHONY: oav-installed
oav-installed:
	@echo "$(ARROW) Detecting oav (https://github.com/Azure/oav)..."
	@which oav > /dev/null || { echo "run 'npm install -g oav' to install oav"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: test-ucp-spec-examples 
test-ucp-spec-examples: oav-installed ## Validates UCP examples conform to UCP OpenAPI Spec
	@echo "$(ARROW) Testing x-ms-examples conform to ucp spec..."
	oav validate-example swagger/specification/ucp/resource-manager/UCP/preview/2022-09-01-privatepreview/ucp.json


