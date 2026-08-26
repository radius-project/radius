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
# shellcheck source=.github/scripts/release-version.sh
source "${SCRIPT_DIR}/release-version.sh"

TARGETS_FILE="${GORELEASER_PARITY_TARGETS:-}"
if [[ -z "${TARGETS_FILE}" ]]; then
    TARGETS_FILE="${REPO_ROOT}/.github/release-parity/targets.json"
fi
COMMAND=""
REGISTRY=""
VERSION=""
CHANNEL=""
ARTIFACTS_FILE=""
ARTIFACTS_DIR=""
IMAGE_LOCK=""
CLI_LOCK=""
OUTPUT=""
CATEGORIES="production,non-go,test"
NAMES=""
VERIFY_ALIASES=false
SOURCE_SHA="${RELEASE_SOURCE_SHA:-}"
TEMP_DIR=""
readonly RETRY_ATTEMPTS="${RELEASE_RETRY_ATTEMPTS:-5}"
readonly RETRY_MAX_DELAY_SECONDS="${RELEASE_RETRY_MAX_DELAY_SECONDS:-15}"
readonly SOURCE_ANNOTATION="org.opencontainers.image.source="
readonly SOURCE_URL="https://github.com/radius-project/radius"

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

    [[ "${error}" =~ (429|5[0-9][0-9]|timeout) ]] && return 0
    [[ "${error}" =~ timed[[:space:]]out ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|temporarily) ]] && return 0
    [[ "${error}" =~ unexpected[[:space:]]eof ]] && return 0
    case "${error}" in
        *"tls handshake"* | *"service unavailable"*) return 0 ;;
    esac
    [[ "${error}" =~ too[[:space:]]many[[:space:]]requests ]] && return 0
    return 1
}

is_not_found_error() {
    local error="${1,,}"

    [[ -n "${error}" ]] || return 1
    case "${error}" in
        *"not found"* | *"manifest unknown"*) return 0 ;;
    esac
    [[ "${error}" =~ (name[[:space:]]unknown|404) ]] && return 0
    case "${error}" in
        *"invalid argument"*"no such file"*) return 0 ;;
    esac
    return 1
}

wait_before_retry() {
    local operation="$1"
    local attempt="$2"
    local delay=$((2 ** (attempt - 1)))
    local jitter_ms=$((RANDOM % 1000))

    if ((delay > RETRY_MAX_DELAY_SECONDS)); then
        delay="${RETRY_MAX_DELAY_SECONDS}"
    fi
    echo "Transient failure during ${operation}; retrying." >&2
    if [[ "${RELEASE_RETRY_NO_SLEEP:-}" != "true" ]]; then
        sleep "${delay}.$(printf '%03d' "${jitter_ms}")"
    fi
}

retry_read() {
    local operation="$1"
    shift
    local attempt
    local output
    local status

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$("$@" 2>&1)"; then
            printf '%s\n' "${output}"
            return 0
        else
            status=$?
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            [[ -z "${output}" ]] || echo "${output}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${output}"; then
            [[ -z "${output}" ]] || echo "${output}" >&2
            return "${status}"
        fi
        wait_before_retry "${operation}" "${attempt}"
    done
}

usage() {
    cat >&2 << 'EOF'
Usage:
  release-oci-artifacts.sh stage-cli --registry <registry> \
    --version <version> --artifacts <artifacts.json> --output <lock.json>
    release-oci-artifacts.sh stage-cli --registry <registry> \
    --version <version> --artifacts-dir <directory> --output <lock.json>
  release-oci-artifacts.sh promote --version <version> \
    --channel <X.Y> --image-lock <images.json> --cli-lock <cli.json>
    release-oci-artifacts.sh verify --version <version> \
        [--channel <X.Y>] [--image-lock <images.json>] [--cli-lock <cli.json>] \
        [--categories <category,...>] [--names <name,...>] \
        [--source-sha <sha>] [--aliases]
    release-oci-artifacts.sh assert-images-absent --registry <registry> \
        --version <version> [--categories <category,...>] [--names <name,...>] \
        [--source-sha <sha>]
EOF
}

