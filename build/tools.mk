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

# yq - pinned version and per-platform SHA-256 checksums consumed by
# build/scripts/install-yq.sh. The script is generic: clear YQ_VERSION to install
# the latest release, and clear a checksum to have it read from the release's own
# published checksums file. Keep the checksums in sync when bumping YQ_VERSION.
YQ_VERSION ?= v4.53.3
YQ_CHECKSUM_LINUX_AMD64 ?= fa52a4e758c63d38299163fbdd1edfb4c4963247918bf9c1c5d31d84789eded4
YQ_CHECKSUM_LINUX_ARM64 ?= 578648e463a11c1b6db6010cbf41eafed6bee79466fcffa1bb446672cf7945ea
YQ_CHECKSUM_DARWIN_AMD64 ?= b4ba1ecce3c47f00803f4f964de38394326c7a32eb6540616e04fb2935a0f08d
YQ_CHECKSUM_DARWIN_ARM64 ?= 877de31753a4dd2401aa048937aa9a7fc4d5f6ce858cf31508c5802954297213

.PHONY: install-yq
install-yq: ## Install the pinned yq YAML processor into a user-owned bin dir (no sudo).
	@YQ_VERSION="$(YQ_VERSION)" \
		YQ_CHECKSUM_LINUX_AMD64="$(YQ_CHECKSUM_LINUX_AMD64)" \
		YQ_CHECKSUM_LINUX_ARM64="$(YQ_CHECKSUM_LINUX_ARM64)" \
		YQ_CHECKSUM_DARWIN_AMD64="$(YQ_CHECKSUM_DARWIN_AMD64)" \
		YQ_CHECKSUM_DARWIN_ARM64="$(YQ_CHECKSUM_DARWIN_ARM64)" \
		YQ_INSTALL_DIR="$(YQ_INSTALL_DIR)" \
		./build/scripts/install-yq.sh

# bicep CLI - pinned version and per-platform SHA-256 checksums consumed by
# build/scripts/install-bicep.sh. Pinned to v0.42.1: v0.43+ rejects br:localhost
# registries used by the local functional tests; bump only after verifying
# localhost support. Azure/bicep publishes no checksums file, so these are
# computed from the pinned release - recompute them (download each
# bicep-<os>-<arch> asset and sha256sum it) when bumping BICEP_VERSION.
BICEP_VERSION ?= v0.42.1
BICEP_CHECKSUM_LINUX_AMD64 ?= aed90eb2c69a6ee2bd70dc0d4354408ac4d04fd9911d3ec8e0cd74ad173e7139
BICEP_CHECKSUM_LINUX_ARM64 ?= b01ac3bb5259096dfbe548138a538d1c4e4a55e6f87f3827e2299fbc2d4e6796
BICEP_CHECKSUM_DARWIN_AMD64 ?= 8219bfd0601a514cc0a814b4b194aed588f4efa68b7c7ac7c9b64f3d84713dd7
BICEP_CHECKSUM_DARWIN_ARM64 ?= 1c66533af4d4d47f875623d88074d28ca7fe7e9dc1f783a62570e8724700aca1

.PHONY: install-bicep
install-bicep: ## Install the pinned Bicep CLI into a user-owned bin dir (no sudo).
	@BICEP_VERSION="$(BICEP_VERSION)" \
		BICEP_CHECKSUM_LINUX_AMD64="$(BICEP_CHECKSUM_LINUX_AMD64)" \
		BICEP_CHECKSUM_LINUX_ARM64="$(BICEP_CHECKSUM_LINUX_ARM64)" \
		BICEP_CHECKSUM_DARWIN_AMD64="$(BICEP_CHECKSUM_DARWIN_AMD64)" \
		BICEP_CHECKSUM_DARWIN_ARM64="$(BICEP_CHECKSUM_DARWIN_ARM64)" \
		BICEP_INSTALL_DIR="$(BICEP_INSTALL_DIR)" \
		./build/scripts/install-bicep.sh

