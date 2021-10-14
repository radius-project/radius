# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

BASE_PACKAGE_NAME := github.com/Azure/radius
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

BINS_OUT_DIR := $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)

LDFLAGS := "-s -w -X $(BASE_PACKAGE_NAME)/pkg/version.channel=$(REL_CHANNEL) -X $(BASE_PACKAGE_NAME)/pkg/version.release=$(REL_VERSION) -X $(BASE_PACKAGE_NAME)/pkg/version.commit=$(GIT_COMMIT) -X $(BASE_PACKAGE_NAME)/pkg/version.version=$(GIT_VERSION)"
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
build-$(1):
	@echo "$(ARROW) Building $(1) to $(BINS_OUT_DIR)/$(1)$(BINARY_EXT)"
	go build \
		-v \
		-gcflags $(GCFLAGS) \
		-ldflags=$(LDFLAGS) \
		-o $(BINS_OUT_DIR)/$(1)$(BINARY_EXT) \
		$(2)/
endef

# defines a target for each binary
BINARIES := docgen rad radius-controller radius-rp testenv
$(foreach ITEM,$(BINARIES),$(eval $(call generateBuildTarget,$(ITEM),./cmd/$(ITEM))))

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
