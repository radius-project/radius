# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Install

RAD_LOCATION := $(shell which rad)

.PHONY: install
install: build-binaries ## Installs a local build for development
	mkdir -p $(HOME)/.rad/{crd,bin}

	@echo "$(ARROW) Installing rad"
	cp $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)/rad$(BINARY_EXT) $(RAD_LOCATION)

	@echo "$(ARROW) Installing radiusd"
	cp $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)/radiusd$(BINARY_EXT) $(HOME)/.rad/bin/
	@echo "$(ARROW) Installing radius-controller"
	cp $(OUT_DIR)/$(GOOS)_$(GOARCH)/$(BUILDTYPE_DIR)/radius-controller$(BINARY_EXT) $(HOME)/.rad/bin/

	@echo "$(ARROW) Installing CRDs"
	rm $(HOME)/.rad/crd/*
	cp -R deploy/Chart/CRDs/ $(HOME)/.rad/crd/

	@echo "$(ARROW) Displaying output"
	tree $(HOME)/.rad -a