# kind (Kubernetes IN Docker) - pinned version and per-platform SHA-256 checksums
# consumed by build/scripts/install-kind.sh. The script is generic: clear
# KIND_VERSION to install the latest release, and clear a checksum to have it read
# from the release's own '<asset>.sha256sum' file. Keep the checksums in sync when
# bumping KIND_VERSION.
KIND_VERSION ?= v0.32.0
KIND_CHECKSUM_LINUX_AMD64 ?= 50030de23cf40a18505f20426f6a8506bedf13c6e509244bd1fa9463721b0f54
KIND_CHECKSUM_LINUX_ARM64 ?= b92cd615e97585de8ddade28ed5cd7feb4248d717c233eea5b03c37298900f5d
KIND_CHECKSUM_DARWIN_AMD64 ?= 295ac6d0d634c9819c9907df45e3017d1f13166bd13c3404c45e79f7faa47498
KIND_CHECKSUM_DARWIN_ARM64 ?= dca67911095a110c2b5c36e26df6cac860c602033e456c0db47be498cdef1ebb

.PHONY: install-kind
install-kind: ## Install the pinned kind cluster tool into a user-owned bin dir (no sudo).
	@KIND_VERSION="$(KIND_VERSION)" \
		KIND_CHECKSUM_LINUX_AMD64="$(KIND_CHECKSUM_LINUX_AMD64)" \
		KIND_CHECKSUM_LINUX_ARM64="$(KIND_CHECKSUM_LINUX_ARM64)" \
		KIND_CHECKSUM_DARWIN_AMD64="$(KIND_CHECKSUM_DARWIN_AMD64)" \
		KIND_CHECKSUM_DARWIN_ARM64="$(KIND_CHECKSUM_DARWIN_ARM64)" \
		KIND_INSTALL_DIR="$(KIND_INSTALL_DIR)" \
		./build/scripts/install-kind.sh

# kubectl (Kubernetes CLI) - pinned version and per-platform SHA-256 checksums
# consumed by build/scripts/install-kubectl.sh. kubectl is published on the
# Kubernetes release CDN (dl.k8s.io), not GitHub. The script is generic: clear
# KUBECTL_VERSION to install the latest stable release, and clear a checksum to
# have it read from the release's own 'kubectl.sha256' file. Keep the checksums in
# sync when bumping KUBECTL_VERSION.
KUBECTL_VERSION ?= v1.36.2
KUBECTL_CHECKSUM_LINUX_AMD64 ?= 1e9045ec32bea85da43de85f0065358529ea7c7a152eca78154fba5b58c27d82
KUBECTL_CHECKSUM_LINUX_ARM64 ?= c957eb8c4bea27a3bb35b269edd9082e27f027f7b76b20b5bf4afebc726c6d3e
KUBECTL_CHECKSUM_DARWIN_AMD64 ?= ce6c5e55cd17559e87e4fb5e73ebbbc2511bcf2b695d7a40c1b1461a9817d4b3
KUBECTL_CHECKSUM_DARWIN_ARM64 ?= 4408c85c83fd3a31adaa555bdf3c7a6c81f74b19449a9060ba31ab91926f023d

.PHONY: install-kubectl
install-kubectl: ## Install the pinned kubectl Kubernetes CLI into a user-owned bin dir (no sudo).
	@KUBECTL_VERSION="$(KUBECTL_VERSION)" \
		KUBECTL_CHECKSUM_LINUX_AMD64="$(KUBECTL_CHECKSUM_LINUX_AMD64)" \
		KUBECTL_CHECKSUM_LINUX_ARM64="$(KUBECTL_CHECKSUM_LINUX_ARM64)" \
		KUBECTL_CHECKSUM_DARWIN_AMD64="$(KUBECTL_CHECKSUM_DARWIN_AMD64)" \
		KUBECTL_CHECKSUM_DARWIN_ARM64="$(KUBECTL_CHECKSUM_DARWIN_ARM64)" \
		KUBECTL_INSTALL_DIR="$(KUBECTL_INSTALL_DIR)" \
		./build/scripts/install-kubectl.sh

