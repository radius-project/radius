#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

mkdir -p "${TEST_DIR}/root module/nested module" "${TEST_DIR}/root module/last"
touch "${TEST_DIR}/root module/go.mod" "${TEST_DIR}/root module/nested module/go.mod" "${TEST_DIR}/root module/last/go.mod"
cd "${TEST_DIR}/root module"

export CALL_LOG="${TEST_DIR}/calls"
export GODEBUG=http2client=0
export INPUT_MODULE_DIRECTORIES="."
export INPUT_RETRIES=5
export INPUT_RETRY_WAIT_SECONDS=5
export FAILURES=0

go() {
    [[ "$*" == "mod download" && "${GODEBUG}" == "http2client=0" ]] || return 99
    local module="${PWD##*/}"
    local count
    count="$(awk -v module="${module}" '$0 == "go " module { count++ } END { print count+0 }' "${CALL_LOG}")"
    echo "go ${module}" >> "${CALL_LOG}"
    [[ "${count}" -ge "${FAILURES}" ]]
}

sleep() {
    echo "sleep $1" >> "${CALL_LOG}"
}

export -f go sleep

run_case() {
    local expected_status="$1"
    : > "${CALL_LOG}"
    local status=0
    bash "${SCRIPT_DIR}/download.sh" > "${TEST_DIR}/output" 2>&1 || status=$?
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
printf 'go root module\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

FAILURES=2
run_case 0
cat > "${TEST_DIR}/expected" <<'EOF'
go root module
sleep 5
go root module
sleep 10
go root module
EOF
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::warning::go mod download in . failed (attempt 2/5); retrying in 10s"

FAILURES=99
run_case 1
cat > "${TEST_DIR}/expected" <<'EOF'
go root module
sleep 5
go root module
sleep 10
go root module
sleep 20
go root module
sleep 40
go root module
EOF
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::error::go mod download in . failed after 5 attempts"

FAILURES=1
INPUT_MODULE_DIRECTORIES=$'.\r\n\nnested module\r\nlast\r\n'
run_case 0
cat > "${TEST_DIR}/expected" <<'EOF'
go root module
sleep 5
go root module
go nested module
sleep 5
go nested module
go last
sleep 5
go last
EOF
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

FAILURES=99
INPUT_RETRIES=1
run_case 1
printf 'go root module\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::error::go mod download in . failed after 1 attempts"

FAILURES=1
INPUT_MODULE_DIRECTORIES=.
INPUT_RETRIES=2
INPUT_RETRY_WAIT_SECONDS=0
run_case 0
printf 'go root module\nsleep 0\ngo root module\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

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
INPUT_RETRY_WAIT_SECONDS=5

INPUT_MODULE_DIRECTORIES=$'.\nmissing'
run_case 1
[[ ! -s "${CALL_LOG}" ]]
assert_output "::error::No go.mod found in missing"

INPUT_MODULE_DIRECTORIES=$'\r\n\n'
run_case 1
[[ ! -s "${CALL_LOG}" ]]
assert_output "::error::module-directories must contain at least one directory"

echo "Go module download tests passed"
