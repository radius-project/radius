#!/usr/bin/env bash

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly ROOT
export GH_TOKEN="offline-fixture-token"
export RETRY_CURL_NO_JITTER=true
export RETRY_CURL_MAX_ATTEMPTS=3
# shellcheck source=.github/scripts/purge-old-images.sh
source "${ROOT}/.github/scripts/purge-old-images.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'rm -rf "${TEST_DIR}"' EXIT

curl() {
    local output="" header_file="" method="GET" url="" auth=""
    while (($#)); do
        case "$1" in
            --output) output="$2"; shift ;;
            --dump-header) header_file="$2"; shift ;;
            --request) method="$2"; shift ;;
            --write-out) shift ;;
            -H) [[ "$2" != Authorization:* ]] || auth="$2"; shift ;;
            https://*) url="$1" ;;
        esac
        shift
    done
    [[ "${auth}" == "Authorization: Bearer ${GH_TOKEN}" ]] || return 2
    [[ "${url}" == https://api.github.com/* ]] || return 2
    printf '%s %s\n' "${method}" "${url}" >> "${TEST_DIR}/calls"
    local code="${CODE:-200}"
    if [[ "${TRANSIENT:-false}" == true &&
        "$(wc -l < "${TEST_DIR}/calls")" -eq 1 ]]; then
        code=503
    fi
    printf 'HTTP/2 %s\r\n%s\r\n' "${code}" "${HEADERS:-}" > "${header_file}"
    printf '%s' "${BODY:-[]}" > "${output}"
    printf '%s %s' "${code}" "${url}"
    [[ "${code}" =~ ^2 ]] || return 22
}
sleep() { printf '%s\n' "$1" >> "${TEST_DIR}/waits"; }

readonly REQUEST_PATH="/orgs/radius-project/packages/container/dev%2Fexample/versions/1"
for code in 200 204 400 404 403 429; do
    : > "${TEST_DIR}/calls"
    CODE="${code}" HEADERS="" cleanup_request github GET "${REQUEST_PATH}" \
        > "${TEST_DIR}/response"
    jq -e --argjson code "${code}" '.status == $code' \
        "${TEST_DIR}/response" > /dev/null
done
: > "${TEST_DIR}/calls"
CODE=404 BODY='{"message":"Not Found"}' \
    HEADERS=$'X-GitHub-Request-Id: ABCD:1234\r\nAuthorization: Bearer offline-fixture-token' \
    cleanup_request github DELETE "${REQUEST_PATH}" > "${TEST_DIR}/response"
jq -e '.status == 404 and .body == "{\"message\":\"Not Found\"}" and
    .headers.requestId == "ABCD:1234" and
    (.headers | keys | sort) == ["remaining", "requestId", "retryAfter"]' \
    "${TEST_DIR}/response" > /dev/null
: > "${TEST_DIR}/calls"
TRANSIENT=true cleanup_request github GET "${REQUEST_PATH}" > "${TEST_DIR}/response"
[[ "$(wc -l < "${TEST_DIR}/calls")" -eq 2 ]]
jq -e '.status == 200' "${TEST_DIR}/response" > /dev/null

: > "${TEST_DIR}/calls"
: > "${TEST_DIR}/waits"
CODE=403 HEADERS="Retry-After: 1" cleanup_request github DELETE "${REQUEST_PATH}" \
    > "${TEST_DIR}/response"
[[ "$(wc -l < "${TEST_DIR}/calls")" -eq 3 ]]
[[ "$(wc -l < "${TEST_DIR}/waits")" -eq 2 ]]
jq -e '.status == 403 and .headers.retryAfter == "1"' \
    "${TEST_DIR}/response" > /dev/null

: > "${TEST_DIR}/calls"
CODE=403 HEADERS="x-ratelimit-remaining: 0" \
    cleanup_request github GET "${REQUEST_PATH}" > "${TEST_DIR}/response"
[[ "$(wc -l < "${TEST_DIR}/calls")" -eq 1 ]]

for unsafe in \
    "/orgs/other/packages" \
    "/orgs/radius-project/packages/container/example" \
    "/orgs/radius-project/packages/container/dev%2Fexample/versions/1?x=1"; do
    if cleanup_request github DELETE "${unsafe}"; then
        echo "Unsafe deletion path accepted: ${unsafe}" >&2
        exit 1
    fi
done
if cleanup_request registry DELETE "/v2/radius-project/dev/example"; then
    echo "Registry deletion accepted" >&2
    exit 1
fi
echo "purge-old-images transport tests passed"
