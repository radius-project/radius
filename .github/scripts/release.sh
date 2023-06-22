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

# does_tag_exist checks if a tag exists in the remote repository
function does_tag_exist() {
  if git ls-remote --tags origin $1 | grep -q $1; then
    true
  else
    false
  fi
}

# Ensure the project-radius/radius repo is cloned
echo "Checking if repositories are cloned..."
if [[ -d "radius" ]]; then
  echo "radius directory exists"
else
  echo "Error: radius directory does not exist. Please clone project-radius/radius repository."
  exit 1
fi

# Ensure the project-radius/bicep repo is cloned
if [[ -d "bicep" ]]; then
  echo "bicep directory exists"
else
  echo "Error: bicep directory does not exist. Please clone project-radius/bicep repository."
  exit 1
fi

# Ensure the project-radius/deployment-engine repo is cloned
if [[ -d "deployment-engine" ]]; then
  echo "deployment-engine directory exists"
else
  echo "Error: deployment-engine directory does not exist. Please clone project-radius/deployment-engine repository."
  exit 1
fi

# Set GitHub username and email
git config --global user.name "Radius CI Bot"
git config --global user.email "radiuscoreteam@service.microsoft.com"

# Get the versions from the versions.yaml file
echo "Getting versions from versions.yaml..."
STABLE_VERSION=$(awk '/^stable:/ {getline; print}' ./radius/versions.yaml | awk '{print $2}')
LATEST_VERSION=$(awk '/^latest:/ {getline; print}' ./radius/versions.yaml | awk '{print $2}')
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

# VERSION_NUMBER is the version number without the 'v' prefix (e.g. 0.1.0)
VERSION_NUMBER=$(echo $VERSION | cut -d 'v' -f 2)

# RELEASE_BRANCH_NAME should be the major and minor version of the VERSION_NUMBER prefixed by 'release/' (e.g. release/0.1)
RELEASE_BRANCH_NAME="release/$(echo $VERSION_NUMBER | cut -d '.' -f 1,2)"

# TAG_NAME should be the version (e.g. v0.1.0)
TAG_NAME="${VERSION}"

# Print the release information
echo "Version: ${VERSION}"
echo "Version number: ${VERSION_NUMBER}"
echo "Release branch name: ${RELEASE_BRANCH_NAME}"
echo "Tag name: ${TAG_NAME}"
echo "Final release: ${FINAL_RELEASE}"

# For each of the repositories, create the release branch and tag
REPOSITORIES=("radius" "bicep" "deployment-engine")
MAIN_BRANCHES=("main" "bicep-extensibility" "main")

echo "Creating release branches and tags..."
for i in "${!REPOSITORIES[@]}"; do
  REPOSITORY="${REPOSITORIES[$i]}"
  MAIN_BRANCH="${MAIN_BRANCHES[$i]}"
  echo "Creating release branches and tags for ${REPOSITORY}..."
  COMMANDS=""
  COMMANDS+="pushd ${REPOSITORY}\n"
  pushd "${REPOSITORY}"
  COMMANDS+="git checkout ${MAIN_BRANCH}\n"
  git checkout "${MAIN_BRANCH}"
  COMMANDS+="git pull origin ${MAIN_BRANCH}\n"
  git pull origin "${MAIN_BRANCH}"
  COMMANDS+="git checkout -B ${RELEASE_BRANCH_NAME}\n"
  git checkout -B "${RELEASE_BRANCH_NAME}"
  COMMANDS+="git pull origin ${RELEASE_BRANCH_NAME}\n"
  git pull origin "${RELEASE_BRANCH_NAME}"
  COMMANDS+="git tag ${TAG_NAME}\n"
  git tag "${TAG_NAME}"
  COMMANDS+="git push origin --tags\n"
  git push origin --tags
  COMMANDS+="git push origin ${RELEASE_BRANCH_NAME}\n"
  git push origin "${RELEASE_BRANCH_NAME}"
  COMMANDS+="popd"
  popd
  echo "\nCommands Run:\n----------\n${COMMANDS}\n----------\n"
done
