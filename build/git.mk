# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

GIT_BINARY_URL?=https://github.com/git/git/archive/refs/tags/v2.40.1.tar.gz

# Downloads and builds Git binary
.PHONY: build-git
build-git:
#	@echo "$(ARROW) Downloading Git dependency - gettext"
#	mkdir -p /tmp/gitbinary/gettext
#	wget https://ftp.gnu.org/pub/gnu/gettext/gettext-0.21.1.tar.gz
#	tar xzf gettext-0.21.1.tar.gz
#	rm -f gettext-0.21.1.tar.gz
#	cd gettext-0.21.1  && \
		make configure && \
		./configure --prefix=/usr && \
		make && \
		sudo make install
#		./configure --prefix=/tmp/gitbinary/gettext && \
#	cp /tmp/gitbinary/gettext/bin/msgfmt /usr/bin/

	@echo "$(ARROW) Downloading Git binary"
	wget $(GIT_BINARY_URL) -O git.tar.gz
	tar xzf git.tar.gz
	rm -f git.tar.gz
	@echo "$(ARROW) Building Git binary"
	cd git-2.40.1 && \
		make configure && \
		./configure --prefix=/tmp/gitbinary && \
		make NO_GETTEXT=1 && \
		sudo make install NO_GETTEXT=1
#	make all doc info && \
#	sudo make install install-doc install-html install-info
# ./configure --prefix=/tmp/gitbinary CPPFLAGS="-I/tmp/gitbinary/gettext/include" LDFLAGS="-L/tmp/gitbinary/gettext/lib" && \
#		make -i all && \
#		sudo make -i install
	mkdir -p $(OUT_DIR)/gitbinary
	cp /tmp/gitbinary/bin/git $(OUT_DIR)/gitbinary/
	chmod +x $(OUT_DIR)/gitbinary/git
	sudo rm -rf /tmp/gitbinary
	@echo "$(ARROW) Copied Git binary to $(OUT_DIR)/gitbinary/"