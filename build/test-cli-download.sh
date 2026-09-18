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

# Default values
OS=${1:-"linux"}
ARCH=${2:-"amd64"}
FILE=${3:-"rad"}
EXT=${4:-""}
MINIMUM_VERSION=${5:-""}

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

echo "Starting CLI download test for $OS/$ARCH"

# Get latest version from GitHub releases API
echo "Fetching latest release version from GitHub API..."
radReleaseUrl="https://api.github.com/repos/radius-project/radius/releases"

# Make API call
# `|| { ... }` rather than a following `$?` check: the script runs under `set -e`, so
# a failing curl inside the assignment exits immediately and a separate check below it
# is never reached. -sS keeps curl quiet on success but lets its own error through.
api_response=$(curl -sS "$radReleaseUrl") || {
    printf 'GitHub API call to %s failed (curl exit %d)\n' "$radReleaseUrl" "$?" >&2
    exit 1
}

echo "GitHub API call successful"

# Extract version from API response using grep, awk, and sed.
# `|| true` stops a non-matching grep (e.g. an API error response with no
# "tag_name") from tripping `set -e` here, before the empty-value check below
# can report a useful error.
RAD_VERSION=$(echo "$api_response" | grep "tag_name" | grep -v rc | awk 'NR==1{print $2}' | sed -n 's/"\(.*\)",/\1/p') || true

if [ -z "$RAD_VERSION" ]; then
    printf 'Failed to extract RAD_VERSION from API response:\n%s\n' "$api_response" >&2
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