# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

# Commit and release info is used by multiple categories of commands

GIT_COMMIT  = $(shell git rev-list -1 HEAD)
GIT_VERSION = $(shell git describe --always --abbrev=7 --dirty --tags)

# Azure Autorest require a --module-version, which is used
# as a telemetry key in the generated API client.
#
# It has the format major.minor.patch-beta.N.
AUTOREST_MODULE_VERSION = $(shell git describe --always --abbrev=0 --tags | sed "s/^v//")

REL_VERSION ?= edge
REL_CHANNEL ?= edge
