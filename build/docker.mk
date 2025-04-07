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
IMAGE_SRC?=https://github.com/radius-project/radius
MANIFEST_DIR?=deploy/manifest/built-in-providers/self-hosted

##@ Docker Images

# Generate a target for each image we define
# Params:
# $(1): the image name for the target
# $(2): the context directory
# $(3): the Dockerfile path
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
.PHONY: docker-multi-arch-build-$(1)
docker-multi-arch-build-$(1): build-$(1)-linux-arm64 build-$(1)-linux-amd64 build-$(1)-linux-arm
	@echo "$(ARROW) Building Go image $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)"
	@cp -v $(3) $(OUT_DIR)/Dockerfile-$(1)

	cd $(OUT_DIR) && docker buildx build -f ./Dockerfile-$(1) \
		--platform linux/amd64,linux/arm64,linux/arm \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.source="$(IMAGE_SRC)" \
		--label org.opencontainers.image.description="$(1)" \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)" \
		$(2)

.PHONY: docker-multi-arch-push-$(1)
docker-multi-arch-push-$(1): build-$(1)-linux-arm64 build-$(1)-linux-amd64 build-$(1)-linux-arm
	@echo "$(ARROW) Building and pushing Go image $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION)"
	@cp -v $(3) $(OUT_DIR)/Dockerfile-$(1)

	# Building and pushing in one step is more efficient with buildx, so we duplicate the command
	# to build and add --push.
	cd $(OUT_DIR) && docker buildx build -f ./Dockerfile-$(1) \
		--platform linux/amd64,linux/arm64,linux/arm \
		-t $(DOCKER_REGISTRY)/$(1):$(DOCKER_TAG_VERSION) \
		--label org.opencontainers.image.source="$(IMAGE_SRC)" \
		--label org.opencontainers.image.description="$(1)" \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)" \
		--push \
		$(2)
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

# Define a target for each image with name and Dockerfile location
APPS_MAP := ucpd:./deploy/images/ucpd \
	applications-rp:./deploy/images/applications-rp \
	dynamic-rp:./deploy/images/dynamic-rp \
	controller:./deploy/images/controller \
	testrp:./test/testrp \
	magpiego:./test/magpiego \
	bicep:./deploy/images/bicep

# copy_manifests copies the manifests to the output directory
.PHONY: copy-manifests
copy-manifests:
	@if [ ! -d "$(MANIFEST_DIR)" ] || [ -z "$$(ls -A $(MANIFEST_DIR))" ]; then \
		echo "MANIFEST_DIR '$(MANIFEST_DIR)' does not exist or is empty"; \
		exit 1; \
	fi
	@mkdir -p $(OUT_DIR)/manifest/built-in-providers/
	@echo "Copying manifests from $(MANIFEST_DIR) to $(OUT_DIR)/manifest/built-in-providers/"
	@cp -v $(MANIFEST_DIR)/* $(OUT_DIR)/manifest/built-in-providers/

# Function to extract the name and the directory of the Dockerfile from the app string
define parseApp
$(eval NAME := $(shell echo $(1) | cut -d: -f1))
$(eval DIR := $(shell echo $(1) | cut -d: -f2))
endef

# This command will dynamically generate the targets for each image in the APPS_MAP list.
$(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP)) $(call generateDockerTargets,$(NAME),.,$(DIR)/Dockerfile,go)))

# This command will dynamically generate the multi-arch targets for each image in the APPS_MAP list.
$(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP)) $(call generateDockerMultiArches,$(NAME),.,$(DIR)/Dockerfile)))

# list of 'outputs' to build all images
DOCKER_BUILD_TARGETS := $(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP))) docker-build-$(NAME))

# list of 'outputs' to push all images
DOCKER_PUSH_TARGETS := $(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP))) docker-push-$(NAME))

# list of 'outputs' to build all multi arch images
DOCKER_BUILD_MULTI_TARGETS := $(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP))) docker-multi-arch-build-$(NAME))

# list of 'outputs' to push all multi arch images
DOCKER_PUSH_MULTI_TARGETS := $(foreach APP,$(APPS_MAP),$(eval $(call parseApp,$(APP))) docker-multi-arch-push-$(NAME))

# targets to build development images
.PHONY: docker-build
docker-build: copy-manifests $(DOCKER_BUILD_TARGETS) ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(DOCKER_PUSH_TARGETS) ## Pushes all Docker images (without building).

# targets to build and push multi arch images. If you run this target in your machine,
# ensure you have qemu and buildx installed by running make configure-buildx.
.PHONY: docker-multi-arch-build
docker-multi-arch-build: copy-manifests $(DOCKER_BUILD_MULTI_TARGETS) ## Builds all docker images for multiple architectures.

.PHONY: docker-multi-arch-push
docker-multi-arch-push: copy-manifests $(DOCKER_PUSH_MULTI_TARGETS) ## Pushes all docker images for multiple architectures after building.
