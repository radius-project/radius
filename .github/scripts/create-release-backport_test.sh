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
readonly SCRIPT="${SCRIPT_DIR}/create-release-backport.sh"

TEST_ROOT=""
REMOTE=""
REPO=""
SOURCE_COMMIT=""
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
    local conflict="$1"
    local seed="${TEST_ROOT}/seed"

    rm -rf "${seed}" "${REMOTE}" "${REPO}"
    mkdir -p "${seed}"
    git -C "${seed}" init -q -b main
    git -C "${seed}" config user.name "Radius Test"
    git -C "${seed}" config user.email "test@example.com"
    git -C "${seed}" config commit.gpgsign false
    mkdir -p "${seed}/.github/workflows"
    printf 'base\n' >"${seed}/file.txt"
    printf 'name: legacy-release\n' \
        >"${seed}/.github/workflows/release.yaml"
    git -C "${seed}" add file.txt .github/workflows/release.yaml
    git -C "${seed}" commit -q -m "chore: initial"
    git -C "${seed}" branch release/0.60

    if [[ "${conflict}" == "true" ]]; then
        git -C "${seed}" checkout -q release/0.60
        printf 'release change\n' >"${seed}/file.txt"
        git -C "${seed}" commit -qam "fix: release branch change"
        git -C "${seed}" checkout -q main
    fi

    printf 'source change\n' >"${seed}/file.txt"
    git -C "${seed}" commit -qam "fix: source change" \
        --author "Source Author <source@example.test>" \
        -m $'BREAKING CHANGE: preserve this source footer\n\nEOF\nbranch=attacker'
    SOURCE_COMMIT="$(git -C "${seed}" rev-parse HEAD)"
    git clone -q --bare "${seed}" "${REMOTE}"
    git clone -q "${REMOTE}" "${REPO}"
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false
    git -C "${REPO}" fetch -q origin \
        '+refs/heads/*:refs/remotes/origin/*'
}

run_backport() {
    rm -rf "${REPO}/out"
    git -C "${REPO}" config --unset user.name || true
    git -C "${REPO}" config --unset user.email || true
    pushd "${REPO}" >/dev/null
    bash "${SCRIPT}" --source-pr 123 --source-commit "${SOURCE_COMMIT}" \
        --source-title 'fix: source change' \
        --source-url 'https://example.test/pull/123' --channel 0.60 \
        --output-dir out
    popd >/dev/null
}

test_successful_backport() {
    local message base_commit

    setup_repo false
    run_backport
    if [[ "$(cat "${REPO}/out/status.txt")" != "success" ]]; then
        fail_test "expected a successful backport"
        return
    fi
    message="$(cat "${REPO}/out/commit-message.txt")"
    if [[ "${message}" != *"cherry picked from commit ${SOURCE_COMMIT}"* ]]; then
        fail_test "successful backport did not preserve -x traceability"
        return
    fi
    if [[ "${message}" != *"BREAKING CHANGE: preserve this source footer"* ]]; then
        fail_test "successful backport dropped the source commit body"
        return
    fi
    if [[ "${message}" != *$'EOF\nbranch=attacker'* ]]; then
        fail_test "successful backport did not preserve hostile body lines"
        return
    fi
    if [[ -d "${REPO}/.github/backport-conflicts" ]]; then
        fail_test "successful backport created a conflict handoff"
        return
    fi
    if git -C "${REPO}" diff --cached --name-only --diff-filter=D |
        grep -Fq '.github/workflows/release.yaml'; then
        fail_test "ordinary backport deleted the legacy release workflow"
        return
    fi
    if [[ "$(git -C "${REPO}" rev-parse HEAD)" != "$(git -C "${REPO}" rev-parse origin/release/0.60)" ]]; then
        fail_test "script created an unsigned local commit"
        return
    fi
    if git -C "${REPO}" diff --cached --quiet; then
        fail_test "successful backport did not leave staged changes"
        return
    fi
    base_commit="$(git -C "${REPO}" rev-parse origin/release/0.60)"
    if ! grep -Fq "<!-- radius-backport-base: ${base_commit} -->" \
        "${REPO}/out/pull-request-body.md"; then
        fail_test "backport PR body did not bind the release base"
        return
    fi
    if [[ "$(cat "${REPO}/out/author.txt")" != "Source Author <source@example.test>" ]]; then
        fail_test "successful backport did not preserve the source author"
        return
    fi
    ((++PASS))
}

