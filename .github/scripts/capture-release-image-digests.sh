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

TARGETS_FILE="${GORELEASER_PARITY_TARGETS:-${REPO_ROOT}/.github/release-parity/targets.json}"
REGISTRY=""
TAG=""
OUTPUT=""
TEMP_DIR=""

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
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

usage() {
    echo "Usage: $0 --registry <registry> --tag <tag> --output <path>" >&2
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

    parse_args "$@"
    require_command docker
    require_command jq
    [[ -n "${REGISTRY}" ]] || fail "registry is required"
    [[ -n "${TAG}" ]] || fail "tag is required"
    [[ -n "${OUTPUT}" ]] || fail "output path is required"
    [[ -f "${TARGETS_FILE}" ]] || fail "targets file not found: ${TARGETS_FILE}"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/image-digests-XXXXXX")"
    entries="${TEMP_DIR}/entries.jsonl"
    : >"${entries}"

    while IFS= read -r name; do
        name="${name%$'\r'}"
        repository="${REGISTRY}/${name}"
        reference="${repository}:${TAG}"
        raw="${TEMP_DIR}/${name}.json"
        docker buildx imagetools inspect --format '{{json .}}' \
            "${reference}" >"${raw}"

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
        [[ "${actual_platforms}" == "${expected_platforms}" ]] ||
            fail "unexpected platform set for ${reference}"

        jq -n \
            --arg name "${name}" \
            --arg reference "${reference}" \
            --arg digest "${digest}" \
            --arg immutableReference "${repository}@${digest}" \
            --argjson platforms "${actual_platforms}" '
            {
                name: $name,
                reference: $reference,
                digest: $digest,
                immutableReference: $immutableReference,
                platforms: $platforms
            }
        ' >>"${entries}"
    done < <(jq -r '
        .images[]
        | select(.category == "production")
        | .name
    ' "${TARGETS_FILE}")

    mkdir -p "$(dirname "${OUTPUT}")"
    jq -S -s 'sort_by(.name)' "${entries}" >"${OUTPUT}"
    echo "Captured production image digests in ${OUTPUT}"
}

main "$@"
