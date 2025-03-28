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

BASE_PACKAGE_NAME := github.com/radius-project/radius
OUT_DIR := ./dist

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOPATH := $(shell go env GOPATH)

# Set GOBIN environment variable.
# If it is not set, use GOPATH/bin.
ifeq (,$(shell go env GOBIN))
	GOBIN=$(shell go env GOPATH)/bin
else
	GOBIN=$(shell go env GOBIN)
endif

# Check Operating System and set binary extension,
# and golangci-lint binary name.
ifeq ($(GOOS),windows)
   BINARY_EXT = .exe
   GOLANGCI_LINT:=golangci-lint.exe
else
   GOLANGCI_LINT:=golangci-lint
endif

# Check if DEBUG is set to 1 or not.
# If DEBUG is set to 1, then build debug binaries.
# If DEBUG is not set, then build release binaries.
ifeq ($(origin DEBUG), undefined)
  BUILDTYPE_DIR:=release
  GCFLAGS:=""
else ifeq ($(DEBUG),0)
  BUILDTYPE_DIR:=release
  GCFLAGS:=""
else
  BUILDTYPE_DIR:=debug
  GCFLAGS:="all=-N -l"
endif

# Linker flags: https://cmake.org/cmake/help/latest/envvar/LDFLAGS.html.
LDFLAGS := "-s -w -X $(BASE_PACKAGE_NAME)/pkg/version.channel=$(REL_CHANNEL) -X $(BASE_PACKAGE_NAME)/pkg/version.release=$(REL_VERSION) -X $(BASE_PACKAGE_NAME)/pkg/version.commit=$(GIT_COMMIT) -X $(BASE_PACKAGE_NAME)/pkg/version.version=$(GIT_VERSION) -X $(BASE_PACKAGE_NAME)/pkg/version.chartVersion=$(CHART_VERSION)"

# Combination of flags into GOARGS.
GOARGS := -v -gcflags $(GCFLAGS) -ldflags $(LDFLAGS)

export GO111MODULE ?= on
export GOPROXY ?= https://proxy.golang.org
export GOSUMDB ?= sum.golang.org
export CGO_ENABLED=0

##@ Build

.PHONY: build
build: build-packages build-binaries build-bicep

.PHONY: build-packages
build-packages: ## Builds all go packages.
	@echo "$(ARROW) Building all packages"
	go build \
		-v \
		-gcflags $(GCFLAGS) \
		-ldflags=$(LDFLAGS) \
		./...

# Generate a target for each binary we define
# Params:
# $(1): the binary name for the target
define generateBuildTarget
.PHONY: build-$(1)
build-$(1): build-$(1)-$(GOOS)-$(GOARCH)
endef

# Generate a target for each binary we define
# Params:
# $(1): the OS
# $(2): the ARCH
# $(3): the binary name for the target
# $(4): the binary main directory
# 
# Note: testrp and magpiego have their own modules.
# That is why we need to change the directory to the binary main directory as we do on line 101.
# Otherwise we get the following error:
# `main module (github.com/radius-project/radius) does not contain package github.com/radius-project/radius/test/testrp`
define generatePlatformBuildTarget
.PHONY: build-$(3)-$(1)-$(2)
build-$(3)-$(1)-$(2):
  $(eval BINS_OUT_DIR_$(1)_$(2) := $(OUT_DIR)/$(1)_$(2)/$(BUILDTYPE_DIR))
	@echo "$(ARROW) Building $(3) on $(1)/$(2) to $(BINS_OUT_DIR_$(1)_$(2))/$(3)$(BINARY_EXT)"
	cd $(4) && GOOS=$(1) GOARCH=$(2) go build \
		-v \
		-gcflags $(GCFLAGS) \
		-ldflags=$(LDFLAGS) \
		-o $(CURDIR)/$(BINS_OUT_DIR_$(1)_$(2))/$(3)$(BINARY_EXT)
endef

# defines a target for each binary
GOOSES := darwin linux windows
GOARCHES := amd64 arm arm64

# List of binaries to build.
# Format: binaryName:binaryMainDirectory
# Example: docgen:./cmd/docgen
BINARIES := docgen:./cmd/docgen \
	rad:./cmd/rad \
	applications-rp:./cmd/applications-rp \
	dynamic-rp:./cmd/dynamic-rp \
	ucpd:./cmd/ucpd \
	controller:./cmd/controller \
	testrp:./test/testrp \
	magpiego:./test/magpiego

# This function parses binary name and entrypoint from an item in the BINARIES list.
define parseBinary
$(eval NAME := $(shell echo $(1) | cut -d: -f1))
$(eval ENTRYPOINT := $(shell echo $(1) | cut -d: -f2))
endef

# Generate build targets for each binary.
$(foreach ITEM,$(BINARIES),$(eval $(call parseBinary,$(ITEM)) $(call generateBuildTarget,$(NAME),$(ENTRYPOINT))))

# Generate platform build targets for each binary and each platform.
# This will generate a target for each combination of OS and ARCH for each item in the BINARIES list.
$(foreach ARCH,$(GOARCHES),$(foreach OS,$(GOOSES),$(foreach ITEM,$(BINARIES),$(eval $(call parseBinary,$(ITEM)) $(call generatePlatformBuildTarget,$(OS),$(ARCH),$(NAME),$(ENTRYPOINT))))))

# Generate a `build` target for each item in the BINARIES list.
BINARY_TARGETS := $(foreach ITEM,$(BINARIES),$(eval $(call parseBinary,$(ITEM))) build-$(NAME))

.PHONY: build-binaries
build-binaries: $(BINARY_TARGETS) ## Builds all go binaries.

.PHONY: clean
clean: ## Cleans output directory.
	@echo "$(ARROW) Cleaning all $(OUT_DIR)"
	rm -rf $(OUT_DIR)

# Due to https://github.com/golangci/golangci-lint/issues/580, we need to add --fix for windows
.PHONY: lint
lint: ## Runs golangci-lint
	$(GOLANGCI_LINT) run --fix --timeout 5m

.PHONY: build-bicep
build-bicep: build-bicep-$(GOOS)-$(GOARCH)

# Generate a target for the bicep container
# Params:
# $(1): the OS
# $(2): the ARCH
define generateBicepBuildTarget
.PHONY: build-bicep-$(1)-$(2)
build-bicep-$(1)-$(2):
	$(eval BINS_OUT_DIR_$(1)_$(2) := $(OUT_DIR)/$(1)_$(2)/$(BUILDTYPE_DIR))
	@echo "$(ARROW) Building bicep container on $(1)/$(2) to $(BINS_OUT_DIR_$(1)_$(2))/bicep"
	./build/install-bicep.sh $(REL_CHANNEL) $(BINS_OUT_DIR_$(1)_$(2))/bicep $(2)
endef

# Generate bicep build targets for each combination of OS and ARCH
$(foreach ARCH,$(GOARCHES),$(foreach OS,$(GOOSES),$(eval $(call generateBicepBuildTarget,$(OS),$(ARCH)))))
