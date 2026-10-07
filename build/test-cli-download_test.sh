#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Test release discovery and downloads without network requests.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    cat "${TEST_ROOT}/stdout" "${TEST_ROOT}/stderr" >&2
    exit 1
}

mkdir -p "${TEST_ROOT}/bin"
cat >"${TEST_ROOT}/bin/curl" <<'EOF'
#!/bin/bash
set -euo pipefail
url=""
header=""
output=""
write_status=false
fail_http=false
follow_redirects=false
while (($#)); do
    case "$1" in
        -H)
            if [[ "$2" == Authorization:* ]]; then header="$2"; fi
            shift 2
            ;;
        -w) write_status=true; shift 2 ;;
        -o) output="$2"; shift 2 ;;
        -sS) shift ;;
        -sSL) follow_redirects=true; shift ;;
        --fail-with-body) fail_http=true; shift ;;
        https://*) url="$1"; shift ;;
        *) echo "Unexpected curl argument: $1" >&2; exit 90 ;;
    esac
done
if [[ "${url}" == https://api.github.com/repos/radius-project/radius/releases ]]; then
    [[ "${header}" == "${EXPECTED_AUTH}" ]] || exit 91
    $write_status && ! $follow_redirects || exit 92
    echo api >>"${CURL_CALLS}"
    if [[ "${MOCK_CURL_STATUS}" != 0 ]]; then
        echo "curl: fixture transport error" >&2
        exit "${MOCK_CURL_STATUS}"
    fi
    printf '%s\n%s' "${MOCK_BODY}" "${MOCK_HTTP_STATUS}"
elif [[ "${url}" == https://github.com/radius-project/radius/releases/download/* ]]; then
    [[ -z "${header}" && -n "${output}" ]] || exit 93
    $fail_http && $follow_redirects || exit 94
    echo "${url}" >>"${CURL_CALLS}"
    if [[ "${MOCK_DOWNLOAD_STATUS}" != 0 ]]; then
        echo "curl: fixture download error" >&2
        exit "${MOCK_DOWNLOAD_STATUS}"
    fi
    printf 'binary fixture\n' >"${output}"
else
    echo "Unexpected URL: ${url}" >&2
    exit 95
fi
EOF
chmod +x "${TEST_ROOT}/bin/curl"
export PATH="${TEST_ROOT}/bin:${PATH}"
export CURL_CALLS="${TEST_ROOT}/calls"
export GH_TOKEN="" GITHUB_TOKEN="" EXPECTED_AUTH=""
export MOCK_HTTP_STATUS=200 MOCK_CURL_STATUS=0 MOCK_DOWNLOAD_STATUS=0
export MOCK_BODY='[
    {"tag_name":"v0.62.0-rc.1","prerelease":true},
    {"tag_name":"v0.62.0-rc2"},
    {"tag_name":"v0.62.0","draft":true},
    {"tag_name":"v0.63.0","prerelease":true},
    {"tag_name":"edge"},
    {"tag_name":"v0.61.0","prerelease":false},
    {"tag_name":"v0.60.0","prerelease":false}
]'
readonly RELEASES="${MOCK_BODY}"
LAST_STATUS=0

run_script() {
    : >"${CURL_CALLS}"
    LAST_STATUS=0
    (
        cd "${TEST_ROOT}"
        bash "${SCRIPT_DIR}/test-cli-download.sh" "$@"
    ) >"${TEST_ROOT}/stdout" 2>"${TEST_ROOT}/stderr" || LAST_STATUS=$?
}

assert_success() {
    [[ "${LAST_STATUS}" == 0 ]] || fail_test "expected success"
}

assert_failure() {
    [[ "${LAST_STATUS}" != 0 ]] || fail_test "expected failure"
    grep -Fq "$1" "${TEST_ROOT}/stderr" || fail_test "missing error: $1"
    [[ ! -s "${TEST_ROOT}/stdout" ]] || fail_test "failure emitted a version"
}

for auth in none github gh; do
    GH_TOKEN="" GITHUB_TOKEN="" EXPECTED_AUTH=""
    case "${auth}" in
        github)
            GITHUB_TOKEN="fixture-github-token"
            EXPECTED_AUTH="Authorization: Bearer ${GITHUB_TOKEN}"
            ;;
        gh)
            GH_TOKEN="fixture-gh-token"
            GITHUB_TOKEN="fixture-github-token"
            EXPECTED_AUTH="Authorization: Bearer ${GH_TOKEN}"
            ;;
    esac
    run_script --resolve-version
    assert_success
    [[ "$(cat "${TEST_ROOT}/stdout")" == v0.61.0 ]] ||
        fail_test "wrong stable release for ${auth}"
    [[ "$(cat "${CURL_CALLS}")" == api ]] ||
        fail_test "discovery must make exactly one API request"
    if grep -Fq fixture- "${TEST_ROOT}/stdout" "${TEST_ROOT}/stderr"; then
        fail_test "token appeared in output"
    fi
done
echo "PASS: stable selection and optional authentication"

for status in 301 401 403 404 429 500; do
    MOCK_HTTP_STATUS="${status}"
    MOCK_BODY='{"message":"API rate limit exceeded"}'
    run_script --resolve-version
    assert_failure "GitHub API call failed (HTTP ${status})"
    grep -Fq 'API rate limit exceeded' "${TEST_ROOT}/stderr" ||
        fail_test "API response was not retained"
    [[ "$(cat "${CURL_CALLS}")" == api ]] ||
        fail_test "HTTP error triggered another request"
    if grep -Eq 'successful|Failed to extract' "${TEST_ROOT}/stderr"; then
        fail_test "HTTP error was treated as successful release data"
    fi
done
echo "PASS: HTTP errors fail before release parsing"

MOCK_HTTP_STATUS=200
for body in 'not json' '{}' '[]' '[{"tag_name":null}]' \
    '[{"tag_name":"v0.62.0-rc.1"}]' '[{"tag_name":"v0.61.0\ninjected=value"}]'; do
    MOCK_BODY="${body}"
    run_script --resolve-version
    assert_failure "Failed to extract a stable RAD_VERSION"
done
MOCK_BODY="${RELEASES}"
MOCK_CURL_STATUS=7
run_script --resolve-version
assert_failure "curl exit 7"
MOCK_CURL_STATUS=0
echo "PASS: invalid API data and transport failures"

run_script linux amd64 rad
assert_success
[[ "$(wc -l <"${CURL_CALLS}")" -eq 2 ]] ||
    fail_test "standalone download must discover and download"
grep -Fxq api "${CURL_CALLS}" || fail_test "standalone discovery was skipped"
echo "PASS: standalone download still resolves the release"

for platform in linux:amd64 linux:arm64 linux:arm darwin:amd64 \
    darwin:arm64 windows:amd64 windows:arm64; do
    os="${platform%:*}"
    arch="${platform#*:}"
    ext=""
    minimum=""
    if [[ "${os}" == windows ]]; then ext=.exe; fi
    if [[ "${platform}" == windows:arm64 ]]; then minimum=v0.60.0; fi
    run_script "${os}" "${arch}" rad "${ext}" "${minimum}" v0.61.0
    assert_success
    expected="https://github.com/radius-project/radius/releases/download"
    expected+="/v0.61.0/rad_${os}_${arch}${ext}"
    [[ "$(cat "${CURL_CALLS}")" == "${expected}" ]] ||
        fail_test "${platform} did not use only the supplied version"
    [[ -s "${TEST_ROOT}/rad_${os}_${arch}${ext}" ]] ||
        fail_test "${platform} binary is missing"
done
echo "PASS: all seven platforms use the resolved version without API calls"

for version in v0.59.9 v0.60.0 v0.61.0 v0.100.0 v1.0.0; do
    run_script windows arm64 rad .exe v0.60.0 "${version}"
    assert_success
    if [[ "${version}" == v0.59.9 ]]; then
        [[ ! -s "${CURL_CALLS}" ]] || fail_test "old release was downloaded"
        grep -Fq 'predates v0.60.0' "${TEST_ROOT}/stdout" ||
            fail_test "skip reason is missing"
    else
        [[ "$(wc -l <"${CURL_CALLS}")" -eq 1 ]] ||
            fail_test "supported release was not downloaded"
    fi
done
for version in v0.61.0-rc.1 invalid $'v0.61.0\ninjected=value'; do
    run_script linux amd64 rad "" "" "${version}"
    [[ "${LAST_STATUS}" != 0 && ! -s "${CURL_CALLS}" ]] ||
        fail_test "invalid version was accepted"
done
run_script windows arm64 rad .exe invalid v0.61.0
[[ "${LAST_STATUS}" != 0 && ! -s "${CURL_CALLS}" ]] ||
    fail_test "invalid minimum version was accepted"
echo "PASS: version validation and Windows arm64 minimum"

MOCK_DOWNLOAD_STATUS=22
run_script linux amd64 rad "" "" v0.61.0
[[ "${LAST_STATUS}" != 0 ]] || fail_test "download error was ignored"
if grep -Fq 'completed successfully' "${TEST_ROOT}/stdout"; then
    fail_test "download error was reported as success"
fi
echo "PASS: binary download failures remain fatal"
