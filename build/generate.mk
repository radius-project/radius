# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

.PHONY: generate
generate: generate-rp-manifest generate-radclient generate-go generate-k8s-manifests generate-controller ## Generates all targets.

.PHONY: generate-rp-manifest
generate-rp-manifest: ## Generates Custom RP manifest that registers our resource types.
	@echo "$(ARROW) Updating manifest..."
	go run cmd/rp-manifest-gen/main.go \
		--input deploy/rp-full.input.json \
		--output deploy/rp-full.json \
		--resources pkg/radrp/schemav3/resource-types.json

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

.PHONY: generate-openapi-specs
generate-openapi-specs:
	@echo "$(ARROW) Generating OpenAPI schema manifest..."

	go run cmd/autorest-schema-gen/main.go \
		--output schemas/rest-api-specs/radius.json \
		`# We can't just do pkg/radrp/schema/*.json because we want to exclude resource-types.json` \
		pkg/radrp/schema/common-types.json \
		pkg/radrp/schema/application.json \
		pkg/radrp/schema/traits.json \
		pkg/radrp/schema/*/*.json

.PHONY: generate-radclient
generate-radclient: generate-node-installed generate-autorest-installed generate-openapi-specs ## Generates the radclient SDK (Autorest).
	autorest --use=@autorest/go@4.0.0-preview.22 \
		--input-file=schemas/rest-api-specs/radius.json \
		--tag=package-2018-09-01-preview \
		--go  \
		--gomod-root=. \
		--output-folder=./pkg/azure/radclient \
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
		rbac:roleName=radius-manager-role \
		paths="./..." \
		output:crd:artifacts:config=deploy/Chart/crds/ \
		output:rbac:artifacts:config=deploy/Chart/templates/ \

generate-controller: generate-controller-gen-installed ## Generate controller code
	$(CONTROLLER_GEN) object:headerFile="boilerplate.go.txt" paths="./..."
