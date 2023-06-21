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

# Accept the version as an input (e.g. 0.1.0-rc.1)
VERSION=$1

DRY_RUN=$2

if [[ -d "radius" ]]; then
  echo "radius directory exists"
else
  echo "Error: radius directory does not exist. Please clone project-radius/radius repository."
  exit 1
fi

if [[ -d "bicep" ]]; then
  echo "bicep directory exists"
else
  echo "Error: bicep directory does not exist. Please clone project-radius/bicep repository."
  exit 1
fi

if [[ -d "deployment-engine" ]]; then
  echo "deployment-engine directory exists"
else
  echo "Error: deployment-engine directory does not exist. Please clone project-radius/deployment-engine repository."
  exit 1
fi

# If version is not provided, exit
if [[ -z "$VERSION" ]]; then
    echo "Error: No version provided. Please provide a valid semver (e.g. 0.1.0 or 0.1.0-rc1)."
    exit 1
fi

# If the version is not a valid semver, exit
if ! [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
    echo "Error: Invalid version provided. Please provide a valid semver (e.g. 0.1.0 or 0.1.0-rc1)."
    exit 1
fi

# Release branch name should be the major and minor version of the VERSION
RELEASE_BRANCH_NAME="release/$(echo $VERSION | cut -d '.' -f 1,2)"

# Tag name should be the version prefixed with 'v'
TAG_NAME="v${VERSION}"

# This is a final release if the version doesn't contain 'rc'
FINAL_RELEASE=$(echo $VERSION | grep -v "rc")

# For each of the repositories, create the release branch and tag
repositories=("radius" "bicep" "deployment-engine")
for repository in "${repositories[@]}"; do
    if [[ -z "$DRY_RUN" ]]; then
        pushd "${repository}"
        git checkout -B "${RELEASE_BRANCH_NAME}"
        git pull origin "${RELEASE_BRANCH_NAME}"
        git tag "${TAG_NAME}"
        git push origin --tags
        git push origin "${RELEASE_BRANCH_NAME}"
        popd
    else 
        echo "Dry run: pushd ${repository}"
        echo "Dry run [project-radius/${repository}]: git checkout -B ${RELEASE_BRANCH_NAME}"
        echo "Dry run [project-radius/${repository}]: git pull origin ${RELEASE_BRANCH_NAME}"
        echo "Dry run [project-radius/${repository}]: git tag ${TAG_NAME}"
        echo "Dry run [project-radius/${repository}]: git push origin --tags"
        echo "Dry run [project-radius/${repository}]: git push origin ${RELEASE_BRANCH_NAME}"
        echo "Dry run: popd"
    fi
done

