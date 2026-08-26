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
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT

TARGETS_FILE="${GORELEASER_PARITY_TARGETS:-}"
if [[ -z "${TARGETS_FILE}" ]]; then
    TARGETS_FILE="${REPO_ROOT}/.github/release-parity/targets.json"
fi
REGISTRY=""
TAG=""
OUTPUT=""
CATEGORIES="production"
NAMES=""
SOURCE_SHA="${RELEASE_SOURCE_SHA:-}"
ALLOW_ABSENT=false
STATE_OUTPUT=""
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

require_command() {
    if ! command -v "$1" > /dev/null 2>&1; then
        fail "required command not found: $1"
    fi
}

is_retryable_error() {
    local error="${1,,}"

    [[ "${error}" =~ (429|404|5[0-9][0-9]|timeout) ]] && return 0
    [[ "${error}" =~ timed[[:space:]]out ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|temporarily) ]] && return 0
    [[ "${error}" =~ unexpected[[:space:]]eof ]] && return 0
    [[ "${error}" =~ manifest[[:space:]]unknown ]] && return 0
    [[ "${error}" =~ not[[:space:]]found ]] && return 0
    return 1
}

inspect_image() {
    local reference="$1"
    local output="$2"
    local attempt
    local error
    local status
    local delay

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if docker buildx imagetools inspect --format '{{json .}}' \
            "${reference}" > "${output}" 2>&1; then
            return
        else
            status=$?
        fi
        error="$(cat "${output}")"
        if ((attempt == RETRY_ATTEMPTS)); then
            case "${error,,}" in
                *"manifest unknown"* | *"not found"* | *404*) return 3 ;;
            esac
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            echo "${error}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${error}"; then
            echo "${error}" >&2
            return "${status}"
        fi
        delay=$((2 ** (attempt - 1)))
        if ((delay > RETRY_MAX_DELAY_SECONDS)); then
            delay="${RETRY_MAX_DELAY_SECONDS}"
        fi
        echo "Transient image lookup failure for ${reference}; retrying." >&2
        if [[ "${RELEASE_RETRY_NO_SLEEP:-}" != "true" ]]; then
            sleep "${delay}.$(printf '%03d' "$((RANDOM % 1000))")"
        fi
    done
}

usage() {
    echo "Usage: $0 --registry <registry> --tag <tag> --output <path>" \
        "[--categories <category,...>] [--names <name,...>]" >&2
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --registry)
                REGISTRY="$2"
                shift 2
                ;;
            --tag)
                TAG="$2"
                shift 2
                ;;
            --output)
                OUTPUT="$2"
                shift 2
                ;;
            --categories)
                CATEGORIES="$2"
                shift 2
                ;;
            --names)
                NAMES="$2"
                shift 2
                ;;
            --source-sha)
                SOURCE_SHA="$2"
                shift 2
                ;;
            --allow-absent)
                ALLOW_ABSENT=true
                shift
                ;;
            --state-output)
                STATE_OUTPUT="$2"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown argument: $1" ;;
        esac
    done
}

