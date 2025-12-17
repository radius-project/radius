#!/bin/bash

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

# Parse rad version output to extract control plane status and version
get_control_plane_info() {
    local cp_line
    cp_line=$(rad version | grep -A1 "STATUS" | tail -1)
    
    CP_STATUS=$(echo "${cp_line}" | awk '{print $1}')
    CP_VERSION=$(echo "${cp_line}" | awk '{print $2}')
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
    get_control_plane_info
    if [[ -z "${CP_STATUS}" || -z "${CP_VERSION}" ]]; then
        echo "Error: Failed to parse control plane status or version from 'rad version' output." >&2
        exit 1
    fi
    echo "Control Plane Status: ${CP_STATUS}"
    echo "Control Plane Version: ${CP_VERSION}"

    # Determine action based on control plane status
    if [[ "${CP_STATUS}" == "Not" ]]; then
        echo ""
        echo "Radius is not installed on the cluster. Installing..."
        if ! rad install kubernetes; then
            echo ""
            echo "============================================================================"
            echo "ERROR: Radius installation failed"
            echo "============================================================================"
            echo "The installation could not be completed."
            echo "Please check the error message above for details."
            exit 1
        fi
        echo "Radius installation complete."
    elif [[ "${CP_VERSION}" == "${cli_version}" ]]; then
        echo ""
        echo "Radius control plane version matches CLI version (${cli_version}). No action needed."
    else
        echo ""
        echo "Version mismatch detected. Attempting upgrade from ${CP_VERSION} to ${cli_version}..."
        if ! rad upgrade kubernetes; then
            echo ""
            echo "============================================================================"
            echo "ERROR: Radius upgrade failed"
            echo "============================================================================"
            echo "The upgrade from version ${CP_VERSION} to ${cli_version} could not be completed."
            echo "This may be due to an incompatible version transition or other upgrade constraints."
            echo "Please check the error message above for details."
            exit 1
        fi
        echo "Radius upgrade complete."
    fi

    echo "============================================================================"
    echo "Radius Control Plane Management Complete"
    echo "============================================================================"
}

main "$@"
