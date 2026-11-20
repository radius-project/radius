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

##@ Install

# Select the installer's host default, independently of the build target GOOS.
ifeq ($(OS),Windows_NT)
RAD_LOCATION ?= $(subst \,/,$(LOCALAPPDATA))/radius/rad.exe
else ifeq ($(shell id -u),0)
RAD_LOCATION ?= /usr/local/bin/rad
else
RAD_LOCATION ?= $(HOME)/.local/bin/rad
endif

.PHONY: install
install: build-binaries ## Installs a local build for development
	@echo "$(ARROW) Installing rad"
	mkdir -p "$$(dirname "$(RAD_LOCATION)")"
	cp "$(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)/rad$(BINARY_EXT)" "$(RAD_LOCATION)"
