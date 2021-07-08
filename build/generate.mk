# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

.PHONY: generate
generate: generate-radclient generate-go generate-k8s-manifests generate-controller ## Generates all targets.

.PHONY: generate-node-installed
generate-node-installed:
	@echo "$(ARROW) Detecting node..."
	@which node > /dev/null || { echo "node is a required dependency"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-autorest-installed
generate-autorest-installed:
	@echo "$(ARROW) Detecting autorest..."
	@which autorest > /dev/null || { echo "run 'npm install -g autorest' to install autorest"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-radclient
generate-radclient: generate-node-installed generate-autorest-installed ## Generates the radclient SDK (Autorest).
	autorest --use=@autorest/go@4.0.0-preview.22 \
		schemas/rest-api-specs/readme.md \
		--tag=package-2018-09-01-preview \
		--go  \
		--gomod-root=. \
		--output-folder=./pkg/radclient \
		--modelerfour.lenient-model-deduplication \
		--license-header=MICROSOFT_MIT_NO_VERSION \
		--file-prefix=zz_generated_ \
		--azure-arm \
		--verbose

.PHONY: generate-mockgen-installed
generate-mockgen-installed:
	@echo "$(ARROW) Detecting mockgen..."
	@which mockgen > /dev/null || { echo "run 'go install github.com/golang/mock/mockgen@v1.5.0' to install mockgen"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-go
generate-go: generate-mockgen-installed ## Generates go with 'go generate' (Mocks).
	@echo "$(ARROW) Running go generate..."
	go generate -v ./...

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
generate-controller-gen-installed:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
generate-kustomize-installed:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))/..
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
generate-k8s-manifests: generate-controller-gen-installed ## Generate Kubernetes deployment manifests
	$(CONTROLLER_GEN) $(CRD_OPTIONS) \
		rbac:roleName=manager-role webhook \
		paths="./..." \
		output:crd:artifacts:config=deploy/k8s/config/crd/bases \
		output:rbac:artifacts:config=deploy/k8s/config/rbac \
		output:webhook:artifacts:config=deploy/k8s/config/webhook

generate-controller: generate-controller-gen-installed ## Generate controller code
	$(CONTROLLER_GEN) object:headerFile="boilerplate.go.txt" paths="./..."

generate-baked-manifests: generate-k8s-manifests generate-kustomize-installed
	cd deploy/k8s/config/manager && $(KUSTOMIZE) edit set image controller=$(DOCKER_REGISTRY)/radius-controller:$(DOCKER_TAG_VERSION)
	$(KUSTOMIZE) build deploy/k8s/config/default > cmd/cli/cmd/radius-k8s.yaml
