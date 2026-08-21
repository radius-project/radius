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
REAL_GIT="$(command -v git)"
readonly REAL_GIT

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
    local planned_commit="${2:-}"
    local fail_branch_fetch="${3:-false}"
    local concurrent_branch_commit="${4:-}"
    local script_path="${PATH}"

    if [[ "${fail_branch_fetch}" == "true" || -n "${concurrent_branch_commit}" ]]; then
        script_path="${TEST_ROOT}/bin:${PATH}"
    fi

    set +e
    LAST_OUTPUT="$(
        cd "${TEST_ROOT}" &&
            PATH="${script_path}" FAIL_BRANCH_FETCH="${fail_branch_fetch}" \
                CONCURRENT_BRANCH_COMMIT="${concurrent_branch_commit}" \
                CONCURRENT_REMOTE="${TEST_ROOT}/${name}.git" \
                CONCURRENT_MARKER="${TEST_ROOT}/${name}.branch-created" \
                bash "${SCRIPT}" "${name}" "${TAG}" "${BRANCH}" "${planned_commit}" 2>&1
    )"
    LAST_STATUS=$?
    set -e
}

install_git_wrapper() {
    mkdir -p "${TEST_ROOT}/bin"
    cat >"${TEST_ROOT}/bin/git" <<EOF
#!/bin/bash
if [[ "\${FAIL_BRANCH_FETCH:-}" == "true" && "\${1:-}" == "fetch" && "\${*: -1}" == "refs/heads/${BRANCH}" ]]; then
    echo "injected release-branch fetch failure" >&2
    exit 42
fi
if [[ -n "\${CONCURRENT_BRANCH_COMMIT:-}" && "\${1:-}" == "push" && ! -e "\${CONCURRENT_MARKER}" ]]; then
    "${REAL_GIT}" --git-dir="\${CONCURRENT_REMOTE}" update-ref \
        "refs/heads/${BRANCH}" "\${CONCURRENT_BRANCH_COMMIT}"
    touch "\${CONCURRENT_MARKER}"
fi
exec "${REAL_GIT}" "\$@"
EOF
    chmod +x "${TEST_ROOT}/bin/git"
}

remote_ref() {
    local name="$1"
    local ref="$2"

    git -C "${TEST_ROOT}/${name}.git" rev-parse --verify --quiet "${ref}" || true
}

install_concurrent_tag_hook() {
    local name="$1"
    local commit="$2"
    local work="${TEST_ROOT}/${name}"
    local remote="${TEST_ROOT}/${name}.git"

    cat >"${work}/.git/hooks/pre-push" <<EOF
#!/bin/bash
git --git-dir="${remote}" update-ref "refs/tags/${TAG}" "${commit}"
rm -f "${work}/.git/hooks/pre-push"
EOF
    chmod +x "${work}/.git/hooks/pre-push"
}

install_concurrent_branch_hook() {
    local name="$1"
    local commit="$2"
    local work="${TEST_ROOT}/${name}"
    local remote="${TEST_ROOT}/${name}.git"

    cat >"${work}/.git/hooks/pre-push" <<EOF
#!/bin/bash
git --git-dir="${remote}" update-ref "refs/heads/${BRANCH}" "${commit}"
rm -f "${work}/.git/hooks/pre-push"
EOF
    chmod +x "${work}/.git/hooks/pre-push"
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

test_tag_at_different_commit_conflicts() {
    setup_repo repo4
    local off_branch
    git -C "${TEST_ROOT}/repo4" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    commit_on repo4 "main-only"
    off_branch="$(git -C "${TEST_ROOT}/repo4" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo4" push --quiet origin main
    git -C "${TEST_ROOT}/repo4" push --quiet origin "${off_branch}:refs/tags/${TAG}"

    run_script repo4
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "a tag at a different commit should be a conflict"
        return
    fi
    if [[ "$(remote_ref repo4 "refs/tags/${TAG}")" != "${off_branch}" ]]; then
        fail_test "a conflicting tag must not be moved"
        return
    fi
    pass
}

test_tag_behind_branch_head_recovers_the_plan() {
    setup_repo repo5
    local tagged_commit
    run_script repo5
    tagged_commit="$(remote_ref repo5 "refs/tags/${TAG}")"
    commit_on repo5 "later work"
    git -C "${TEST_ROOT}/repo5" push --quiet origin "HEAD:refs/heads/${BRANCH}"

    run_script repo5
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "an immutable tag should recover the plan after the branch advances: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo5 "refs/tags/${TAG}")" != "${tagged_commit}" ]]; then
        fail_test "the recovered immutable tag must not move when the branch advances"
        return
    fi
    pass
}

