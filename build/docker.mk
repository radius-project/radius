# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

DOCKER_REGISTRY?=$(shell whoami)
DOCKER_TAG_VERSION?=latest

##@ Docker Images

# Generate a target for each image we define
# Params:
# $(1): the image name for the target
# $(2): the Dockerfile path
define generateDockerTargets
ifeq ($(strip $(4)),go)
.PHONY: docker-build-$(1)
docker-build-$(1): build-$(1)-linux-amd64
	@echo "$(ARROW) Building Go image $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)"
	$(eval DOCKER_OUT_DIR=$(OUT_DIR)/docker/linux_amd64)
	@mkdir -p $(DOCKER_OUT_DIR)/$(1)
	@cp -v $(3) $(DOCKER_OUT_DIR)/$(1)
	@cp -v $(BINS_OUT_DIR_linux_amd64)/$(1) $(DOCKER_OUT_DIR)/$(1)

	cd $(DOCKER_OUT_DIR)/$(1) && docker build $(2) \
		--platform linux/amd64 \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)"
else
docker-build-$(1):
	@echo "$(ARROW) Building image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)"
	docker build $(2) -f $(3) \
		-t $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)"
endif
.PHONY: docker-push-$(1)
docker-push-$(1):
	@echo "$(ARROW) Pushing image $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)"
	docker push $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)
endef

define generateDockerMultiArches
.PHONY: docker-multi-arch-push-$(1)
docker-multi-arch-push-$(1): build-$(1)-linux-arm64 build-$(1)-linux-amd64 build-$(1)-linux-arm
	@echo "$(ARROW) Building Go image $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)"
	@cp -v $(3) $(OUT_DIR)/Dockerfile-$(1)

	cd $(OUT_DIR) && docker buildx build -f ./Dockerfile-$(1) \
		--platform linux/arm64,linux/arm,linux/amd64 \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)" \
		--push .
endef

# defines a target for each image
DOCKER_IMAGES := ucpd appcore-rp

$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerTargets,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile, go)))

# multi arch container image targets for each binaries
$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerMultiArches,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile.multi)))

# magpie comes from our test directory.
$(eval $(call generateDockerTargets,magpiego,./test/magpiego/,./test/magpiego/Dockerfile))

# testrp comes from our test directory.
$(eval $(call generateDockerTargets,testrp,./test/testrp/,./test/testrp/Dockerfile))

# list of 'outputs' to build all images
DOCKER_BUILD_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-build-$(IMAGE))

# list of 'outputs' to push all images
DOCKER_PUSH_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-push-$(IMAGE))

# list of 'outputs' to push all multi arch images
DOCKER_PUSH_MULTI_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-multi-arch-push-$(IMAGE))

# targets to build test images
.PHONY: docker-test-tool-build
docker-test-tool-build: docker-build-magpiego docker-build-testrp ## Builds all Docker images.

.PHONY: docker-test-tool-push
docker-test-tool-push: docker-push-magpiego docker-push-testrp ## Builds all Docker images.

# targets to build development images
.PHONY: docker-build
docker-build: $(DOCKER_BUILD_TARGETS) docker-test-tool-build ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(DOCKER_PUSH_TARGETS) docker-test-tool-push ## Pushes all Docker images (without building).

# targets to build and push multi arch images
.PHONY: docker-multi-arch-push
docker-multi-arch-push: $(DOCKER_PUSH_MULTI_TARGETS) ## Pushes all docker images after building.
