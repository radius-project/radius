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

set -euo pipefail

# Print usage information
function print_usage() {
    echo "Usage: $0 <DIRECTORY> <REGISTRY_PATH> <RECIPE_VERSION>"
    echo ""
    echo "  Publishes all recipes in the repository to a container registry."
    echo ""
    echo "  DIRECTORY: Directory containing the recipes to publish. For example, ./test/functional/testdata/recipes"
    echo "  REGISTRY_PATH: Registry hostname and path prefix. For example, ghcr.io/radius-project/dev/test/recipes"
    echo "  RECIPE_VERSION: Version of the recipe to publish. For example, pr-19293"
    echo "  BICEP_RECIPE_PUBLISH_PARALLELISM: Optional worker limit (1-4); defaults to 1."
    echo ""
}

# Verify that the required arguments are present
if [[ $# -ne 3 ]]; then
    echo "Error: Missing required arguments"
    echo ""
    print_usage
    exit 1
fi

DIRECTORY=$1
REGISTRY_PATH=$2
RECIPE_VERSION=$3
parallelism="${BICEP_RECIPE_PUBLISH_PARALLELISM:-1}"
case "${parallelism}" in
1 | 2 | 3 | 4) ;;
*)
    echo "Error: BICEP_RECIPE_PUBLISH_PARALLELISM must be 1, 2, 3, or 4." >&2
    exit 2
    ;;
esac
if [[ ! -d "${DIRECTORY}" ]]; then
    echo "Error: Recipe directory does not exist: ${DIRECTORY}" >&2
    exit 1
fi
publish_pids=()
publish_refs=()
wait_publish_workers() {
    local pid status=0
    for pid in "${publish_pids[@]}"; do
        wait "${pid}" || status=$?
    done
    return "${status}"
}
trap 'trap "" INT TERM; wait' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# We create output that's intended to be consumed by the GitHub Action summary. If we're
# not running in a GitHub Action, we'll just silence the output.
GITHUB_STEP_SUMMARY="${GITHUB_STEP_SUMMARY:-/dev/null}"

echo "## Recipes published to $REGISTRY_PATH" >>"$GITHUB_STEP_SUMMARY"

while IFS= read -r -d '' RECIPE; do
    FILENAME=$(basename "$RECIPE")
    PUBLISH_REF="$REGISTRY_PATH/${FILENAME%.*}:$RECIPE_VERSION"

    # Skip files that start with _. These are not recipes, they are modules that are
    # used by the recipes.
    if [[ "${FILENAME}" == _* ]]; then
        echo "Skipping $RECIPE"
        continue
    fi

    if ((parallelism > 1)); then
        for ref in "${publish_refs[@]}"; do
            if [[ "${ref}" == "${PUBLISH_REF}" ]]; then
                echo "Error: Duplicate publish reference: ${PUBLISH_REF}" >&2
                exit 2
            fi
        done
        publish_refs+=("${PUBLISH_REF}")
    fi
    if ((${#publish_pids[@]} == parallelism)); then
        wait "${publish_pids[0]}"
        publish_pids=("${publish_pids[@]:1}")
    fi
    echo "Publishing $RECIPE to $PUBLISH_REF"
    echo "- $PUBLISH_REF" >>"$GITHUB_STEP_SUMMARY"
    if ((parallelism == 1)); then
        rad bicep publish --file "$RECIPE" --target "br:$PUBLISH_REF"
    else
        rad bicep publish --file "$RECIPE" --target "br:$PUBLISH_REF" &
        publish_pids+=("$!")
    fi
done < <(find "$DIRECTORY" -type f -name "*.bicep" -print0)
wait_publish_workers
trap - EXIT INT TERM