# Dapr CLI - pinned version and per-platform SHA-256 checksums (of the release
# tarball) consumed by build/scripts/install-dapr.sh. The script is generic: clear
# DAPR_VERSION to install the latest release, and clear a checksum to have it
# read from the release's own '<asset>.sha256' file. Keep the checksums in sync
# when bumping DAPR_VERSION. This pins the Dapr CLI only; the Dapr runtime and
# dashboard versions used by `dapr init -k` are pinned in the workflows.
DAPR_VERSION ?= v1.15.2
DAPR_CHECKSUM_LINUX_AMD64 ?= 09328bc0e4353036b824c2ec9cf7cabf4d75b4fc00ca02d80ae3e4374ee27eda
DAPR_CHECKSUM_LINUX_ARM64 ?= b49244701a191c1e843211383703be9f2cd086a1db259c9789672f7e4e82ad55
DAPR_CHECKSUM_DARWIN_AMD64 ?= 42a36e667559aef0fb6357fbe8f0fdbf1a6d9ea0ba8484c32e90ea61ddf15ba0
DAPR_CHECKSUM_DARWIN_ARM64 ?= 176f455ea1961cdb59ab0e9ec3e4900b877576a9a2178d3b4b2619bfe947643f

.PHONY: install-dapr
install-dapr: ## Install the pinned Dapr CLI into a user-owned bin dir (no sudo).
	@DAPR_VERSION="$(DAPR_VERSION)" \
		DAPR_CHECKSUM_LINUX_AMD64="$(DAPR_CHECKSUM_LINUX_AMD64)" \
		DAPR_CHECKSUM_LINUX_ARM64="$(DAPR_CHECKSUM_LINUX_ARM64)" \
		DAPR_CHECKSUM_DARWIN_AMD64="$(DAPR_CHECKSUM_DARWIN_AMD64)" \
		DAPR_CHECKSUM_DARWIN_ARM64="$(DAPR_CHECKSUM_DARWIN_ARM64)" \
		DAPR_INSTALL_DIR="$(DAPR_INSTALL_DIR)" \
		./build/scripts/install-dapr.sh

# Helm CLI - pinned version and per-platform SHA-256 checksums (of the release
# tarball) consumed by build/scripts/install-helm.sh. Helm is published on the
# Helm release CDN (get.helm.sh), not GitHub. The script is generic: clear
# HELM_VERSION to install the latest release, and clear a checksum to have it read
# from the release's own '<tarball>.sha256sum' file. Keep the checksums in sync
# when bumping HELM_VERSION.
HELM_VERSION ?= v4.2.2
HELM_CHECKSUM_LINUX_AMD64 ?= 9adafecab4d406853bba163a70e9f104f47dbbf65ce24b7653bae7e36150bcb6
HELM_CHECKSUM_LINUX_ARM64 ?= 78803142087a0069fa4b50d3f32a84d3ef25c14d1ee8a40fbccf86a6216d2f36
HELM_CHECKSUM_DARWIN_AMD64 ?= 10c1e36ee8c5f2e2ee25a16599cb03ab74c0953cd889cacb980a49ba4b6574ba
HELM_CHECKSUM_DARWIN_ARM64 ?= 5410a0dae3d5d91f45653b161260d9301aabc4ae80ae50a6605d66884b6df8ea

.PHONY: install-helm
install-helm: ## Install the pinned Helm CLI into a user-owned bin dir (no sudo).
	@HELM_VERSION="$(HELM_VERSION)" \
		HELM_CHECKSUM_LINUX_AMD64="$(HELM_CHECKSUM_LINUX_AMD64)" \
		HELM_CHECKSUM_LINUX_ARM64="$(HELM_CHECKSUM_LINUX_ARM64)" \
		HELM_CHECKSUM_DARWIN_AMD64="$(HELM_CHECKSUM_DARWIN_AMD64)" \
		HELM_CHECKSUM_DARWIN_ARM64="$(HELM_CHECKSUM_DARWIN_ARM64)" \
		HELM_INSTALL_DIR="$(HELM_INSTALL_DIR)" \
		./build/scripts/install-helm.sh

