# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

# Will be set by our build workflow, this is just a default
TEST_TIMEOUT ?=1h
RADIUS_CONTAINER_LOG_PATH ?=./dist/container_logs

.PHONY: test
test: ## Runs unit tests, excluding kubernetes controller tests
	CGO_ENABLED=1 go test -race ./pkg/...

.PHONY: test-functional-azure
test-functional-azure: ## Runs Azure functional tests
	CGO_ENABLED=1 go test -race ./test/functional/azure/... -timeout ${TEST_TIMEOUT} -v -parallel 20

test-functional-kubernetes: ## Runs Kubernetes functional tests
	CGO_ENABLED=1 go test -race ./test/functional/kubernetes/... -timeout ${TEST_TIMEOUT} -v -parallel 20

ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.19.2
ENV_SETUP=$(GOBIN)/setup-envtest

test-get-envtools:
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

test-controller: generate-k8s-manifests generate-controller test-get-envtools ## Runs controller tests, note arm64 version not available.
	KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" CGO_ENABLED=1 go test -race ./test/integration/kubernetes/...

test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin" ./build/validate-bicep.sh
