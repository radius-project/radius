# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Test

.PHONY: test
test: ## Runs unit tests, excluding kubernetes controller tests
	go test ./pkg/...

.PHONY: test-integration
test-integration: ## Runs integration tests
	go test ./test/integrationtests/... -timeout 1h -v

ENVTEST_ASSETS_DIR=$(shell pwd)/bin
K8S_VERSION=1.19.2
ENV_SETUP=$(GOBIN)/setup-envtest

test-get-envtools:
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	
## KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT="true" to enable logs from env setup pieces
test-controller: generate-k8s-manifests generate-controller test-get-envtools ## Runs controller tests, note arm64 version not available.
	KUBEBUILDER_ASSETS="$(shell $(ENV_SETUP) use -p path ${K8S_VERSION} --arch amd64)" go test ./test/controllertests/...  
