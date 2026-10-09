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
readonly SELECT_SCRIPT="${SCRIPT_DIR}/release-get-version.sh"
readonly RECONCILE_SCRIPT="${SCRIPT_DIR}/release-create-tag-and-branch.sh"
readonly TAG="v0.61.0"
readonly BRANCH="release/0.61"
readonly REPOSITORIES=(radius recipes dashboard bicep-types-aws)

TEST_ROOT=""
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
    exit 1
}

setup_repository() {
    local repository="$1"
    local remote="${TEST_ROOT}/${repository}.git"
    local work="${TEST_ROOT}/${repository}"

    git init --quiet --bare --initial-branch=main "${remote}"
    git init --quiet --initial-branch=main "${work}"
    git -C "${work}" remote add origin "${remote}"
    git -C "${work}" config user.name "Radius Test"
    git -C "${work}" config user.email "test@example.com"
    git -C "${work}" config commit.gpgsign false
    git -C "${work}" config tag.gpgsign false
    echo "${repository}" >"${work}/README.md"
    git -C "${work}" add README.md
    git -C "${work}" commit --quiet -m "seed ${repository}"
    git -C "${work}" push --quiet origin main
}

setup_repositories() {
    local repository

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-get-version-test-XXXXXX")"
    for repository in "${REPOSITORIES[@]}"; do
        setup_repository "${repository}"
    done
}

run_selector() {
    local versions="$1"
    shift
    local output_file="${TEST_ROOT}/github-output"

    : >"${output_file}"
    set +e
    LAST_OUTPUT="$(
        cd "${TEST_ROOT}" &&
            GITHUB_OUTPUT="${output_file}" bash "${SELECT_SCRIPT}" "${versions}" "$@" 2>&1
    )"
    LAST_STATUS=$?
    set -e
}

output_value() {
    local name="$1"

    sed -n "s/^${name}=//p" "${TEST_ROOT}/github-output"
}

remote_tag() {
    local repository="$1"
    local tag="$2"

    git -C "${TEST_ROOT}/${repository}.git" rev-parse --verify --quiet "refs/tags/${tag}" || true
}

reconcile_all() {
    local repository output

    for repository in "${REPOSITORIES[@]}"; do
        if ! output="$(
            cd "${TEST_ROOT}" &&
                bash "${RECONCILE_SCRIPT}" "${repository}" "${TAG}" "${BRANCH}" 2>&1
        )"; then
            fail_test "failed to reconcile ${repository}: ${output}"
        fi
    done
}

test_selects_version_missing_from_any_repository() {
    local radius_head
    radius_head="$(git -C "${TEST_ROOT}/radius" rev-parse HEAD)"
    git -C "${TEST_ROOT}/radius" push --quiet origin "${radius_head}:refs/heads/${BRANCH}"
    git -C "${TEST_ROOT}/radius" push --quiet origin "${radius_head}:refs/tags/${TAG}"

    run_selector "${TAG}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -eq 0 ]] || fail_test "partial release should remain selectable: ${LAST_OUTPUT}"
    [[ "$(output_value release-version)" == "${TAG}" ]] || fail_test "partial release selected the wrong version"
    [[ "${LAST_OUTPUT}" == *"recipes dashboard bicep-types-aws"* ]] || fail_test "partial release did not report the missing repositories"

    reconcile_all
    for repository in "${REPOSITORIES[@]}"; do
        [[ -n "$(remote_tag "${repository}" "${TAG}")" ]] || fail_test "${repository} was not reconciled"
    done
}

test_skips_version_complete_in_every_repository() {
    run_selector "${TAG}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "a fully released version should not be selected"
    [[ "${LAST_OUTPUT}" == *"no release version found"* ]] || fail_test "fully released version failed for the wrong reason"
}

test_rc_branch_and_channel() {
    local rc="v0.62.0-rc.1"

    run_selector "${rc}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -eq 0 ]] || fail_test "RC selection failed: ${LAST_OUTPUT}"
    [[ "$(output_value release-branch-name)" == "release/0.62" ]] || fail_test "RC release branch was parsed incorrectly"
    [[ "$(output_value release-channel)" == "0.62.0-rc.1" ]] || fail_test "RC release channel was parsed incorrectly"
}

test_new_legacy_rc_is_rejected() {
    local rc="v0.62.0-rc2"

    run_selector "${rc}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "a new legacy RC must not be selected"
    [[ "${LAST_OUTPUT}" == *"cannot create legacy RC"* ]] || fail_test "new legacy RC failed for the wrong reason"
    [[ "${LAST_OUTPUT}" == *"use v0.62.0-rc.2 for a new release"* ]] || fail_test "new legacy RC must recommend the dotted version"
    [[ -z "$(output_value release-version)" ]] || fail_test "rejected RC must not emit a release version"

    git -C "${TEST_ROOT}/radius" tag "${rc}"
    run_selector "${rc}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "an unpublished local tag must not establish historical release state"
    [[ -z "$(output_value release-version)" ]] || fail_test "local-only legacy RC must not emit a release version"
}

