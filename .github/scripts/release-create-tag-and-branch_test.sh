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
readonly SCRIPT="${SCRIPT_DIR}/release-create-tag-and-branch.sh"
readonly TAG="v0.61.0"
readonly BRANCH="release/0.61"

TEST_ROOT=""
FAILURES=0
TESTS=0
LAST_STATUS=0
LAST_OUTPUT=""

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "FAIL: $*" >&2
    FAILURES=$((FAILURES + 1))
}

pass() {
    TESTS=$((TESTS + 1))
}

# Commit signing is enabled globally on some developer machines, which would
# make every fixture commit prompt or fail, so it is disabled per repository.
configure_repo() {
    local dir="$1"

    git -C "${dir}" config user.name "Radius Test"
    git -C "${dir}" config user.email "test@example.com"
    git -C "${dir}" config commit.gpgsign false
    git -C "${dir}" config tag.gpgsign false
}

setup_repo() {
    local name="$1"
    local remote="${TEST_ROOT}/${name}.git"
    local work="${TEST_ROOT}/${name}"

    git init --quiet --bare --initial-branch=main "${remote}"
    git init --quiet --initial-branch=main "${work}"
    git -C "${work}" remote add origin "${remote}"
    configure_repo "${work}"
    echo "seed" >"${work}/README.md"
    git -C "${work}" add README.md
    git -C "${work}" commit --quiet -m "seed"
    git -C "${work}" push --quiet origin main
}

commit_on() {
    local name="$1"
    local message="$2"
    local work="${TEST_ROOT}/${name}"

    echo "${message}" >>"${work}/README.md"
    git -C "${work}" add README.md
    git -C "${work}" commit --quiet -m "${message}"
}

run_script() {
    local name="$1"

    set +e
    LAST_OUTPUT="$(cd "${TEST_ROOT}" && bash "${SCRIPT}" "${name}" "${TAG}" "${BRANCH}" 2>&1)"
    LAST_STATUS=$?
    set -e
}

remote_ref() {
    local name="$1"
    local ref="$2"

    git -C "${TEST_ROOT}/${name}.git" rev-parse --verify --quiet "${ref}" || true
}

test_creates_branch_and_tag() {
    setup_repo repo1
    local head
    head="$(git -C "${TEST_ROOT}/repo1" rev-parse HEAD)"

    run_script repo1
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "reconciliation should succeed: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo1 "refs/heads/${BRANCH}")" != "${head}" ]]; then
        fail_test "release branch should be created at HEAD"
        return
    fi
    if [[ "$(remote_ref repo1 "refs/tags/${TAG}")" != "${head}" ]]; then
        fail_test "tag should be created at HEAD"
        return
    fi
    pass
}

test_rerun_is_idempotent() {
    setup_repo repo2
    local head
    head="$(git -C "${TEST_ROOT}/repo2" rev-parse HEAD)"

    run_script repo2
    run_script repo2
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "rerun should succeed: ${LAST_OUTPUT}"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"already released"* ]]; then
        fail_test "rerun should report the tag as already released"
        return
    fi
    if [[ "$(remote_ref repo2 "refs/tags/${TAG}")" != "${head}" ]]; then
        fail_test "rerun must not move the tag"
        return
    fi
    pass
}

test_adopts_existing_release_branch() {
    setup_repo repo3
    local branch_head
    # Release branch is one commit behind main, so the tag must land on the
    # branch head rather than on the checked-out main commit.
    branch_head="$(git -C "${TEST_ROOT}/repo3" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo3" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    commit_on repo3 "main-only"
    git -C "${TEST_ROOT}/repo3" push --quiet origin main

    run_script repo3
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "existing branch should be adopted: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo3 "refs/tags/${TAG}")" != "${branch_head}" ]]; then
        fail_test "tag should be created on the release branch head"
        return
    fi
    pass
}

test_tag_off_release_branch_conflicts() {
    setup_repo repo4
    local off_branch
    git -C "${TEST_ROOT}/repo4" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    commit_on repo4 "main-only"
    off_branch="$(git -C "${TEST_ROOT}/repo4" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo4" push --quiet origin main
    git -C "${TEST_ROOT}/repo4" push --quiet origin "${off_branch}:refs/tags/${TAG}"

    run_script repo4
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "a tag off the release branch should be a conflict"
        return
    fi
    if [[ "$(remote_ref repo4 "refs/tags/${TAG}")" != "${off_branch}" ]]; then
        fail_test "a conflicting tag must not be moved"
        return
    fi
    pass
}

test_tag_behind_branch_head_is_accepted() {
    setup_repo repo5
    local tagged_commit
    run_script repo5
    tagged_commit="$(remote_ref repo5 "refs/tags/${TAG}")"
    commit_on repo5 "later work"
    git -C "${TEST_ROOT}/repo5" push --quiet origin "HEAD:refs/heads/${BRANCH}"

    run_script repo5
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "a tag reachable from the branch should be accepted: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo5 "refs/tags/${TAG}")" != "${tagged_commit}" ]]; then
        fail_test "an already released tag must not move when the branch advances"
        return
    fi
    pass
}

test_pushes_only_the_named_tag() {
    setup_repo repo6
    git -C "${TEST_ROOT}/repo6" tag "v9.9.9-unrelated"

    run_script repo6
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "reconciliation should succeed: ${LAST_OUTPUT}"
        return
    fi
    if [[ -n "$(remote_ref repo6 "refs/tags/v9.9.9-unrelated")" ]]; then
        fail_test "only the named tag may be pushed"
        return
    fi
    pass
}

test_requires_arguments() {
    setup_repo repo7

    set +e
    LAST_OUTPUT="$(cd "${TEST_ROOT}" && bash "${SCRIPT}" repo7 "${TAG}" 2>&1)"
    LAST_STATUS=$?
    set -e
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"required"* ]]; then
        fail_test "missing arguments should fail"
        return
    fi

    set +e
    LAST_OUTPUT="$(cd "${TEST_ROOT}" && bash "${SCRIPT}" missing "${TAG}" "${BRANCH}" 2>&1)"
    LAST_STATUS=$?
    set -e
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not found"* ]]; then
        fail_test "a missing repository directory should fail"
        return
    fi
    pass
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-tag-branch-test-XXXXXX")"

    test_creates_branch_and_tag
    test_rerun_is_idempotent
    test_adopts_existing_release_branch
    test_tag_off_release_branch_conflicts
    test_tag_behind_branch_head_is_accepted
    test_pushes_only_the_named_tag
    test_requires_arguments

    if [[ "${FAILURES}" -ne 0 ]]; then
        echo "release tag and branch reconciliation tests failed (${FAILURES})" >&2
        exit 1
    fi
    echo "release tag and branch reconciliation tests passed (${TESTS} tests)"
}

main "$@"