test_exact_planned_tag_is_accepted_after_branch_advances() {
    setup_repo repo8
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo8" rev-parse HEAD)"
    run_script repo8 "${planned_commit}"
    commit_on repo8 "later work"
    git -C "${TEST_ROOT}/repo8" push --quiet origin "HEAD:refs/heads/${BRANCH}"

    run_script repo8 "${planned_commit}"
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "the exact planned tag should be accepted after the branch advances: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo8 "refs/tags/${TAG}")" != "${planned_commit}" ]]; then
        fail_test "the exact planned tag must not move when the branch advances"
        return
    fi
    pass
}

test_ancestor_tag_conflicts_with_newer_explicit_plan() {
    setup_repo repo17
    local tagged_commit planned_commit
    tagged_commit="$(git -C "${TEST_ROOT}/repo17" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo17" push --quiet origin "${tagged_commit}:refs/tags/${TAG}"
    commit_on repo17 "planned release"
    planned_commit="$(git -C "${TEST_ROOT}/repo17" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo17" push --quiet origin "${planned_commit}:refs/heads/${BRANCH}"

    run_script repo17 "${planned_commit}"
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"expected planned commit"* ]]; then
        fail_test "an ancestor tag should conflict with a newer explicit plan"
        return
    fi
    if [[ "$(remote_ref repo17 "refs/tags/${TAG}")" != "${tagged_commit}" ]]; then
        fail_test "an ancestor tag conflict must not move the existing tag"
        return
    fi
    pass
}

test_absent_branch_is_created_at_planned_commit_not_head() {
    setup_repo repo10
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo10" rev-parse HEAD)"
    commit_on repo10 "checkout advanced"

    run_script repo10 "${planned_commit}"
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "planned commit reconciliation should succeed: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo10 "refs/heads/${BRANCH}")" != "${planned_commit}" ]]; then
        fail_test "release branch should be pinned to the planned commit rather than checkout HEAD"
        return
    fi
    if [[ "$(remote_ref repo10 "refs/tags/${TAG}")" != "${planned_commit}" ]]; then
        fail_test "release tag should be pinned to the planned commit rather than checkout HEAD"
        return
    fi
    pass
}

test_matching_annotated_tag_is_accepted() {
    setup_repo repo11
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo11" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo11" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    git -C "${TEST_ROOT}/repo11" tag --annotate "${TAG}" -m "release"
    git -C "${TEST_ROOT}/repo11" push --quiet origin "refs/tags/${TAG}"

    run_script repo11 "${planned_commit}"
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "an annotated tag at the planned commit should be accepted: ${LAST_OUTPUT}"
        return
    fi
    pass
}

test_concurrent_exact_tag_creation_is_accepted() {
    setup_repo repo12
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo12" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo12" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    install_concurrent_tag_hook repo12 "${planned_commit}"

    run_script repo12 "${planned_commit}"
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "concurrent exact tag creation should reconcile successfully: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo12 "refs/tags/${TAG}")" != "${planned_commit}" ]]; then
        fail_test "concurrent exact tag creation produced the wrong target"
        return
    fi
    pass
}

test_concurrent_conflicting_tag_creation_fails() {
    setup_repo repo13
    local planned_commit conflicting_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo13" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo13" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    commit_on repo13 "conflicting"
    conflicting_commit="$(git -C "${TEST_ROOT}/repo13" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo13" push --quiet origin "${conflicting_commit}:refs/heads/conflicting"
    install_concurrent_tag_hook repo13 "${conflicting_commit}"

    run_script repo13 "${planned_commit}"
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"could not create tag"* ]]; then
        fail_test "concurrent conflicting tag creation should fail: status=${LAST_STATUS}; output=${LAST_OUTPUT}; remote=$(remote_ref repo13 "refs/tags/${TAG}")"
        return
    fi
    if [[ "$(remote_ref repo13 "refs/tags/${TAG}")" != "${conflicting_commit}" ]]; then
        fail_test "concurrent conflicting tag must not be moved"
        return
    fi
    pass
}

test_concurrent_descendant_branch_creation_is_accepted() {
    setup_repo repo15
    local planned_commit descendant
    planned_commit="$(git -C "${TEST_ROOT}/repo15" rev-parse HEAD)"
    commit_on repo15 "descendant"
    descendant="$(git -C "${TEST_ROOT}/repo15" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo15" push --quiet origin "${descendant}:refs/heads/concurrent"
    install_concurrent_branch_hook repo15 "${descendant}"

    run_script repo15 "${planned_commit}"
    if [[ "${LAST_STATUS}" -ne 0 ]]; then
        fail_test "concurrent descendant branch creation should be accepted: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo15 "refs/tags/${TAG}")" != "${planned_commit}" ]]; then
        fail_test "concurrent descendant branch must still tag the planned commit"
        return
    fi
    pass
}

