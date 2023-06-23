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

set -x

# does_tag_exist checks if a tag exists in the remote repository
function does_tag_exist() {
  if git ls-remote --tags origin $1 | grep -q $1; then
    true
  else
    false
  fi
}

# Ensure the project-radius/radius repo is cloned
if [[ -d "radius" ]]; then
  echo "radius directory exists"
else
  echo "Error: radius directory does not exist. Please clone project-radius/radius repository."
  exit 1
fi

VERSION_FILE_PATH="./radius/versions.json"

# Get the versions from the versions.json file
echo "Getting versions from ${VERSION_FILE_PATH}..."

STABLE_VERSION=$(cat ${VERSION_FILE_PATH} | jq -r '.stable.version')
LATEST_VERSION=$(cat ${VERSION_FILE_PATH} | jq -r '.latest.version')

if [[ -z "$STABLE_VERSION" ]]; then
  echo "Error: stable version not found. Please check versions.json."
  exit 1
fi

if [[ -z "$LATEST_VERSION" ]]; then
  echo "Error: latest version not found. Please check versions.json."
  exit 1
fi

echo "Stable version: ${STABLE_VERSION}"
echo "Latest version: ${LATEST_VERSION}"

# FINAL_RELEASE marks whether or not this is a final release
FINAL_RELEASE="false"

# VERSION is the new tag version to create
# this will be populated with either the stable or latest version
VERSION=""

# Check the existing tags from GitHub to determine if stable or latest version changed
# Note that we shouldn't be changing both at the same time. If we do, we'll use the stable version
echo "Checking if ${LATEST_VERSION} tag exists..."
pushd "radius"
if does_tag_exist "${LATEST_VERSION}"; then
  echo "Latest version tag ${LATEST_VERSION} already exists."
else
  echo "Latest version tag ${LATEST_VERSION} does not exist."
  VERSION="${LATEST_VERSION}"
  FINAL_RELEASE="false"
fi

echo "Checking if ${STABLE_VERSION} tag exists..."
if does_tag_exist "${STABLE_VERSION}"; then
  echo "Stable version tag ${STABLE_VERSION} already exists."
else
  echo "Latest version tag ${STABLE_VERSION} does not exist."
  VERSION="${STABLE_VERSION}"
  FINAL_RELEASE="true"
fi
popd

# If the version is empty, then we have an error
if [[ -z "$VERSION" ]]; then
  echo "Error: new version not found. Please check versions.yaml."
  exit 1
fi

# Print the release information
echo "Release Version: ${VERSION}"

# Write the release information to GITHUB_ENV
echo "RELEASE_VERSION=${VERSION}" >> $GITHUB_ENV
