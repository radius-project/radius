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

# Fail immediately if any command fails
set -e

# Get command line arguments
DIRECTORY=$1
REGISTRY_PATH=$2
RECIPE_VERSION=$3

# Print usage information
function print_usage() {
    echo "Usage: $0 <DIRECTORY> <REGISTRY_PATH> <RECIPE_VERSION>"
    echo ""
    echo "  Publishes all recipes in the repository to a container registry."
    echo ""
    echo "  DIRECTORY: Directory containing the recipes to publish. For example, ./test/functional/testdata/recipes"
    echo "  REGISTRY_PATH: Registry hostname and path prefix. For example, ghcr.io/radius-project/dev/test/recipes"
    echo "  RECIPE_VERSION: Version of the recipe to publish. For example, pr-19293"
    echo ""
}

# Verify that the required arguments are present
if [[ $# -ne 3 ]]; then
    echo "Error: Missing required arguments"
    echo ""
    print_usage
    exit 1
fi

# We create output that's intended to be consumed by the GitHub Action summary. If we're
# not running in a GitHub Action, we'll just silence the output.
if [[ -z "$GITHUB_STEP_SUMMARY" ]]; then
    GITHUB_STEP_SUMMARY=/dev/null
fi

echo "## Recipes published to $REGISTRY_PATH" >>$GITHUB_STEP_SUMMARY

for RECIPE in $(find "$DIRECTORY" -type f -name "*.bicep"); do
    FILENAME=$(basename $RECIPE)
    PUBLISH_REF="$REGISTRY_PATH/${FILENAME%.*}:$RECIPE_VERSION"

    # Skip files that start with _. These are not recipes, they are modules that are
    # used by the recipes.
    if [[ $(basename $RECIPE) =~ ^_.* ]]; then
        echo "Skipping $RECIPE"
        continue
    fi

    echo "Publishing $RECIPE to $PUBLISH_REF"
    echo "- $PUBLISH_REF" >>$GITHUB_STEP_SUMMARY
    rad bicep publish --file $RECIPE --target "br:$PUBLISH_REF"
done
