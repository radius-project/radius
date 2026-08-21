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

# A versions.yaml change first lands on main and is later cherry-picked to an
# existing release branch. Skip the main run only while that triggering commit
# is absent from the release branch. Any partial release state resumes.

set -euo pipefail

fail() {
    echo "Error: $*" >&2
    exit 1
}

remote_commit() {
    local repository="$1"
    local ref="$2"

    git -C "${repository}" ls-remote origin "${ref}" | cut -f1
}

main() {
    local repository="${1:-}"
    local tag="${2:-}"
    local branch="${3:-}"
    local trigger_ref="${4:-}"
    local trigger_commit="${5:-}"
    local branch_commit tag_commit skip=false

    [[ -d "${repository}" ]] || fail "repository directory not found: ${repository}"
    [[ -n "${tag}" && -n "${branch}" && -n "${trigger_ref}" && -n "${trigger_commit}" ]] ||
        fail "tag, branch, trigger ref, and trigger commit are required"
    [[ -n "${GITHUB_OUTPUT:-}" ]] || fail "GITHUB_OUTPUT is not set"

    branch_commit="$(remote_commit "${repository}" "refs/heads/${branch}")"
    tag_commit="$(remote_commit "${repository}" "refs/tags/${tag}")"

    if [[ -n "${branch_commit}" && "${trigger_ref}" == "refs/heads/main" && -z "${tag_commit}" ]]; then
        if ! git -C "${repository}" cat-file -e "${trigger_commit}^{commit}" 2>/dev/null; then
            git -C "${repository}" fetch --quiet origin "${trigger_commit}"
        fi
        git -C "${repository}" fetch --quiet origin "refs/heads/${branch}"
        if git -C "${repository}" merge-base --is-ancestor "${trigger_commit}" FETCH_HEAD; then
            echo "Release branch contains the triggering commit; resuming reconciliation."
        else
            echo "Release branch does not contain the triggering main commit; waiting for its cherry-pick."
            skip=true
        fi
    fi

    echo "result=${skip}" >>"${GITHUB_OUTPUT}"
}

main "$@"
