# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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