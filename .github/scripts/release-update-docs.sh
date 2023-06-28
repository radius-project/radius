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
REPOSITORY="docs"

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

# In docs/config.toml, change baseURL to https://docs.radapp.dev/ instead of https://edge.docs.radapp.dev/
awk '{gsub(/baseURL = \"https:\/\/edge\.docs\.radapp.dev\/\"/,"baseURL = \"https:\/\/docs.radapp.dev\/\""); print}' docs/config.toml > docs/config.toml.tmp
mv docs/config.toml.tmp docs/config.toml

# In docs/config.toml, change version to VERSION instead of edge
VERSION_STRING_REPLACEMENT="version = \"${CHANNEL_VERSION}\""
awk -v REPLACEMENT="${VERSION_STRING_REPLACEMENT}" '{gsub(/version = \"edge\"/, REPLACEMENT); print}' docs/config.toml > docs/config.toml.tmp
mv docs/config.toml.tmp docs/config.toml

# In docs/config.toml, change chart_version (Helm chart) to VERSION_NUMBER
CHART_VERSION_STRING_REPLACEMENT="chart_version = \"${VERSION_NUMBER}\""
awk -v REPLACEMENT="${CHART_VERSION_STRING_REPLACEMENT}" '{gsub(/chart_version = \"[^\n]+\"/, REPLACEMENT); print}' docs/config.toml > docs/config.toml.tmp
mv docs/config.toml.tmp docs/config.toml

# In docs/layouts/partials/hooks/body-end.html, change indexName to radapp-dev instead of radapp-dev-edge
awk '{gsub(/indexName: '\''radapp-dev-edge'\''/, "indexName: '\''radapp-dev'\''"); print}' docs/layouts/partials/hooks/body-end.html > docs/layouts/partials/hooks/body-end.html.tmp
mv docs/layouts/partials/hooks/body-end.html.tmp docs/layouts/partials/hooks/body-end.html

# In docs/content/getting-started/install/index.md, update the binary download links with the new version number
BINARY_STRING_REPLACEMENT=": https:\/\/get\.radapp\.dev\/tools\/rad\/${CHANNEL}\/"
awk -v REPLACEMENT="${BINARY_STRING_REPLACEMENT}" '{gsub(/: https:\/\/get\.radapp\.dev\/tools\/rad\/[^\/]+\//, REPLACEMENT); print}' docs/content/getting-started/install/index.md > docs/content/getting-started/install/index.md.tmp
mv docs/content/getting-started/install/index.md.tmp docs/content/getting-started/install/index.md

git add --all
git commit -m "Update docs for ${VERSION}"
git push origin "${CHANNEL_VERSION}"

popd
