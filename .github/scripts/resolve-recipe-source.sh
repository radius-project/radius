#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
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

# Select contrib main only for a new final; otherwise verify the reviewed SHA.
set -euo pipefail

root="${SIBLING_REPOSITORY_ROOT:-https://github.com/radius-project}"
repo="${root%/}/resource-types-contrib.git"
case "${1:-}:$#" in
    --capture:1)
        git ls-remote --exit-code "${repo}" refs/heads/main | cut -f1
        ;;
    --plan:2)
        commit="$(yq -o=json '.' "$2" | jq -er '
            select(.releaseType == "final") | .recipeSourceCommit |
            select(type == "string" and test("^[0-9a-f]{40}\\z"))
        ')" || {
            echo "Expected a final plan with a full recipeSourceCommit." >&2
            exit 1
        }
        scratch="$(mktemp -d)"
        trap 'rm -rf "${scratch}"' EXIT
        git init -q --bare "${scratch}"
        git -C "${scratch}" remote add origin "${repo}"
        git -C "${scratch}" fetch -q --no-tags --filter=tree:0 \
            origin refs/heads/main
        # Match an actual commit in main's history, not a peeled tag or ref.
        if ! git -C "${scratch}" rev-list FETCH_HEAD | grep -Fx "${commit}" > /dev/null; then
            echo "Recipe source ${commit} is not reachable from resource-types-contrib main." >&2
            exit 1
        fi
        printf '%s\n' "${commit}"
        ;;
    *)
        echo "Usage: resolve-recipe-source.sh --capture | --plan <final-plan.yaml>" >&2
        exit 1
        ;;
esac