# k3d (k3s in Docker) - pinned version and per-platform SHA-256 checksums consumed
# by build/scripts/install-k3d.sh. The script is generic: clear K3D_VERSION to
# install the latest release, and clear a checksum to have it read from the
# release's own combined 'checksums.txt' file. Keep the checksums in sync when
# bumping K3D_VERSION.
K3D_VERSION ?= v5.9.0
K3D_CHECKSUM_LINUX_AMD64 ?= 06d8f25bc3a971c4eb29e0ff08429b180402db0f4dec838c9eac427e296800a0
K3D_CHECKSUM_LINUX_ARM64 ?= 03cde5cf23e6e8e67de5a039ecf26e5b85aca82fba3e5d13dadf904cd218a250
K3D_CHECKSUM_DARWIN_AMD64 ?= b4aabc37534f95b9c764e7823f2df923f50d57600837aa60a06266cce47db732
K3D_CHECKSUM_DARWIN_ARM64 ?= fe106541d5d0a3f18debcd4d432a16f8c0ce3e6ddc06f8fbb6f696a122313e00

.PHONY: install-k3d
install-k3d: ## Install the pinned k3d cluster tool into a user-owned bin dir (no sudo).
	@K3D_VERSION="$(K3D_VERSION)" \
		K3D_CHECKSUM_LINUX_AMD64="$(K3D_CHECKSUM_LINUX_AMD64)" \
		K3D_CHECKSUM_LINUX_ARM64="$(K3D_CHECKSUM_LINUX_ARM64)" \
		K3D_CHECKSUM_DARWIN_AMD64="$(K3D_CHECKSUM_DARWIN_AMD64)" \
		K3D_CHECKSUM_DARWIN_ARM64="$(K3D_CHECKSUM_DARWIN_ARM64)" \
		K3D_INSTALL_DIR="$(K3D_INSTALL_DIR)" \
		./build/scripts/install-k3d.sh

# golangci-lint - pinned version and per-platform SHA-256 checksums (of the
# release tarball) consumed by build/scripts/install-golangci-lint.sh. The script
# is generic: clear GOLANGCI_LINT_VERSION to install the latest release, and clear
# a checksum to have it read from the release's own
# 'golangci-lint-<version>-checksums.txt' file. Keep the checksums in sync when
# bumping GOLANGCI_LINT_VERSION.
GOLANGCI_LINT_VERSION ?= v2.12.2
GOLANGCI_LINT_CHECKSUM_LINUX_AMD64 ?= 8df580d2670fed8fa984aac0507099af8df275e665215f5c7a2ae3943893a553
GOLANGCI_LINT_CHECKSUM_LINUX_ARM64 ?= 44cd40a8c76c86755375adfeea52cfd3533cb43d7bd647771e0ae065e166df3a
GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64 ?= f6f06d94b6241521c53d15450c5209b028270bf966f842afb11c030c79f5bc16
GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64 ?= a9c54498731b3128f79e090be6110f3e5fffccc617b08142ed244d4126c73f29

.PHONY: install-golangci-lint
install-golangci-lint: ## Install the pinned golangci-lint into a user-owned bin dir (no sudo).
	@GOLANGCI_LINT_VERSION="$(GOLANGCI_LINT_VERSION)" \
		GOLANGCI_LINT_CHECKSUM_LINUX_AMD64="$(GOLANGCI_LINT_CHECKSUM_LINUX_AMD64)" \
		GOLANGCI_LINT_CHECKSUM_LINUX_ARM64="$(GOLANGCI_LINT_CHECKSUM_LINUX_ARM64)" \
		GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64="$(GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64)" \
		GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64="$(GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64)" \
		GOLANGCI_LINT_INSTALL_DIR="$(GOLANGCI_LINT_INSTALL_DIR)" \
		./build/scripts/install-golangci-lint.sh

