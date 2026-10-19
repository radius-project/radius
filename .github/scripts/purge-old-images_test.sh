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
    local output="" header_file="" method="GET" url="" authorization=""

    while (($#)); do
        case "$1" in
            -H)
                [[ "$2" == Authorization:* ]] && authorization="$2"
                shift
                ;;
            --output) output="$2"; shift ;;
            --dump-header) header_file="$2"; shift ;;
            --request) method="$2"; shift ;;
            --write-out) shift ;;
            https://*) url="$1" ;;
        esac
        shift
    done
    [[ "${authorization}" == "Authorization: Bearer test-token" ]] ||
        fail_test "GitHub token was not sent"
    if [[ "${method}" == DELETE ]]; then
        printf '%s\n' "${url}" >> "${TEST_DIR}/deletes"
        local delete_count
        local -a delete_codes
        delete_count="$(wc -l < "${TEST_DIR}/deletes")"
        read -r -a delete_codes <<< "${DELETE_CODES:-204}"
        local code_index=$((delete_count - 1))
        if ((code_index >= ${#delete_codes[@]})); then
            code_index=$((${#delete_codes[@]} - 1))
        fi
        local delete_code="${delete_codes[code_index]}"
        printf 'HTTP/2 %s\r\n%s\r\n' "${delete_code}" \
            "${DELETE_HEADERS:-}" > "${header_file}"
        if [[ -n "${DELETE_MESSAGE:-}" ]]; then
            printf '{"message":"%s"}' "${DELETE_MESSAGE}" > "${output}"
        else
            printf '%s' "${DELETE_BODY:-}" > "${output}"
        fi
        printf '%s %s' "${delete_code}" "${url}"
        [[ "${delete_code}" =~ ^2 ]] && return 0
        return 22
    fi

    printf '%s\n' "${url}" >> "${TEST_DIR}/gets"
    if [[ "${GET_FAILURE_PATH:-}" == packages && "${url}" == *"/packages?"* ]] ||
        [[ "${GET_FAILURE_PATH:-}" == versions && "${url}" == *"/versions?"* ]]; then
        local failure_code="${GET_FAILURE_CODE:-400}"
        printf 'HTTP/2 %s\r\n%s\r\n' "${failure_code}" \
            "${GET_FAILURE_HEADERS:-}" > "${header_file}"
        printf '{"message":"request failed"}' > "${output}"
        printf '%s %s' "${failure_code}" "${url}"
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
            if [[ "${PACKAGE_PAGINATION:-false}" == true &&
                "${url}" == *"&page=1" ]]; then
                printf '[' > "${output}"
                local index
                for ((index = 1; index <= 100; index++)); do
                    ((index == 1)) || printf ',' >> "${output}"
                    printf '{"name":"dev/package-%s"}' "${index}" \
                        >> "${output}"
                done
                printf ']' >> "${output}"
            elif [[ "${PACKAGE_PAGINATION:-false}" == true ]]; then
                printf '[{"name":"dev/package-final"}]' > "${output}"
            else
                cat > "${output}" <<'JSON'
[{"name":"dev/example"},{"name":"production/example"}]
JSON
            fi
            ;;
        *"/dev%2Fexample/versions"*)
            if [[ "${INVALID_JSON_PATH:-}" == versions ]]; then
                printf 'invalid JSON' > "${output}"
            elif [[ "${VERSION_PAGINATION:-false}" == true &&
                "${url}" == *"&page=1" ]]; then
                printf '[' > "${output}"
                local index
                for ((index = 1; index <= 100; index++)); do
                    ((index == 1)) || printf ',' >> "${output}"
                    printf '{"id":%s,"updated_at":"2020-01-01T00:00:00Z",' \
                        "${index}" >> "${output}"
                    printf '"metadata":{"container":{"tags":["pr-%s"]}}}' \
                        "${index}" >> "${output}"
                done
                printf ']' >> "${output}"
            elif [[ "${VERSION_PAGINATION:-false}" == true ]]; then
                cat > "${output}" <<'JSON'
[
  {
    "id": 201,
    "updated_at": "2020-01-01T00:00:00Z",
    "metadata": {"container": {"tags": ["pr-201"]}}
  }
]
JSON
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
DELETE_CODES="500 204"
DELETE_BODY='Server Error'
export DELETE_CODES DELETE_BODY
main > "${TEST_DIR}/output" 2>&1
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 2 ]] ||
    fail_test "transient delete was not retried"
grep -q "Deleted dev/example version 101" "${TEST_DIR}/output" ||
    fail_test "retried delete did not succeed"

unset DELETE_CODES DELETE_BODY
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

cleanup
: > "${TEST_DIR}/gets"
PACKAGE_PAGINATION=true
SKIP_TRANSIENT_FAILURE=true
export PACKAGE_PAGINATION SKIP_TRANSIENT_FAILURE
WORK_DIR="$(mktemp -d)"
packages="$(list_packages)"
[[ "$(wc -l <<< "${packages}")" -eq 101 ]] ||
    fail_test "package pagination count"
grep -q '^dev/package-1$' <<< "${packages}" ||
    fail_test "first package page missing"
grep -q '^dev/package-final$' <<< "${packages}" ||
    fail_test "second package page missing"
unset PACKAGE_PAGINATION

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
VERSION_PAGINATION=true
export VERSION_PAGINATION
main > "${TEST_DIR}/output" 2>&1
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 101 ]] ||
    fail_test "version pagination delete count"
grep -q '/versions/1$' "${TEST_DIR}/deletes" ||
    fail_test "first version page was not deleted"
grep -q '/versions/201$' "${TEST_DIR}/deletes" ||
    fail_test "second version page was not deleted"
unset VERSION_PAGINATION SKIP_TRANSIENT_FAILURE

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
VERSION_PAGINATION=true
SKIP_TRANSIENT_FAILURE=true
DELETE_CODES="400 204"
DELETE_MESSAGE="You cannot delete the last tagged version"
export VERSION_PAGINATION SKIP_TRANSIENT_FAILURE DELETE_CODES DELETE_MESSAGE
main > "${TEST_DIR}/output" 2>&1 ||
    fail_test "permanent version delete failure stopped cleanup"
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 101 ]] ||
    fail_test "cleanup did not continue after a skipped version"
