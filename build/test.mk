# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

# Will be set by our build workflow, this is just a default
TEST_TIMEOUT ?=1h

.PHONY: test
test: ## Runs unit tests, excluding kubernetes controller tests
	go test ./pkg/...

.PHONY: test-functional-azure
test-functional-azure: ## Runs Azure functional tests
	go test ./test/functional/azure/... -timeout ${TEST_TIMEOUT} -v -parallel 20
	
test-controller: generate-k8s-manifests generate-controller controller-install## Runs controller tests, note arm64 version not available.
	go test ./test/controllertests/...

test-validate-bicep: ## Validates that all .bicep files compile cleanly
	BICEP_PATH="${HOME}/.rad/bin" ./build/validate-bicep.sh

test-controller-clean: generate-k8s-manifests generate-controller setup-kind controller-install test-controller ## Runs controller tests, note arm64 version not available.
	kind delete cluster --name radius-kind

setup-kind:
	kind create cluster --name radius-kind

delete-kind: 
	kind delete cluster --name radius-kind
	