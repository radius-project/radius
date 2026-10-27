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

# Lists or verifies the build provenance subjects of a staged Radius release:
# every CLI asset, every Radius-built image, and the Helm chart. Both modes read
# the same normalized checksums and image locks, so verification covers exactly
# what the release workflow attested.
#
# Requires jq; verify mode also requires gh.

set -euo pipefail

readonly SIGNER_WORKFLOW_PATH=".github/workflows/build-release.yaml"
readonly RETRY_ATTEMPTS="${RELEASE_RETRY_ATTEMPTS:-5}"
readonly RETRY_MAX_DELAY_SECONDS="${RELEASE_RETRY_MAX_DELAY_SECONDS:-15}"

MODE=""
VERSION=""
SOURCE_SHA=""
TARGETS_FILE=""
ASSETS_DIR=""
CHART_DIGEST=""
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

usage() {
    cat >&2 << 'EOF'
Usage:
  release-attestations.sh subjects --version <version> --targets <file>
      --assets-dir <dir> --chart-digest <digest> --output <file>
  release-attestations.sh verify --version <version> --source-sha <commit>
      --targets <file> --assets-dir <dir> --chart-digest <digest>
EOF
}

parse_args() {
    MODE="${1:-}"
    if [[ $# -gt 0 ]]; then
        shift
    fi
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --version)
                VERSION="${2:-}"
                shift 2
                ;;
            --source-sha)
                SOURCE_SHA="${2:-}"
                shift 2
                ;;
            --targets)
                TARGETS_FILE="${2:-}"
                shift 2
                ;;
            --assets-dir)
                ASSETS_DIR="${2:-}"
                shift 2
                ;;
            --chart-digest)
                CHART_DIGEST="${2:-}"
                shift 2
                ;;
            --output)
                OUTPUT="${2:-}"
                shift 2
                ;;
            *)
                usage
                fail "unknown argument: $1"
                ;;
        esac
    done
}

