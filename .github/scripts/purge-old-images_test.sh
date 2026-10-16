#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly ROOT
export PACKAGE_OWNER="radius-project"
export GH_TOKEN="test-token"
export PURGE_CUTOFF_EPOCH="2000000000"
export RETRY_CURL_NO_JITTER=true
# shellcheck source=.github/scripts/purge-old-images.sh
source "${ROOT}/.github/scripts/purge-old-images.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'cleanup; rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

curl() {
    local output="" header_file="" method="GET" url=""

    while (($#)); do
        case "$1" in
            --output) output="$2"; shift ;;
            --dump-header) header_file="$2"; shift ;;
            --request) method="$2"; shift ;;
            --write-out) shift ;;
            https://*) url="$1" ;;
        esac
        shift
    done
    if [[ "${method}" == DELETE ]]; then
        printf '%s\n' "${url}" >> "${TEST_DIR}/deletes"
        printf '%s' "${DELETE_BODY:-}" > "${output}"
        printf '%s' "${DELETE_CODE:-204}"
        return "${DELETE_STATUS:-0}"
    fi

    printf '%s\n' "${url}" >> "${TEST_DIR}/gets"
    if [[ "${GET_FAILURE_PATH:-}" == packages && "${url}" == *"/packages?"* ]] ||
        [[ "${GET_FAILURE_PATH:-}" == versions && "${url}" == *"/versions?"* ]]; then
        printf 'HTTP/2 400\r\n\r\n' > "${header_file}"
        printf 'request failed' > "${output}"
        printf '400 %s' "${url}"
        return 22
    fi
    local get_count
    get_count="$(wc -l < "${TEST_DIR}/gets")"
    if [[ "${SKIP_TRANSIENT_FAILURE:-false}" != true &&
        "${get_count}" -eq 1 ]]; then
        printf 'HTTP/2 503\r\n\r\n' > "${header_file}"
        printf 'temporary' > "${output}"
        printf '503 %s' "${url}"
        return 22
    fi

    printf 'HTTP/2 200\r\n\r\n' > "${header_file}"
    case "${url}" in
        *"/packages?package_type="*)
            cat > "${output}" <<'JSON'
[{"name":"dev/example"},{"name":"production/example"}]
JSON
            ;;
        *"/dev%2Fexample/versions"*)
            if [[ "${INVALID_JSON_PATH:-}" == versions ]]; then
                printf 'invalid JSON' > "${output}"
            else
                cat > "${output}" <<'JSON'
[
  {
    "id": 101,
    "updated_at": "2020-01-01T00:00:00Z",
    "metadata": {"container": {"tags": ["pr-101"]}}
  },
  {
    "id": 102,
    "updated_at": "2020-01-01T00:00:00Z",
    "metadata": {"container": {"tags": ["latest"]}}
  },
  {
    "id": 103,
    "updated_at": "2099-01-01T00:00:00Z",
    "metadata": {"container": {"tags": ["pr-103"]}}
  }
]
JSON
            fi
            ;;
        *) fail_test "unexpected GET ${url}" ;;
    esac
    printf '200 %s' "${url}"
}

sleep() {
    :
}

: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
main
[[ "$(wc -l < "${TEST_DIR}/gets")" -eq 3 ]] ||
    fail_test "read retry count"
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 1 ]] ||
    fail_test "delete count"
grep -q '/versions/101$' "${TEST_DIR}/deletes" ||
    fail_test "wrong version deleted"

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
DELETE_CODE=500
DELETE_BODY='Server Error'
export DELETE_CODE DELETE_BODY
if main > "${TEST_DIR}/output" 2>&1; then
    fail_test "delete failure was accepted"
fi
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 1 ]] ||
    fail_test "delete was retried"
grep -q "Delete failed with HTTP 500" "${TEST_DIR}/output" ||
    fail_test "delete failure was not reported"

unset DELETE_CODE DELETE_BODY
cleanup
for failure_path in packages versions; do
    : > "${TEST_DIR}/gets"
    GET_FAILURE_PATH="${failure_path}"
    SKIP_TRANSIENT_FAILURE=true
    export GET_FAILURE_PATH SKIP_TRANSIENT_FAILURE
    if main > "${TEST_DIR}/output" 2>&1; then
        fail_test "${failure_path} read failure was accepted"
    fi
done
unset GET_FAILURE_PATH

cleanup
: > "${TEST_DIR}/gets"
INVALID_JSON_PATH=versions
export INVALID_JSON_PATH
if main > "${TEST_DIR}/output" 2>&1; then
    fail_test "versions JSON failure was accepted"
fi
unset INVALID_JSON_PATH SKIP_TRANSIENT_FAILURE

echo "purge-old-images tests passed"
