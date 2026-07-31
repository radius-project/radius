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

##@ Tools

TOOL_MANIFEST := build/tools.yaml
TOOL_MAKE_INCLUDE := build/tools.generated.mk
TOOL_UPDATER_GOOS := $(shell go env GOHOSTOS)
TOOL_UPDATER_GOARCH := $(shell go env GOHOSTARCH)
TOOL_UPDATER_BINARY := bin/tool-updater$(if $(filter windows,$(TOOL_UPDATER_GOOS)),.exe,)
TOOL_UPDATER_SOURCES := $(wildcard cmd/tool-updater/*.go) $(wildcard internal/tooling/*.go)

include $(TOOL_MAKE_INCLUDE)

$(TOOL_UPDATER_BINARY): $(TOOL_UPDATER_SOURCES) go.mod go.sum
	@mkdir -p bin
	@GOOS="$(TOOL_UPDATER_GOOS)" GOARCH="$(TOOL_UPDATER_GOARCH)" go build -o "$@" ./cmd/tool-updater

$(TOOL_MAKE_INCLUDE): $(TOOL_MANIFEST) $(TOOL_UPDATER_BINARY)
	@"$(TOOL_UPDATER_BINARY)" generate-make --manifest "$(TOOL_MANIFEST)" --output "$@"

.PHONY: update-tools
update-tools: $(TOOL_UPDATER_BINARY) ## Check tool releases and refresh versions and checksums in the manifest.
	@"$(TOOL_UPDATER_BINARY)" update --manifest "$(TOOL_MANIFEST)" --makefile "$(TOOL_MAKE_INCLUDE)"

.PHONY: install-yq
install-yq: ## Install the pinned yq YAML processor into a user-owned bin dir (no sudo).
	@YQ_VERSION="$(YQ_VERSION)" \
		YQ_CHECKSUM_LINUX_AMD64="$(YQ_CHECKSUM_LINUX_AMD64)" \
		YQ_CHECKSUM_LINUX_ARM64="$(YQ_CHECKSUM_LINUX_ARM64)" \
		YQ_CHECKSUM_DARWIN_AMD64="$(YQ_CHECKSUM_DARWIN_AMD64)" \
		YQ_CHECKSUM_DARWIN_ARM64="$(YQ_CHECKSUM_DARWIN_ARM64)" \
		YQ_INSTALL_DIR="$(YQ_INSTALL_DIR)" \
		./build/scripts/install-yq.sh

.PHONY: install-bicep
install-bicep: ## Install the pinned Bicep CLI into a user-owned bin dir (no sudo).
	@BICEP_VERSION="$(BICEP_VERSION)" \
		BICEP_CHECKSUM_LINUX_AMD64="$(BICEP_CHECKSUM_LINUX_AMD64)" \
		BICEP_CHECKSUM_LINUX_ARM64="$(BICEP_CHECKSUM_LINUX_ARM64)" \
		BICEP_CHECKSUM_DARWIN_AMD64="$(BICEP_CHECKSUM_DARWIN_AMD64)" \
		BICEP_CHECKSUM_DARWIN_ARM64="$(BICEP_CHECKSUM_DARWIN_ARM64)" \
		BICEP_INSTALL_DIR="$(BICEP_INSTALL_DIR)" \
		./build/scripts/install-bicep.sh

.PHONY: install-kind
install-kind: ## Install the pinned kind cluster tool into a user-owned bin dir (no sudo).
	@KIND_VERSION="$(KIND_VERSION)" \
		KIND_CHECKSUM_LINUX_AMD64="$(KIND_CHECKSUM_LINUX_AMD64)" \
		KIND_CHECKSUM_LINUX_ARM64="$(KIND_CHECKSUM_LINUX_ARM64)" \
		KIND_CHECKSUM_DARWIN_AMD64="$(KIND_CHECKSUM_DARWIN_AMD64)" \
		KIND_CHECKSUM_DARWIN_ARM64="$(KIND_CHECKSUM_DARWIN_ARM64)" \
		KIND_INSTALL_DIR="$(KIND_INSTALL_DIR)" \
		./build/scripts/install-kind.sh

.PHONY: install-kubectl
install-kubectl: ## Install the pinned kubectl Kubernetes CLI into a user-owned bin dir (no sudo).
	@KUBECTL_VERSION="$(KUBECTL_VERSION)" \
		KUBECTL_CHECKSUM_LINUX_AMD64="$(KUBECTL_CHECKSUM_LINUX_AMD64)" \
		KUBECTL_CHECKSUM_LINUX_ARM64="$(KUBECTL_CHECKSUM_LINUX_ARM64)" \
		KUBECTL_CHECKSUM_DARWIN_AMD64="$(KUBECTL_CHECKSUM_DARWIN_AMD64)" \
		KUBECTL_CHECKSUM_DARWIN_ARM64="$(KUBECTL_CHECKSUM_DARWIN_ARM64)" \
		KUBECTL_INSTALL_DIR="$(KUBECTL_INSTALL_DIR)" \
		./build/scripts/install-kubectl.sh

.PHONY: install-dapr
install-dapr: ## Install the pinned Dapr CLI into a user-owned bin dir (no sudo).
	@DAPR_VERSION="$(DAPR_VERSION)" \
		DAPR_CHECKSUM_LINUX_AMD64="$(DAPR_CHECKSUM_LINUX_AMD64)" \
		DAPR_CHECKSUM_LINUX_ARM64="$(DAPR_CHECKSUM_LINUX_ARM64)" \
		DAPR_CHECKSUM_DARWIN_AMD64="$(DAPR_CHECKSUM_DARWIN_AMD64)" \
		DAPR_CHECKSUM_DARWIN_ARM64="$(DAPR_CHECKSUM_DARWIN_ARM64)" \
		DAPR_INSTALL_DIR="$(DAPR_INSTALL_DIR)" \
		./build/scripts/install-dapr.sh

.PHONY: install-helm
install-helm: ## Install the pinned Helm CLI into a user-owned bin dir (no sudo).
	@HELM_VERSION="$(HELM_VERSION)" \
		HELM_CHECKSUM_LINUX_AMD64="$(HELM_CHECKSUM_LINUX_AMD64)" \
		HELM_CHECKSUM_LINUX_ARM64="$(HELM_CHECKSUM_LINUX_ARM64)" \
		HELM_CHECKSUM_DARWIN_AMD64="$(HELM_CHECKSUM_DARWIN_AMD64)" \
		HELM_CHECKSUM_DARWIN_ARM64="$(HELM_CHECKSUM_DARWIN_ARM64)" \
		HELM_INSTALL_DIR="$(HELM_INSTALL_DIR)" \
		./build/scripts/install-helm.sh

.PHONY: install-k3d
install-k3d: ## Install the pinned k3d cluster tool into a user-owned bin dir (no sudo).
	@K3D_VERSION="$(K3D_VERSION)" \
		K3D_CHECKSUM_LINUX_AMD64="$(K3D_CHECKSUM_LINUX_AMD64)" \
		K3D_CHECKSUM_LINUX_ARM64="$(K3D_CHECKSUM_LINUX_ARM64)" \
		K3D_CHECKSUM_DARWIN_AMD64="$(K3D_CHECKSUM_DARWIN_AMD64)" \
		K3D_CHECKSUM_DARWIN_ARM64="$(K3D_CHECKSUM_DARWIN_ARM64)" \
		K3D_INSTALL_DIR="$(K3D_INSTALL_DIR)" \
		./build/scripts/install-k3d.sh

.PHONY: install-golangci-lint
install-golangci-lint: ## Install the pinned golangci-lint into a user-owned bin dir (no sudo).
	@GOLANGCI_LINT_VERSION="$(GOLANGCI_LINT_VERSION)" \
		GOLANGCI_LINT_CHECKSUM_LINUX_AMD64="$(GOLANGCI_LINT_CHECKSUM_LINUX_AMD64)" \
		GOLANGCI_LINT_CHECKSUM_LINUX_ARM64="$(GOLANGCI_LINT_CHECKSUM_LINUX_ARM64)" \
		GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64="$(GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64)" \
		GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64="$(GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64)" \
		GOLANGCI_LINT_INSTALL_DIR="$(GOLANGCI_LINT_INSTALL_DIR)" \
		./build/scripts/install-golangci-lint.sh

.PHONY: install-terraform
install-terraform: ## Install the pinned Terraform CLI into a user-owned bin dir (no sudo).
	@TERRAFORM_VERSION="$(TERRAFORM_VERSION)" \
		TERRAFORM_CHECKSUM_LINUX_AMD64="$(TERRAFORM_CHECKSUM_LINUX_AMD64)" \
		TERRAFORM_CHECKSUM_LINUX_ARM64="$(TERRAFORM_CHECKSUM_LINUX_ARM64)" \
		TERRAFORM_CHECKSUM_DARWIN_AMD64="$(TERRAFORM_CHECKSUM_DARWIN_AMD64)" \
		TERRAFORM_CHECKSUM_DARWIN_ARM64="$(TERRAFORM_CHECKSUM_DARWIN_ARM64)" \
		TERRAFORM_INSTALL_DIR="$(TERRAFORM_INSTALL_DIR)" \
		./build/scripts/install-terraform.sh

.PHONY: install-stern
install-stern: ## Install the pinned stern log tailing tool into a user-owned bin dir (no sudo).
	@STERN_VERSION="$(STERN_VERSION)" \
		STERN_CHECKSUM_LINUX_AMD64="$(STERN_CHECKSUM_LINUX_AMD64)" \
		STERN_CHECKSUM_LINUX_ARM64="$(STERN_CHECKSUM_LINUX_ARM64)" \
		STERN_CHECKSUM_DARWIN_AMD64="$(STERN_CHECKSUM_DARWIN_AMD64)" \
		STERN_CHECKSUM_DARWIN_ARM64="$(STERN_CHECKSUM_DARWIN_ARM64)" \
		STERN_INSTALL_DIR="$(STERN_INSTALL_DIR)" \
		./build/scripts/install-stern.sh

.PHONY: install-oras
install-oras: ## Install the pinned ORAS CLI into a user-owned bin dir (no sudo).
	@ORAS_VERSION="$(ORAS_VERSION)" \
		ORAS_CHECKSUM_LINUX_AMD64="$(ORAS_CHECKSUM_LINUX_AMD64)" \
		ORAS_CHECKSUM_LINUX_ARM64="$(ORAS_CHECKSUM_LINUX_ARM64)" \
		ORAS_CHECKSUM_DARWIN_AMD64="$(ORAS_CHECKSUM_DARWIN_AMD64)" \
		ORAS_CHECKSUM_DARWIN_ARM64="$(ORAS_CHECKSUM_DARWIN_ARM64)" \
		ORAS_INSTALL_DIR="$(ORAS_INSTALL_DIR)" \
		./build/scripts/install-oras.sh

.PHONY: install-shellcheck
install-shellcheck: ## Install the pinned ShellCheck into a user-owned bin dir (no sudo).
	@SHELLCHECK_VERSION="$(SHELLCHECK_VERSION)" \
		SHELLCHECK_CHECKSUM_LINUX_AMD64="$(SHELLCHECK_CHECKSUM_LINUX_AMD64)" \
		SHELLCHECK_CHECKSUM_LINUX_ARM64="$(SHELLCHECK_CHECKSUM_LINUX_ARM64)" \
		SHELLCHECK_CHECKSUM_DARWIN_AMD64="$(SHELLCHECK_CHECKSUM_DARWIN_AMD64)" \
		SHELLCHECK_CHECKSUM_DARWIN_ARM64="$(SHELLCHECK_CHECKSUM_DARWIN_ARM64)" \
		SHELLCHECK_INSTALL_DIR="$(SHELLCHECK_INSTALL_DIR)" \
		./build/scripts/install-shellcheck.sh

.PHONY: install-jq
install-jq: ## Install the pinned jq JSON processor into a user-owned bin dir (no sudo).
	@JQ_VERSION="$(JQ_VERSION)" \
		JQ_CHECKSUM_LINUX_AMD64="$(JQ_CHECKSUM_LINUX_AMD64)" \
		JQ_CHECKSUM_LINUX_ARM64="$(JQ_CHECKSUM_LINUX_ARM64)" \
		JQ_CHECKSUM_DARWIN_AMD64="$(JQ_CHECKSUM_DARWIN_AMD64)" \
		JQ_CHECKSUM_DARWIN_ARM64="$(JQ_CHECKSUM_DARWIN_ARM64)" \
		JQ_INSTALL_DIR="$(JQ_INSTALL_DIR)" \
		./build/scripts/install-jq.sh

.PHONY: install-dlv
install-dlv: ## Install the pinned Delve (dlv) Go debugger via 'go install'.
	@DLV_VERSION="$(DLV_VERSION)" \
		DLV_INSTALL_DIR="$(DLV_INSTALL_DIR)" \
		./build/scripts/install-dlv.sh
