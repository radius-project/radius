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

# ============================================================================
# Tests for .github/scripts/changelog-range.sh
#
# Builds a throwaway repository that mirrors the Radius branching topology:
# old releases tagged on main, newer releases tagged on release/* branches.
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT="${SCRIPT_DIR}/changelog-range.sh"

TEST_ROOT=""
REPO=""
PASS=0
FAIL=0

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

commit() {
    git -C "${REPO}" commit -q --allow-empty -m "$1"
}

# Build: main carries the historical v0.46.0 tag; 0.59 and 0.60 are released
# from release branches, so their tags are unreachable from main.
setup_repo() {
    REPO="${TEST_ROOT}/repo"
    mkdir -p "${REPO}"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false

    commit "initial"
    commit "old work"
    git -C "${REPO}" tag v0.46.0

    commit "work after 0.46"
    BRANCH_POINT_59="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" branch release/0.59
    git -C "${REPO}" checkout -q release/0.59
    commit "0.59 release prep"
    git -C "${REPO}" tag v0.59.0
    commit "0.59 patch work"

    git -C "${REPO}" checkout -q main
    commit "more main work"
    BRANCH_POINT_60="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" branch release/0.60
    git -C "${REPO}" checkout -q release/0.60
    commit "0.60 release prep"
    git -C "${REPO}" tag v0.60.0

    git -C "${REPO}" checkout -q main
    commit "unreleased main work"
}

run_range() {
    (cd "${REPO}" && bash "${SCRIPT}" "$@" 2>/dev/null)
}

test_main_uses_newest_channel_branch_point() {
    local range expected
    range="$(run_range)"
    expected="${BRANCH_POINT_60}..HEAD"
    if [[ "${range}" != "${expected}" ]]; then
        fail_test "main should start at the 0.60 branch point; got '${range}', want '${expected}'"
        return
    fi
    ((++PASS))
}

test_main_does_not_reach_back_to_old_tag() {
    local range old_commit
    range="$(run_range)"
    old_commit="$(git -C "${REPO}" rev-parse "v0.46.0^{commit}")"
    if [[ "${range}" == "${old_commit}"* ]]; then
        fail_test "main must not fall back to the ancient v0.46.0 tag"
        return
    fi
    ((++PASS))
}

test_release_branch_uses_its_own_release() {
    local range expected
    range="$(run_range --ref release/0.59)"
    expected="$(git -C "${REPO}" rev-parse "v0.59.0^{commit}")..release/0.59"
    if [[ "${range}" != "${expected}" ]]; then
        fail_test "release/0.59 should start at v0.59.0; got '${range}', want '${expected}'"
        return
    fi
    ((++PASS))
}

test_release_branch_excludes_other_channel() {
    local range start
    range="$(run_range --ref release/0.59)"
    start="${range%%..*}"
    # Starting at the branch point would replay commits already in v0.59.0.
    if [[ "${start}" == "${BRANCH_POINT_59}" ]]; then
        fail_test "release/0.59 must not start at the branch point it shares with main"
        return
    fi
    ((++PASS))
}

test_unknown_ref_fails() {
    if run_range --ref does-not-exist >/dev/null 2>&1; then
        fail_test "expected a non-zero exit for an unknown ref"
        return
    fi
    ((++PASS))
}

test_repo_without_stable_tags_fails() {
    local bare="${TEST_ROOT}/no-tags"
    mkdir -p "${bare}"
    git -C "${bare}" init -q -b main
    git -C "${bare}" config user.name "Radius Test"
    git -C "${bare}" config user.email "test@example.com"
    git -C "${bare}" config commit.gpgsign false
    git -C "${bare}" commit -q --allow-empty -m "initial"
    git -C "${bare}" tag v0.61.0-rc.1

    if (cd "${bare}" && bash "${SCRIPT}" >/dev/null 2>&1); then
        fail_test "expected a non-zero exit when only pre-release tags exist"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/changelog-range-test-XXXXXX")"
    setup_repo

    test_main_uses_newest_channel_branch_point
    test_main_does_not_reach_back_to_old_tag
    test_release_branch_uses_its_own_release
    test_release_branch_excludes_other_channel
    test_unknown_ref_fails
    test_repo_without_stable_tags_fails

    if ((FAIL > 0)); then
        echo "changelog range tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "changelog range tests passed (${PASS} tests)"
}

main "$@"
