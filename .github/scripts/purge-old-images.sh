#!/usr/bin/env bash

set -euo pipefail

REPOSITORY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=build/scripts/retry-curl.sh
source "${REPOSITORY_ROOT}/build/scripts/retry-curl.sh"

readonly API_URL="${GITHUB_API_URL:-https://api.github.com}"
readonly PACKAGE_OWNER="${PACKAGE_OWNER:?PACKAGE_OWNER is required}"
readonly PACKAGE_PREFIX="${PACKAGE_PREFIX:-dev/}"
readonly PACKAGE_TAG_PREFIX="${PACKAGE_TAG_PREFIX:-pr-}"
readonly RETENTION_DAYS="${RETENTION_DAYS:-7}"
readonly DRY_RUN="${PURGE_DRY_RUN:-false}"
readonly RATE_LIMIT_STATUS=75

WORK_DIR=""
DELETED_COUNT=0
MISSING_COUNT=0

cleanup() {
    if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
        rm -rf "${WORK_DIR}"
    fi
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

api_headers() {
    printf '%s\n' \
        -H "Accept: application/vnd.github+json" \
        -H "Authorization: Bearer ${GH_TOKEN}" \
        -H "X-GitHub-Api-Version: 2022-11-28"
}

api_get() {
    local path="$1"
    local output="$2"
    local header_file="${WORK_DIR}/get-headers"
    local status=0
    local http_code
    local -a headers

    mapfile -t headers < <(api_headers)
    http_code="$(
        retry_curl --fail-with-body "${headers[@]}" \
            --dump-header "${header_file}" --output "${output}" \
            --write-out '%{http_code}' "${API_URL}${path}"
    )" || status=$?
    if ((status == 0)); then
        return 0
    fi
    if is_rate_limited "${header_file}" "${http_code}"; then
        return "${RATE_LIMIT_STATUS}"
    fi
    echo "GET ${path} returned HTTP ${http_code}:" \
        "$(response_message "${output}")" >&2
    return "${status}"
}

response_message() {
    local response="$1"

    [[ -s "${response}" ]] || return 0
    jq -r '.message // empty' "${response}" 2>/dev/null ||
        head -c 500 "${response}"
}

# GitHub reports primary limits with x-ratelimit-remaining: 0 and
# secondary limits with HTTP 429 or a 403 that includes Retry-After.
is_rate_limited() {
    local header_file="$1"
    local http_code="$2"

    case "${http_code}" in
        429) return 0 ;;
        403)
            [[ "$(retry_curl_header_value \
                "${header_file}" x-ratelimit-remaining)" == 0 ||
                -n "$(retry_curl_header_value \
                    "${header_file}" retry-after)" ]]
            return
            ;;
    esac
    return 1
}

urlencode() {
    jq -rn --arg value "$1" '$value | @uri'
}

list_packages() {
    local page=1
    local response="${WORK_DIR}/packages.json"
    local count
    local path

    while true; do
        path="/orgs/${PACKAGE_OWNER}/packages"
        path+="?package_type=container&per_page=100&page=${page}"
        api_get "${path}" "${response}" || return "$?"
        jq -r --arg prefix "${PACKAGE_PREFIX}" '
            .[] | .name | select(startswith($prefix))
        ' "${response}" || return "$?"
        count="$(jq -er 'length' "${response}")" || return "$?"
        ((count == 100)) || return 0
        page=$((page + 1))
    done
}

list_expired_versions() {
    local package_name="$1"
    local cutoff="$2"
    local encoded_name
    local page=1
    local response="${WORK_DIR}/versions.json"
    local count
    local path

    encoded_name="$(urlencode "${package_name}")" || return "$?"
    while true; do
        path="/orgs/${PACKAGE_OWNER}/packages/container/${encoded_name}"
        path+="/versions?per_page=100&page=${page}"
        api_get "${path}" "${response}" || return "$?"
        jq -r --arg prefix "${PACKAGE_TAG_PREFIX}" \
            --argjson cutoff "${cutoff}" '
            .[] |
            select((.updated_at | fromdateiso8601) < $cutoff) |
            select(any(.metadata.container.tags[]?; startswith($prefix))) |
            .id
        ' "${response}" || return "$?"
        count="$(jq -er 'length' "${response}")" || return "$?"
        ((count == 100)) || return 0
        page=$((page + 1))
    done
}

