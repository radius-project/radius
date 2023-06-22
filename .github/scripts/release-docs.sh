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

# Ensure the project-radius/docs repo is cloned
if [[ -d "docs" ]]; then
  echo "docs directory exists"
else
  echo "Error: docs directory does not exist. Please clone project-radius/docs repository."
  exit 1
fi

VERSION=$1 # v0.21.0 or v0.21.0-rc1
VERSION_NUMBER=$2 # 0.21.0 or 0.21.0-rc1
CHANNEL=$3 # 0.21
CHANNEL_WITH_V="v${CHANNEL}" # v0.21

pushd "docs"

git checkout edge
git checkout -B "${CHANNEL_WITH_V}"

# In docs/config.toml, change baseURL to https://docs.radapp.dev/ instead of https://edge.docs.radapp.dev/
awk '{gsub(/baseURL = \"https:\/\/edge\.docs\.radapp.dev\/\"/,"baseURL = \"https:\/\/docs.radapp.dev\/\""); print}' docs/config.toml > docs/config.toml.tmp
mv docs/config.toml.tmp docs/config.toml

# In docs/config.toml, change version to VERSION instead of edge
VERSION_STRING_REPLACEMENT="version = \"${CHANNEL_WITH_V}\""
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
git push origin "${CHANNEL_WITH_V}"

popd