parse_args() {
    [[ $# -gt 0 ]] || {
        usage
        exit 1
    }
    COMMAND="$1"
    shift

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --registry)
                REGISTRY="${2:-}"
                shift 2
                ;;
            --version)
                VERSION="${2:-}"
                shift 2
                ;;
            --channel)
                CHANNEL="${2:-}"
                shift 2
                ;;
            --artifacts)
                ARTIFACTS_FILE="${2:-}"
                shift 2
                ;;
            --artifacts-dir)
                ARTIFACTS_DIR="${2:-}"
                shift 2
                ;;
            --image-lock)
                IMAGE_LOCK="${2:-}"
                shift 2
                ;;
            --cli-lock)
                CLI_LOCK="${2:-}"
                shift 2
                ;;
            --output)
                OUTPUT="${2:-}"
                shift 2
                ;;
            --categories)
                CATEGORIES="${2:-}"
                shift 2
                ;;
            --names)
                NAMES="${2:-}"
                shift 2
                ;;
            --source-sha)
                SOURCE_SHA="${2:-}"
                shift 2
                ;;
            --aliases)
                VERIFY_ALIASES=true
                shift
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown argument: $1" ;;
        esac
    done
}

validate_version() {
    if ! is_radius_release_version "${VERSION}"; then
        fail "version must be a Radius release version"
    fi
}

