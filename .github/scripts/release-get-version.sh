#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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

REPOSITORY=$1

if [[ -z "$REPOSITORY" ]]; then
  echo "Error: REPOSITORY is not set."
  exit 1
fi

VERSION_FILE_PATH="./${REPOSITORY}/versions.yaml"

# Get the version from the versions.yaml file
echo "Getting versions from ${VERSION_FILE_PATH}..."

VERSION=$(cat ${VERSION_FILE_PATH} | yq '.supported[0].version')

if [[ -z "$VERSION" ]]; then
  echo "Error: version not found. Please check versions.yaml."
  exit 1
fi

# FINAL_RELEASE marks whether or not this is a final release
FINAL_RELEASE="false"

# Check if the version is a final release
if [[ $VERSION == *"-"* ]]; then
  FINAL_RELEASE="false"
else
  FINAL_RELEASE="true"
fi

# Print the release information
echo "Release Version: ${VERSION}"
echo "Final Release: ${FINAL_RELEASE}"

# Write the release information to GITHUB_ENV
echo "RELEASE_VERSION=${VERSION}" >> $GITHUB_ENV
echo "FINAL_RELEASE=${FINAL_RELEASE}" >> $GITHUB_ENV
