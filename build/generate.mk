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

# generate-rad-corerp-client-2025-08-01-preview is a new target, which will replace generate-rad-corerp-client in future, once all resources of Radius.Core are ready and Applications.Core is deprecated.
.PHONY: generate
generate: generate-cleanup generate-rad-corerp-client generate-rad-corerp-client-2025-08-01-preview generate-rad-datastoresrp-client generate-rad-messagingrp-client generate-rad-daprrp-client generate-rad-ucp-client generate-genericcliclient generate-go generate-bicep-types generate-ucp-crd generate-controller ## Generates all targets.

.PHONY: generate-tsp-installed
generate-tsp-installed: generate-pnpm-installed
	@echo "$(ARROW) Detecting tsp..."
	@pnpm -C typespec exec tsp --help > /dev/null 2>&1 || { \
		echo "$(ARROW) TypeSpec not found. Installing TypeSpec dependencies..."; \
		pnpm -C typespec install --frozen-lockfile --config.confirm-modules-purge=false; \
	}
	@echo "$(ARROW) OK"

.PHONY: generate-pnpm-installed
generate-pnpm-installed: generate-node-installed
	@echo "$(ARROW) Setting up pnpm via corepack..."
	@corepack enable pnpm
	@corepack install
	@echo "$(ARROW) OK"

.PHONY: tsp-format-check
tsp-format-check: generate-tsp-installed ## Checks TypeSpec format
	@echo "$(ARROW) Checking TypeSpec format..."
	pnpm -C typespec exec tsp format --check "**/*.tsp"
	@echo "$(ARROW) OK"

.PHONY: generate-openapi-spec
generate-openapi-spec: # Generates all Radius OpenAPI specs from TypeSpec.
	@echo  "Generating openapi specs from typespec models."
	cd typespec/UCP && pnpm exec tsp compile .
	cd typespec/Applications.Core && pnpm exec tsp compile .
	cd typespec/Applications.Dapr && pnpm exec tsp compile .
	cd typespec/Applications.Messaging && pnpm exec tsp compile .
	cd typespec/Applications.Datastores && pnpm exec tsp compile .
	cd typespec/Radius.Core && pnpm exec tsp compile .

