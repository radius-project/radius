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
# This script aligns the codebase with the installed Radius release version
# while preserving the workflow infrastructure from the triggering commit.
#
# The problem: When running long-running tests against the current Radius
# release, the tests and Go code must match the release version. If tests
# from main are run against an older release, they may fail due to API
# changes, new features, or dependency mismatches.
#
# The solution:
# 1. Detect the installed Radius release version from the CLI
# 2. Checkout the entire codebase at the release tag
# 3. Restore .github/ and build/ from the triggering commit (GITHUB_SHA)
#
# This ensures:
# - Go code, tests, and dependencies match the installed release
# - Workflow scripts and build infrastructure are from the current branch
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME

usage() {
    echo "Usage: ${SCRIPT_NAME}"
    echo ""
    echo "Aligns the codebase with the installed Radius release version."
    echo "Preserves .github/ and build/ directories from the triggering commit."
    echo ""
    echo "Required environment variables:"
    echo "  GITHUB_SHA - The commit SHA that triggered the workflow"
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

    if [[ -z "${GITHUB_SHA:-}" ]]; then
        echo "Error: GITHUB_SHA environment variable is not set"
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
    echo "Workflow commit: ${GITHUB_SHA}"

    echo ""
    echo "Saving workflow infrastructure to temp location..."
    local temp_dir
    temp_dir=$(mktemp -d)
    cp -r .github "${temp_dir}/.github"
    cp -r build "${temp_dir}/build"

    echo ""
    echo "Fetching release tag ${release_tag}..."
    git fetch origin "refs/tags/${release_tag}:refs/tags/${release_tag}"

    echo ""
    echo "Checking out codebase at release tag ${release_tag}..."
    git checkout "${release_tag}" -- .

    echo ""
    echo "Updating submodules to match release tag..."
    git submodule update --init --recursive

    echo ""
    echo "Restoring workflow infrastructure from ${GITHUB_SHA}..."
    rm -rf .github build
    cp -r "${temp_dir}/.github" .github
    cp -r "${temp_dir}/build" build
    rm -rf "${temp_dir}"

    echo ""
    echo "Verifying checkout..."
    local checkout_version
    checkout_version=$(git describe --tags --always 2>/dev/null || echo "unknown")
    echo "Git describe output: ${checkout_version}"
    
    # Verify that go.mod exists (basic sanity check)
    if [[ ! -f "go.mod" ]]; then
        echo "Error: go.mod not found after checkout. Something went wrong."
        exit 1
    fi

    echo ""
    echo "============================================================================"
    echo "Codebase Alignment Complete"
    echo "============================================================================"
    echo "Go code and tests: ${release_tag}"
    echo "Workflow infrastructure: ${GITHUB_SHA}"
}

main "$@"
