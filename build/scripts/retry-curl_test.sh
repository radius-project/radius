#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=build/scripts/retry-curl.sh
source "${SCRIPT_DIR}/retry-curl.sh"

unset SECONDS
SECONDS=0
RETRY_CURL_NO_JITTER=true
export RETRY_CURL_NO_JITTER

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

curl() {
    local output="" headers="" timeout="" speed_limit="" speed_time=""
    local status http_code effective_url="https://example.test/final"

    while (($#)); do
        case "$1" in
            --fail | --fail-with-body)
                printf '%s\n' "$1" > "${TEST_DIR}/fail-flag"
                ;;
            --output) output="$2"; shift ;;
            --dump-header) headers="$2"; shift ;;
            --max-time) timeout="$2"; shift ;;
            --speed-limit) speed_limit="$2"; shift ;;
            --speed-time) speed_time="$2"; shift ;;
        esac
        shift
    done
    [[ -n "${output}" && -n "${headers}" ]] ||
        fail_test "curl output paths"
    [[ "${timeout}" -gt 0 && "${timeout}" -le 300 ]] ||
        fail_test "attempt timeout"
    [[ "${speed_limit}" -eq 1024 && "${speed_time}" -eq 30 ]] ||
        fail_test "stalled transfer limits"

    local curl_calls
    curl_calls="$(( $(< "${TEST_DIR}/curl-calls") + 1 ))"
    printf '%s\n' "${curl_calls}" > "${TEST_DIR}/curl-calls"
    status="${RESPONSES[curl_calls - 1]}"
    http_code="${status#*:}"
    status="${status%%:*}"
    printf '%s' "${RESPONSE_HEADERS:-}" > "${headers}"
    printf 'HTTP/2 %s\r\n' "${http_code}" >> "${headers}"
    if [[ -n "${RETRY_AFTER:-}" ]]; then
        printf 'Retry-After: %s\r\n' "${RETRY_AFTER}" >> "${headers}"
    fi
    printf '\r\n' >> "${headers}"
    if ((status == 0)); then
        printf 'complete' > "${output}"
    else
        printf 'partial' > "${output}"
    fi
    printf '%s %s' "${http_code}" "${effective_url}"
    return "${status}"
}

sleep() {
    DELAYS+=("$1")
    SECONDS=$((SECONDS + $1))
}

reset_case() {
    RESPONSES=("$@")
    printf '0\n' > "${TEST_DIR}/curl-calls"
    DELAYS=()
    RETRY_AFTER=""
    RESPONSE_HEADERS=""
    SECONDS=0
}

reset_case 22:503 56:200 0:200
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file
[[ "$(< "${TEST_DIR}/curl-calls")" -eq 3 ]] ||
    fail_test "transient retry count"
[[ "${DELAYS[*]}" == "1 2" ]] || fail_test "exponential backoff"
[[ "$(< "${TEST_DIR}/download")" == complete ]] ||
    fail_test "partial output retained"

for response in 22:400 22:401 22:403 22:404 22:501 23:200 60:000; do
    reset_case "${response}" 0:200
    status=0
    retry_curl -fsSL -o "${TEST_DIR}/download" \
        https://example.test/file || status=$?
    [[ "${status}" -eq "${response%%:*}" ]] ||
        fail_test "permanent status ${response}"
    [[ "$(< "${TEST_DIR}/curl-calls")" -eq 1 ]] ||
        fail_test "retried permanent status ${response}"
done

reset_case 22:404
status=0
http_code="$(
    retry_curl -fsSL -o "${TEST_DIR}/download" -w '%{http_code}' \
        https://example.test/file
)" || status=$?
[[ "${status}" -eq 22 && "${http_code}" == 404 ]] ||
    fail_test "HTTP code output on failure"

reset_case 22:429 0:200
RETRY_AFTER=10
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file
[[ "${DELAYS[*]}" == 10 ]] || fail_test "Retry-After"

reset_case 22:503 0:200
RETRY_AFTER="Thu, 01 Jan 1970 00:20:50 GMT"
date() {
    case "$*" in
        "-u +%s") echo 1000 ;;
        "-u -d "*) echo 1250 ;;
        *) command date "$@" ;;
    esac
}
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file
unset -f date
[[ "${DELAYS[*]}" == 250 ]] || fail_test "HTTP-date Retry-After"

reset_case 22:503 0:200
RESPONSE_HEADERS=$'HTTP/2 302\r\nRetry-After: 300\r\n\r\n'
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file
[[ "${DELAYS[*]}" == 1 ]] || fail_test "redirect Retry-After"

reset_case 22:429 0:200
RETRY_AFTER=300
status=0
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file || status=$?
[[ "${status}" -eq 22 && "$(< "${TEST_DIR}/curl-calls")" -eq 1 ]] ||
    fail_test "server wait exceeded budget"

reset_case 0:200
effective_url="$(
    retry_curl -fsSLI -o /dev/null -w '%{url_effective}' \
        https://example.test/latest
)"
[[ "${effective_url}" == https://example.test/final ]] ||
    fail_test "effective URL"

reset_case 0:200
payload="$(retry_curl -fsSL https://example.test/text)"
[[ "${payload}" == complete ]] || fail_test "stdout payload"

reset_case 22:503 22:503
RETRY_CURL_MAX_ATTEMPTS=2
status=0
retry_curl -fsSL -o "${TEST_DIR}/download" \
    https://example.test/file || status=$?
unset RETRY_CURL_MAX_ATTEMPTS
[[ "${status}" -eq 22 && "$(< "${TEST_DIR}/curl-calls")" -eq 2 ]] ||
    fail_test "attempt limit"

status=0
retry_curl -fsSL http://example.test/file || status=$?
[[ "${status}" -eq 2 ]] || fail_test "HTTP rejection"

reset_case 0:200
retry_curl -o "${TEST_DIR}/download" https://example.test/file
[[ "$(< "${TEST_DIR}/fail-flag")" == --fail ]] ||
    fail_test "default fail flag"

reset_case 22:400
RESPONSE_HEADERS=$'X-Test: kept\r\n'
status=0
retry_curl --fail-with-body --dump-header "${TEST_DIR}/headers" \
    -o "${TEST_DIR}/download" https://example.test/file || status=$?
[[ "${status}" -eq 22 ]] || fail_test "fail-with-body status"
[[ "$(< "${TEST_DIR}/fail-flag")" == --fail-with-body ]] ||
    fail_test "fail-with-body flag"
grep -q '^X-Test: kept' "${TEST_DIR}/headers" ||
    fail_test "response headers were not copied"
grep -q '^HTTP/2 400' "${TEST_DIR}/headers" ||
    fail_test "response status header was not copied"

echo "retry-curl tests passed"
