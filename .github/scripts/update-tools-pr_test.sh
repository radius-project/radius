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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    local actual="$1"
    local expected="$2"
    [[ "${actual}" == *"${expected}"* ]] ||
        fail "expected '${actual}' to contain '${expected}'"
}

readonly REPOSITORY="${TEST_ROOT}/repository"
readonly REMOTE="${TEST_ROOT}/remote.git"
readonly SEED="${TEST_ROOT}/seed"
readonly FAKE_BIN="${TEST_ROOT}/bin"
readonly GH_LOG_PATH="${TEST_ROOT}/gh.log"

git init -q --bare "${REMOTE}"
git init -q -b main "${SEED}"
git -C "${SEED}" config user.name "Test User"
git -C "${SEED}" config user.email "test@example.com"
echo "original" >"${SEED}/tools.yaml"
git -C "${SEED}" add tools.yaml
git -C "${SEED}" commit -q -m "initial"
git -C "${SEED}" remote add origin "${REMOTE}"
git -C "${SEED}" push -q origin main

# The workflow checks out the base branch only, so the automation branch never
# gains a remote-tracking ref. Mirror that here or the pushes below lease
# against a ref that CI does not have.
git clone -q --single-branch --branch main "${REMOTE}" "${REPOSITORY}"

mkdir -p "${FAKE_BIN}"
cat >"${FAKE_BIN}/gh" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >>"${GH_LOG}"
if [[ "$1 $2" == "pr list" ]]; then
    printf '%s\n' "${EXISTING_PR:-}"
fi
EOF
chmod +x "${FAKE_BIN}/gh"

run_script() {
    local existing_pr="$1"
    PATH="${FAKE_BIN}:${PATH}" \
        REPO_ROOT="${REPOSITORY}" \
        GH_LOG="${GH_LOG_PATH}" \
        EXISTING_PR="${existing_pr}" \
        GH_TOKEN="test-token" \
        GIT_USER_EMAIL="automation@example.com" \
        GIT_USER_NAME="Automation Bot" \
        PR_BRANCH="automation/update-tools" \
        PR_TITLE="chore: update pinned tool versions" \
        bash "${SCRIPT_DIR}/update-tools-pr.sh" >/dev/null
}

echo "first update" >>"${REPOSITORY}/tools.yaml"
run_script ""

commit_message="$(
    git --git-dir="${REMOTE}" log -1 --format=%B \
        refs/heads/automation/update-tools
)"
assert_contains "${commit_message}" "chore: update pinned tool versions"
assert_contains \
    "${commit_message}" \
    "Signed-off-by: Automation Bot <automation@example.com>"
assert_contains "$(cat "${GH_LOG_PATH}")" "pr create"

if git -C "${REPOSITORY}" show-ref --quiet \
    refs/remotes/origin/automation/update-tools; then
    fail "fixture drifted from CI: the automation branch is now tracked"
fi

before="$(
    git --git-dir="${REMOTE}" rev-parse refs/heads/automation/update-tools
)"
git -C "${REPOSITORY}" checkout -q main
echo "second update" >>"${REPOSITORY}/tools.yaml"
run_script "42"
after="$(
    git --git-dir="${REMOTE}" rev-parse refs/heads/automation/update-tools
)"
assert_contains "$(cat "${GH_LOG_PATH}")" "pr edit 42"
[[ "${before}" != "${after}" ]] ||
    fail "expected the remote automation branch to be updated"

echo "update-tools-pr tests passed"
