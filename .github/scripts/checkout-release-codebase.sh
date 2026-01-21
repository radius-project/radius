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

# ============================================================================
# Checkout Release Codebase
#
# This script clones the Radius repository at the tag matching the installed
# Radius CLI version into a "current_release" subfolder.
#
# This allows long-running tests to run against the release codebase while
# keeping the main repository clone intact for workflow infrastructure.
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly RELEASE_DIR="current_release"

usage() {
    echo "Usage: ${SCRIPT_NAME}"
    echo ""
    echo "Clones the Radius repository at the installed release tag into"
    echo "a '${RELEASE_DIR}' subfolder."
    echo ""
    echo "Requires rad CLI to be installed and in PATH."
    exit 0
}

get_cli_version() {
    rad version | grep -A1 "RELEASE" | tail -1 | awk '{print $1}'
}

main() {
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        usage
    fi

    if ! command -v rad &> /dev/null; then
        echo "Error: rad CLI not found in PATH"
        exit 1
    fi

    echo "============================================================================"
    echo "Checkout Release Codebase"
    echo "============================================================================"

    local release_version
    release_version=$(get_cli_version)
    if [[ -z "${release_version}" ]]; then
        echo "Error: Failed to parse CLI version from 'rad version' output"
        exit 1
    fi

    # Validate that we have a proper semantic version, not "edge"
    if [[ "${release_version}" == "edge" ]]; then
        echo "Error: CLI reports 'edge' version instead of a release version."
        echo "This script requires an official Radius release to be installed."
        echo "Please verify that the Radius CLI was installed from an official release."
        exit 1
    fi

    # Validate version format (should be semver like X.Y.Z or X.Y.Z-rcN)
    if ! [[ "${release_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
        echo "Error: Invalid version format '${release_version}'"
        echo "Expected semantic version format (e.g., '0.54.0' or '0.54.0-rc1')"
        exit 1
    fi

    local release_tag="v${release_version}"
    echo "Installed Radius version: ${release_version}"
    echo "Release tag: ${release_tag}"

    # Remove existing release directory if present
    if [[ -d "${RELEASE_DIR}" ]]; then
        echo ""
        echo "Removing existing ${RELEASE_DIR} directory..."
        rm -rf "${RELEASE_DIR}"
    fi

    echo ""
    echo "Cloning repository at tag ${release_tag} into ${RELEASE_DIR}..."
    git clone --depth 1 --branch "${release_tag}" --recurse-submodules \
        "https://github.com/radius-project/radius.git" "${RELEASE_DIR}"

    echo ""
    echo "Verifying clone..."
    if [[ ! -f "${RELEASE_DIR}/go.mod" ]]; then
        echo "Error: go.mod not found in ${RELEASE_DIR}. Something went wrong."
        exit 1
    fi

    local checkout_version
    checkout_version=$(cd "${RELEASE_DIR}" && git describe --tags --always 2>/dev/null || echo "unknown")
    echo "Cloned version: ${checkout_version}"

    echo ""
    echo "============================================================================"
    echo "Release Codebase Clone Complete"
    echo "============================================================================"
    echo "Release codebase location: ${RELEASE_DIR}"
    echo "Release tag: ${release_tag}"

    # Output the release directory for use in subsequent workflow steps
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "release-dir=${RELEASE_DIR}" >> "${GITHUB_OUTPUT}"
    fi
}

main "$@"
