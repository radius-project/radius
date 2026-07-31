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

set -euo pipefail

readonly REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel)}"
readonly PR_BASE="${PR_BASE:-main}"

require_environment() {
    local name
    for name in GH_TOKEN GIT_USER_EMAIL GIT_USER_NAME PR_BRANCH PR_TITLE; do
        if [[ -z "${!name:-}" ]]; then
            echo "Error: ${name} is required" >&2
            return 1
        fi
    done
}

pull_request_body() {
    cat <<'EOF'
This automated PR refreshes the pinned command-line tool versions and SHA-256
checksums from the release sources declared in `build/tools.yaml`.

`build/tools.generated.mk` is generated from the manifest and committed with
it. Bicep remains intentionally held at its compatibility-pinned version until
local `br:localhost` functional tests support a newer release.
EOF
}

# The workflow checks out only the base branch, so there is no remote-tracking
# ref to lease against. Read the destination SHA from the remote instead; an
# empty value leases on the branch not existing yet.
push_branch() {
    local expected
    expected="$(
        git ls-remote origin "refs/heads/${PR_BRANCH}" | awk '{print $1}'
    )"
    git push --force-with-lease="refs/heads/${PR_BRANCH}:${expected}" \
        origin "HEAD:${PR_BRANCH}"
}

require_environment
cd "${REPO_ROOT}"

git config user.name "${GIT_USER_NAME}"
git config user.email "${GIT_USER_EMAIL}"
git checkout -B "${PR_BRANCH}"
git add -A
git commit --signoff -m "${PR_TITLE}"
gh auth setup-git
push_branch

existing_pr="$(
    gh pr list --state open --head "${PR_BRANCH}" \
        --json number --jq '.[0].number'
)"
body="$(pull_request_body)"
if [[ -n "${existing_pr}" ]]; then
    gh pr edit "${existing_pr}" --title "${PR_TITLE}" --body "${body}"
    echo "Updated PR #${existing_pr}"
else
    gh pr create --base "${PR_BASE}" --head "${PR_BRANCH}" \
        --title "${PR_TITLE}" --body "${body}"
fi
