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

BASE_PACKAGE_NAME := github.com/project-radius/radius
OUT_DIR := ./dist

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOPATH := $(shell go env GOPATH)

ifeq (,$(shell go env GOBIN))
	GOBIN=$(shell go env GOPATH)/bin
else
	GOBIN=$(shell go env GOBIN)
endif

ifeq ($(GOOS),windows)
   BINARY_EXT = .exe
   GOLANGCI_LINT:=golangci-lint.exe
else
   GOLANGCI_LINT:=golangci-lint
endif

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

LDFLAGS := "-s -w -X $(BASE_PACKAGE_NAME)/pkg/version.channel=$(REL_CHANNEL) -X $(BASE_PACKAGE_NAME)/pkg/version.release=$(REL_VERSION) -X $(BASE_PACKAGE_NAME)/pkg/version.commit=$(GIT_COMMIT) -X $(BASE_PACKAGE_NAME)/pkg/version.version=$(GIT_VERSION) -X $(BASE_PACKAGE_NAME)/pkg/version.chartVersion=$(CHART_VERSION)"
GOARGS := -v -gcflags $(GCFLAGS) -ldflags $(LDFLAGS)

export GO111MODULE ?= on
export GOPROXY ?= https://proxy.golang.org
export GOSUMDB ?= sum.golang.org
export CGO_ENABLED=0

##@ Build

.PHONY: build
build: build-packages build-binaries ## Build all go targets.

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
# $(2): the binary main directory
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
define generatePlatformBuildTarget
.PHONY: build-$(3)-$(1)-$(2)
build-$(3)-$(1)-$(2):
  $(eval BINS_OUT_DIR_$(1)_$(2) := $(OUT_DIR)/$(1)_$(2)/$(BUILDTYPE_DIR))
	@echo "$(ARROW) Building $(3) on $(1)/$(2) to $(BINS_OUT_DIR_$(1)_$(2))/$(3)$(BINARY_EXT)"
	GOOS=$(1) GOARCH=$(2) go build \
		-v \
		-gcflags $(GCFLAGS) \
		-ldflags=$(LDFLAGS) \
		-o $(BINS_OUT_DIR_$(1)_$(2))/$(3)$(BINARY_EXT) \
		$(4)/
endef

# defines a target for each binary
GOOSES := darwin linux windows
GOARCHES := amd64 arm arm64
BINARIES := docgen rad appcore-rp applink-rp ucp ucpd
$(foreach ITEM,$(BINARIES),$(eval $(call generateBuildTarget,$(ITEM),./cmd/$(ITEM))))
$(foreach ARCH,$(GOARCHES),$(foreach OS,$(GOOSES),$(foreach ITEM,$(BINARIES),$(eval $(call generatePlatformBuildTarget,$(OS),$(ARCH),$(ITEM),./cmd/$(ITEM))))))

# list of 'outputs' to build for all binaries
BINARY_TARGETS:=$(foreach ITEM,$(BINARIES),build-$(ITEM))

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

