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
readonly SCRIPT="${SCRIPT_DIR}/capture-release-sibling-commits.sh"

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

create_repository() {
    local name="$1"
    local with_release="$2"
    local seed="${TEST_ROOT}/${name}-seed"
    local origin="${TEST_ROOT}/${name}.git"

    git init --quiet --bare "${origin}"
    git init --quiet -b main "${seed}"
    git -C "${seed}" config user.name "Release Test"
    git -C "${seed}" config user.email "release-test@example.com"
    git -C "${seed}" config commit.gpgsign false
    printf '%s main\n' "${name}" >"${seed}/state.txt"
    git -C "${seed}" add state.txt
    git -C "${seed}" commit --quiet -m "Initial ${name} state"
    git -C "${seed}" remote add origin "${origin}"
    git -C "${seed}" push --quiet origin main
    if [[ "${with_release}" == "true" ]]; then
        git -C "${seed}" checkout --quiet -b release/0.61
        printf '%s release\n' "${name}" >"${seed}/state.txt"
        git -C "${seed}" commit --quiet -am "Release ${name} state"
        git -C "${seed}" push --quiet origin release/0.61
    fi
}

main() {
    local output recipes_main dashboard_release aws_main

    command -v jq >/dev/null || exit 1
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-siblings-test-XXXXXX")"
    create_repository recipes false
    create_repository dashboard true
    create_repository bicep-types-aws false
    output="${TEST_ROOT}/siblings.json"
    SIBLING_REPOSITORY_ROOT="${TEST_ROOT}" bash "${SCRIPT}" \
        --channel 0.61 --output "${output}" >/dev/null

    recipes_main="$(git --git-dir="${TEST_ROOT}/recipes.git" rev-parse main)"
    dashboard_release="$(
        git --git-dir="${TEST_ROOT}/dashboard.git" \
            rev-parse release/0.61
    )"
    aws_main="$(
        git --git-dir="${TEST_ROOT}/bicep-types-aws.git" rev-parse main
    )"
    if ! jq -e --arg recipes "${recipes_main}" \
        --arg dashboard "${dashboard_release}" --arg aws "${aws_main}" '
        map(.name) == ["recipes", "dashboard", "bicep-types-aws"] and
        (.[] | select(.name == "recipes") |
            .sourceRef == "main" and .sourceCommit == $recipes) and
        (.[] | select(.name == "dashboard") |
            .sourceRef == "release/0.61" and
            .sourceCommit == $dashboard) and
        (.[] | select(.name == "bicep-types-aws") |
            .sourceRef == "main" and .sourceCommit == $aws)
    ' "${output}" >/dev/null; then
        fail_test "captured sibling commits do not match remote branches"
    else
        ((++PASS))
    fi

    if SIBLING_REPOSITORY_ROOT="${TEST_ROOT}" bash "${SCRIPT}" \
        --channel invalid --output "${output}" >/dev/null 2>&1; then
        fail_test "invalid channel unexpectedly passed"
    else
        ((++PASS))
    fi

    if ((FAIL > 0)); then
        echo "Sibling commit tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Sibling commit tests passed (${PASS} tests)"
}

main "$@"
