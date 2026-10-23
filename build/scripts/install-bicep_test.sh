#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=build/scripts/install-bicep.sh
source "${SCRIPT_DIR}/install-bicep.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
trap 'cleanup; rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

curl() {
    local output="" header_file="" write_out=""
    local effective_url="${RELEASES_URL}/tag/v0.46.1"

    printf '%s\n' "$*" > "${TEST_DIR}/curl-args"
    while (($#)); do
        case "$1" in
            --output) output="$2"; shift ;;
            --dump-header) header_file="$2"; shift ;;
            --write-out) write_out="$2"; shift ;;
            --head) printf 'head\n' > "${TEST_DIR}/head" ;;
        esac
        shift
    done
    [[ -n "${output}" && -n "${header_file}" ]] ||
        fail_test "curl output paths"
    [[ "${write_out}" == '%{http_code} %{url_effective}' ]] ||
        fail_test "curl response metadata"

    printf 'HTTP/2 200\r\n\r\n' > "${header_file}"
    if [[ "${output}" != /dev/null ]]; then
        printf '%s' "${PAYLOAD}" > "${output}"
    fi
    printf '200 %s' "${effective_url}"
}

uname() {
    case "$1" in
        -s) echo Linux ;;
        -m) echo x86_64 ;;
    esac
}

PAYLOAD="verified bicep fixture"
GITHUB_TOKEN="test-token"
export GITHUB_TOKEN
gh_curl -fsSL -o "${TEST_DIR}/download" \
    "${RELEASES_URL}/download/v0.46.1/bicep-linux-x64"
[[ "$(< "${TEST_DIR}/download")" == "${PAYLOAD}" ]] ||
    fail_test "download payload"
grep -q "Authorization:" "${TEST_DIR}/curl-args" ||
    fail_test "authenticated request"

[[ "$(resolve_latest_version)" == v0.46.1 ]] ||
    fail_test "latest version resolution"
[[ -s "${TEST_DIR}/head" ]] || fail_test "latest version must use HEAD"

BICEP_VERSION=v0.46.1
BICEP_OS=linux
BICEP_ARCH=arm64
GITHUB_TOKEN=""
GITHUB_PATH="${TEST_DIR}/github-path"
export BICEP_VERSION BICEP_OS BICEP_ARCH GITHUB_TOKEN GITHUB_PATH
if command -v sha256sum >/dev/null 2>&1; then
    BICEP_CHECKSUM_LINUX_ARM64="$(printf '%s' "${PAYLOAD}" | sha256sum)"
else
    BICEP_CHECKSUM_LINUX_ARM64="$(
        printf '%s' "${PAYLOAD}" | shasum -a 256
    )"
fi
BICEP_CHECKSUM_LINUX_ARM64="${BICEP_CHECKSUM_LINUX_ARM64%% *}"
export BICEP_CHECKSUM_LINUX_ARM64

main "${TEST_DIR}/install" > "${TEST_DIR}/main-log" 2>&1
[[ "$(< "${TEST_DIR}/install/bicep")" == "${PAYLOAD}" ]] ||
    fail_test "verified binary was not installed"
[[ ! -e "${GITHUB_PATH}" ]] || fail_test "cross-platform PATH export"
cleanup

PAYLOAD="corrupt fixture"
printf '%s' "previous bicep fixture" > "${TEST_DIR}/install/bicep"
if (trap cleanup EXIT; main "${TEST_DIR}/install") \
    > "${TEST_DIR}/main-log" 2>&1; then
    fail_test "checksum mismatch was accepted"
fi
grep -q "checksum verification failed" "${TEST_DIR}/main-log" ||
    fail_test "checksum failure was not reported"
[[ "$(< "${TEST_DIR}/install/bicep")" == "previous bicep fixture" ]] ||
    fail_test "checksum failure replaced the existing binary"

echo "Bicep installer tests passed"
