# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

.PHONY: generate
generate: generate-arm-json generate-radclient generate-go generate-bicep-types ## Generates all targets.

.PHONY: generate-arm-json
generate-arm-json: generate-jq-installed ## Generates ARM-JSON from our environment creation Bicep files
	@echo "$(ARROW) Updating ARM-JSON..."
	az bicep build --file deploy/rp-full.bicep
	jq 'del(.metadata, .resources[].properties.template.metadata)' deploy/rp-full.json  > deploy/rp-full.tmp && mv deploy/rp-full.tmp deploy/rp-full.json

.PHONY: generate-jq-installed
generate-jq-installed:
	@echo "$(ARROW) Detecting jq..."
	@which node > /dev/null || { echo "jq is a required dependency"; exit 1; }
	@echo "$(ARROW) OK"

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
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest --use=@autorest/go@4.0.0-preview.29 \
        --module-version=$(AUTOREST_MODULE_VERSION) \
        --input-file=swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/applications.json \
        --tag=2022-03-15-privatepreview \
        --go  \
        --gomod-root=. \
        --output-folder=./pkg/common/radclient \
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

.PHONY: generate-bicep-types
generate-bicep-types: generate-node-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
ifneq (, $(shell which autorest))
	@echo "$(ARROW) Remove outdated autorest extensions and download latest version of autorest-core..."
	autorest --reset
endif
	@echo "$(ARROW) Build autorest.bicep..."
	cd hack/bicep-types-radius/src/autorest.bicep; \
	npm ci && npm run build; \
	cd ../generator; \
	echo "Run generator from hack/bicep-types-radius/src/generator dir"; \
	npm ci && npm run generate -- --specs-dir ../../../../swagger

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
