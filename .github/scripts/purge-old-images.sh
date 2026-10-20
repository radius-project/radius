#!/usr/bin/env bash

# Shared, bounded HTTP transport and safe default entry point for package cleanup.
set -euo pipefail

CLEANUP_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly CLEANUP_ROOT
# shellcheck source=build/scripts/retry-curl.sh
source "${CLEANUP_ROOT}/build/scripts/retry-curl.sh"

cleanup_request() {
    local service="$1" method="$2" path="$3"
    local work code status=0 token=""
    local -a headers=(-H "Accept: application/vnd.github+json")
    [[ "${method}" == GET || "${method}" == DELETE ]] || return 2
    case "${service}" in
        github)
            [[ "${path}" == /orgs/radius-project/packages* ||
                "${path}" == /repos/radius-project/radius ||
                "${path}" == /repos/radius-project/radius/* ]] || return 2
            [[ "${path}" != *".."* && "${path}" != *$'\n'* ]] || return 2
            if [[ "${method}" == DELETE ]]; then
                [[ "${path}" =~ ^/orgs/radius-project/packages/container/[a-zA-Z0-9%._-]+/versions/[1-9][0-9]*$ ]] ||
                    return 2
            fi
            headers+=(-H "Authorization: Bearer ${GH_TOKEN:?GH_TOKEN is required}"
                -H "X-GitHub-Api-Version: 2022-11-28")
            path="https://api.github.com${path}"
            ;;
        registry)
            [[ "${method}" == GET &&
                "${path}" =~ ^/v2/radius-project/[a-z0-9/._-]+/manifests/sha256:[a-f0-9]{64}$ &&
                "${path}" != *".."* ]] || return 2
            local package="${path#/v2/}"
            package="${package%/manifests/*}"
            token="$(retry_curl \
                "https://ghcr.io/token?service=ghcr.io&scope=repository:${package}:pull" |
                jq -er '.token | select(type == "string" and length > 0)')" ||
                return "$?"
            headers=(-H "Authorization: Bearer ${token}"
                -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
            path="https://ghcr.io${path}"
            ;;
        *) return 2 ;;
    esac
    work="$(mktemp -d)"
    code="$(retry_curl --request "${method}" --fail-with-body \
        "${headers[@]}" --output "${work}/body" \
        --dump-header "${work}/headers" --write-out '%{http_code}' \
        "${path}")" || status=$?
    if [[ ! "${code}" =~ ^[0-9]{3}$ || ! -f "${work}/body" ]]; then
        echo "Cleanup transport failed (curl ${status})" >&2
        rm -rf "${work}"
        return 1
    fi
    jq -n --argjson status "${code}" --rawfile body "${work}/body" \
        --arg remaining "$(retry_curl_header_value \
            "${work}/headers" x-ratelimit-remaining)" \
        --arg retryAfter "$(retry_curl_header_value \
            "${work}/headers" retry-after)" \
        '{status: $status, body: $body,
          headers: {remaining: $remaining, retryAfter: $retryAfter}}'
    rm -rf "${work}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    if [[ "${1:-}" == --request && $# == 4 ]]; then
        cleanup_request "$2" "$3" "$4"
    else
        exec node "${CLEANUP_ROOT}/.github/scripts/package-cleanup.mjs" "$@"
    fi
fi
