# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

##@ Generate (Code and Schema Generation)

GOOS ?= $(shell go env GOOS)
CONTROLLER_TOOLS_VERSION ?= v0.12.0

ifeq ($(GOOS),windows)
   CMD_EXT = .cmd
endif

.PHONY: generate
generate: generate-genericcliclient generate-rad-corerp-client generate-rad-corerp-client-2025-08-01-preview generate-rad-datastoresrp-client generate-rad-messagingrp-client generate-rad-daprrp-client generate-rad-ucp-client generate-go generate-bicep-types generate-ucp-crd generate-controller ## Generates all targets.

.PHONY: generate-tsp-installed
generate-tsp-installed:
	@echo "$(ARROW) Detecting tsp..."
	cd typespec/ && npx$(CMD_EXT) -q -y tsp --help > /dev/null || { echo "run 'npm ci' in typespec directory."; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-openapi-spec
generate-openapi-spec: # Generates all Radius OpenAPI specs from TypeSpec.
	@echo  "Generating openapi specs from typespec models."
	cd typespec/UCP && npx$(CMD_EXT) tsp compile . 
	cd typespec/Applications.Core && npx$(CMD_EXT) tsp compile .
	cd typespec/Applications.Dapr && npx$(CMD_EXT) tsp compile .
	cd typespec/Applications.Messaging && npx$(CMD_EXT) tsp compile .
	cd typespec/Applications.Datastores && npx$(CMD_EXT) tsp compile .
	cd typespec/Radius.Core && npx$(CMD_EXT) tsp compile .

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
	@which controller-gen > /dev/null || { echo "run 'go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0'"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-ucp-crd
generate-ucp-crd: generate-controller-gen-installed ## Generates the CRDs for UCP APIServer store.
	@echo "$(ARROW) Generating CRDs for ucp.dev..."
	controller-gen object:headerFile=./boilerplate.go.txt paths=./pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1/...
	controller-gen crd paths=./pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1/... output:crd:dir=./deploy/Chart/crds/ucpd

.PHONY: generate-controller
generate-controller: generate-controller-gen-installed ## Generates the CRDs for the Radius controller.
	@echo "$(ARROW) Generating CRDs for radapp.io..."
	controller-gen object:headerFile=./boilerplate.go.txt paths=./pkg/controller/api/...
	controller-gen crd paths=./pkg/controller/api/... output:crd:dir=./deploy/Chart/crds/radius

.PHONY: generate-genericcliclient
generate-genericcliclient: generate-node-installed generate-autorest-installed
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/cli/clients_new/README.md --tag=2023-10-01-preview

.PHONY: generate-rad-corerp-client
generate-rad-corerp-client: generate-node-installed generate-autorest-installed generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/corerp/api/README.md --tag=core-2023-10-01-preview && rm pkg/corerp/api/v20231001preview/go.mod

.PHONY: generate-rad-corerp-client-2025-08-01-preview
generate-rad-corerp-client-2025-08-01-preview: generate-node-installed generate-autorest-installed generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK for 2025-08-01-preview (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/corerp/api/README.md --tag=core-2025-08-01-preview && rm pkg/corerp/api/v20250801preview/go.mod

.PHONY: generate-rad-datastoresrp-client
generate-rad-datastoresrp-client: generate-node-installed generate-autorest-installed generate-tsp-installed generate-openapi-spec ## Generates the datastoresrp client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/datastoresrp/api/README.md --tag=datastores-2023-10-01-preview && rm pkg/datastoresrp/api/v20231001preview/go.mod

.PHONY: generate-rad-messagingrp-client
generate-rad-messagingrp-client: generate-node-installed generate-autorest-installed generate-tsp-installed generate-openapi-spec ## Generates the messagingrp client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/messagingrp/api/README.md --tag=messaging-2023-10-01-preview && rm pkg/messagingrp/api/v20231001preview/go.mod

.PHONY: generate-rad-daprrp-client
generate-rad-daprrp-client: generate-node-installed generate-autorest-installed generate-tsp-installed generate-openapi-spec ## Generates the daprrp client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/daprrp/api/README.md --tag=dapr-2023-10-01-preview && rm pkg/daprrp/api/v20231001preview/go.mod

.PHONY: generate-rad-ucp-client
generate-rad-ucp-client: generate-node-installed generate-autorest-installed test-ucp-spec-examples ## Generates the UCP client SDK (Autorest).
	@echo "$(AUTOREST_MODULE_VERSION) is module version"
	autorest pkg/ucp/api/README.md --tag=ucp-2023-10-01-preview && rm pkg/ucp/api/v20231001preview/go.mod

.PHONY: generate-mockgen-installed
generate-mockgen-installed:
	@echo "$(ARROW) Detecting mockgen..."
	@which mockgen > /dev/null || { echo "run 'go install go.uber.org/mock/mockgen@v0.4.0' to install mockgen"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-go
generate-go: generate-mockgen-installed ## Generates go with 'go generate' (Mocks).
	@echo "$(ARROW) Running go generate..."
	go generate -v ./...

.PHONY: generate-bicep-types
generate-bicep-types: generate-node-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Build autorest.bicep..."
	git submodule update --init --recursive; \
	npm --prefix bicep-types/src/bicep-types install; \
	npm --prefix bicep-types/src/bicep-types ci && npm --prefix bicep-types/src/bicep-types run build; \
	npm --prefix hack/bicep-types-radius/src/autorest.bicep ci && npm --prefix hack/bicep-types-radius/src/autorest.bicep run build; \
	echo "Run generator from hack/bicep-types-radius/src/generator dir"; \
	npm --prefix hack/bicep-types-radius/src/generator ci && npm --prefix hack/bicep-types-radius/src/generator run generate -- --specs-dir ../../../../swagger --release-version ${VERSION} --verbose


.PHONY: generate-containerinstance-client
generate-containerinstance-client: generate-node-installed generate-autorest-installed  ## Generates the Container Instances SDK (Autorest).
	autorest \
		pkg/sdk/aci-specification/containerinstance/resource-manager/readme.md \
		--go \
		--tag=package-preview-2024-11

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
