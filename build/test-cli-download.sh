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

echo "Starting CLI download test for $OS/$ARCH"

# Get latest version - replicate original workflow logic with debug output
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

# Extract RAD_VERSION using the original workflow parsing logic
RAD_VERSION=$(echo "$api_response" | grep "tag_name" | grep -v rc | awk 'NR==1{print $2}' | sed -n 's/"\(.*\)",/\1/p')

if [ -z "$RAD_VERSION" ]; then
    echo "Failed to extract RAD_VERSION from API response"
    exit 1
fi

echo "Successfully retrieved RAD_VERSION: $RAD_VERSION"

# Download file - replicate original workflow logic
filename="${FILE}_${OS}_${ARCH}${EXT}"
download_url="https://github.com/radius-project/radius/releases/download/$RAD_VERSION/$filename"

echo "Downloading $filename from $download_url"
curl -sSLI -w "%{http_code}" "$download_url" --fail-with-body -o "$filename"

# Test Linux amd64 binary if applicable - adapted from original x64 condition
if [ "$OS" == "linux" ] && [ "$ARCH" == "amd64" ]; then
    echo "Testing Linux amd64 binary..."
    chmod +x "./$filename"
    "./$filename" version
fi

echo "CLI download test completed successfully for $OS/$ARCH"