grep -q '::warning::Skipped dev/example version 1: HTTP 400 You cannot' \
    "${TEST_DIR}/output" || fail_test "skipped version warning"
grep -q 'Deleted 100, already deleted 0, skipped 1' "${TEST_DIR}/output" ||
    fail_test "cleanup summary"

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
DELETE_CODES="403"
DELETE_MESSAGE="Must have admin rights"
export DELETE_CODES DELETE_MESSAGE
if main > "${TEST_DIR}/output" 2>&1; then
    fail_test "authorization failure was accepted"
fi
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 1 ]] ||
    fail_test "cleanup continued after an authorization failure"
grep -q 'HTTP 403: Must have admin rights' "${TEST_DIR}/output" ||
    fail_test "delete error body was not shown"

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
DELETE_CODES="204 403"
DELETE_HEADERS="x-ratelimit-remaining: 0"
DELETE_MESSAGE="API rate limit exceeded"
export DELETE_CODES DELETE_HEADERS DELETE_MESSAGE
main > "${TEST_DIR}/output" 2>&1 ||
    fail_test "delete rate limit failed the cleanup"
[[ "$(wc -l < "${TEST_DIR}/deletes")" -eq 2 ]] ||
    fail_test "cleanup continued after the rate limit"
grep -q '::warning::GitHub API rate limit reached' "${TEST_DIR}/output" ||
    fail_test "rate limit warning"
unset VERSION_PAGINATION DELETE_CODES DELETE_HEADERS DELETE_MESSAGE

cleanup
: > "${TEST_DIR}/gets"
: > "${TEST_DIR}/deletes"
GET_FAILURE_PATH=versions
GET_FAILURE_CODE=403
GET_FAILURE_HEADERS="x-ratelimit-remaining: 0"
export GET_FAILURE_PATH GET_FAILURE_CODE GET_FAILURE_HEADERS
main > "${TEST_DIR}/output" 2>&1 ||
    fail_test "read rate limit failed the cleanup"
[[ ! -s "${TEST_DIR}/deletes" ]] ||
    fail_test "versions were deleted after a read rate limit"
GET_FAILURE_HEADERS=""
if main > "${TEST_DIR}/output" 2>&1; then
    fail_test "read authorization failure was accepted"
fi
grep -q 'returned HTTP 403: request failed' "${TEST_DIR}/output" ||
    fail_test "read error body was not shown"
unset GET_FAILURE_PATH GET_FAILURE_CODE GET_FAILURE_HEADERS
unset SKIP_TRANSIENT_FAILURE

echo "purge-old-images tests passed"