validate_source_sha() {
    if [[ ! "${SOURCE_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
        fail "source SHA must be a full commit SHA"
    fi
}

artifact_path() {
    local name="$1"
    local matches
    local path

    if [[ -n "${ARTIFACTS_DIR}" ]]; then
        path="${ARTIFACTS_DIR%/}/${name}"
        if [[ ! -f "${path}" ]]; then
            fail "CLI artifact not found: ${path}"
        fi
        printf '%s\n' "${path}"
        return
    fi

    matches="$(jq -c --arg name "${name}" '[
        .[]
        | select(
            .name == $name
            and .type == "Binary"
            and .extra.ID == "rad"
        )
    ]' "${ARTIFACTS_FILE}")"
    if [[ "$(jq 'length' <<< "${matches}")" != "1" ]]; then
        fail "expected one GoReleaser binary for ${name}"
    fi
    path="$(jq -r '.[0].path' <<< "${matches}")"
    if [[ "${path}" != /* ]]; then
        path="${REPO_ROOT}/${path}"
    fi
    if [[ ! -f "${path}" ]]; then
        fail "CLI artifact not found: ${path}"
    fi
    printf '%s\n' "${path}"
}

push_cli_artifact() {
    local reference="$1"
    local artifact="$2"
    local attempt
    local output
    local status
    local observed

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(
            oras push "${reference}" "${artifact}" \
                --annotation "${SOURCE_ANNOTATION}${SOURCE_URL}" \
                2>&1
        )"; then
            [[ -z "${output}" ]] || echo "${output}"
            if ! cli_artifact_matches "${reference}" "${artifact}"; then
                fail "published CLI artifact failed verification: ${reference}"
            fi
            return 0
        else
            status=$?
        fi
        if ! is_retryable_error "${output}"; then
            echo "${output}" >&2
            return "${status}"
        fi
        if cli_artifact_matches "${reference}" "${artifact}"; then
            echo "The uncertain push created ${reference}; skipping retry."
            return 0
        fi
        if observed="$(oras resolve "${reference}" 2> /dev/null)"; then
            if [[ "${observed}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
                fail "immutable CLI tag ${reference} contains different bytes"
            fi
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            echo "${output}" >&2
            return "${status}"
        fi
        wait_before_retry "CLI OCI publication" "${attempt}"
    done
}

cli_artifact_matches() {
    local reference="$1"
    local artifact="$2"
    local artifact_name
    local pull_dir
    local pull_status=0

    artifact_name="$(basename "${artifact}")"
    pull_dir="$(mktemp -d "${TEMP_DIR}/pull-XXXXXX")"
    pull_cli_artifact "${reference}" "${pull_dir}" || pull_status=$?
    if ((pull_status != 0)); then
        rm -rf "${pull_dir}"
        # 3 means the tag resolved and then vanished, which the caller
        # reports differently from an unreadable artifact.
        ((pull_status == 3)) && return 3
        return 2
    fi
    if [[ ! -f "${pull_dir}/${artifact_name}" ]]; then
        rm -rf "${pull_dir}"
        return 1
    fi
    if find "${pull_dir}" -mindepth 1 -type f \
        ! -path "${pull_dir}/${artifact_name}" | grep -q .; then
        rm -rf "${pull_dir}"
        return 1
    fi
    if ! cmp -s "${artifact}" "${pull_dir}/${artifact_name}"; then
        rm -rf "${pull_dir}"
        return 1
    fi
    rm -rf "${pull_dir}"
}

pull_cli_artifact() {
    local reference="$1"
    local pull_dir="$2"
    local attempt
    local output
    local status

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        rm -rf "${pull_dir}"
        mkdir -p "${pull_dir}"
        if output="$(
            oras pull "${reference}" --output "${pull_dir}" 2>&1
        )"; then
            return
        else
            status=$?
        fi
        if is_not_found_error "${output}"; then
            return 3
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            echo "${output}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${output}"; then
            echo "${output}" >&2
            return "${status}"
        fi
        wait_before_retry "CLI OCI content lookup" "${attempt}"
    done
}

reconcile_cli_reference() {
    local reference="$1"
    local artifact="$2"
    local attempt
    local output
    local status

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(oras resolve "${reference}" 2>&1)"; then
            if cli_artifact_matches "${reference}" "${artifact}"; then
                printf 'reuse\n'
                return 0
            else
                status=$?
            fi
            if [[ "${status}" == "1" ]]; then
                fail "immutable CLI tag ${reference} contains different bytes"
            fi
            if [[ "${status}" == "3" ]]; then
                fail "resolved CLI tag disappeared during content verification"
            fi
            fail "cannot verify immutable CLI tag ${reference}"
        else
            status=$?
        fi
        if is_not_found_error "${output}"; then
            if ((attempt < 2)); then
                wait_before_retry "CLI OCI absence confirmation" "${attempt}"
                continue
            fi
            printf 'absent\n'
            return 0
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            fail "cannot reconcile ${reference}: ${output}"
        fi
        if ! is_retryable_error "${output}"; then
            fail "cannot reconcile ${reference}: ${output}"
        fi
        wait_before_retry "CLI OCI preflight" "${attempt}"
    done
}

# A subshell function so the directory change cannot leak. oras names the
# layer after the path it is given, so the canonical basename must be the
# argument and the staging directory must be the working directory.
stage_cli_reference() (
    local reference="$1"
    local artifact_dir="$2"
    local artifact_name="$3"
    local reference_state

    cd "${artifact_dir}"
    reference_state="$(
        reconcile_cli_reference "${reference}" "./${artifact_name}"
    )"
    if [[ "${reference_state}" == "absent" ]]; then
        push_cli_artifact "${reference}" "./${artifact_name}"
    fi
)

stage_cli() {
    local entries
    local name
    local os
    local arch
    local path
    local canonical_name
    local staging_dir
    local repository
    local reference
    local digest

    require_command jq
    require_command oras
    validate_version
    validate_source_sha
    [[ -n "${REGISTRY}" ]] || fail "registry is required"
    if [[ -z "${ARTIFACTS_DIR}" ]]; then
        if [[ ! -f "${ARTIFACTS_FILE}" ]]; then
            fail "artifacts file not found: ${ARTIFACTS_FILE}"
        fi
    fi
    [[ -n "${OUTPUT}" ]] || fail "output is required"
    if [[ ! -f "${TARGETS_FILE}" ]]; then
        fail "targets file not found: ${TARGETS_FILE}"
    fi

    REGISTRY="${REGISTRY%/}"
    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-cli-oci-XXXXXX")"
    entries="${TEMP_DIR}/entries.jsonl"
    : > "${entries}"

    while IFS=$'\t' read -r name os arch; do
        path="$(artifact_path "${name}")"
        if [[ "${os}" == "windows" ]]; then
            canonical_name="rad.exe"
        else
            canonical_name="rad"
        fi
        staging_dir="${TEMP_DIR}/artifacts/${name}"
        mkdir -p "${staging_dir}"
        cp "${path}" "${staging_dir}/${canonical_name}"
        repository="${REGISTRY}/rad/${os}-${arch}"
        reference="${repository}:${VERSION}"
        stage_cli_reference "${reference}" "${staging_dir}" "${canonical_name}"
        digest="$(
            retry_read "CLI OCI digest lookup" oras resolve "${reference}"
        )"
        if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
            fail "invalid OCI digest for ${reference}"
        fi

        jq -n \
            --arg name "${name}" \
            --arg repository "${repository}" \
            --arg reference "${reference}" \
            --arg digest "${digest}" \
            --arg immutableReference "${repository}@${digest}" '
            {
                name: $name,
                repository: $repository,
                reference: $reference,
                digest: $digest,
                immutableReference: $immutableReference
            }
        ' >> "${entries}"
    done < <(jq -r '
        .cliAssets[]
        | [.name, .os, .arch]
        | @tsv
    ' "${TARGETS_FILE}")

    mkdir -p "$(dirname "${OUTPUT}")"
    jq -S -s --arg version "${VERSION}" --arg sourceSha "${SOURCE_SHA}" '
        {
            version: $version,
            sourceSha: $sourceSha,
            artifacts: sort_by(.name)
        }
    ' "${entries}" > "${OUTPUT}"
    echo "Published immutable CLI OCI artifacts and wrote ${OUTPUT}"
}

assert_lock_members() {
    local expected="$1"
    local actual="$2"
    local description="$3"

    if ! jq -e -n \
        --argjson expected "${expected}" \
        --argjson actual "${actual}" \
        '$expected == $actual' > /dev/null; then
        fail "${description} do not match the release contract"
    fi
}

resolve_image_digest() {
    local reference="$1"

    retry_read "image digest lookup" \
        docker buildx imagetools inspect --format '{{json .}}' \
        "${reference}" \
                       | jq -er '.manifest.digest'
}

resolve_cli_digest() {
    local reference="$1"

    retry_read "CLI OCI digest lookup" oras resolve "${reference}"
}

selected_image_names() {
    jq -r \
        --arg categories "${CATEGORIES}" \
        --arg names "${NAMES}" '
        ($categories | split(",")) as $categories
        | ($names | split(",") | map(select(length > 0))) as $names
        | .images[]
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
    ' "${TARGETS_FILE}"
}

image_reference_state() {
    local reference="$1"
    local attempt
    local output
    local status

    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(
            docker buildx imagetools inspect --format '{{json .}}' \
                "${reference}" 2>&1
        )"; then
            printf 'exists\n'
            return
        else
            status=$?
        fi
        if is_not_found_error "${output}"; then
            printf 'absent\n'
            return
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            fail "cannot reconcile ${reference}: ${output}"
        fi
        if ! is_retryable_error "${output}"; then
            fail "cannot reconcile ${reference}: ${output}"
        fi
        wait_before_retry "image preflight" "${attempt}"
    done
}

assert_images_absent() {
    local name
    local reference
    local state
    local selected=0

    require_command docker
    require_command jq
    validate_version
    validate_source_sha
    [[ -n "${REGISTRY}" ]] || fail "registry is required"
    if [[ ! -f "${TARGETS_FILE}" ]]; then
        fail "targets file not found: ${TARGETS_FILE}"
    fi
    REGISTRY="${REGISTRY%/}"

    while IFS= read -r name; do
        ((++selected))
        reference="${REGISTRY}/${name}:${VERSION}"
        state="$(image_reference_state "${reference}")"
        if [[ "${state}" == "exists" ]]; then
            fail "immutable image ${reference} exists without a durable lock"
        fi
    done < <(selected_image_names)
    if ((selected == 0)); then
        fail "no Radius-built images match the selection"
    fi
}

verify_image_alias() {
    local reference="$1"
    local expected_digest="$2"
    local actual_digest

    if ! actual_digest="$(
        resolve_image_digest "${reference}"
    )"; then
        fail "image reference cannot be resolved: ${reference}"
    fi
    if [[ "${actual_digest}" != "${expected_digest}" ]]; then
        fail "image alias ${reference} has an unexpected digest"
    fi
}

verify_cli_alias() {
    local reference="$1"
    local expected_digest="$2"
    local actual_digest

    if ! actual_digest="$(resolve_cli_digest "${reference}")"; then
        fail "CLI reference cannot be resolved: ${reference}"
    fi
    if [[ "${actual_digest}" != "${expected_digest}" ]]; then
        fail "CLI alias ${reference} has an unexpected digest"
    fi
}

image_aliases_match() {
    local repository="$1"
    local channel="$2"
    local digest="$3"

    local channel_digest
    local latest_digest

    channel_digest="$(
        resolve_image_digest "${repository}:${channel}" 2> /dev/null
    )" \
        || return 1
    latest_digest="$(
        resolve_image_digest "${repository}:latest" 2> /dev/null
    )" \
        || return 1
    [[ "${channel_digest}" == "${digest}" &&
        "${latest_digest}" == "${digest}" ]]
}

promote_image_aliases() {
    local repository="$1"
    local channel="$2"
    local digest="$3"
    local immutable_reference="$4"
    local attempt
    local output
    local status

    if image_aliases_match "${repository}" "${channel}" "${digest}"; then
        return
    fi
    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(
            docker buildx imagetools create \
                --tag "${repository}:${channel}" \
                --tag "${repository}:latest" \
                "${immutable_reference}" 2>&1
        )"; then
            verify_image_alias "${repository}:${channel}" "${digest}"
            verify_image_alias "${repository}:latest" "${digest}"
            return
        else
            status=$?
        fi
        if image_aliases_match "${repository}" "${channel}" "${digest}"; then
            return
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            echo "${output}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${output}"; then
            echo "${output}" >&2
            return "${status}"
        fi
        wait_before_retry "image alias promotion" "${attempt}"
    done
}

cli_aliases_match() {
    local repository="$1"
    local channel="$2"
    local digest="$3"

    local channel_digest
    local latest_digest

    channel_digest="$(
        resolve_cli_digest "${repository}:${channel}" 2> /dev/null
    )" \
        || return 1
    latest_digest="$(
        resolve_cli_digest "${repository}:latest" 2> /dev/null
    )" \
        || return 1
    [[ "${channel_digest}" == "${digest}" &&
        "${latest_digest}" == "${digest}" ]]
}

promote_cli_aliases() {
    local repository="$1"
    local channel="$2"
    local digest="$3"
    local immutable_reference="$4"
    local attempt
    local output
    local status

    if cli_aliases_match "${repository}" "${channel}" "${digest}"; then
        return
    fi
    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if output="$(
            oras tag "${immutable_reference}" "${channel}" latest 2>&1
        )"; then
            verify_cli_alias "${repository}:${channel}" "${digest}"
            verify_cli_alias "${repository}:latest" "${digest}"
            return
        else
            status=$?
        fi
        if cli_aliases_match "${repository}" "${channel}" "${digest}"; then
            return
        fi
        if ((attempt == RETRY_ATTEMPTS)); then
            echo "${output}" >&2
            return "${status}"
        fi
        if ! is_retryable_error "${output}"; then
            echo "${output}" >&2
            return "${status}"
        fi
        wait_before_retry "CLI alias promotion" "${attempt}"
    done
}

verify_locks() {
    local expected_channel=""
    local expected_images
    local actual_images
    local expected_cli
    local actual_cli
    local reference
    local repository
    local digest
    local immutable_reference

    require_command jq
    validate_version
    validate_source_sha
    if [[ ! -f "${TARGETS_FILE}" ]]; then
        fail "targets file not found: ${TARGETS_FILE}"
    fi

    if [[ -n "${CHANNEL}" ]]; then
        if [[ ! "${VERSION}" =~ ^(0|[1-9][0-9]*)\.([0-9]+)\.[0-9]+$ ]]; then
            fail "stable version is invalid"
        fi
        expected_channel="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
        if [[ "${CHANNEL}" != "${expected_channel}" ]]; then
            fail "channel ${CHANNEL} does not match version ${VERSION}"
        fi
    fi
    if [[ "${VERIFY_ALIASES}" == "true" && -z "${CHANNEL}" ]]; then
        fail "channel is required when verifying aliases"
    fi

    if [[ -n "${IMAGE_LOCK}" ]]; then
        require_command docker
        if [[ ! -f "${IMAGE_LOCK}" ]]; then
            fail "image lock not found: ${IMAGE_LOCK}"
        fi
        expected_images="$(jq -c \
            --arg categories "${CATEGORIES}" \
            --arg names "${NAMES}" '[
            ($categories | split(",")) as $categories
            | ($names | split(",") | map(select(length > 0))) as $names
            | .images[]
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
        ] | sort' "${TARGETS_FILE}")"
        actual_images="$(jq -c '[.[].name] | sort' "${IMAGE_LOCK}")"
        assert_lock_members "${expected_images}" "${actual_images}" \
            "image lock members"
        if ! jq -e --arg sourceSha "${SOURCE_SHA}" '
            all(.[]; .sourceSha == $sourceSha)
        ' "${IMAGE_LOCK}" > /dev/null; then
            fail "image lock source does not match the release source"
        fi

        while IFS=$'\t' read -r reference digest immutable_reference; do
            if [[ "${reference}" != *":${VERSION}" ]]; then
                fail "image lock has wrong version: ${reference}"
            fi
            if [[ "${immutable_reference}" != *"@${digest}" ]]; then
                fail "image lock has an invalid immutable reference"
            fi
            verify_image_alias "${reference}" "${digest}"
            if [[ "${VERIFY_ALIASES}" == "true" ]]; then
                repository="${immutable_reference%@*}"
                verify_image_alias "${repository}:${CHANNEL}" "${digest}"
                verify_image_alias "${repository}:latest" "${digest}"
            fi
        done < <(jq -r '.[] |
            [.reference, .digest, .immutableReference] | @tsv
        ' "${IMAGE_LOCK}")
    fi

    if [[ -n "${CLI_LOCK}" ]]; then
        require_command oras
        if [[ ! -f "${CLI_LOCK}" ]]; then
            fail "CLI lock not found: ${CLI_LOCK}"
        fi
        if ! jq -e --arg version "${VERSION}" '.version == $version' \
            "${CLI_LOCK}" > /dev/null; then
            fail "CLI lock version does not match"
        fi
        if ! jq -e --arg sourceSha "${SOURCE_SHA}" \
            '.sourceSha == $sourceSha' "${CLI_LOCK}" > /dev/null; then
            fail "CLI lock source does not match the release source"
        fi
        expected_cli="$(jq -c '[.cliAssets[].name] | sort' "${TARGETS_FILE}")"
        actual_cli="$(jq -c '[.artifacts[].name] | sort' "${CLI_LOCK}")"
        assert_lock_members "${expected_cli}" "${actual_cli}" \
            "CLI lock members"

        while IFS=$'\t' read -r reference digest immutable_reference; do
            if [[ "${reference}" != *":${VERSION}" ]]; then
                fail "CLI lock has wrong version: ${reference}"
            fi
            if [[ "${immutable_reference}" != *"@${digest}" ]]; then
                fail "CLI lock has an invalid immutable reference"
            fi
            verify_cli_alias "${reference}" "${digest}"
            if [[ "${VERIFY_ALIASES}" == "true" ]]; then
                repository="${immutable_reference%@*}"
                verify_cli_alias "${repository}:${CHANNEL}" "${digest}"
                verify_cli_alias "${repository}:latest" "${digest}"
            fi
        done < <(jq -r '.artifacts[] |
            [.reference, .digest, .immutableReference] | @tsv
        ' "${CLI_LOCK}")
    fi

    if [[ -z "${IMAGE_LOCK}" && -z "${CLI_LOCK}" ]]; then
        fail "at least one lock file is required"
    fi
}

promote_aliases() {
    local expected_channel
    local immutable_reference
    local repository
    local reference
    local digest

    require_command docker
    require_command jq
    require_command oras
    validate_version
    if [[ "${VERSION}" == *-* ]]; then
        fail "prereleases must not promote mutable aliases"
    fi
    if [[ ! "${VERSION}" =~ ^(0|[1-9][0-9]*)\.([0-9]+)\.[0-9]+$ ]]; then
        fail "stable version is invalid"
    fi
    expected_channel="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
    if [[ "${CHANNEL}" != "${expected_channel}" ]]; then
        fail "channel ${CHANNEL} does not match version ${VERSION}"
    fi
    CATEGORIES="production,non-go,test"
    verify_locks

    while IFS=$'\t' read -r reference digest immutable_reference; do
        if [[ "${reference}" != *":${VERSION}" ]]; then
            fail "image lock has wrong version: ${reference}"
        fi
        if [[ "${immutable_reference}" != *"@${digest}" ]]; then
            fail "image lock has an invalid immutable reference"
        fi
        repository="${immutable_reference%@*}"
        promote_image_aliases "${repository}" "${CHANNEL}" "${digest}" \
            "${immutable_reference}"
    done < <(jq -r '.[] |
        [.reference, .digest, .immutableReference] | @tsv
    ' "${IMAGE_LOCK}")

    while IFS=$'\t' read -r reference digest immutable_reference; do
        if [[ "${reference}" != *":${VERSION}" ]]; then
            fail "CLI lock has wrong version: ${reference}"
        fi
        if [[ "${immutable_reference}" != *"@${digest}" ]]; then
            fail "CLI lock has an invalid immutable reference"
        fi
        repository="${immutable_reference%@*}"
        promote_cli_aliases "${repository}" "${CHANNEL}" "${digest}" \
            "${immutable_reference}"
    done < <(jq -r '.artifacts[] |
        [.reference, .digest, .immutableReference] | @tsv
    ' "${CLI_LOCK}")

    VERIFY_ALIASES=true
    verify_locks
    echo "Promoted ${CHANNEL} and latest from immutable release digests"
}

main() {
    parse_args "$@"
    case "${COMMAND}" in
        stage-cli) stage_cli ;;
        promote) promote_aliases ;;
        verify) verify_locks ;;
        assert-images-absent) assert_images_absent ;;
        *) fail "unknown command: ${COMMAND}" ;;
    esac
}

main "$@"
