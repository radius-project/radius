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
# It has the format ${major}.${minor}.${patch}[-beta.${N}], where
# all ${major}, ${minor}, ${patch}, ${N} are numbers.
#
# Currently we don't use this yet, so just setting to 0.0.1 to
# make autorest happy.
AUTOREST_MODULE_VERSION = 0.0.1

REL_VERSION ?= edge
REL_CHANNEL ?= edge
CHART_VERSION ?= 0.42.42-dev