test_legacy_rc_remains_accepted() {
    local rc="v0.62.0-rc2"

    git -C "${TEST_ROOT}/radius" push --quiet origin "HEAD:refs/tags/${rc}"
    run_selector "${rc}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -eq 0 ]] || fail_test "legacy RC selection failed: ${LAST_OUTPUT}"
    [[ "$(output_value release-version)" == "${rc}" ]] || fail_test "historical reconciliation must preserve the exact tag name"
    [[ "$(output_value release-branch-name)" == "release/0.62" ]] || fail_test "legacy RC release branch was parsed incorrectly"
    [[ "$(output_value release-channel)" == "0.62.0-rc2" ]] || fail_test "legacy RC release channel was parsed incorrectly"
    [[ "${LAST_OUTPUT}" == *"use v0.62.0-rc.2 for new releases"* ]] || fail_test "legacy RC selection did not recommend the dotted form"
}

test_annotated_legacy_sibling_tag_allows_resume() {
    local rc="v0.62.0-rc3"

    git -C "${TEST_ROOT}/recipes" tag --annotate "${rc}" -m release
    git -C "${TEST_ROOT}/recipes" push --quiet origin "refs/tags/${rc}"
    run_selector "${rc}" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -eq 0 ]] || fail_test "an annotated legacy sibling tag must allow resume: ${LAST_OUTPUT}"
    [[ "$(output_value release-version)" == "${rc}" ]] || fail_test "annotated legacy reconciliation must preserve the exact tag name"

    for repository in radius dashboard bicep-types-aws; do
        git -C "${TEST_ROOT}/${repository}" push --quiet origin "HEAD:refs/tags/${rc}"
    done
    run_selector "${rc},v0.63.0-rc.1" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -eq 0 ]] || fail_test "completed legacy releases must not block a new dotted release: ${LAST_OUTPUT}"
    [[ "$(output_value release-version)" == "v0.63.0-rc.1" ]] || fail_test "completed historical version should be skipped"
}

test_rejects_unsupported_release_versions() {
    run_selector "v0.62.0-rc.01" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "RC identifiers with leading zeroes should fail"
    [[ "${LAST_OUTPUT}" == *"unsupported release version"* ]] || fail_test "invalid dotted RC failed for the wrong reason"

    run_selector "v0.62.0-rc.0" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "RC zero should fail"
    [[ "${LAST_OUTPUT}" == *"unsupported release version"* ]] || fail_test "RC zero failed for the wrong reason"

    run_selector "v0.62.0-beta.1" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "unsupported prerelease identifiers should fail"
    [[ "${LAST_OUTPUT}" == *"unsupported release version"* ]] || fail_test "unsupported prerelease failed for the wrong reason"
}

test_rejects_multiple_incomplete_versions() {
    run_selector "v0.63.0,v0.62.0" "${REPOSITORIES[@]}"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "multiple incomplete versions should fail"
    [[ "${LAST_OUTPUT}" == *"multiple versions"* ]] || fail_test "multiple versions failed for the wrong reason"
}

test_requires_repository_and_output() {
    run_selector "v0.63.0"
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "missing repositories should fail"

    set +e
    LAST_OUTPUT="$(cd "${TEST_ROOT}" && env -u GITHUB_OUTPUT bash "${SELECT_SCRIPT}" v0.63.0 radius 2>&1)"
    LAST_STATUS=$?
    set -e
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "missing GITHUB_OUTPUT should fail"
}

test_remote_query_errors_are_not_treated_as_missing_tags() {
    setup_repository broken-remote
    git -C "${TEST_ROOT}/broken-remote" remote set-url origin "${TEST_ROOT}/missing.git"

    run_selector "v0.63.0" broken-remote
    [[ "${LAST_STATUS}" -ne 0 ]] || fail_test "a remote query error should fail"
    [[ "${LAST_OUTPUT}" == *"could not query tag"* ]] || fail_test "remote query failed for the wrong reason"
}

main() {
    setup_repositories
    test_selects_version_missing_from_any_repository
    test_skips_version_complete_in_every_repository
    test_rc_branch_and_channel
    test_new_legacy_rc_is_rejected
    test_legacy_rc_remains_accepted
    test_annotated_legacy_sibling_tag_allows_resume
    test_rejects_unsupported_release_versions
    test_rejects_multiple_incomplete_versions
    test_requires_repository_and_output
    test_remote_query_errors_are_not_treated_as_missing_tags
    echo "release version selection tests passed (10 tests)"
}

main "$@"