.PHONY: generate-node-installed
generate-node-installed:
	@echo "$(ARROW) Detecting node..."
	@which node > /dev/null || { echo "node is a required dependency"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-ucp-crd
generate-ucp-crd: ## Generates the CRDs for UCP APIServer store.
	@echo "$(ARROW) Generating CRDs for ucp.dev..."
	go tool controller-gen object:headerFile=./boilerplate.go.txt paths=./pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1/...
	go tool controller-gen crd paths=./pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1/... output:crd:dir=./deploy/Chart/crds/ucpd

.PHONY: generate-controller
generate-controller: ## Generates the CRDs for the Radius controller.
	@echo "$(ARROW) Generating CRDs for radapp.io..."
	go tool controller-gen object:headerFile=./boilerplate.go.txt paths=./pkg/controller/api/...
	go tool controller-gen crd paths=./pkg/controller/api/... output:crd:dir=./deploy/Chart/crds/radius

.PHONY: generate-cleanup
generate-cleanup: ## Deletes all generated code.
	@echo "$(ARROW) Deleting generated code..."
	find . -type f -name 'zz_*.go' ! -name 'zz_*.deepcopy.go' -delete
	@echo "$(ARROW) Done."

.PHONY: generate-rad-corerp-client
generate-rad-corerp-client: generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/corerp/api/v20231001preview'"
	cd typespec/Applications.Core && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/corerp/api/v20231001preview/zz_generated_*.go
	cp typespec/Applications.Core/.tsp-go-tmp/zz_generated_*.go pkg/corerp/api/v20231001preview/
	rm -rf typespec/Applications.Core/.tsp-go-tmp
	go fmt ./pkg/corerp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-corerp-client-2025-08-01-preview
generate-rad-corerp-client-2025-08-01-preview: generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK for 2025-08-01-preview (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/corerp/api/v20250801preview'"
	cd typespec/Radius.Core && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/corerp/api/v20250801preview/zz_generated_*.go
	rm -f pkg/corerp/api/v20250801preview/fake/zz_generated_*.go
	cp typespec/Radius.Core/.tsp-go-tmp/zz_generated_*.go pkg/corerp/api/v20250801preview/
	cp typespec/Radius.Core/.tsp-go-tmp/fake/zz_generated_*.go pkg/corerp/api/v20250801preview/fake/
	rm -rf typespec/Radius.Core/.tsp-go-tmp
	go fmt ./pkg/corerp/api/v20250801preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-datastoresrp-client
generate-rad-datastoresrp-client: generate-tsp-installed generate-openapi-spec ## Generates the datastoresrp client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/datastoresrp/api/v20231001preview'"
	cd typespec/Applications.Datastores && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/datastoresrp/api/v20231001preview/zz_generated_*.go
	cp typespec/Applications.Datastores/.tsp-go-tmp/zz_generated_*.go pkg/datastoresrp/api/v20231001preview/
	rm -rf typespec/Applications.Datastores/.tsp-go-tmp
	go fmt ./pkg/datastoresrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-messagingrp-client
generate-rad-messagingrp-client: generate-tsp-installed generate-openapi-spec ## Generates the messagingrp client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/messagingrp/api/v20231001preview'"
	cd typespec/Applications.Messaging && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/messagingrp/api/v20231001preview/zz_generated_*.go
	cp typespec/Applications.Messaging/.tsp-go-tmp/zz_generated_*.go pkg/messagingrp/api/v20231001preview/
	rm -rf typespec/Applications.Messaging/.tsp-go-tmp
	go fmt ./pkg/messagingrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-daprrp-client
generate-rad-daprrp-client: generate-tsp-installed generate-openapi-spec ## Generates the daprrp client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/daprrp/api/v20231001preview'"
	cd typespec/Applications.Dapr && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/daprrp/api/v20231001preview/zz_generated_*.go
	cp typespec/Applications.Dapr/.tsp-go-tmp/zz_generated_*.go pkg/daprrp/api/v20231001preview/
	rm -rf typespec/Applications.Dapr/.tsp-go-tmp
	go fmt ./pkg/daprrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-ucp-client
generate-rad-ucp-client: generate-tsp-installed test-ucp-spec-examples ## Generates the UCP client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/ucp/api/v20231001preview'"
	cd typespec/UCP && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/ucp/api/v20231001preview/zz_generated_*.go
	rm -f pkg/ucp/api/v20231001preview/fake/zz_generated_*.go
	cp typespec/UCP/.tsp-go-tmp/zz_generated_*.go pkg/ucp/api/v20231001preview/
	cp typespec/UCP/.tsp-go-tmp/fake/zz_generated_*.go pkg/ucp/api/v20231001preview/fake/
	rm -rf typespec/UCP/.tsp-go-tmp
	go fmt ./pkg/ucp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-genericcliclient
generate-genericcliclient: generate-tsp-installed ## Generates the generic CLI client SDK (TypeSpec Go emitter).
	@echo "$(ARROW) Generating 'pkg/cli/clients_new/generated'"
	cd typespec/GenericResource && pnpm exec tsp compile . --emit=@azure-tools/typespec-go
	rm -f pkg/cli/clients_new/generated/zz_generated_*.go
	rm -f pkg/cli/clients_new/generated/fake/zz_generated_*.go
	cp typespec/GenericResource/.tsp-go-tmp/zz_generated_*.go pkg/cli/clients_new/generated/
	cp typespec/GenericResource/.tsp-go-tmp/fake/zz_generated_*.go pkg/cli/clients_new/generated/fake/
	rm -rf typespec/GenericResource/.tsp-go-tmp
	go fmt ./pkg/cli/clients_new/generated/...
	@echo "$(ARROW) Done."

.PHONY: generate-go
generate-go: ## Generates go with 'go generate' (Mocks).
	@echo "$(ARROW) Running go generate..."
	go generate -v ./...

# Generates Bicep types.json files for the default contrib resource type
# namespaces listed in deploy/manifest/defaults.yaml.
#
# Each entry in defaults.yaml uses <namespace>/<typeName> format
# (e.g. Radius.Compute/containers). This target:
#   1. Reads defaults.yaml to discover which types to include.
#   2. Groups entries by namespace.
#   3. For each namespace, passes ALL per-type manifest files to
#      `generate` so they are merged into a single output.
#
# Per-type manifest files live under deploy/manifest/built-in-providers/self-hosted/
# as individual YAML files (e.g. containers.yaml, routes.yaml).
DEFAULTS_YAML := deploy/manifest/defaults.yaml
BICEP_TYPES_CONTRIB_API_VERSION ?= 2025-08-01-preview
BICEP_TYPES_OUTPUT_BASE := hack/bicep-types-radius/generated/radius
BICEP_TYPES_CONTRIB_MANIFEST_DIR := deploy/manifest/built-in-providers/self-hosted

.PHONY: generate-bicep-types
generate-bicep-types: ## Generate Bicep extensibility types
	@$(MAKE) generate-bicep-types-core
	@$(MAKE) generate-bicep-types-contrib
	@$(MAKE) rebuild-bicep-types-index

.PHONY: generate-bicep-types-core
generate-bicep-types-core: generate-node-installed generate-pnpm-installed ## Generate Bicep extensibility types from OpenAPI specs.
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Build autorest.bicep..."
	CI=true pnpm -C hack/bicep-types-radius/src/autorest.bicep install && pnpm -C hack/bicep-types-radius/src/autorest.bicep run build; \
	echo "Run generator from hack/bicep-types-radius/src/generator dir"; \
	CI=true pnpm -C hack/bicep-types-radius/src/generator install && pnpm -C hack/bicep-types-radius/src/generator run generate --specs-dir ../../../../swagger --release-version ${VERSION} --verbose

.PHONY: generate-yq-installed
generate-yq-installed:
	@echo "$(ARROW) Detecting yq..."
	@which yq > /dev/null || { echo "yq not found. Run 'make install-yq' to install the pinned version into a user-owned bin dir."; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-bicep-types-contrib
generate-bicep-types-contrib: generate-yq-installed ## Generates Bicep types.json files for default contrib namespaces from defaults.yaml.
	@echo "$(ARROW) Generating Bicep types for default contrib resource type namespaces..."
	build/scripts/generate-bicep-types-contrib.sh \
		"$(DEFAULTS_YAML)" \
		"$(BICEP_TYPES_CONTRIB_MANIFEST_DIR)" \
		"$(BICEP_TYPES_OUTPUT_BASE)" \
		"$(BICEP_TYPES_CONTRIB_API_VERSION)"

.PHONY: rebuild-bicep-types-index
rebuild-bicep-types-index:
	@echo "$(ARROW) Rebuilding unified Bicep types index..."
	CI=true pnpm -C hack/bicep-types-radius/src/generator run rebuild-index --release-version ${VERSION}

# Publishing the unified `radius` Bicep extension. Runnable locally against any
# OCI registry (e.g. a local Zot/CRane-backed registry, or biceptypes.azurecr.io
# after `az acr login`). Both BICEP_PUBLISH_TARGET and the local Bicep CLI must
# be available. CI workflows (added separately) call this target after
# generating types and authenticating to the registry.
#
# Example:
#   make publish-bicep-extension BICEP_PUBLISH_TARGET=br:biceptypes.azurecr.io/radius:latest
BICEP_PUBLISH_INDEX := $(BICEP_TYPES_OUTPUT_BASE)/../index.json
BICEP_PUBLISH_TARGET ?=

.PHONY: publish-bicep-extension
publish-bicep-extension: ## Publish the unified `radius` Bicep extension to BICEP_PUBLISH_TARGET. Requires generate-bicep-types to have been run.
	@if [ -z "$(BICEP_PUBLISH_TARGET)" ]; then \
		echo "ERROR: BICEP_PUBLISH_TARGET must be set (e.g. br:biceptypes.azurecr.io/radius:latest)"; \
		exit 1; \
	fi
	@if [ ! -f "$(BICEP_PUBLISH_INDEX)" ]; then \
		echo "ERROR: $(BICEP_PUBLISH_INDEX) does not exist; run 'make generate-bicep-types' first"; \
		exit 1; \
	fi
	@which bicep > /dev/null || { echo "ERROR: 'bicep' CLI not found in PATH"; exit 1; }
	@echo "$(ARROW) Publishing Bicep extension index $(BICEP_PUBLISH_INDEX) -> $(BICEP_PUBLISH_TARGET)"
	bicep publish-extension "$(BICEP_PUBLISH_INDEX)" --target "$(BICEP_PUBLISH_TARGET)" --force

# NOTE: The Azure Container Instances SDK in pkg/sdk/v20241101preview is now
# maintained by hand. It targets the 2024-11-01-preview NGroups/CGProfile APIs
# which are not published in github.com/Azure/azure-sdk-for-go and have no
# TypeSpec source, so there is no code-generation step for it anymore. Apply any
# API changes directly to the Go sources under pkg/sdk/v20241101preview.

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
