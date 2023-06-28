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

VERSION=$1 # (e.g. v0.1.0)
REPOSITORY="samples"

if [[ -z "$VERSION" ]]; then
  echo "Error: VERSION is not set."
  exit 1
fi

# VERSION_NUMBER is the version without the 'v' prefix (e.g. 0.1.0)
VERSION_NUMBER=$(echo $VERSION | cut -d 'v' -f 2)

# CHANNEL is the major and minor version of the VERSION_NUMBER (e.g. 0.1)
CHANNEL="$(echo $VERSION_NUMBER | cut -d '.' -f 1,2)"

# CHANNEL_VERSION is the version with the 'v' prefix (e.g. v0.1)
CHANNEL_VERSION="v${CHANNEL}"

echo "Version: ${VERSION}"
echo "Version number: ${VERSION_NUMBER}"
echo "Channel: ${CHANNEL}"
echo "Channel version: ${CHANNEL_VERSION}"

echo "Creating release branch for ${REPOSITORY}..."

pushd $REPOSITORY
git checkout -B "${CHANNEL_VERSION}"
git push origin "${CHANNEL_VERSION}"
popd
