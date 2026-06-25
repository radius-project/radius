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
DAPR_VERSION ?= v1.18.0
DAPR_CHECKSUM_LINUX_AMD64 ?= 2a94739e0aa101289d88418225319562bc6800db273b3d9cf819a0efd1ea1bfe
DAPR_CHECKSUM_LINUX_ARM64 ?= 99d93e1dde04225204e2feb33191a1df97c87bb7d88abd10d1523f29a88d35d2
DAPR_CHECKSUM_DARWIN_AMD64 ?= 2a7b7f3e4dfa5f8408b183bf840ab518766d91c3338e540cf84e16b5eb561604
DAPR_CHECKSUM_DARWIN_ARM64 ?= 7d564d6aa29a68caab53e9aa4bcb4aabd9da5829992f3c8c297df3095ef5678b

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
