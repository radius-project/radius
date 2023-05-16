# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

GIT_BINARY_URL?=https://github.com/git/git/archive/refs/tags/v2.40.1.tar.gz

# Downloads and builds Git binary
.PHONY: build-git
build-git:
	@echo "$(ARROW) Downloading Git binary"
	wget $(GIT_BINARY_URL) -O git.tar.gz
	tar xzf git.tar.gz
	rm -f git.tar.gz
	mkdir -p /tmp/gitbinary
	@echo "$(ARROW) Building Git binary"
	cd git-2.40.1 && \
		make configure && \
		./configure --prefix=/tmp/gitbinary && \
		make NO_GETTEXT=1 && \
		sudo make install NO_GETTEXT=1
	mkdir -p $(OUT_DIR)/gitbinary
	cp /tmp/gitbinary/bin/git $(OUT_DIR)/gitbinary/
	chmod +x $(OUT_DIR)/gitbinary/git
	sudo rm -rf /tmp/gitbinary