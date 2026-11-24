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

# Read the reviewed pin without converting ambiguous YAML keys to one value.
set -euo pipefail

root="${SIBLING_REPOSITORY_ROOT:-https://github.com/radius-project}"
repo="${root%/}/resource-types-contrib.git"
commit="$(yq -o=json -I=0 'to_entries' "${1:?Expected a final plan}" | jq -ers '
    select(length == 1) | .[0] | select(length == (unique_by(.key) | length)) |
    from_entries | select(.releaseType == "final") | .recipeSourceCommit |
    strings | select(test("^[0-9a-f]{40}\\z"))
')" || {
    echo "Expected one unambiguous final plan with a full recipeSourceCommit." >&2
    exit 1
}
scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT
git init -q --bare "${scratch}"
git -C "${scratch}" remote add origin "${repo}"
git -C "${scratch}" fetch -q --no-tags --filter=tree:0 origin refs/heads/main
if ! git -C "${scratch}" rev-list FETCH_HEAD | grep -Fx "${commit}" > /dev/null; then
    echo "Recipe source ${commit} is not reachable from resource-types-contrib main." >&2
    exit 1
fi
printf '%s\n' "${commit}"