test_concurrent_divergent_branch_creation_fails() {
    setup_repo repo16
    local planned_commit divergent
    planned_commit="$(git -C "${TEST_ROOT}/repo16" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo16" checkout --quiet --orphan divergent
    git -C "${TEST_ROOT}/repo16" rm --quiet -rf .
    echo divergent >"${TEST_ROOT}/repo16/divergent.txt"
    git -C "${TEST_ROOT}/repo16" add divergent.txt
    git -C "${TEST_ROOT}/repo16" commit --quiet -m divergent
    divergent="$(git -C "${TEST_ROOT}/repo16" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo16" push --quiet origin "${divergent}:refs/heads/concurrent"
    install_concurrent_branch_hook repo16 "${divergent}"

    run_script repo16 "${planned_commit}"
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "concurrent divergent branch creation should fail: ${LAST_OUTPUT}"
        return
    fi
    if [[ -n "$(remote_ref repo16 "refs/tags/${TAG}")" ]]; then
        fail_test "a divergent release branch must not create a tag"
        return
    fi
    pass
}

test_concurrent_ancestor_branch_is_not_fast_forwarded() {
    setup_repo repo18
    local ancestor planned_commit
    ancestor="$(git -C "${TEST_ROOT}/repo18" rev-parse HEAD)"
    commit_on repo18 "planned release"
    planned_commit="$(git -C "${TEST_ROOT}/repo18" rev-parse HEAD)"
    run_script repo18 "${planned_commit}" false "${ancestor}"
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "a concurrently created ancestor branch should not be fast-forwarded: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(remote_ref repo18 "refs/heads/${BRANCH}")" != "${ancestor}" ]]; then
        fail_test "concurrent ancestor branch was modified"
        return
    fi
    if [[ -n "$(remote_ref repo18 "refs/tags/${TAG}")" ]]; then
        fail_test "concurrent ancestor branch must not create a tag"
        return
    fi
    pass
}

test_fetch_failure_stops_before_tag_creation() {
    setup_repo repo19
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo19" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo19" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    git -C "${TEST_ROOT}/repo19" fetch --quiet origin main

    run_script repo19 "${planned_commit}" true
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"could not fetch release branch"* ]]; then
        fail_test "a branch fetch failure should stop reconciliation: ${LAST_OUTPUT}"
        return
    fi
    if [[ -n "$(remote_ref repo19 "refs/tags/${TAG}")" ]]; then
        fail_test "a failed branch fetch must not create a tag"
        return
    fi
    pass
}

test_planned_commit_must_be_reachable_from_branch() {
    setup_repo repo9
    local planned_commit
    planned_commit="$(git -C "${TEST_ROOT}/repo9" rev-parse HEAD)"
    git -C "${TEST_ROOT}/repo9" push --quiet origin "HEAD:refs/heads/${BRANCH}"
    git -C "${TEST_ROOT}/repo9" checkout --quiet --orphan unrelated
    git -C "${TEST_ROOT}/repo9" rm --quiet -rf .
    echo "unrelated" >"${TEST_ROOT}/repo9/unrelated.txt"
    git -C "${TEST_ROOT}/repo9" add unrelated.txt
    git -C "${TEST_ROOT}/repo9" commit --quiet -m "unrelated"
    planned_commit="$(git -C "${TEST_ROOT}/repo9" rev-parse HEAD)"

    run_script repo9 "${planned_commit}"
    if [[ "${LAST_STATUS}" -eq 0 || "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "a planned commit outside the release branch should fail"
        return
    fi
    if [[ -n "$(remote_ref repo9 "refs/tags/${TAG}")" ]]; then
        fail_test "an unreachable planned commit must not create a tag"
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
    install_git_wrapper

    test_creates_branch_and_tag
    test_rerun_is_idempotent
    test_adopts_existing_release_branch
    test_tag_at_different_commit_conflicts
    test_tag_behind_branch_head_recovers_the_plan
    test_exact_planned_tag_is_accepted_after_branch_advances
    test_ancestor_tag_conflicts_with_newer_explicit_plan
    test_absent_branch_is_created_at_planned_commit_not_head
    test_matching_annotated_tag_is_accepted
    test_concurrent_exact_tag_creation_is_accepted
    test_concurrent_conflicting_tag_creation_fails
    test_concurrent_descendant_branch_creation_is_accepted
    test_concurrent_divergent_branch_creation_fails
    test_concurrent_ancestor_branch_is_not_fast_forwarded
    test_fetch_failure_stops_before_tag_creation
    test_planned_commit_must_be_reachable_from_branch
    test_pushes_only_the_named_tag
    test_requires_arguments

    if [[ "${FAILURES}" -ne 0 ]]; then
        echo "release tag and branch reconciliation tests failed (${FAILURES})" >&2
        exit 1
    fi
    echo "release tag and branch reconciliation tests passed (${TESTS} tests)"
}

main "$@"
