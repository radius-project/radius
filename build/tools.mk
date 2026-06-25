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
