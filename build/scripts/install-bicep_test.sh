#!/usr/bin/env bash

# Check the Bicep download policy with a fake clock and curl responses.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=build/scripts/install-bicep.sh
source "${SCRIPT_DIR}/install-bicep.sh"

# Unsetting SECONDS removes Bash's clock behavior for deterministic tests.
unset SECONDS
SECONDS=0

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'cleanup; rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

count_lines() {
    local _line count=0
    while IFS= read -r _line; do
        count=$((count + 1))
    done < "$1"
    printf '%s\n' "$count"
}

curl() {
    local output="" timeout="" connect_timeout="" format=""
    local status http_code url="${RELEASES_URL}/tag/v0.46.1"
    while (($#)); do
        case "$1" in
            -o) output="$2"; shift ;;
            -w) format="$2"; shift ;;
            --max-time) timeout="$2"; shift ;;
            --connect-timeout) connect_timeout="$2"; shift ;;
            --head) echo head >> "${TEST_DIR}/heads" ;;
            --retry*) fail_test "curl must not have a second retry policy" ;;
        esac
        shift
    done
    [[ "$connect_timeout" == 15 ]] || fail_test "connection timeout"
    [[ "$format" == '%{http_code} %{url_effective}' ]] \
        || fail_test "response metadata"
    [[ "$timeout" -gt 0 && "$timeout" -le 60 ]] \
        || fail_test "attempt timeout"
    echo "$timeout" >> "${TEST_DIR}/timeouts"
    local count
    count="$(count_lines "${TEST_DIR}/timeouts")"
    status="${RESPONSES[count - 1]:-${RESPONSES[${#RESPONSES[@]} - 1]}}"
    http_code="${status#*:}"
    status="${status%%:*}"
    if [[ "$output" != /dev/null ]]; then
        if [[ "$status" == 0 ]]; then
            printf '%s' "$PAYLOAD" > "$output"
        else
            printf partial > "$output"
        fi
    fi
    printf '%s %s' "$http_code" "$url"
    return "$status"
}

sleep() {
    echo "$1" >> "${TEST_DIR}/delays"
    SECONDS=$((SECONDS + $1))
}

reset_case() {
    : > "${TEST_DIR}/timeouts"
    : > "${TEST_DIR}/delays"
    : > "${TEST_DIR}/heads"
    RESPONSES=("$@")
    PAYLOAD="verified bicep fixture"
    SECONDS=0
}

check_request() {
    local expected="$1" actual=0
    gh_curl "${RELEASES_URL}/download/v0.46.1/bicep-linux-x64" \
        "${TEST_DIR}/download" > "${TEST_DIR}/url" \
        2> "${TEST_DIR}/log" || actual=$?
    [[ "$actual" == "$expected" ]] \
        || fail_test "expected curl status ${expected}, got ${actual}"
}

for response in 22:408 22:429 22:500 22:502 22:503 22:504 \
    5:000 6:000 7:000 18:200 28:000 52:000 55:200 56:200; do
    reset_case "$response" 0:200
    check_request 0
    [[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 2 ]] \
        || fail_test "did not retry ${response}"
    [[ "$(< "${TEST_DIR}/download")" == "$PAYLOAD" ]] \
        || fail_test "partial download was retained"
done

for response in 22:400 22:401 22:403 22:404 22:501 23:200 60:000; do
    reset_case "$response" 0:200
    check_request "${response%%:*}"
    [[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 1 ]] \
        || fail_test "retried permanent error ${response}"
done

# Six failed requests exhausted the old five-retry policy.
reset_case 22:500 22:500 22:500 22:500 22:500 22:500 0:200
check_request 0
[[ "$(paste -sd, "${TEST_DIR}/delays")" == 1,2,4,8,16,32 ]] \
    || fail_test "unexpected backoff"

reset_case 22:500
check_request 22
[[ "$SECONDS" -eq 243 ]] || fail_test "unexpected retry deadline"
[[ "$(tail -1 "${TEST_DIR}/timeouts")" -eq 57 ]] \
    || fail_test "last attempt did not use remaining budget"
[[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 10 ]] \
    || fail_test "unbounded HTTP retries"

reset_case 56:200
check_request 56
[[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 10 ]] \
    || fail_test "unbounded dropped-connection retries"

reset_case 22:500 56:200 22:404 0:200
check_request 22
[[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 3 ]] \
    || fail_test "permanent error after transient errors"

# A request must not start after the shared deadline.
sleep() { SECONDS=$((SECONDS + 300)); }
reset_case 22:500
check_request 28
[[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 1 ]] \
    || fail_test "request started after deadline"

# A clock that does not advance still stops at the retry-count limit.
sleep() { echo "$1" >> "${TEST_DIR}/delays"; }
reset_case 22:500
check_request 22
[[ "$(count_lines "${TEST_DIR}/timeouts")" -eq 11 ]] \
    || fail_test "retry-count limit"

reset_case 0:200
[[ "$(resolve_latest_version)" == v0.46.1 ]] \
    || fail_test "latest version resolution"
[[ "$(count_lines "${TEST_DIR}/heads")" -eq 1 ]] \
    || fail_test "latest version must use HEAD"

# Exercise checksum validation and preserve an existing binary on failure.
uname() {
    case "$1" in
        -s) echo Linux ;;
        -m) echo x86_64 ;;
    esac
}
BICEP_VERSION=v0.46.1
BICEP_OS=linux
BICEP_ARCH=arm64
GITHUB_TOKEN=""
GITHUB_PATH="${TEST_DIR}/github-path"
export BICEP_VERSION BICEP_OS BICEP_ARCH GITHUB_TOKEN GITHUB_PATH
reset_case 0:200
if command -v sha256sum >/dev/null 2>&1; then
    BICEP_CHECKSUM_LINUX_ARM64="$(printf '%s' "$PAYLOAD" | sha256sum)"
else
    BICEP_CHECKSUM_LINUX_ARM64="$(printf '%s' "$PAYLOAD" | shasum -a 256)"
fi
BICEP_CHECKSUM_LINUX_ARM64="${BICEP_CHECKSUM_LINUX_ARM64%% *}"
export BICEP_CHECKSUM_LINUX_ARM64
main "${TEST_DIR}/install" > "${TEST_DIR}/main-log" 2>&1
[[ "$(< "${TEST_DIR}/install/bicep")" == "$PAYLOAD" ]] \
    || fail_test "verified binary was not installed"
[[ ! -e "$GITHUB_PATH" ]] || fail_test "cross-platform PATH export"
cleanup

reset_case 0:200
PAYLOAD="corrupt fixture"
if (trap cleanup EXIT; main "${TEST_DIR}/install") \
    > "${TEST_DIR}/main-log" 2>&1; then
    fail_test "checksum mismatch was accepted"
fi
[[ "$(< "${TEST_DIR}/install/bicep")" == "verified bicep fixture" ]] \
    || fail_test "checksum failure replaced the existing binary"

echo "PASS: Bicep download policy and checksum checks"