test_release_metadata_backport_removes_legacy_workflow() {
    setup_repo false
    mkdir -p "${REPO}/.github/release-plans"
    printf 'schemaVersion: 2\n' \
        >"${REPO}/.github/release-plans/v0.60.1.yaml"
    git -C "${REPO}" add .github/release-plans/v0.60.1.yaml
    git -C "${REPO}" commit -q \
        -m "chore(release): prepare v0.60.1"
    SOURCE_COMMIT="$(git -C "${REPO}" rev-parse HEAD)"

    run_backport
    if ! git -C "${REPO}" diff --cached --name-only --diff-filter=D |
        grep -Fqx '.github/workflows/release.yaml'; then
        fail_test "release metadata backport kept the legacy workflow"
        return
    fi
    if ! git -C "${REPO}" diff --cached --name-only --diff-filter=A |
        grep -Fqx '.github/release-plans/v0.60.1.yaml'; then
        fail_test "release metadata backport dropped the immutable plan"
        return
    fi
    ((++PASS))
}

test_conflict_creates_safe_handoff() {
    local handoff
    local fresh="${TEST_ROOT}/fresh"
    local cherry_pick_status

    setup_repo true
    run_backport
    if [[ "$(cat "${REPO}/out/status.txt")" != "conflict" ]]; then
        fail_test "expected a conflict handoff"
        return
    fi
    if git -C "${REPO}" grep -nE '^(<<<<<<<|=======|>>>>>>>)' HEAD -- \
        ':!*.md' >/dev/null; then
        fail_test "conflict markers were committed"
        return
    fi
    handoff="${REPO}/out/conflict-handoff.md"
    local base_commit
    base_commit="$(git -C "${REPO}" rev-parse origin/release/0.60)"
    if ! grep -Fq "git reset --hard ${base_commit}" "${handoff}" ||
        ! grep -Fq "git cherry-pick -x ${SOURCE_COMMIT}" "${handoff}"; then
        fail_test "handoff does not contain exact recovery commands"
        return
    fi
    if [[ "$(git -C "${REPO}" show HEAD:file.txt)" != "release change" ]]; then
        fail_test "conflict branch did not preserve the release base"
        return
    fi

    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" commit -q -F out/commit-message.txt
    git -C "${REPO}" push -q origin \
        "HEAD:refs/heads/automation/backport-123-to-0.60"
    git clone -q --single-branch --branch release/0.60 "${REMOTE}" "${fresh}"
    git -C "${fresh}" config user.name "Radius Test"
    git -C "${fresh}" config user.email "test@example.com"
    git -C "${fresh}" config commit.gpgsign false
    git -C "${fresh}" fetch -q origin "${SOURCE_COMMIT}" \
        'refs/heads/automation/backport-123-to-0.60:refs/remotes/origin/automation/backport-123-to-0.60' \
        'refs/heads/release/0.60:refs/remotes/origin/release/0.60'
    git -C "${fresh}" checkout -q -B automation/backport-123-to-0.60 \
        origin/automation/backport-123-to-0.60
    git -C "${fresh}" reset --hard -q "${base_commit}"
    set +e
    git -C "${fresh}" cherry-pick -x "${SOURCE_COMMIT}" >/dev/null 2>&1
    cherry_pick_status=$?
    set -e
    if ((cherry_pick_status == 0)); then
        fail_test "fresh-clone handoff did not reproduce the expected conflict"
        return
    fi
    git -C "${fresh}" cherry-pick --abort
    if [[ -s "${REPO}/out/author.txt" ]]; then
        fail_test "conflict handoff must stay authored by the bot"
        return
    fi
    ((++PASS))
}

test_rejects_advanced_release_branch() {
    local stale_base

    setup_repo false
    stale_base="$(git -C "${REPO}" rev-parse origin/release/0.60)"
    git -C "${REPO}" checkout -q release/0.60
    git -C "${REPO}" commit -q --allow-empty -m "fix: advance release branch"
    git -C "${REPO}" push -q origin release/0.60
    git -C "${REPO}" fetch -q origin \
        'refs/heads/release/0.60:refs/remotes/origin/release/0.60'

    if (
        cd "${REPO}"
        bash "${SCRIPT}" --source-pr 123 --source-commit "${SOURCE_COMMIT}" \
            --source-title 'fix: source change' \
            --source-url 'https://example.test/pull/123' --channel 0.60 \
            --output-dir out --expected-base "${stale_base}"
    ) >/dev/null 2>&1; then
        fail_test "expected an advanced release branch to fail"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-backport-test-XXXXXX")"
    REMOTE="${TEST_ROOT}/remote.git"
    REPO="${TEST_ROOT}/repo"

    test_successful_backport
    test_release_metadata_backport_removes_legacy_workflow
    test_conflict_creates_safe_handoff
    test_rejects_advanced_release_branch

    if ((FAIL > 0)); then
        echo "create release backport tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "create release backport tests passed (${PASS} tests)"
}

main "$@"
