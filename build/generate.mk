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
generate: generate-cleanup generate-genericcliclient generate-rad-corerp-client generate-rad-corerp-client-2025-08-01-preview generate-rad-datastoresrp-client generate-rad-messagingrp-client generate-rad-daprrp-client generate-rad-ucp-client generate-go generate-bicep-types generate-ucp-crd generate-controller ## Generates all targets.

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

.PHONY: generate-controller-gen-installed
generate-controller-gen-installed:
	@echo "$(ARROW) Detecting controller-gen..."
	@which controller-gen > /dev/null || { echo "run 'go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0'"; exit 1; }
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

.PHONY: generate-cleanup
generate-cleanup: ## Deletes all generated code.
	@echo "$(ARROW) Deleting generated code..."
	find . -type f -name 'zz_*.go' ! -name 'zz_*.deepcopy.go' -delete
	@echo "$(ARROW) Done."

.PHONY: generate-genericcliclient
generate-genericcliclient: generate-tsp-installed
	@echo "$(ARROW) Generating 'pkg/cli/clients_new/generated'"
	pnpm -C typespec exec autorest ../pkg/cli/clients_new/README.md --tag=2023-10-01-preview && rm pkg/cli/clients_new/generated/go.mod && go fmt ./pkg/cli/clients_new/generated/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-corerp-client
generate-rad-corerp-client: generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK (Autorest).
	@echo "$(ARROW) Generating 'pkg/corerp/api/v20231001preview'"
	pnpm -C typespec exec autorest ../pkg/corerp/api/README.md --tag=core-2023-10-01-preview && rm pkg/corerp/api/v20231001preview/go.mod && go fmt ./pkg/corerp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-corerp-client-2025-08-01-preview
generate-rad-corerp-client-2025-08-01-preview: generate-tsp-installed generate-openapi-spec ## Generates the corerp client SDK for 2025-08-01-preview (Autorest).
	@echo "$(ARROW) Generating 'pkg/corerp/api/v20250801preview'"
	pnpm -C typespec exec autorest ../pkg/corerp/api/README.md --tag=core-2025-08-01-preview && rm pkg/corerp/api/v20250801preview/go.mod && go fmt ./pkg/corerp/api/v20250801preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-datastoresrp-client
generate-rad-datastoresrp-client: generate-tsp-installed generate-openapi-spec ## Generates the datastoresrp client SDK (Autorest).
	@echo "$(ARROW) Generating 'pkg/datastoresrp/api/v20231001preview'"
	pnpm -C typespec exec autorest ../pkg/datastoresrp/api/README.md --tag=datastores-2023-10-01-preview && rm pkg/datastoresrp/api/v20231001preview/go.mod && go fmt ./pkg/datastoresrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-messagingrp-client
generate-rad-messagingrp-client: generate-tsp-installed generate-openapi-spec ## Generates the messagingrp client SDK (Autorest).
	@echo "$(ARROW) Generating 'pkg/messagingrp/api/v20231001preview'"
	pnpm -C typespec exec autorest ../pkg/messagingrp/api/README.md --tag=messaging-2023-10-01-preview && rm pkg/messagingrp/api/v20231001preview/go.mod && go fmt ./pkg/messagingrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-daprrp-client
generate-rad-daprrp-client: generate-tsp-installed generate-openapi-spec ## Generates the daprrp client SDK (Autorest).
	@echo "$(ARROW) Generating 'pkg/daprrp/api/v20231001preview'"
	pnpm -C typespec exec autorest ../pkg/daprrp/api/README.md --tag=dapr-2023-10-01-preview && rm pkg/daprrp/api/v20231001preview/go.mod && go fmt ./pkg/daprrp/api/v20231001preview/...
	@echo "$(ARROW) Done."

.PHONY: generate-rad-ucp-client
generate-rad-ucp-client: generate-tsp-installed test-ucp-spec-examples ## Generates the UCP client SDK (Autorest).
	pnpm -C typespec exec autorest ../pkg/ucp/api/README.md --tag=ucp-2023-10-01-preview && rm pkg/ucp/api/v20231001preview/go.mod && go fmt ./pkg/ucp/api/v20231001preview/...

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
generate-bicep-types: generate-node-installed generate-pnpm-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Build autorest.bicep..."
	CI=true pnpm -C hack/bicep-types-radius/src/autorest.bicep install && pnpm -C hack/bicep-types-radius/src/autorest.bicep run build; \
	echo "Run generator from hack/bicep-types-radius/src/generator dir"; \
	CI=true pnpm -C hack/bicep-types-radius/src/generator install && pnpm -C hack/bicep-types-radius/src/generator run generate --specs-dir ../../../../swagger --release-version ${VERSION} --verbose
	@echo "$(ARROW) Generating Bicep types for default contrib resource type namespaces..."
	@$(MAKE) generate-bicep-types-contrib
	@echo "$(ARROW) Rebuilding unified Bicep types index..."
	CI=true pnpm -C hack/bicep-types-radius/src/generator run rebuild-index --release-version ${VERSION}

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

.PHONY: generate-yq-installed
generate-yq-installed:
	@echo "$(ARROW) Detecting yq..."
	@which yq > /dev/null || { echo "run 'go install github.com/mikefarah/yq/v4@latest' to install yq"; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-bicep-types-contrib
generate-bicep-types-contrib: generate-yq-installed ## Generates Bicep types.json files for default contrib namespaces from defaults.yaml.
	# Discover unique namespaces from defaults.yaml.
	@NAMESPACES=$$(yq '.defaultRegistration[]' $(DEFAULTS_YAML) | sed 's|/.*||' | sort -u) && \
	for ns in $$NAMESPACES; do \
		ns_lower=$$(echo "$$ns" | tr '[:upper:]' '[:lower:]') && \
		out_dir="$(BICEP_TYPES_OUTPUT_BASE)/$$ns_lower/$(BICEP_TYPES_CONTRIB_API_VERSION)" && \
		manifest_args="" && \
		for entry in $$(yq '.defaultRegistration[]' $(DEFAULTS_YAML) | grep "^$$ns/"); do \
			type_name=$$(echo "$$entry" | cut -d'/' -f2) && \
			manifest="$(BICEP_TYPES_CONTRIB_MANIFEST_DIR)/$$type_name.yaml" && \
			if [ ! -f "$$manifest" ]; then \
				echo "ERROR: Manifest not found: $$manifest (from entry '$$entry')"; \
				exit 1; \
			fi && \
			manifest_args="$$manifest_args $$manifest"; \
		done && \
		echo "  -> $$ns ($$manifest_args) -> $$out_dir" && \
		go run ./bicep-tools/cmd/manifest-to-bicep generate $$manifest_args "$$out_dir" || exit 1; \
	done

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


.PHONY: generate-containerinstance-client
generate-containerinstance-client: generate-tsp-installed  ## Generates the Container Instances SDK (Autorest).
	pnpm -C typespec exec autorest \
		../pkg/sdk/aci-specification/containerinstance/resource-manager/readme.md \
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
