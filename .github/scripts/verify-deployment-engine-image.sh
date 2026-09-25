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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly ROOT

IMAGE="ghcr.io/radius-project/deployment-engine"
TAG=""
SIGNED_TAG=""
SOURCE_COMMIT=""
OUTPUT_FILE=""
STATE_OUTPUT=""
EXPECTED_DIGEST=""
ALLOW_ABSENT=false
TARGETS_FILE="${GORELEASER_PARITY_TARGETS:-${ROOT}/.github/release-parity/targets.json}"
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

usage() {
    cat <<'EOF'
Usage: verify-deployment-engine-image.sh --tag <tag> --signed-tag <vX.Y.Z> \
    --source-commit <sha> --output <path> \
    [--expected-digest <sha256:...>] [--allow-absent --state-output <path>]
EOF
}

is_retryable_error() {
    local error="${1,,}"

    [[ "${error}" =~ (429|404|5[0-9][0-9]|timeout) ]] && return 0
    [[ "${error}" =~ timed[[:space:]]out ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|temporarily|unexpected[[:space:]]eof) ]] &&
        return 0
    [[ "${error}" =~ (manifest[[:space:]]unknown|not[[:space:]]found) ]]
}

inspect_image() {
    local reference="$1"
    local output="$2"
    local attempt error status delay missing

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if docker buildx imagetools inspect --format '{{json .}}' \
            "${reference}" >"${output}" 2>&1; then
            return
        else
            status=$?
        fi
        error="$(<"${output}")"
        case "${error,,}" in
            *"manifest unknown"* | *"not found"* | *404*) missing=true ;;
            *) missing=false ;;
        esac
        if ((attempt == RETRY_ATTEMPTS)); then
            if [[ "${missing}" == "true" ]]; then
                return 3
            fi
            echo "${error}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${error}"; then
            echo "${error}" >&2
            return "${status}"
        fi
        delay=$((2 ** (attempt - 1)))
        ((delay <= RETRY_MAX_DELAY_SECONDS)) ||
            delay="${RETRY_MAX_DELAY_SECONDS}"
        if [[ "${RELEASE_RETRY_NO_SLEEP:-}" != "true" ]]; then
            sleep "${delay}.$(printf '%03d' "$((RANDOM % 1000))")"
        fi
    done
}

write_state() {
    local state="$1"
    local digest="${2:-}"

    if [[ -n "${STATE_OUTPUT}" ]]; then
        mkdir -p "$(dirname "${STATE_OUTPUT}")"
        printf '%s\n' "${state}" >"${STATE_OUTPUT}"
    fi
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "state=${state}" >>"${GITHUB_OUTPUT}"
        if [[ -n "${digest}" ]]; then
            echo "digest=${digest}" >>"${GITHUB_OUTPUT}"
        fi
    fi
}

main() {
    local reference raw digest expected_platforms actual_platforms
    local actual_refs actual_revisions
    local inspect_status=0

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --tag)
                TAG="${2:-}"
                shift 2
                ;;
            --signed-tag)
                SIGNED_TAG="${2:-}"
                shift 2
                ;;
            --source-commit)
                SOURCE_COMMIT="${2:-}"
                shift 2
                ;;
            --output)
                OUTPUT_FILE="${2:-}"
                shift 2
                ;;
            --expected-digest)
                EXPECTED_DIGEST="${2:-}"
                shift 2
                ;;
            --allow-absent)
                ALLOW_ABSENT=true
                shift
                ;;
            --state-output)
                STATE_OUTPUT="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    [[ "${TAG}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+(-rc\.[1-9][0-9]*)?)?$ ]] ||
        fail "tag must use X.Y, X.Y.Z, or X.Y.Z-rc.N format"
    [[ "${SIGNED_TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[1-9][0-9]*)?$ ]] ||
        fail "signed tag must use vX.Y.Z or vX.Y.Z-rc.N format"
    [[ "${SOURCE_COMMIT}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "source commit must be a full commit SHA"
    [[ -n "${OUTPUT_FILE}" ]] || fail "output path is required"
    if [[ -n "${EXPECTED_DIGEST}" &&
        ! "${EXPECTED_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        fail "expected digest is invalid"
    fi
    if [[ "${ALLOW_ABSENT}" == "true" && -z "${STATE_OUTPUT}" ]]; then
        fail "--allow-absent requires --state-output"
    fi
    command -v docker >/dev/null || fail "required command not found: docker"
    command -v jq >/dev/null || fail "required command not found: jq"
    [[ -f "${TARGETS_FILE}" ]] || fail "release targets file not found"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/de-image-XXXXXX")"
    raw="${TEMP_DIR}/image.json"
    reference="${IMAGE}:${TAG}"
    inspect_image "${reference}" "${raw}" || inspect_status=$?
    if ((inspect_status != 0)); then
        if [[ "${ALLOW_ABSENT}" == "true" && "${inspect_status}" == "3" ]]; then
            mkdir -p "$(dirname "${OUTPUT_FILE}")"
            printf 'null\n' >"${OUTPUT_FILE}"
            write_state absent
            echo "Deployment Engine image ${reference} is absent."
            return
        fi
        fail "cannot inspect Deployment Engine image ${reference}"
    fi

    digest="$(jq -er '
        .manifest.digest | select(test("^sha256:[0-9a-f]{64}$"))
    ' "${raw}")" || fail "invalid manifest digest for ${reference}"
    expected_platforms="$(jq -c '
        .images[] | select(.name == "deployment-engine") |
        .requiredPlatforms | sort
    ' "${TARGETS_FILE}")"
    actual_platforms="$(jq -c '
        def platform_name($platform):
            $platform.os + "/" + $platform.architecture +
            (if ($platform.variant // "") == "" then ""
             else "/" + $platform.variant end);
        [.manifest.manifests[] | select(.platform.os != "unknown") |
            platform_name(.platform)] | sort
    ' "${raw}")"
    [[ "${actual_platforms}" == "${expected_platforms}" ]] ||
        fail "unexpected platform set for ${reference}"
    actual_refs="$(jq -c '[
        .image | to_entries[] | select(.key != "unknown/unknown") |
        .value.config.Labels."org.opencontainers.image.ref"
    ] | unique' "${raw}")"
    [[ "${actual_refs}" == "[\"refs/tags/${SIGNED_TAG}\"]" ]] ||
        fail "unexpected signed tag label for ${reference}"
    actual_revisions="$(jq -c '[
        .image | to_entries[] | select(.key != "unknown/unknown") |
        .value.config.Labels."org.opencontainers.image.revision"
    ] | unique' "${raw}")"
    [[ "${actual_revisions}" == "[\"${SOURCE_COMMIT}\"]" ]] ||
        fail "unexpected source revision for ${reference}"
    if [[ -n "${EXPECTED_DIGEST}" && "${digest}" != "${EXPECTED_DIGEST}" ]]; then
        fail "Deployment Engine image ${reference} moved from ${EXPECTED_DIGEST} to ${digest}"
    fi

    mkdir -p "$(dirname "${OUTPUT_FILE}")"
    jq -n --arg reference "${reference}" --arg digest "${digest}" \
        --argjson platforms "${actual_platforms}" '{
            reference: $reference,
            digest: $digest,
            platforms: $platforms
        }' >"${OUTPUT_FILE}"
    write_state complete "${digest}"
    echo "Verified Deployment Engine image ${reference}@${digest}."
}

main "$@"
