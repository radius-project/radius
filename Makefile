# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

ARROW := \033[34;1m=>\033[0m

# order matters for these
include build/help.mk build/version.mk build/build.mk build/util.mk build/generate.mk build/test.mk build/controller.mk build/git.mk build/docker.mk build/recipes.mk build/install.mk build/debug.mk