main() {
    local entries
    local name
    local repository
    local reference
    local raw
    local digest
    local expected_platforms
    local actual_platforms
    local actual_revisions
    local actual_versions
    local inspect_status
    local selected=0
    local captured=0

    parse_args "$@"
    require_command docker
    require_command jq
    [[ -n "${REGISTRY}" ]] || fail "registry is required"
    [[ -n "${TAG}" ]] || fail "tag is required"
    [[ -n "${OUTPUT}" ]] || fail "output path is required"
    if [[ ! "${SOURCE_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
        fail "source SHA must be a full commit SHA"
    fi
    if [[ ! -f "${TARGETS_FILE}" ]]; then
        fail "targets file not found: ${TARGETS_FILE}"
    fi

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/image-digests-XXXXXX")"
    entries="${TEMP_DIR}/entries.jsonl"
    : > "${entries}"

    while IFS= read -r name; do
        ((++selected))
        name="${name%$'\r'}"
        repository="${REGISTRY}/${name}"
        reference="${repository}:${TAG}"
        raw="${TEMP_DIR}/${name}.json"
        if inspect_image "${reference}" "${raw}"; then
            ((++captured))
        else
            inspect_status=$?
            if [[ "${ALLOW_ABSENT}" == "true" &&
                "${inspect_status}" == "3" ]]; then
                continue
            fi
            fail "cannot inspect image: ${reference}"
        fi

        digest="$(jq -er '
            .manifest.digest
            | select(test("^sha256:[0-9a-f]{64}$"))
        ' "${raw}")" || fail "invalid manifest digest for ${reference}"
        expected_platforms="$(jq -c --arg name "${name}" '
            .images[]
            | select(.name == $name)
            | .requiredPlatforms
            | sort
        ' "${TARGETS_FILE}")"
        actual_platforms="$(jq -c '
            def platform_name($platform):
                $platform.os + "/" + $platform.architecture
                + (if ($platform.variant // "") == ""
                    then ""
                    else "/" + $platform.variant
                  end);
            [.manifest.manifests[]
                | select(.platform.os != "unknown")
                | platform_name(.platform)]
            | sort
        ' "${raw}")"
        if [[ "${actual_platforms}" != "${expected_platforms}" ]]; then
            fail "unexpected platform set for ${reference}"
        fi
        actual_revisions="$(jq -c '[
            .image
            | to_entries[]
            | select(.key != "unknown/unknown")
            | .value.config.Labels."org.opencontainers.image.revision"
        ] | unique' "${raw}")"
        if [[ "${actual_revisions}" != "[\"${SOURCE_SHA}\"]" ]]; then
            fail "unexpected source revision for ${reference}"
        fi
        actual_versions="$(jq -c '[
            .image
            | to_entries[]
            | select(.key != "unknown/unknown")
            | .value.config.Labels."org.opencontainers.image.version"
        ] | unique' "${raw}")"
        if [[ "${actual_versions}" != "[\"${TAG}\"]" ]]; then
            fail "unexpected image version label for ${reference}"
        fi

        jq -n \
            --arg name "${name}" \
            --arg reference "${reference}" \
            --arg digest "${digest}" \
            --arg sourceSha "${SOURCE_SHA}" \
            --arg immutableReference "${repository}@${digest}" \
            --argjson platforms "${actual_platforms}" '
            {
                name: $name,
                reference: $reference,
                digest: $digest,
                sourceSha: $sourceSha,
                immutableReference: $immutableReference,
                platforms: $platforms
            }
        ' >> "${entries}"
    done < <(jq -r \
        --arg categories "${CATEGORIES}" \
        --arg names "${NAMES}" '
        ($categories | split(",")) as $categories
        | ($names | split(",") | map(select(length > 0))) as $names
        |
        .images[]
        | select(
            .radiusBuild == true
            and (
                if ($names | length) > 0
                then (.name as $name | $names | index($name))
                else (.category as $category
                    | $categories | index($category))
                end
            )
        )
        | .name
    ' "${TARGETS_FILE}")

    if ((selected == 0)); then
        fail "no Radius-built images match the selection"
    fi
    if ((captured > 0 && captured < selected)); then
        fail "only ${captured} of ${selected} expected images exist"
    fi

    mkdir -p "$(dirname "${OUTPUT}")"
    jq -S -s 'sort_by(.name)' "${entries}" > "${OUTPUT}"
    if [[ -n "${STATE_OUTPUT}" ]]; then
        mkdir -p "$(dirname "${STATE_OUTPUT}")"
        if ((captured == selected)); then
            printf 'complete\n' > "${STATE_OUTPUT}"
        else
            printf 'absent\n' > "${STATE_OUTPUT}"
        fi
    fi
    if [[ "${ALLOW_ABSENT}" != "true" ]]; then
        if ((captured != selected)); then
            fail "expected images are absent"
        fi
    fi
    echo "Captured production image digests in ${OUTPUT}"
}

main "$@"
