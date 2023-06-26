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
MAIN_BRANCH=$2
VERSION=$3
FINAL_RELEASE=$4

if [[ -z "$REPOSITORY" ]]; then
  echo "Error: REPOSITORY is not set."
  exit 1
fi

if [[ -z "$MAIN_BRANCH" ]]; then
  echo "Error: MAIN_BRANCH is not set."
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  echo "Error: VERSION is not set."
  exit 1
fi

if [[ -z "$FINAL_RELEASE" ]]; then
  echo "Error: FINAL_RELEASE is not set."
  exit 1
fi

# VERSION_NUMBER is the version number without the 'v' prefix (e.g. 0.1.0)
VERSION_NUMBER=$(echo $VERSION | cut -d 'v' -f 2)

# RELEASE_BRANCH_NAME should be the major and minor version of the VERSION_NUMBER prefixed by 'release/' (e.g. release/0.1)
RELEASE_BRANCH_NAME="release/$(echo $VERSION_NUMBER | cut -d '.' -f 1,2)"

# TAG_NAME should be the version (e.g. v0.1.0)
TAG_NAME=$VERSION

echo "Version: ${VERSION}"
echo "Version number: ${VERSION_NUMBER}"
echo "Release branch name: ${RELEASE_BRANCH_NAME}"
echo "Tag name: ${TAG_NAME}"
echo "Final release: ${FINAL_RELEASE}"

echo "Creating release branches and tags for ${REPOSITORY}..."

pushd $REPOSITORY
RELEASE_BRANCH_EXISTS=$(git ls-remote --heads origin refs/heads/$RELEASE_BRANCH_NAME | grep refs/heads/$RELEASE_BRANCH_NAME > /dev/null)
if [ "$?" == "1" ]; then
  echo "Creating release branch ${RELEASE_BRANCH_NAME}..."
  git checkout -b $RELEASE_BRANCH_NAME
  # git push origin $RELEASE_BRANCH_NAME
else
  echo "Release branch ${RELEASE_BRANCH_NAME} already exists. Checking out..."
  git fetch origin $RELEASE_BRANCH_NAME
  git checkout --track origin/$RELEASE_BRANCH_NAME
fi
git tag $TAG_NAME
# git push origin --tags
popd
