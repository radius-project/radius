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

# RAD_VERSION is required - it should be passed from the workflow
if [ -z "$RAD_VERSION" ]; then
    echo "Error: RAD_VERSION is required but not provided."
    echo "Usage: $0 <OS> <ARCH> <FILE> <EXT> <RAD_VERSION>"
    exit 1
fi

echo "Using RAD_VERSION: $RAD_VERSION"

# Download the CLI binary from GitHub releases
filename="${FILE}_${OS}_${ARCH}${EXT}"
download_url="https://github.com/radius-project/radius/releases/download/$RAD_VERSION/$filename"

echo "Downloading $filename from $download_url"
curl -sSL "$download_url" --fail-with-body -o "$filename"

echo "CLI download test completed successfully for $OS/$ARCH"