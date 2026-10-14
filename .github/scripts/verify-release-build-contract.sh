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

# A pushed tag runs the tagged commit's workflows, not those on main.
set -euo pipefail

readonly BUILD_WORKFLOW=".github/workflows/build-release.yaml"
readonly -a REQUIRED_JOBS=(release-preflight goreleaser-release finalize-release)
COMMIT=""

fail() {
    echo "Error: $*" >&2
    exit 1
}

reject() {
    fail "release commit ${COMMIT} $*; migrate its release branch to the current release pipeline first"
}

yq_true() {
    [[ "$(git show "${COMMIT}:$1" 2>/dev/null | yq "$2" 2>/dev/null)" == "true" ]]
}

main() {
    local job name

    [[ $# -eq 1 ]] || fail "usage: verify-release-build-contract.sh <commit>"
    command -v yq >/dev/null || fail "required command not found: yq"
    COMMIT="$(git rev-parse --verify --quiet "$1^{commit}")" ||
        fail "release commit does not exist: $1"

    git cat-file -e "${COMMIT}:.goreleaser.yaml" 2>/dev/null ||
        reject "has no .goreleaser.yaml"
    git cat-file -e "${COMMIT}:${BUILD_WORKFLOW}" 2>/dev/null ||
        reject "has no ${BUILD_WORKFLOW}"
    yq_true "${BUILD_WORKFLOW}" '[.on.push.tags[]? | select(. == "v*")] | length > 0' ||
        reject "does not run ${BUILD_WORKFLOW} on v* tags"
    for job in "${REQUIRED_JOBS[@]}"; do
        yq_true "${BUILD_WORKFLOW}" ".jobs | has(\"${job}\")" ||
            reject "has no ${job} job in ${BUILD_WORKFLOW}"
    done

    # Path-only push triggers also run for tags; only a tag filter marks a publisher.
    while IFS= read -r name; do
        [[ "${name}" =~ \.ya?ml$ && ".github/workflows/${name}" != "${BUILD_WORKFLOW}" ]] ||
            continue
        if yq_true ".github/workflows/${name}" '(.on | tag) == "!!map" and
            ((.on.push // {}) | tag) == "!!map" and
            ((.on.push // {}) | (has("tags") or has("tags-ignore")))'; then
            reject "also runs .github/workflows/${name} on tag pushes"
        fi
    done < <(git ls-tree --name-only "${COMMIT}:.github/workflows")

    echo "Release commit ${COMMIT} starts ${BUILD_WORKFLOW} on its tag."
}

main "$@"