validate_args() {
    if [[ "${MODE}" != "subjects" && "${MODE}" != "verify" ]]; then
        usage
        fail "mode must be subjects or verify"
    fi
    if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc\.[1-9][0-9]*)?$ ]]; then
        fail "invalid release version: ${VERSION}"
    fi
    [[ -f "${TARGETS_FILE}" ]] || fail "targets file not found: ${TARGETS_FILE}"
    [[ -d "${ASSETS_DIR}" ]] || fail "assets directory not found: ${ASSETS_DIR}"
    if [[ ! "${CHART_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        fail "invalid Helm chart digest: ${CHART_DIGEST}"
    fi
    if [[ "${MODE}" == "subjects" ]]; then
        [[ -n "${OUTPUT}" ]] || fail "--output is required"
    elif [[ ! "${SOURCE_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
        fail "invalid source commit: ${SOURCE_SHA}"
    fi
}

# Prints "<name><TAB><sha256:digest><TAB><verification target>" per subject.
list_subjects() {
    # Staging normalizes every CLI sidecar to "<sha256> *<asset>".
    local -r checksum_pattern='^([0-9a-f]{64}) \*(.+)$'
    local cli_names
    local retained_images
    local name
    local checksum_file
    local checksum
    local lock_file
    local chart_repository
    local image_subjects
    local -a lock_files=("${ASSETS_DIR}/production-image-digests.json")

    cli_names="$(jq -er '.cliAssets[].name' "${TARGETS_FILE}")"
    while IFS= read -r name; do
        checksum_file="${ASSETS_DIR}/${name}.sha256"
        if [[ ! -f "${checksum_file}" ]]; then
            fail "staged checksum not found: ${checksum_file}"
        fi
        checksum="$(< "${checksum_file}")"
        if [[ ! "${checksum}" =~ ${checksum_pattern} ||
            "${BASH_REMATCH[2]}" != "${name}" ]]; then
            fail "staged checksum is not normalized: ${checksum_file}"
        fi
        printf '%s\t%s\t%s\n' "${name}" "sha256:${BASH_REMATCH[1]}" \
            "${ASSETS_DIR}/${name}"
    done <<< "${cli_names}"

    retained_images="$(jq -r '.images[]
        | select(.radiusBuild and .category != "production") | .name' \
        "${TARGETS_FILE}")"
    while IFS= read -r name; do
        if [[ -n "${name}" ]]; then
            lock_files+=("${ASSETS_DIR}/${name}-image-digests.json")
        fi
    done <<< "${retained_images}"
    for lock_file in "${lock_files[@]}"; do
        [[ -f "${lock_file}" ]] || fail "image lock not found: ${lock_file}"
    done
    image_subjects="$(jq -ers --slurpfile targets "${TARGETS_FILE}" '
        add as $locks
        | $targets[0] as $targets
        | ([$targets.images[] | select(.radiusBuild) | .name] | sort)
            as $expected
        | if ($locks | map(.name) | sort) != $expected then
            error("image locks do not cover exactly the Radius-built images")
          else
            $locks[]
            | ($targets.imageRegistry + "/" + .name) as $repository
            | if (.digest | tostring | test("^sha256:[0-9a-f]{64}$")) and
                .immutableReference == ($repository + "@" + .digest) then
                [$repository, .digest, "oci://" + .immutableReference] | @tsv
              else
                error("invalid image lock entry for \(.name)")
              end
          end
    ' "${lock_files[@]}")"
    printf '%s\n' "${image_subjects}"

    chart_repository="$(jq -er '.helm.orasReference' "${TARGETS_FILE}")"
    printf '%s\t%s\t%s\n' "${chart_repository}" "${CHART_DIGEST}" \
        "oci://${chart_repository}@${CHART_DIGEST}"
}

is_retryable_error() {
    local error="${1,,}"

    # A missing attestation is final; gh output also contains hex digests, so
    # match status codes only in their HTTP context.
    [[ "${error}" =~ http[[:space:]]404 ]] && return 1
    [[ "${error}" =~ http[[:space:]](429|5[0-9][0-9]) ]] && return 0
    [[ "${error}" =~ (bad|service|gateway)[[:space:]](gateway|unavailable|timeout) ]] && return 0
    [[ "${error}" =~ (timeout|timed[[:space:]]out) ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|temporarily) ]] && return 0
    [[ "${error}" =~ unexpected[[:space:]]eof ]] && return 0
    return 1
}

verify_subject() {
    local name="$1"
    local target="$2"
    local attempt
    local delay
    local output

    shift 2
    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(gh attestation verify "${target}" "$@" 2>&1)"; then
            echo "Verified build provenance for ${name}"
            return
        fi
        if ((attempt == RETRY_ATTEMPTS)) || ! is_retryable_error "${output}"; then
            echo "${output}" >&2
            fail "build provenance verification failed for ${name}"
        fi
        delay=$((2 ** (attempt - 1)))
        if ((delay > RETRY_MAX_DELAY_SECONDS)); then
            delay="${RETRY_MAX_DELAY_SECONDS}"
        fi
        echo "Transient provenance lookup failure for ${name}; retrying." >&2
        if [[ "${RELEASE_RETRY_NO_SLEEP:-}" != "true" ]]; then
            sleep "${delay}.$(printf '%03d' "$((RANDOM % 1000))")"
        fi
    done
}

write_subjects() {
    mkdir -p "$(dirname "${OUTPUT}")"
    awk -F '\t' '{ sub(/^sha256:/, "", $2); print $2 "  " $1 }' \
        "${TEMP_DIR}/subjects.tsv" > "${OUTPUT}"
    echo "Prepared $(wc -l < "${OUTPUT}") build provenance subjects"
}

verify_subjects() {
    local repository
    local name
    local target
    local -a policy

    command -v gh > /dev/null || fail "required command not found: gh"
    repository="$(jq -er '.repository' "${TARGETS_FILE}")"
    policy=(
        --repo "${repository}"
        --signer-workflow "${repository}/${SIGNER_WORKFLOW_PATH}"
        --source-ref "refs/tags/v${VERSION}"
        --source-digest "${SOURCE_SHA}"
        --deny-self-hosted-runners
    )
    while IFS=$'\t' read -r name _ target; do
        if [[ "${target}" != oci://* && ! -f "${target}" ]]; then
            fail "release asset not found: ${target}"
        fi
        verify_subject "${name}" "${target}" "${policy[@]}"
    done < "${TEMP_DIR}/subjects.tsv"
}

main() {
    parse_args "$@"
    validate_args
    command -v jq > /dev/null || fail "required command not found: jq"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-attestations-XXXXXX")"
    list_subjects > "${TEMP_DIR}/subjects.tsv"
    if [[ "${MODE}" == "subjects" ]]; then
        write_subjects
    else
        verify_subjects
    fi
}

main "$@"