# Terraform CLI - per-platform SHA-256 checksums (of the release zip) consumed by
# build/scripts/install-terraform.sh. TERRAFORM_VERSION itself is defined in
# build/build.mk, where it defaults to the .terraform-version file (overridable,
# e.g. `make install-terraform TERRAFORM_VERSION=1.15.0`) and is also embedded into
# the rad binary. Keep these checksums in sync when bumping .terraform-version. The
# script is generic: clear a checksum to have it read from the release's own
# 'terraform_<version>_SHA256SUMS' file.
TERRAFORM_CHECKSUM_LINUX_AMD64 ?= 2e5cffc20a0b48a67a76268723bd5a10b8666f69b2aa4f04906e206726bedd63
TERRAFORM_CHECKSUM_LINUX_ARM64 ?= 863002085b886453795d9ff4b8989b8468784478150b70ba8a1df3e3ad66da99
TERRAFORM_CHECKSUM_DARWIN_AMD64 ?= c15326e1af102d2767d40208a0157d1402057f80192991f56803b66457304cf3
TERRAFORM_CHECKSUM_DARWIN_ARM64 ?= 5bc0b11b7a63c8984a41d82523356df46f7833c2e9651a39a7f8919422de5cde

.PHONY: install-terraform
install-terraform: ## Install the pinned Terraform CLI into a user-owned bin dir (no sudo).
	@TERRAFORM_VERSION="$(TERRAFORM_VERSION)" \
		TERRAFORM_CHECKSUM_LINUX_AMD64="$(TERRAFORM_CHECKSUM_LINUX_AMD64)" \
		TERRAFORM_CHECKSUM_LINUX_ARM64="$(TERRAFORM_CHECKSUM_LINUX_ARM64)" \
		TERRAFORM_CHECKSUM_DARWIN_AMD64="$(TERRAFORM_CHECKSUM_DARWIN_AMD64)" \
		TERRAFORM_CHECKSUM_DARWIN_ARM64="$(TERRAFORM_CHECKSUM_DARWIN_ARM64)" \
		TERRAFORM_INSTALL_DIR="$(TERRAFORM_INSTALL_DIR)" \
		./build/scripts/install-terraform.sh

# stern (multi-pod Kubernetes log tailing) - pinned version and per-platform
# SHA-256 checksums (of the release tarball) consumed by
# build/scripts/install-stern.sh. The script is generic: clear STERN_VERSION to
# install the latest release, and clear a checksum to have it read from the
# release's own combined 'checksums.txt' file. Keep the checksums in sync when
# bumping STERN_VERSION.
STERN_VERSION ?= v1.34.0
STERN_CHECKSUM_LINUX_AMD64 ?= 7754adfa653939240f7d20fff4ada9b69cda40c9e70732301f67bb8045f1ef3e
STERN_CHECKSUM_LINUX_ARM64 ?= e215cfc5e42d71e93b77d3fac8f0df7d736271f44b2d92a5b417eaa588edff3a
STERN_CHECKSUM_DARWIN_AMD64 ?= 153355317f21e565ea10bc710d4c2e3d98fd06f83cae5eb927e7031cc724a7a6
STERN_CHECKSUM_DARWIN_ARM64 ?= 4014d84096e1e603ee115864e03a1e15fb9bae9876647bf7bb8031eee278dcd3

.PHONY: install-stern
install-stern: ## Install the pinned stern log tailing tool into a user-owned bin dir (no sudo).
	@STERN_VERSION="$(STERN_VERSION)" \
		STERN_CHECKSUM_LINUX_AMD64="$(STERN_CHECKSUM_LINUX_AMD64)" \
		STERN_CHECKSUM_LINUX_ARM64="$(STERN_CHECKSUM_LINUX_ARM64)" \
		STERN_CHECKSUM_DARWIN_AMD64="$(STERN_CHECKSUM_DARWIN_AMD64)" \
		STERN_CHECKSUM_DARWIN_ARM64="$(STERN_CHECKSUM_DARWIN_ARM64)" \
		STERN_INSTALL_DIR="$(STERN_INSTALL_DIR)" \
		./build/scripts/install-stern.sh

