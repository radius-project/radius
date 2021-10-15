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
	@echo "$(ARROW) Building Go image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)"
	@mkdir -p $(OUT_DIR)/docker/linux_amd64/$(1)
	@cp $(3) $(OUT_DIR)/docker/linux_amd64/$(1)
	@cp $(BINS_OUT_DIR_linux_amd64)/$(1) $(OUT_DIR)/docker/linux_amd64/$(1)

	cd $(OUT_DIR)/docker/linux_amd64/$(1) && docker build $(2) \
		-t $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) \
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
	@echo "$(ARROW) Pushing image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)"
	docker push $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)
endef

# defines a target for each image
DOCKER_IMAGES := radius-rp radius-controller
$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerTargets,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile, go)))

# magpie comes from our test directory.
$(eval $(call generateDockerTargets,magpie,./test/magpie/,./test/magpie/Dockerfile))

# list of 'outputs' to build all images
DOCKER_BUILD_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-build-$(IMAGE)) docker-build-magpie

# list of 'outputs' to push all images
DOCKER_PUSH_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-push-$(IMAGE)) docker-push-magpie

.PHONY: docker-build
docker-build: $(DOCKER_BUILD_TARGETS) ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(DOCKER_PUSH_TARGETS) ## Pushes all Docker images (without building).
