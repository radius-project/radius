#!/bin/bash

# ------------------------------------------------------------
# Copyright 2025 The Radius Authors.
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

set -xe

# Docker tag version to be copied
# (e.g. 0.1, 0.1.0-rc1)
DOCKER_TAG_VERSION=$1

if [[ -z "$DOCKER_TAG_VERSION" ]]; then
  echo "Error: DOCKER_TAG_VERSION is not set."
  exit 1
fi

docker pull radiusdeploymentengine.azurecr.io:"$DOCKER_TAG_VERSION"
docker tag radiusdeploymentengine.azurecr.io:"$DOCKER_TAG_VERSION" ghcr.io/radius-project/deployment-engine:"$DOCKER_TAG_VERSION"
docker push ghcr.io/radius-project/deployment-engine:"$DOCKER_TAG_VERSION"