delete_version() {
    local package_name="$1"
    local version_id="$2"
    local encoded_name
    local response="${WORK_DIR}/delete-response"
    local header_file="${WORK_DIR}/delete-headers"
    local message
    local status=0
    local http_code
    local path
    local -a headers

    if [[ "${DRY_RUN}" == true ]]; then
        echo "Would delete ${package_name} version ${version_id}"
        return 0
    fi

    encoded_name="$(urlencode "${package_name}")"
    path="/orgs/${PACKAGE_OWNER}/packages/container/${encoded_name}"
    path+="/versions/${version_id}"
    mapfile -t headers < <(api_headers)
    http_code="$(
        retry_curl --request DELETE --fail-with-body "${headers[@]}" \
            --dump-header "${header_file}" --output "${response}" \
            --write-out '%{http_code}' "${API_URL}${path}"
    )" || status=$?
    case "${http_code}" in
        204)
            echo "Deleted ${package_name} version ${version_id}"
            DELETED_COUNT=$((DELETED_COUNT + 1))
            return 0
            ;;
        404)
            echo "${package_name} version ${version_id} was already deleted"
            MISSING_COUNT=$((MISSING_COUNT + 1))
            return 0
            ;;
    esac

    if is_rate_limited "${header_file}" "${http_code}"; then
        return "${RATE_LIMIT_STATUS}"
    fi
    message="$(response_message "${response}" | tr '\n' ' ')"
    echo "Delete of ${package_name} version ${version_id} returned" \
        "HTTP ${http_code}: ${message}" >&2
    if ((status != 0)); then
        return "${status}"
    fi
    return 1
}

purge_packages() {
    local cutoff="$1"
    local packages="$2"
    local package_name
    local version_id
    local versions

    while IFS= read -r package_name; do
        [[ -n "${package_name}" ]] || continue
        versions="$(
            list_expired_versions "${package_name}" "${cutoff}"
        )" || return "$?"
        while IFS= read -r version_id; do
            [[ -n "${version_id}" ]] || continue
            delete_version "${package_name}" "${version_id}" ||
                return "$?"
        done <<< "${versions}"
    done <<< "${packages}"
}

main() {
    [[ -n "${GH_TOKEN:-}" ]] || fail "GH_TOKEN is required"
    command -v curl >/dev/null 2>&1 || fail "curl is required"
    command -v jq >/dev/null 2>&1 || fail "jq is required"
    [[ "${RETENTION_DAYS}" =~ ^[1-9][0-9]*$ ]] ||
        fail "RETENTION_DAYS must be a positive integer"

    WORK_DIR="$(mktemp -d)"
    trap cleanup EXIT
    DELETED_COUNT=0
    MISSING_COUNT=0

    local cutoff
    cutoff="${PURGE_CUTOFF_EPOCH:-$(
        date -u -d "${RETENTION_DAYS} days ago" +%s
    )}"
    local packages
    local status=0
    packages="$(list_packages)" || status=$?
    if ((status == 0)); then
        purge_packages "${cutoff}" "${packages}" || status=$?
    fi

    if [[ "${DRY_RUN}" != true ]]; then
        echo "Deleted ${DELETED_COUNT}, already deleted ${MISSING_COUNT}" \
            "package versions"
    fi
    if ((status == RATE_LIMIT_STATUS)); then
        echo "::warning::GitHub API rate limit reached; the next" \
            "scheduled run continues the cleanup"
        return 0
    fi
    return "${status}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
