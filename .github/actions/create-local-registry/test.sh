#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

export CALL_LOG="${TEST_DIR}/calls"
export INPUT_IMAGE="registry:2"
export INPUT_RETRIES=5
export INPUT_RETRY_WAIT_SECONDS=5
export FAILURES=0

docker() {
    [[ "$*" == "pull registry:2" ]] || return 99
    local count
    count="$(grep -c '^docker$' "${CALL_LOG}" || true)"
    echo "docker" >> "${CALL_LOG}"
    [[ "${count}" -ge "${FAILURES}" ]]
}

sleep() {
    echo "sleep $1" >> "${CALL_LOG}"
}

export -f docker sleep

run_case() {
    local expected_status="$1"
    : > "${CALL_LOG}"
    local status=0
    bash "${SCRIPT_DIR}/pull-image.sh" > "${TEST_DIR}/output" 2>&1 || status=$?
    if [[ "${status}" -ne "${expected_status}" ]]; then
        cat "${TEST_DIR}/output"
        echo "Expected exit ${expected_status}, got ${status}" >&2
        exit 1
    fi
}

assert_output() {
    grep -F -- "$1" "${TEST_DIR}/output" > /dev/null
}

run_case 0
printf 'docker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

FAILURES=2
run_case 0
printf 'docker\nsleep 5\ndocker\nsleep 10\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::warning::docker pull registry:2 failed (attempt 2/5); retrying in 10s"

FAILURES=99
run_case 1
printf 'docker\nsleep 5\ndocker\nsleep 10\ndocker\nsleep 20\ndocker\nsleep 40\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::error::docker pull registry:2 failed after 5 attempts"

for INPUT_RETRIES in 0 -1 invalid 01; do
    run_case 1
    [[ ! -s "${CALL_LOG}" ]]
    assert_output "::error::retries must be a positive integer"
done
INPUT_RETRIES=5

for INPUT_RETRY_WAIT_SECONDS in -1 invalid 01; do
    run_case 1
    [[ ! -s "${CALL_LOG}" ]]
    assert_output "::error::retry-wait-seconds must be a nonnegative integer"
done

echo "Local registry image pull tests passed"
