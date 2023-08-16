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

# does_tag_exist checks if a tag exists in the remote repository
function does_tag_exist() {
  if git ls-remote --tags origin $1 | grep -q $1; then
    true
  else
    false
  fi
}

# Comma-separated list of versions
# (e.g. v0.1.0,v0.2.0,v0.3.0)
VERSIONS=$1

if [[ -z "$VERSIONS" ]]; then
  echo "Error: VERSIONS is not set."
  exit 1
fi

RELEASE_VERSION=""
RELEASE_BRANCH_NAME=""

pushd radius
for VERSION in $(echo $VERSIONS | sed "s/,/ /g")
do
  # VERSION_NUMBER is the version number without the 'v' prefix (e.g. 0.1.0)
  VERSION_NUMBER=$(echo $VERSION | cut -d 'v' -f 2)

  # BRANCH_NAME should be the major and minor version of the VERSION_NUMBER prefixed by 'release/' (e.g. release/0.1)
  BRANCH_NAME="release/$(echo $VERSION_NUMBER | cut -d '.' -f 1,2)"

  if does_tag_exist $VERSION; then
    echo "Tag $VERSION already exists in the remote repository $REPOSITORY. Skipping..."
    exit 0
  elif [[ -z "$RELEASE_VERSION" ]]; then
    RELEASE_VERSION=$VERSION
    RELEASE_BRANCH_NAME=$BRANCH_NAME
  else
    echo "Error: Updating multiple versions at once is not supported."
    exit 1
  fi
done
popd

if [[ -z "$RELEASE_VERSION" ]]; then
  echo "Error: No release version found."
  exit 1
fi

if [[ -z "$RELEASE_BRANCH_NAME" ]]; then
  echo "Error: No release branch name found."
  exit 1
fi

echo "Release version: ${RELEASE_VERSION}"
echo "Release branch name: ${RELEASE_BRANCH_NAME}"
echo "release-version::$RELEASE_VERSION" >> $GITHUB_OUTPUT
echo "release-branch-name::$RELEASE_BRANCH_NAME" >> $GITHUB_OUTPUT
