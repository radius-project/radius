# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
# http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

ARROW := \033[34;1m=>\033[0m

# order matters for these
include build/help.mk build/version.mk build/build.mk build/util.mk build/generate.mk build/test.mk build/controller.mk build/git.mk build/docker.mk build/recipes.mk build/install.mk build/debug.mk
