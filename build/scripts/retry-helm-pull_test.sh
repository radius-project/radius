#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=build/scripts/retry-helm-pull.sh
source "${SCRIPT_DIR}/retry-helm-pull.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

helm() {
    local call
    call="$(( $(< "${TEST_DIR}/calls") + 1 ))"
    printf '%s\n' "${call}" > "${TEST_DIR}/calls"
    printf '%s\n' "$*" >> "${TEST_DIR}/arguments"
    case "${RESPONSES[call - 1]}" in
        success) return 0 ;;
        *) printf '%s\n' "${RESPONSES[call - 1]}" >&2; return 1 ;;
    esac
}

sleep() {
    printf '%s\n' "$1" >> "${TEST_DIR}/delays"
}

reset_case() {
    RESPONSES=("$@")
    printf '0\n' > "${TEST_DIR}/calls"
    : > "${TEST_DIR}/arguments"
    : > "${TEST_DIR}/delays"
}

RETRY_HELM_NO_JITTER=true
export RETRY_HELM_NO_JITTER

reset_case "HTTP 503 Service Unavailable" "unexpected EOF" success
retry_helm_pull oci://example.test/chart --version 1.0.0
[[ "$(< "${TEST_DIR}/calls")" -eq 3 ]] ||
    fail_test "transient retry count"
[[ "$(paste -sd, "${TEST_DIR}/delays")" == 1,2 ]] ||
    fail_test "retry delays"

for error in "401 Unauthorized" "403 Forbidden" "404 Not Found" \
    "manifest unknown" "invalid reference" \
    "404 Not Found: https://example.test/500/chart.tgz" \
    "401 Unauthorized from registry.example.test:5000"; do
    reset_case "${error}" success
    status=0
    retry_helm_pull oci://example.test/chart || status=$?
    [[ "${status}" -eq 1 && "$(< "${TEST_DIR}/calls")" -eq 1 ]] ||
        fail_test "retried permanent error ${error}"
done

for error in "HTTP/2 503 Service Unavailable" \
    "unexpected status code 429 Too Many Requests"; do
    reset_case "${error}" success
    retry_helm_pull oci://example.test/chart
    [[ "$(< "${TEST_DIR}/calls")" -eq 2 ]] ||
        fail_test "did not retry HTTP status ${error}"
done

reset_case "connection reset" "connection reset"
RETRY_HELM_MAX_ATTEMPTS=2
status=0
retry_helm_pull oci://example.test/chart || status=$?
unset RETRY_HELM_MAX_ATTEMPTS
[[ "${status}" -eq 1 && "$(< "${TEST_DIR}/calls")" -eq 2 ]] ||
    fail_test "attempt limit"

echo "retry-helm-pull tests passed"
