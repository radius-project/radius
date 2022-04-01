# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Install

RAD_LOCATION := $(shell which rad)

.PHONY: install
install: build-binaries ## Installs a local build for development

	@echo "$(ARROW) Installing rad"
	cp $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)/rad$(BINARY_EXT) $(RAD_LOCATION)

	@echo "$(ARROW) Displaying output"
	tree $(HOME)/.rad -a