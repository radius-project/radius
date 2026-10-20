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
readonly RECONCILE="${SCRIPT_DIR}/release-create-tag-and-branch.sh"
readonly VERSION="v0.61.0-rc.1"
readonly RELEASE_BRANCH="release/0.61"
readonly REPOSITORIES=(recipes dashboard bicep-types-aws radius)

TEST_ROOT=""
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

setup_repository() {
    local name="$1"
    local origin="${TEST_ROOT}/${name}.git"
    local work="${TEST_ROOT}/${name}"

    git init --quiet --bare --initial-branch=main "${origin}"
    git init --quiet --initial-branch=main "${work}"
    git -C "${work}" config user.name "Release Test"
    git -C "${work}" config user.email "release-test@example.com"
    git -C "${work}" config commit.gpgsign false
    printf '%s\n' "${name}" >"${work}/state.txt"
    git -C "${work}" add state.txt
    git -C "${work}" commit --quiet -m "Initial ${name} state"
    git -C "${work}" remote add origin "${origin}"
    git -C "${work}" push --quiet origin main
}

setup_fixture() {
    local name

    rm -rf "${TEST_ROOT}"
    mkdir -p "${TEST_ROOT}"
    for name in "${REPOSITORIES[@]}"; do
        setup_repository "${name}"
        git -C "${TEST_ROOT}/${name}" rev-parse HEAD \
            >"${TEST_ROOT}/${name}.planned"
    done
    printf '0\n' >"${TEST_ROOT}/de-dispatch-count"
}

publish_deployment_engine() {
    local count

    if [[ -f "${TEST_ROOT}/de-complete" ]]; then
        return
    fi
    count="$(<"${TEST_ROOT}/de-dispatch-count")"
    printf '%s\n' "$((count + 1))" >"${TEST_ROOT}/de-dispatch-count"
    printf 'complete\n' >"${TEST_ROOT}/de-complete"
}

run_attempt() {
    local fail_after="$1"
    local name planned

    publish_deployment_engine
    [[ "${fail_after}" != "deployment-engine" ]] || return 99
    for name in recipes dashboard bicep-types-aws; do
        planned="$(<"${TEST_ROOT}/${name}.planned")"
        (
            cd "${TEST_ROOT}"
            bash "${RECONCILE}" "${name}" "${VERSION}" \
                "${RELEASE_BRANCH}" "${planned}" >/dev/null 2>&1
        )
        [[ "${fail_after}" != "${name}" ]] || return 99
    done
    name=radius
    planned="$(<"${TEST_ROOT}/${name}.planned")"
    (
        cd "${TEST_ROOT}"
        bash "${RECONCILE}" "${name}" "${VERSION}" \
            "${RELEASE_BRANCH}" "${planned}" >/dev/null 2>&1
    )
    [[ "${fail_after}" != "radius" ]] || return 99
}

assert_complete() {
    local name planned tag branch

    if [[ "$(<"${TEST_ROOT}/de-dispatch-count")" != "1" ]]; then
        fail_test "resume duplicated the Deployment Engine dispatch"
        return 1
    fi
    for name in "${REPOSITORIES[@]}"; do
        planned="$(<"${TEST_ROOT}/${name}.planned")"
        tag="$(
            git --git-dir="${TEST_ROOT}/${name}.git" \
                rev-parse "refs/tags/${VERSION}^{commit}"
        )"
        branch="$(
            git --git-dir="${TEST_ROOT}/${name}.git" \
                rev-parse "refs/heads/${RELEASE_BRANCH}^{commit}"
        )"
        if [[ "${tag}" != "${planned}" || "${branch}" != "${planned}" ]]; then
            fail_test "${name} moved away from its frozen commit"
            return 1
        fi
    done
}

test_resume_after_stage() {
    local stage="$1"
    local status

    setup_fixture
    set +e
    run_attempt "${stage}"
    status=$?
    set -e
    if [[ "${status}" != "99" ]]; then
        fail_test "${stage} failure injection did not interrupt the run"
        return
    fi
    run_attempt none
    run_attempt none
    assert_complete || return
    ((++PASS))
}

main() {
    local stage

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/controller-resume-XXXXXX")"
    for stage in deployment-engine recipes dashboard bicep-types-aws radius; do
        test_resume_after_stage "${stage}"
    done

    if ((FAIL > 0)); then
        echo "Controller resume tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Controller resume tests passed (${PASS} tests)"
}

main "$@"
