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
# Manage Radius Control Plane Installation
#
# This script detects the installed Radius control plane version on the
# connected Kubernetes cluster and takes appropriate action:
# - If not installed: runs rad install kubernetes
# - If same version as CLI: skips installation (no action needed)
# - If different version: attempts rad upgrade kubernetes
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME

usage() {
    echo "Usage: ${SCRIPT_NAME}"
    echo ""
    echo "Manages Radius control plane installation based on version comparison."
    echo "Requires rad CLI to be installed and in PATH."
    exit 0
}

# Parse rad version output to extract CLI version
get_cli_version() {
    rad version | grep -A1 "RELEASE" | tail -1 | awk '{print $1}'
}

main() {
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        usage
    fi

    # Verify rad CLI is available
    if ! command -v rad &> /dev/null; then
        echo "Error: rad CLI not found in PATH"
        exit 1
    fi

    echo "============================================================================"
    echo "Radius Control Plane Management"
    echo "============================================================================"

    # Get CLI version
    local cli_version
    cli_version=$(get_cli_version)
    if [[ -z "${cli_version}" ]]; then
        echo "Error: Failed to parse CLI version from 'rad version' output." >&2
        exit 1
    fi
    echo "CLI Version: ${cli_version}"

    # Get control plane info
    local cp_line cp_status cp_version
    cp_line=$(rad version | grep -A1 "STATUS" | tail -1)
    cp_status=$(echo "${cp_line}" | awk '{print $1}')
    cp_version=$(echo "${cp_line}" | awk '{print $2}')
    if [[ -z "${cp_status}" || -z "${cp_version}" ]]; then
        echo "Error: Failed to parse control plane status or version from 'rad version' output." >&2
        exit 1
    fi
    echo "Control Plane Status: ${cp_status}"
    echo "Control Plane Version: ${cp_version}"

    # Determine action based on control plane status
    if [[ "${cp_status}" == "Not" ]]; then
        echo ""
        echo "Radius is not installed on the cluster. Installing..."
        if ! rad install kubernetes \
            --set global.azureWorkloadIdentity.enabled=true \
            --set database.enabled=true; then
            echo ""
            echo "============================================================================"
            echo "ERROR: Radius installation failed"
            echo "============================================================================"
            echo "The installation could not be completed."
            echo "Please check the error message above for details."
            exit 1
        fi
        echo "Radius installation complete."
    elif [[ "${cp_version}" == "${cli_version}" ]]; then
        echo ""
        echo "Radius control plane version matches CLI version (${cli_version}). No action needed."
    else
        echo ""
        echo "Version mismatch detected. Attempting upgrade from ${cp_version} to ${cli_version}..."
        # There are scenarios when an upgrade may not be possible, and we are relying on the rad upgrade command to
        # detect and report an error, which will cause the workflow to fail. Manual intervention may be required in such cases.
        if ! rad upgrade kubernetes; then
            echo ""
            echo "============================================================================"
            echo "ERROR: Radius upgrade failed"
            echo "============================================================================"
            echo "The upgrade from version ${cp_version} to ${cli_version} could not be completed."
            echo "This may be due to an incompatible version transition or other upgrade constraints."
            echo "Please check the error message above for details and manually upgrade if necessary."
            exit 1
        fi
        echo "Radius upgrade complete."
    fi

    echo "============================================================================"
    echo "Radius Control Plane Management Complete"
    echo "============================================================================"
}

main "$@"
