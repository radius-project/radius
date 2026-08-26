#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

set -euo pipefail

ARCHIVE=""
REPOSITORY=""
CHART_NAME=""
VERSION=""
TEMP_DIR=""
readonly RETRY_ATTEMPTS="${RELEASE_RETRY_ATTEMPTS:-5}"
readonly RETRY_MAX_DELAY_SECONDS="${RELEASE_RETRY_MAX_DELAY_SECONDS:-15}"

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

fail() {
    echo "Error: $*" >&2
    exit 1
}

is_retryable_error() {
    local error="${1,,}"

    [[ "${error}" =~ (429|5[0-9][0-9]|timeout) ]] && return 0
    [[ "${error}" =~ timed[[:space:]]out ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|unexpected[[:space:]]eof) ]] && return 0
    return 1
}

is_not_found_error() {
    local error="${1,,}"

    [[ "${error}" =~ (not[[:space:]]found|manifest[[:space:]]unknown|404) ]]
}

wait_before_retry() {
    local attempt="$1"
    local delay=$((2 ** (attempt - 1)))

    if ((delay > RETRY_MAX_DELAY_SECONDS)); then
        delay="${RETRY_MAX_DELAY_SECONDS}"
    fi
    if [[ "${RELEASE_RETRY_NO_SLEEP:-}" != "true" ]]; then
        sleep "${delay}.$(printf '%03d' "$((RANDOM % 1000))")"
    fi
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --archive)
                ARCHIVE="${2:-}"
                shift 2
                ;;
            --repository)
                REPOSITORY="${2:-}"
                shift 2
                ;;
            --name)
                CHART_NAME="${2:-}"
                shift 2
                ;;
            --version)
                VERSION="${2:-}"
                shift 2
                ;;
            *) fail "unknown argument: $1" ;;
        esac
    done
}

pull_chart() {
    local output_dir="$1"
    local attempt
    local output
    local status
    local absence_count=0

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        rm -rf "${output_dir}"
        mkdir -p "${output_dir}"
        if output="$(
            helm pull "${REPOSITORY%/}/${CHART_NAME}" \
                --version "${VERSION}" \
                --destination "${output_dir}" 2>&1
        )"; then
            printf 'found\n'
            return
        else
            status=$?
        fi
        if is_not_found_error "${output}"; then
            ((++absence_count))
            if ((absence_count >= 2)); then
                printf 'absent\n'
                return
            fi
            wait_before_retry "${attempt}"
            continue
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            fail "cannot inspect chart ${CHART_NAME}:${VERSION}: ${output}"
        fi
        if ! is_retryable_error "${output}"; then
            fail "cannot inspect chart ${CHART_NAME}:${VERSION}: ${output}"
        fi
        wait_before_retry "${attempt}"
    done
    return "${status}"
}

verify_pulled_chart() {
    local output_dir="$1"
    local pulled="${output_dir}/${CHART_NAME}-${VERSION}.tgz"
    local local_content="${TEMP_DIR}/local-content"
    local remote_content="${TEMP_DIR}/remote-content"

    if [[ ! -f "${pulled}" ]]; then
        fail "pulled chart archive is missing: ${pulled}"
    fi
    rm -rf "${local_content}" "${remote_content}"
    mkdir -p "${local_content}" "${remote_content}"
    tar -xzf "${ARCHIVE}" -C "${local_content}"
    tar -xzf "${pulled}" -C "${remote_content}"
    if ! diff -qr "${local_content}" "${remote_content}" > /dev/null; then
        fail "immutable chart ${CHART_NAME}:${VERSION} has different content"
    fi
}

push_chart() {
    local attempt
    local output
    local status
    local state
    local pulled="${TEMP_DIR}/pulled"

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(helm push "${ARCHIVE}" "${REPOSITORY}" 2>&1)"; then
            state="$(pull_chart "${pulled}")"
            if [[ "${state}" != "found" ]]; then
                fail "pushed chart cannot be resolved"
            fi
            verify_pulled_chart "${pulled}"
            return
        else
            status=$?
        fi
        if is_retryable_error "${output}"; then
            state="$(pull_chart "${pulled}")"
            if [[ "${state}" == "found" ]]; then
                verify_pulled_chart "${pulled}"
                return
            fi
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            fail "cannot push chart ${CHART_NAME}:${VERSION}: ${output}"
        fi
        if ! is_retryable_error "${output}"; then
            fail "cannot push chart ${CHART_NAME}:${VERSION}: ${output}"
        fi
        wait_before_retry "${attempt}"
    done
    return "${status}"
}

main() {
    local state
    local pulled

    parse_args "$@"
    command -v helm > /dev/null || fail "required command not found: helm"
    command -v tar > /dev/null || fail "required command not found: tar"
    [[ -f "${ARCHIVE}" ]] || fail "chart archive not found: ${ARCHIVE}"
    [[ -n "${REPOSITORY}" ]] || fail "repository is required"
    [[ -n "${CHART_NAME}" ]] || fail "chart name is required"
    [[ -n "${VERSION}" ]] || fail "version is required"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-helm-XXXXXX")"
    pulled="${TEMP_DIR}/pulled"
    state="$(pull_chart "${pulled}")"
    if [[ "${state}" == "found" ]]; then
        verify_pulled_chart "${pulled}"
        echo "Reused immutable Helm chart ${CHART_NAME}:${VERSION}"
        return
    fi
    push_chart
    echo "Published immutable Helm chart ${CHART_NAME}:${VERSION}"
}

main "$@"
