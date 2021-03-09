# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

################################################################################
# Variables                                                                    #
################################################################################

export GO111MODULE ?= on
export GOPROXY ?= https://proxy.golang.org
export GOSUMDB ?= sum.golang.org
GIT_COMMIT  = $(shell git rev-list -1 HEAD)
GIT_VERSION = $(shell git describe --always --abbrev=7 --dirty)
CGO			?= 0


WEBAPP_BINARY  = radius-rp
CLI_BINARY = rad

REL_VERSION ?= edge

LOCAL_ARCH := $(shell uname -m)
ifeq ($(LOCAL_ARCH),x86_64)
	TARGET_ARCH_LOCAL = amd64
else ifeq ($(shell echo $(LOCAL_ARCH) | head -c 5),armv8)
	TARGET_ARCH_LOCAL = arm64
else ifeq ($(shell echo $(LOCAL_ARCH) | head -c 4),armv)
	TARGET_ARCH_LOCAL = arm
else
	TARGET_ARCH_LOCAL = amd64
endif
export GOARCH ?= $(TARGET_ARCH_LOCAL)

LOCAL_OS := $(shell uname)
ifeq ($(LOCAL_OS),Linux)
   TARGET_OS_LOCAL = linux
   GOLANGCI_LINT:=golangci-lint
   export ARCHIVE_EXT = .tar.gz
else ifeq ($(LOCAL_OS),Darwin)
   TARGET_OS_LOCAL = darwin
   GOLANGCI_LINT:=golangci-lint
   export ARCHIVE_EXT = .tar.gz
else
   TARGET_OS_LOCAL ?= windows
   BINARY_EXT_LOCAL = .exe
   GOLANGCI_LINT:=golangci-lint.exe
   export ARCHIVE_EXT = .zip
endif
export GOOS ?= $(TARGET_OS_LOCAL)
export BINARY_EXT ?= $(BINARY_EXT_LOCAL)

# Use the variable H to add a header (equivalent to =>) to informational output
H = $(shell printf "\033[34;1m=>\033[0m")

ifeq ($(origin DEBUG), undefined)
  BUILDTYPE_DIR:=release
  GCFLAGS:=""
else ifeq ($(DEBUG),0)
  BUILDTYPE_DIR:=release
  GCFLAGS:=""
else
  BUILDTYPE_DIR:=debug
  GCFLAGS:="all=-N -l"
  $(info $(H) Build with debugger information)
endif

################################################################################
# Go build details                                                             #
################################################################################
OUT_DIR := ./dist

BINS_OUT_DIR := $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)
LDFLAGS := "-s -w -X main.version=$(REL_VERSION)"
GOPATH := $(shell go env GOPATH)

ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

################################################################################
# Docker build details                                                         #
################################################################################

DOCKER_REGISTRY?=$(shell whoami)
DOCKER_TAG_VERSION?=latest
DOCKER_IMAGE=$(DOCKER_REGISTRY)/radius-rp:$(DOCKER_TAG_VERSION)

################################################################################
# Target: build                                                                #
################################################################################
.PHONY: build
build: buildrp buildcli

################################################################################
# Target: build rp                                                             #
################################################################################
.PHONY: buildrp
buildrp: $(WEBAPP_BINARY)

$(WEBAPP_BINARY):
	$(info $(H) Building RP from 'cmd/rp/main.go')
	CGO_ENABLED=$(CGO) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build \
	-gcflags $(GCFLAGS) \
	-ldflags $(LDFLAGS) \
	-o $(BINS_OUT_DIR)/$(WEBAPP_BINARY)$(BINARY_EXT) \
	./cmd/rp/main.go;
	$(info $(H) Built RP in '$(BINS_OUT_DIR)/$(WEBAPP_BINARY)$(BINARY_EXT)')

################################################################################
# Target: build cli                                                            #
################################################################################
.PHONY: buildcli
buildcli: $(CLI_BINARY)

$(CLI_BINARY):
	$(info $(H) Building CLI from 'cmd/cli/main.go')
	CGO_ENABLED=$(CGO) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build \
	-gcflags $(GCFLAGS) \
	-ldflags $(LDFLAGS) \
	-o $(BINS_OUT_DIR)/$(CLI_BINARY)$(BINARY_EXT) \
	./cmd/cli/main.go;
	$(info $(H) Built CLI in '$(BINS_OUT_DIR)/$(CLI_BINARY)$(BINARY_EXT)')

################################################################################
# Target: generate                                                             #
################################################################################
.PHONY: generate
generate: download-controller-gen
	$(CONTROLLER_GEN) \
		object:headerFile="./boilerplate.go.txt" \
		paths="./pkg/apis/..."
	go generate -v ./... 

# find or download controller-gen
# download controller-gen if necessary
download-controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
		set -e ;\
		CONTROLLER_GEN_TMP_DIR="$$(mktemp -d)" ;\
		cd "$$CONTROLLER_GEN_TMP_DIR" ;\
		GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ; \
		rm -rf "$$CONTROLLER_GEN_TMP_DIR" ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

################################################################################
# Target: lint                                                                 #
################################################################################
# Due to https://github.com/golangci/golangci-lint/issues/580, we need to add --fix for windows
.PHONY: lint
lint:
	$(GOLANGCI_LINT) run --fix

################################################################################
# Target: test - unit testing                                                  #
################################################################################
.PHONY: test
test:
	go test ./pkg/...

################################################################################
# Target: e2e-tests - run nightly integration tests                                                  #
################################################################################
.PHONY: e2e-tests
e2e-tests:
	go test ./test/e2e-tests/... -timeout 900s

################################################################################
# Target: clean                                                                #
################################################################################
.PHONY: clean
clean:
	rm -rf $(OUT_DIR)

################################################################################
# Target: docker                                                               #
################################################################################
.PHONY: docker
docker:
	$(info $(H) Building image as '$(DOCKER_IMAGE)')
	docker build . -f ./deploy/rp/Dockerfile -t $(DOCKER_IMAGE)

################################################################################
# Target: dockerpush                                                           #
################################################################################
.PHONY: dockerpush
dockerpush:
	$(info $(H) Pushing image '$(DOCKER_IMAGE)')
	docker push $(DOCKER_IMAGE)

################################################################################
# Target: runmongo                                                             #
################################################################################
.PHONY: runmongo
runmongo:
	docker run \
	-d \
	-p 27017:27017 \
	--hostname mongo \
	-e MONGO_INITDB_ROOT_USERNAME=mongoadmin \
	-e MONGO_INITDB_ROOT_PASSWORD=secret \
	-e MONGO_INITDB_DATABASE=rpdb \
	mongo

################################################################################
# Target: deployment-script                                                    #
################################################################################
#
# This is useful for troubleshooting the deployment script locally
# 
# To use:
# - run this target - this will open a shell in a container
# - az login
# - az account set -s <subscription>
# - ./initialize-cluster.sh <resource-group> <cluster-name>
.PHONY: deployment-script
deployment-script:
	$(info $(H) Building image as 'deployment-script')
	docker build deploy/ -f ./deploy/deployment-script/Dockerfile -t deployment-script
	docker run \
	--rm \
	-it \
	-v ${HOME}/.ssh:/root/.ssh \
	--entrypoint '/bin/bash' \
	deployment-script
