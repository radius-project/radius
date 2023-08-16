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

# Repo to create tags and branches for (e.g. radius)
REPOSITORY=$1

# Tag name (e.g. v0.1.0)
TAG_NAME=$2

# Release branch name (e.g. release/0.1)
RELEASE_BRANCH_NAME=$3

if [[ -z "$REPOSITORY" ]]; then
  echo "Error: REPOSITORY is not set."
  exit 1
fi

if [[ -z "$TAG_NAME" ]]; then
  echo "Error: TAG_NAME is not set."
  exit 1
fi

if [[ -z "$RELEASE_BRANCH_NAME" ]]; then
  echo "Error: RELEASE_BRANCH_NAME is not set."
  exit 1
fi

echo "Creating release branch and tags for ${REPOSITORY}..."
pushd $REPOSITORY
RELEASE_BRANCH_EXISTS=$(git ls-remote --heads origin refs/heads/$RELEASE_BRANCH_NAME)
if [ -z "$RELEASE_BRANCH_EXISTS" ]; then
  echo "Creating release branch ${RELEASE_BRANCH_NAME}..."
  git checkout -b $RELEASE_BRANCH_NAME
  git push origin $RELEASE_BRANCH_NAME
else
  echo "Release branch ${RELEASE_BRANCH_NAME} already exists. Checking out..."
  git fetch origin $RELEASE_BRANCH_NAME
  git checkout --track origin/$RELEASE_BRANCH_NAME
fi
echo "Creating tag ${TAG_NAME}..."
git tag $TAG_NAME
git push origin --tags
popd
