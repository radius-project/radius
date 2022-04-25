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

# defines a target for each image
DOCKER_IMAGES := radius-rp
$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerTargets,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile, go)))

# magpie comes from our test directory.
$(eval $(call generateDockerTargets,magpiego,./test/magpiego/,./test/magpiego/Dockerfile))

# list of 'outputs' to build all images
DOCKER_BUILD_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-build-$(IMAGE)) docker-build-magpiego

# list of 'outputs' to push all images
DOCKER_PUSH_TARGETS:=$(foreach IMAGE,$(DOCKER_IMAGES),docker-push-$(IMAGE)) docker-push-magpiego

.PHONY: docker-build
docker-build: $(DOCKER_BUILD_TARGETS) ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(DOCKER_PUSH_TARGETS) ## Pushes all Docker images (without building).
