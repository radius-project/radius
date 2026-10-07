#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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

# --resolve-version prints only the latest stable version to stdout (requires jq).
# GH_TOKEN or GITHUB_TOKEN optionally authenticates release discovery.
# The sixth argument supplies a resolved version and skips discovery.

# Default values
OS=${1:-"linux"}
ARCH=${2:-"amd64"}
FILE=${3:-"rad"}
EXT=${4:-""}
MINIMUM_VERSION=${5:-""}
RAD_VERSION=${6:-""}

resolve_version() {
    local -r release_url="https://api.github.com/repos/radius-project/radius/releases"
    local -r token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
    local headers=(-H "Accept: application/vnd.github+json")
    local response status version

    if ! command -v jq >/dev/null 2>&1; then
        echo "jq is required to resolve the latest CLI release version" >&2
        return 1
    fi
    if [[ -n "${token}" ]]; then
        headers+=(-H "Authorization: Bearer ${token}")
    fi

    echo "Fetching latest release version from GitHub API..." >&2
    response=$(curl -sS "${headers[@]}" -w '\n%{http_code}' \
        "${release_url}") || {
        printf 'GitHub API call to %s failed (curl exit %d)\n' \
            "${release_url}" "$?" >&2
        return 1
    }
    status=${response##*$'\n'}
    response=${response%$'\n'*}
    if [[ "${status}" != "200" ]]; then
        printf 'GitHub API call failed (HTTP %s):\n%s\n' \
            "${status}" "${response}" >&2
        return 1
    fi

    if ! version=$(jq -er '
        if type != "array" then error("Expected a releases array") else
            [.[] | select(.draft != true and .prerelease != true)
                | .tag_name | select(type == "string")
                | select(test("^v[0-9]+\\.[0-9]+\\.[0-9]+$"))][0] // empty
        end
    ' <<<"${response}"); then
        printf 'Failed to extract a stable RAD_VERSION (HTTP %s):\n%s\n' \
            "${status}" "${response}" >&2
        return 1
    fi

    printf '%s\n' "${version}"
}

version_is_at_least() {
    local -r actual="${1#v}"
    local -r minimum="${2#v}"
    local -r version_pattern='^[0-9]+\.[0-9]+\.[0-9]+$'
    local -a actual_parts
    local -a minimum_parts
    local index

    if [[ ! "${actual}" =~ ${version_pattern} ]]; then
        echo "Invalid release version: ${1}" >&2
        return 2
    fi

    if [[ ! "${minimum}" =~ ${version_pattern} ]]; then
        echo "Invalid minimum version: ${2}" >&2
        return 2
    fi

    IFS=. read -r -a actual_parts <<< "${actual}"
    IFS=. read -r -a minimum_parts <<< "${minimum}"

    for index in 0 1 2; do
        if ((10#${actual_parts[$index]} > 10#${minimum_parts[$index]})); then
            return 0
        fi
        if ((10#${actual_parts[$index]} < 10#${minimum_parts[$index]})); then
            return 1
        fi
    done

    return 0
}

if [[ "${1:-}" == "--resolve-version" ]]; then
    resolve_version
    exit 0
fi

echo "Starting CLI download test for $OS/$ARCH"

if [[ -z "${RAD_VERSION}" ]]; then
    RAD_VERSION=$(resolve_version)
fi
if [[ ! "${RAD_VERSION}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf 'Invalid release version: %s\n' "${RAD_VERSION}" >&2
    exit 1
fi

echo "Successfully retrieved RAD_VERSION: $RAD_VERSION"

if [[ -n "${MINIMUM_VERSION}" ]]; then
    if version_is_at_least "${RAD_VERSION}" "${MINIMUM_VERSION}"; then
        :
    else
        compare_status=$?
        if ((compare_status == 2)); then
            exit 1
        fi

        echo "Skipping CLI download test for ${OS}/${ARCH}: latest stable" \
            "release ${RAD_VERSION} predates ${MINIMUM_VERSION}"
        exit 0
    fi
fi

# Download the CLI binary from GitHub releases
filename="${FILE}_${OS}_${ARCH}${EXT}"
download_url="https://github.com/radius-project/radius/releases/download/$RAD_VERSION/$filename"

echo "Downloading $filename from $download_url"
curl -sSL "$download_url" --fail-with-body -o "$filename"

echo "CLI download test completed successfully for $OS/$ARCH"