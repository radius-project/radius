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
RAD_VERSION=${5:-""}

echo "Starting CLI download test for $OS/$ARCH"

# Use provided version or get latest version from GitHub releases API
if [ -n "$RAD_VERSION" ]; then
    echo "Using provided RAD_VERSION: $RAD_VERSION"
else
    echo "Fetching latest release version from GitHub API..."
    radReleaseUrl="https://api.github.com/repos/radius-project/radius/releases"

    # Make API call
    api_response=$(curl -s "$radReleaseUrl")
    curl_exit_code=$?

    if [ $curl_exit_code -ne 0 ]; then
        echo "GitHub API call failed with exit code: $curl_exit_code"
        exit 1
    fi

    echo "GitHub API call successful"

    # Extract version from API response using jq
    # Select the first release that is not an RC (release candidate)
    RAD_VERSION=$(echo "$api_response" | jq -r '[.[] | select(.tag_name | test("rc") | not)] | .[0].tag_name // empty') || true

    if [ -z "$RAD_VERSION" ]; then
        echo "Failed to extract RAD_VERSION from API response"
        echo "API Response (first 50 lines):"
        echo "$api_response" | head -n 50
        exit 1
    fi

    echo "Successfully retrieved RAD_VERSION: $RAD_VERSION"
fi

# Download the CLI binary from GitHub releases
filename="${FILE}_${OS}_${ARCH}${EXT}"
download_url="https://github.com/radius-project/radius/releases/download/$RAD_VERSION/$filename"

echo "Downloading $filename from $download_url"
curl -sSL "$download_url" --fail-with-body -o "$filename"

echo "CLI download test completed successfully for $OS/$ARCH"