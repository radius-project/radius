# ------------------------------------------------------------
# Consolidated artifact build targets
#
# Target: artifacts
#   Generates all artifacts needed by tests/releases:
#   - Go binaries for current platform (and optionally multi-arch)
#   - Bicep CLI wrapper
#   - Docker images (build or multi-arch build)
#   - Helm chart package in dist/Charts/helm
#   - Copies built-in provider manifests into dist/manifest
#
# Usage examples:
#   make artifacts GIT_COMMIT=$(git rev-parse HEAD)
#   make artifacts GIT_COMMIT=$(git rev-parse HEAD) DOCKER_MULTI_ARCH=1
#   make artifacts-no-docker GIT_COMMIT=$(git rev-parse HEAD)
#
# Notes:
# - Set GIT_COMMIT to pin the commit embedded in binaries/images.
# - REL_VERSION/REL_CHANNEL/CHART_VERSION can be set by the caller or the CI script to pin versions.
# - Set DOCKER_MULTI_ARCH=1 to build multi-arch images; otherwise builds single-arch (amd64) images.
# - Set SKIP_IMAGES=1 to skip building images.
# - Set SKIP_HELM=1 to skip Helm package.
# - Set MANIFEST_DIR to override manifest source (default in build/docker.mk).
# ------------------------------------------------------------

# Defaults
DOCKER_MULTI_ARCH ?= 0
SKIP_IMAGES ?= 0
SKIP_HELM ?= 0

ARTIFACT_DIR := ./dist
HELM_PACKAGE_DIR := $(ARTIFACT_DIR)/Charts/helm
HELM_CHARTS_DIR := deploy/Chart

.PHONY: artifacts
artifacts: ## Build all artifacts for tests/releases (accepts HASH)
	@echo "$(ARROW) Generating artifacts with GIT_COMMIT=$(GIT_COMMIT) REL_VERSION=$(REL_VERSION) REL_CHANNEL=$(REL_CHANNEL) CHART_VERSION=$(CHART_VERSION)"
	$(MAKE) build
ifeq ($(SKIP_IMAGES),0)
ifeq ($(DOCKER_MULTI_ARCH),1)
	$(MAKE) docker-multi-arch-build
else
	$(MAKE) docker-build
endif
endif
ifeq ($(SKIP_HELM),0)
	@echo "$(ARROW) Packaging Helm chart..."
	@mkdir -p $(HELM_PACKAGE_DIR)
	helm package $(HELM_CHARTS_DIR) --version $(CHART_VERSION) --app-version $(REL_VERSION) --destination $(HELM_PACKAGE_DIR)
endif
	@echo "$(ARROW) Artifacts complete in $(ARTIFACT_DIR)"

.PHONY: artifacts-no-docker
artifacts-no-docker: SKIP_IMAGES=1
artifacts-no-docker: artifacts ## Build artifacts, skipping Docker images
