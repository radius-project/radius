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
# Tests for .github/scripts/build-summary.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT="${SCRIPT_DIR}/build-summary.sh"

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

pass_test() {
    ((++PASS))
}

# Run the script with a dedicated summary file and capture its exit code.
# Sets SUMMARY_FILE, STATUS, and OUTPUT for the caller to assert on.
run_summary() {
    local name="$1"
    shift

    SUMMARY_FILE="${TEST_ROOT}/${name}.md"
    : >"${SUMMARY_FILE}"

    STATUS=0
    OUTPUT="$(GITHUB_STEP_SUMMARY="${SUMMARY_FILE}" bash "${SCRIPT}" "$@" 2>&1)" || STATUS=$?
}

test_all_success_passes() {
    run_summary "all-success" "build-check:success" "build-and-push-images:success"

    if [[ "${STATUS}" -ne 0 ]]; then
        fail_test "expected exit 0 for all-success, got ${STATUS}"
        return
    fi
    if ! grep -q "ALL BUILDS PASSED" "${SUMMARY_FILE}"; then
        fail_test "summary is missing the success verdict"
        return
    fi
    if ! grep -q "| build-check | ✅ success |" "${SUMMARY_FILE}"; then
        fail_test "summary is missing the build-check row"
        return
    fi
    pass_test
}

test_skipped_counts_as_success() {
    run_summary "skipped" "build-check:success" "build-and-push-bicep-types:skipped"

    if [[ "${STATUS}" -ne 0 ]]; then
        fail_test "expected skipped to pass, got exit ${STATUS}"
        return
    fi
    if ! grep -q "| build-and-push-bicep-types | ✅ skipped |" "${SUMMARY_FILE}"; then
        fail_test "skipped job should render as passing"
        return
    fi
    pass_test
}

test_failure_fails() {
    run_summary "failure" "build-check:success" "build-and-push-images:failure"

    if [[ "${STATUS}" -eq 0 ]]; then
        fail_test "expected non-zero exit when a job failed"
        return
    fi
    if ! grep -q "BUILD FAILED" "${SUMMARY_FILE}"; then
        fail_test "summary is missing the failure verdict"
        return
    fi
    if [[ "${OUTPUT}" != *"::error::"* ]]; then
        fail_test "expected a workflow error annotation"
        return
    fi
    pass_test
}

test_cancelled_fails() {
    run_summary "cancelled" "build-check:cancelled"

    if [[ "${STATUS}" -eq 0 ]]; then
        fail_test "expected non-zero exit when a job was cancelled"
        return
    fi
    pass_test
}

test_malformed_argument_fails() {
    run_summary "malformed" "build-check"

    if [[ "${STATUS}" -eq 0 ]]; then
        fail_test "expected non-zero exit for a malformed argument"
        return
    fi
    if [[ "${OUTPUT}" != *"malformed argument"* ]]; then
        fail_test "expected a malformed-argument message, got: ${OUTPUT}"
        return
    fi
    # A rejected argument must not leave a partial table behind.
    if [[ -s "${SUMMARY_FILE}" ]]; then
        fail_test "malformed input should not write a summary"
        return
    fi
    pass_test
}

test_no_arguments_fails() {
    run_summary "no-args"

    if [[ "${STATUS}" -eq 0 ]]; then
        fail_test "expected non-zero exit when no jobs were supplied"
        return
    fi
    pass_test
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/build-summary-test-XXXXXX")"

    test_all_success_passes
    test_skipped_counts_as_success
    test_failure_fails
    test_cancelled_fails
    test_malformed_argument_fails
    test_no_arguments_fails

    if ((FAIL > 0)); then
        echo "build summary tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "build summary tests passed (${PASS} tests)"
}

main "$@"
