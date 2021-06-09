# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

.PHONY: test
test: ## Runs unit tests.
	go test ./pkg/...

.PHONY: test-integration
test-integration: ## Runs integration tests
	go test ./test/integrationtests/... -timeout 1h -v

ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.19.2

test-controller: generate-k8s-manifests generate-controller ## Runs controller tests
	source deploy/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out
