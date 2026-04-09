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

# Verify that manifests are successfully registered in the UCP pod
# This should be called after rad install kubernetes completes
verify_manifests_registered() {
    echo ""
    echo "Verifying manifests are registered..."

    local log_file="registermanifest_logs.txt"
    rm -f "${log_file}"

    # Find the pod with container "ucp"
    local pod_name
    pod_name=$(
        kubectl get pods -n radius-system \
            -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.containers[*].name}{"\n"}{end}' \
        | grep "ucp" \
        | head -n1 \
        | cut -d" " -f1
    )
    echo "Found ucp pod: ${pod_name}"

    if [[ -z "${pod_name}" ]]; then
        echo "No pod with container 'ucp' found in namespace radius-system."
        exit 1
    fi

    # Poll logs for up to 20 iterations, 30 seconds each (up to 10 minutes total)
    local _i
    for _i in {1..20}; do
        kubectl logs "${pod_name}" -n radius-system | tee "${log_file}" > /dev/null

        # Exit on error
        if grep -qi "Service initializer terminated with error" "${log_file}"; then
            echo "Error found in ucp logs."
            grep -i "Service initializer terminated with error" "${log_file}"
            exit 1
        fi

        # Check for success
        if grep -q "Successfully registered manifests" "${log_file}"; then
            echo "Successfully registered manifests - message found."
            break
        fi

        echo "Logs not ready, waiting 30 seconds..."
        sleep 30
    done

    # Final check to ensure success message was found
    if ! grep -q "Successfully registered manifests" "${log_file}"; then
        echo "Manifests not registered after 10 minutes."
        exit 1
    fi

    echo "Manifest verification complete."
}

# Actively verify that resource types are registered and the Radius API is
# able to serve requests. Unlike verify_manifests_registered (which reads
# historical pod logs), this makes a live API call.
# Returns: 0 = healthy, 1 = provider missing, 2 = query failed
verify_resource_types_available() {
    echo ""
    echo "Verifying resource types are available..."

    # Ensure a workspace exists so rad CLI can reach the cluster.
    local workspace_output workspace_exit_code
    workspace_output=$(rad workspace create kubernetes --force 2>&1) &&
        workspace_exit_code=0 || workspace_exit_code=$?

    if [[ ${workspace_exit_code} -ne 0 ]]; then
        echo "ERROR: Failed to create Radius Kubernetes workspace (exit code: ${workspace_exit_code})."
        echo "rad workspace create output: ${workspace_output}"
        return 2
    fi

    # List registered resource providers. Applications.Core must be present
    # for environment/container operations to work.
    local output exit_code
    output=$(rad resource-provider list 2>&1) && exit_code=0 || exit_code=$?

    if [[ ${exit_code} -ne 0 ]]; then
        echo "ERROR: Failed to query registered resource providers (exit code: ${exit_code})."
        echo "rad resource-provider list output: ${output}"
        return 2
    fi

    if echo "${output}" | grep -Fq "Applications.Core"; then
        echo "Resource types are available (Applications.Core provider found)."
        return 0
    fi

    echo "ERROR: Applications.Core resource provider is NOT registered."
    echo "rad resource-provider list output: ${output}"
    return 1
}

# Save the list of Radius UCP resources to skip-delete-resources-list.txt
# This file is used by the cleanup job to avoid deleting Radius-managed resources.
save_skip_resources_list() {
    echo "Saving list of resources not to be deleted..."
    local tmp_file
    tmp_file="$(mktemp skip-delete-resources-list.txt.XXXXXX)"

    if kubectl get resources.ucp.dev -n radius-system --no-headers \
        -o custom-columns=":metadata.name" > "${tmp_file}"; then
        mv "${tmp_file}" skip-delete-resources-list.txt
        echo "Skip resources list saved."
    else
        echo "Error: Failed to retrieve UCP resources from cluster." >&2
        rm -f "${tmp_file}"
        exit 1
    fi
}

# Install Radius on the cluster
install_radius() {
    echo "Installing Radius..."
    if ! rad install kubernetes \
        --set global.azureWorkloadIdentity.enabled=true \
        --set database.enabled=false; then
        echo ""
        echo "============================================================================"
        echo "ERROR: Radius installation failed"
        echo "============================================================================"
        echo "The installation could not be completed."
        echo "Please check the error message above for details."
        exit 1
    fi
    echo "Radius installation complete."

    verify_manifests_registered

    save_skip_resources_list
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
        echo "Radius is not installed on the cluster."
        install_radius
    elif [[ "${cp_version}" == "edge" ]]; then
        echo ""
        echo "Edge version detected. Uninstalling and reinstalling with release version..."
        if ! rad uninstall kubernetes --purge --yes; then
            echo ""
            echo "============================================================================"
            echo "ERROR: Radius uninstall failed"
            echo "============================================================================"
            echo "The uninstall could not be completed."
            echo "Please check the error message above for details."
            exit 1
        fi
        echo "Radius uninstall complete."
        install_radius
    elif [[ "${cp_version}" == "${cli_version}" ]]; then
        echo ""
        echo "Radius control plane version matches CLI version (${cli_version}). Skipping install/upgrade."

        # Verify resource types with retry for transient failures.
        local check_result=0
        verify_resource_types_available || check_result=$?

        if [[ ${check_result} -eq 2 ]]; then
            # Query failed (connectivity/auth issue). Retry once after a brief wait
            # before taking destructive action.
            echo ""
            echo "Resource type query failed. Retrying in 30 seconds..."
            sleep 30
            check_result=0
            verify_resource_types_available || check_result=$?
        fi

        if [[ ${check_result} -eq 0 ]]; then
            save_skip_resources_list
        elif [[ ${check_result} -eq 1 ]]; then
            echo ""
            echo "Resource types missing despite matching versions. Reinstalling Radius..."
            if ! rad uninstall kubernetes --purge --yes; then
                echo "Warning: Uninstall failed, continuing with install attempt..."
            fi
            install_radius
        else
            echo ""
            echo "ERROR: Unable to verify resource types after retry."
            echo "This may indicate a connectivity or authentication issue."
            exit 1
        fi
    else
        echo ""
        echo "Version mismatch detected. Attempting upgrade from ${cp_version} to ${cli_version}..."
        # There are scenarios when an upgrade may not be possible, and we are relying on the rad upgrade command to
        # detect and report an error, which will cause the workflow to fail. Manual intervention may be required in such cases.
        # NOTE: Helm upgrades do not automatically reuse values from the previous release.
        # We must re-apply critical chart values or they will reset to chart defaults.
        # - global.azureWorkloadIdentity.enabled defaults to false and is required for Azure WI auth in this workflow.
        # https://github.com/radius-project/radius/issues/11218
        if ! rad upgrade kubernetes \
            --set global.azureWorkloadIdentity.enabled=true \
            --set database.enabled=false; then
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
        save_skip_resources_list
    fi

    echo "============================================================================"
    echo "Radius Control Plane Management Complete"
    echo "============================================================================"
}

main "$@"