# ORAS (OCI Registry As Storage) CLI - pinned version and per-platform SHA-256
# checksums (of the release tarball) consumed by build/scripts/install-oras.sh.
# The script is generic: clear ORAS_VERSION to install the latest release, and
# clear a checksum to have it read from the release's own
# 'oras_<version>_checksums.txt' file. Keep the checksums in sync when bumping
# ORAS_VERSION.
ORAS_VERSION ?= v1.3.2
ORAS_CHECKSUM_LINUX_AMD64 ?= 9229ccc6d17bb282039ad4a69abb16dcb887a5bce567c075d731d9b3c7ad8eaf
ORAS_CHECKSUM_LINUX_ARM64 ?= 8db4a223bd6034deff198e791ea7cb3af0840df25b7e9f370e2f1f3fd20d389b
ORAS_CHECKSUM_DARWIN_AMD64 ?= 2621f6b252b222f6fbf4e114d2fcaa0cec6b632624ffaf73143f66e4e0994f86
ORAS_CHECKSUM_DARWIN_ARM64 ?= 7929f792cf272268412375ecad6f0fb3c20f164368d5b57966e67ad6d36eca53

.PHONY: install-oras
install-oras: ## Install the pinned ORAS CLI into a user-owned bin dir (no sudo).
	@ORAS_VERSION="$(ORAS_VERSION)" \
		ORAS_CHECKSUM_LINUX_AMD64="$(ORAS_CHECKSUM_LINUX_AMD64)" \
		ORAS_CHECKSUM_LINUX_ARM64="$(ORAS_CHECKSUM_LINUX_ARM64)" \
		ORAS_CHECKSUM_DARWIN_AMD64="$(ORAS_CHECKSUM_DARWIN_AMD64)" \
		ORAS_CHECKSUM_DARWIN_ARM64="$(ORAS_CHECKSUM_DARWIN_ARM64)" \
		ORAS_INSTALL_DIR="$(ORAS_INSTALL_DIR)" \
		./build/scripts/install-oras.sh

# ShellCheck (static analysis for shell scripts) - pinned version and per-platform
# SHA-256 checksums (of the release '.tar.xz' archive) consumed by
# build/scripts/install-shellcheck.sh. ShellCheck is published as a per-platform
# archive on koalaman/shellcheck's GitHub releases and publishes no checksums
# file, so these are computed from the pinned release - recompute them (download
# each shellcheck-<version>.<os>.<arch>.tar.xz asset and sha256sum it) when
# bumping SHELLCHECK_VERSION. The script is generic: clear SHELLCHECK_VERSION to
# install the latest release, and clear a checksum to install without
# verification.
SHELLCHECK_VERSION ?= v0.11.0
SHELLCHECK_CHECKSUM_LINUX_AMD64 ?= 8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198
SHELLCHECK_CHECKSUM_LINUX_ARM64 ?= 12b331c1d2db6b9eb13cfca64306b1b157a86eb69db83023e261eaa7e7c14588
SHELLCHECK_CHECKSUM_DARWIN_AMD64 ?= 3c89db4edcab7cf1c27bff178882e0f6f27f7afdf54e859fa041fca10febe4c6
SHELLCHECK_CHECKSUM_DARWIN_ARM64 ?= 56affdd8de5527894dca6dc3d7e0a99a873b0f004d7aabc30ae407d3f48b0a79

.PHONY: install-shellcheck
install-shellcheck: ## Install the pinned ShellCheck into a user-owned bin dir (no sudo).
	@SHELLCHECK_VERSION="$(SHELLCHECK_VERSION)" \
		SHELLCHECK_CHECKSUM_LINUX_AMD64="$(SHELLCHECK_CHECKSUM_LINUX_AMD64)" \
		SHELLCHECK_CHECKSUM_LINUX_ARM64="$(SHELLCHECK_CHECKSUM_LINUX_ARM64)" \
		SHELLCHECK_CHECKSUM_DARWIN_AMD64="$(SHELLCHECK_CHECKSUM_DARWIN_AMD64)" \
		SHELLCHECK_CHECKSUM_DARWIN_ARM64="$(SHELLCHECK_CHECKSUM_DARWIN_ARM64)" \
		SHELLCHECK_INSTALL_DIR="$(SHELLCHECK_INSTALL_DIR)" \
		./build/scripts/install-shellcheck.sh
