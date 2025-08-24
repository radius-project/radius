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

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Default values
OS=${1:-"linux"}
ARCH=${2:-"amd64"}
FILE=${3:-"rad"}
EXT=${4:-""}

print_info "Starting CLI download test for $OS/$ARCH"

# Get latest version with enhanced debugging
print_info "Fetching latest release version from GitHub API..."
radReleaseUrl="https://api.github.com/repos/radius-project/radius/releases"

print_info "Making API call to: $radReleaseUrl"
api_response=$(curl -s "$radReleaseUrl")
api_exit_code=$?

if [ $api_exit_code -ne 0 ]; then
    print_error "GitHub API call failed with exit code: $api_exit_code"
    exit 1
fi

print_info "GitHub API call successful, parsing response..."
print_info "API response size: $(echo "$api_response" | wc -c) characters"

# Check if response contains expected data
if ! echo "$api_response" | grep -q "tag_name"; then
    print_error "GitHub API response does not contain tag_name field"
    print_info "First 500 characters of response:"
    echo "$api_response" | head -c 500
    exit 1
fi

# Extract version tag from the first non-release candidate entry
# This command finds the first "tag_name" field, excludes release candidates,
# takes the first match, and extracts the version string between quotes
RAD_VERSION=$(echo "$api_response" | grep "tag_name" | grep -v rc | awk 'NR==1{print $2}' | sed -n 's/"\(.*\)",/\1/p')

if [ -z "$RAD_VERSION" ]; then
    print_error "Failed to extract RAD_VERSION from API response"
    print_info "Lines containing tag_name:"
    echo "$api_response" | grep "tag_name" | head -5
    exit 1
fi

print_success "Successfully retrieved RAD_VERSION: $RAD_VERSION"

# Construct download URL
download_url="https://github.com/radius-project/radius/releases/download/$RAD_VERSION/${FILE}_${OS}_${ARCH}${EXT}"
filename="${FILE}_${OS}_${ARCH}${EXT}"

print_info "Download URL: $download_url"
print_info "Target filename: $filename"

# Download file
print_info "Downloading $filename..."
curl_output=$(curl -sSLI -w "%{http_code}" "$download_url" --fail-with-body -o "$filename" 2>&1)
curl_exit_code=$?

if [ $curl_exit_code -ne 0 ]; then
    print_error "Download failed with exit code: $curl_exit_code"
    print_error "curl output: $curl_output"
    exit 1
fi

print_success "Successfully downloaded $filename"

# Test Linux x64 binary if applicable
if [ "$OS" == "linux" ] && [ "$ARCH" == "amd64" ]; then
    print_info "Testing Linux x64 binary..."
    chmod +x "./$filename"
    
    print_info "Running version command..."
    version_output=$("./$filename" version 2>&1)
    version_exit_code=$?
    
    if [ $version_exit_code -ne 0 ]; then
        print_error "Version command failed with exit code: $version_exit_code"
        print_error "Version output: $version_output"
        exit 1
    fi
    
    print_success "Version command successful:"
    echo "$version_output"
fi

print_success "CLI download test completed successfully for $OS/$ARCH"