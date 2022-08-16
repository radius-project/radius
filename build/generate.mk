# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

.PHONY: generate
generate: generate-radclient generate-genericcliclient generate-rad-corerp-client generate-rad-connectorrp-client generate-go generate-bicep-types generate-ucp-crd ## Generates all targets.

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

.PHONY: generate-controller-gen-installed
generate-controller-gen-installed:
	@echo "$(ARROW) Detecting controller-gen..."
	@which controller-gen > /dev/null || { echo "run 'go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.1'"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-ucp-crd
generate-ucp-crd: generate-controller-gen-installed
	@echo "$(ARROW) Generating CRDs for ucp.dev..."
	controller-gen object paths=./pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1/... object:headerFile=./boilerplate.go.txt
	controller-gen rbac:roleName=manager-role crd paths=./pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1/... output:crd:dir=./deploy/Chart/crds/ucpd

.PHONY: generate-radclient
generate-radclient: generate-node-installed generate-autorest-installed ## Generates the radclient SDK (Autorest).
	autorest --use=@autorest/go@4.0.0-preview.29 \
        --module-version=$(AUTOREST_MODULE_VERSION) \
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

.PHONY: generate-genericcliclient
generate-genericcliclient: generate-node-installed generate-autorest-installed
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/cli/clients_new/README.md --tag=2022-03-15-privatepreview

.PHONY: generate-rad-corerp-client
generate-rad-corerp-client: generate-node-installed generate-autorest-installed ## Generates the radclient SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/corerp/api/README.md --tag=2022-03-15-privatepreview

.PHONY: generate-rad-connectorrp-client
generate-rad-connectorrp-client: generate-node-installed generate-autorest-installed ## Generates the connectorrp client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/connectorrp/api/README.md --tag=connector-2022-03-15-privatepreview

.PHONY: generate-mockgen-installed
generate-mockgen-installed:
	@echo "$(ARROW) Detecting mockgen..."
	@which mockgen > /dev/null || { echo "run 'go install github.com/golang/mock/mockgen@v1.5.0' to install mockgen"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-go
generate-go: generate-mockgen-installed ## Generates go with 'go generate' (Mocks).
	@echo "$(ARROW) Running go generate..."
	go generate -v ./...

.PHONY: generate-bicep-types
generate-bicep-types: generate-node-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Build autorest.bicep..."
	cd hack/bicep-types-radius/src/autorest.bicep; \
	npm ci && npm run build; \
	cd ../generator; \
	echo "Run generator from hack/bicep-types-radius/src/generator dir"; \
	npm ci && npm run generate -- --specs-dir ../../../../swagger --verbose

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))/..
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get -u $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
