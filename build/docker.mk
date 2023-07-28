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

DOCKER_REGISTRY?=$(shell whoami)
DOCKER_TAG_VERSION?=latest
IMAGE_SRC?=https://github.com/project-radius/radius

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
	@cp -v $(3) $(OUT_DIR)/Dockerfile-$(1)

	cd $(OUT_DIR) && docker build $(2) -f ./Dockerfile-$(1) \
		--platform linux/amd64 \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.source="$(IMAGE_SRC)" \
		--label org.opencontainers.image.description="$(1)" \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)"
else
docker-build-$(1):
	@echo "$(ARROW) Building image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)"
	docker build $(2) -f $(3) \
		-t $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.source="$(IMAGE_SRC)" \
		--label org.opencontainers.image.description="$(1)" \
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
		--platform linux/amd64,linux/arm64,linux/arm \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.source="$(IMAGE_SRC)" \
		--label org.opencontainers.image.description="$(1)" \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)" \
		--push $(2)
endef

# configure-buildx is to initialize qemu and buildx environment.
.PHONY: configure-buildx
configure-buildx:
	docker pull multiarch/qemu-user-static
	docker run --privileged multiarch/qemu-user-static --reset -p yes
	if ! docker buildx ls | grep -w radius-builder > /dev/null; then \
		docker buildx create --name radius-builder && \
		docker buildx inspect --builder radius-builder --bootstrap; \
	fi

# defines a target for each image
DOCKER_IMAGES := ucpd applications-rp

$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerTargets,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile, go)))

# multi arch container image targets for each binaries
$(foreach IMAGE,$(DOCKER_IMAGES),$(eval $(call generateDockerMultiArches,$(IMAGE),.,./deploy/images/$(IMAGE)/Dockerfile)))

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
.PHONY: docker-test-image-build
docker-test-image-build: docker-build-magpiego docker-build-testrp ## Builds all Docker images.

.PHONY: docker-test-image-push
docker-test-image-push: docker-push-magpiego docker-push-testrp ## Builds all Docker images.

# targets to build development images
.PHONY: docker-build
docker-build: $(DOCKER_BUILD_TARGETS) docker-test-image-build ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(DOCKER_PUSH_TARGETS) docker-test-image-push ## Pushes all Docker images (without building).

# targets to build and push multi arch images. If you run this target in your machine,
# ensure you have qemu and buildx installed by running make configure-buildx.
.PHONY: docker-multi-arch-push
docker-multi-arch-push: $(DOCKER_PUSH_MULTI_TARGETS) ## Pushes all docker images after building.
