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
.PHONY: docker-build-$(1)
docker-build-$(1):
	@echo "$(ARROW) Building image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) from $(2)"
	docker build $(2) \
		-f $(3) \
		-t $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) \
		--build-arg LDFLAGS=$(LDFLAGS) \
		--label org.opencontainers.image.version="$(REL_VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_COMMIT)"

.PHONY: docker-push-$(1)
docker-push-$(1):
	@echo "$(ARROW) Pushing image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)"
	docker push $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION)
endef

# magpie comes from our test directory.
$(eval $(call generateDockerTargets,magpie,./test/magpie/,./test/magpie/Dockerfile))

# list of 'outputs' to build all images
DOCKER_BUILD_TARGETS:=docker-build-magpie

# list of 'outputs' to push all images
DOCKER_PUSH_TARGETS:=docker-push-magpie

.PHONY: docker-build
docker-build: $(DOCKER_BUILD_TARGETS) ## Builds all Docker images.

.PHONY: docker-push
docker-push: $(KO_PUBLISH_TARGETS) docker-push-magpie ## Pushes all Docker images


# Generate a target for each image we define
# Params:
# $(1): the image name for the target
define generateKoTargets
.PHONY: ko-publish-$(1)
ko-publish-$(1): install-ko
	@echo "$(ARROW) Building image $(DOCKER_REGISTRY)/$(1)\:$(DOCKER_TAG_VERSION) from $(2)"
	$(eval $@_KO := $(shell mktemp -d))
	@echo "builds:" > $($@_KO)/.ko.yaml
	@echo "- id: $(1)" >> $($@_KO)/.ko.yaml
	@echo "  main: ./cmd/$(1)" >> $($@_KO)/.ko.yaml
	@echo "  ldflags:" >> $($@_KO)/.ko.yaml
	@echo "  - $(LDFLAGS)" >> $($@_KO)/.ko.yaml

	KO_DOCKER_REPO=$(DOCKER_REGISTRY) KO_CONFIG_PATH=$($@_KO) \
      ko publish ./cmd/$(1) -B --tags $(DOCKER_TAG_VERSION)

	@rm -rf ${$@_KO}
endef

# Generate an 'alias' for `ko` images
# Params:
# $(1): the image name for the target
define generateAliasTargets
.PHONY: docker-push-$(1)
docker-push-$(1): ko-publish-$(1)
endef

# list of 'outputs' to push all images
KO_IMAGES:=radius-rp radius-controller

# Generate a ko-publish-x target for each image
$(foreach IMAGE,$(KO_IMAGES),$(eval $(call generateKoTargets,$(IMAGE))))
$(foreach IMAGE,$(KO_IMAGES),$(eval $(call generateAliasTargets,$(IMAGE))))
KO_TARGETS=$(foreach IMAGE,$(KO_IMAGES),ko-publish-$(IMAGE))

.PHONY: ko-publish
ko-publish: $(KO_TARGETS)

.PHONY: install-ko
install-ko:
go install github.com/google/ko:latest
