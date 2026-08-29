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
readonly SCRIPT="${SCRIPT_DIR}/validate-release-merge-group.sh"

TEST_ROOT=""
REPO=""
BASE_SHA=""
RELEASE_SHA=""
GROUP_BASE_SHA=""
GROUP_SHA=""
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

setup_repo() {
    REPO="${TEST_ROOT}/repo"
    rm -rf "${REPO}"
    mkdir -p "${REPO}"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false
    printf 'base\n' >"${REPO}/base.txt"
    git -C "${REPO}" add base.txt
    git -C "${REPO}" commit -q -m "chore: initial"
    BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"

    git -C "${REPO}" checkout -q -b release-pr
    mkdir -p "${REPO}/docs/release-notes" \
        "${REPO}/.github/release-plans"
    printf 'supported: []\n' >"${REPO}/versions.yaml"
    printf '# Changelog\n' >"${REPO}/CHANGELOG.md"
    printf '# Release notes\n' \
        >"${REPO}/docs/release-notes/v0.61.0-rc.1.md"
    printf 'schemaVersion: 2\n' \
        >"${REPO}/.github/release-plans/v0.61.0-rc.1.yaml"
    git -C "${REPO}" add versions.yaml CHANGELOG.md docs/release-notes \
        .github/release-plans
    git -C "${REPO}" commit -q -m "chore(release): prepare v0.61.0-rc.1"
    RELEASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    cat >"${REPO}/candidates.json" <<EOF
[{"number":123,"head_sha":"${RELEASE_SHA}","files":[".github/release-plans/v0.61.0-rc.1.yaml","CHANGELOG.md","docs/release-notes/v0.61.0-rc.1.md","versions.yaml"]}]
EOF
}

create_squash_group() {
    local extra_change="$1"
    local advance_base="${2:-false}"

    git -C "${REPO}" checkout -q main
    git -C "${REPO}" reset -q --hard "${BASE_SHA}"
    if [[ "${advance_base}" == "true" ]]; then
        printf 'already on main\n' >"${REPO}/advanced-base.txt"
        git -C "${REPO}" add advanced-base.txt
        git -C "${REPO}" commit -q -m "fix: advance main before queueing"
    fi
    GROUP_BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" diff --binary "${BASE_SHA}" "${RELEASE_SHA}" |
        git -C "${REPO}" apply --index
    if [[ "${extra_change}" == "true" ]]; then
        printf 'other\n' >"${REPO}/other.txt"
        git -C "${REPO}" add other.txt
    fi
    git -C "${REPO}" commit -q -m "squash merge group"
    GROUP_SHA="$(git -C "${REPO}" rev-parse HEAD)"
}

run_validator() {
    local merge_group_sha="$1"
    local status

    pushd "${REPO}" >/dev/null
    set +e
    bash "${SCRIPT}" --candidates-file candidates.json \
        --merge-group-sha "${merge_group_sha}" --base-sha "${GROUP_BASE_SHA}" \
        --output-file selected.txt
    status=$?
    set -e
    popd >/dev/null
    return "${status}"
}

test_accepts_squash_release_only_group() {
    local group_sha
    create_squash_group false
    group_sha="${GROUP_SHA}"
    if git -C "${REPO}" merge-base --is-ancestor \
        "${RELEASE_SHA}" "${group_sha}"; then
        fail_test "fixture must not make the PR head an ancestor"
        return
    fi
    if ! run_validator "${group_sha}" >/dev/null; then
        fail_test "expected a squash release-only group to pass"
        return
    fi
    if [[ "$(<"${REPO}/selected.txt")" != "123" ]]; then
        fail_test "selector did not identify release PR #123"
        return
    fi
    ((++PASS))
}

test_rejects_group_with_extra_changes() {
    local group_sha
    create_squash_group true
    group_sha="${GROUP_SHA}"
    if run_validator "${group_sha}" >/dev/null 2>&1; then
        fail_test "expected a batched merge group to fail"
        return
    fi
    ((++PASS))
}

test_selects_release_pr_on_advanced_base() {
    local group_sha
    create_squash_group false true
    group_sha="${GROUP_SHA}"
    if [[ "$(git -C "${REPO}" rev-parse "${RELEASE_SHA}^{tree}")" == "$(git -C "${REPO}" rev-parse "${group_sha}^{tree}")" ]]; then
        fail_test "fixture must produce different complete trees"
        return
    fi
    if ! run_validator "${group_sha}" >/dev/null; then
        fail_test "expected selector to identify the release PR on a newer base"
        return
    fi
    ((++PASS))
}

test_accepts_group_without_release_pr() {
    local group_sha

    git -C "${REPO}" checkout -q main
    git -C "${REPO}" reset -q --hard "${BASE_SHA}"
    printf 'other\n' >"${REPO}/other.txt"
    git -C "${REPO}" add other.txt
    git -C "${REPO}" commit -q -m "fix: unrelated change"
    GROUP_BASE_SHA="${BASE_SHA}"
    group_sha="$(git -C "${REPO}" rev-parse HEAD)"
    if ! run_validator "${group_sha}" >/dev/null; then
        fail_test "expected an unrelated merge group to pass"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-merge-group-XXXXXX")"

    setup_repo
    test_accepts_squash_release_only_group
    setup_repo
    test_rejects_group_with_extra_changes
    setup_repo
    test_selects_release_pr_on_advanced_base
    setup_repo
    test_accepts_group_without_release_pr

    if ((FAIL > 0)); then
        echo "release merge-group tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "release merge-group tests passed (${PASS} tests)"
}

main "$@"
