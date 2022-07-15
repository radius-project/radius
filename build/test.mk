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

.PHONY: test-functional-azure
test-functional-azure: ## Runs Azure functional tests
	CGO_ENABLED=1 go test ./test/functional/azure/... -timeout ${TEST_TIMEOUT} -v -parallel 20 $(GOTEST_OPTS)

test-functional-localdev: ## Runs Local Dev functional tests
	CGO_ENABLED=1 go test ./test/functional/localdev/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-kubernetes: ## Runs Kubernetes functional tests
	CGO_ENABLED=1 go test ./test/functional/kubernetes/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-corerp: ## Runs Applications.Core functional tests
	CGO_ENABLED=1 go test ./test/functional/corerp/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-functional-ucp: ## Runs UCP functional tests
	CGO_ENABLED=1 go test ./test/functional/ucp/... -timeout ${TEST_TIMEOUT} -v -parallel 5 $(GOTEST_OPTS)

test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin" ./build/validate-bicep.sh
