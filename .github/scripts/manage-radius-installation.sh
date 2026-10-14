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
# - If same version as CLI: reconciles required chart values when needed
# - If different version: attempts rad upgrade kubernetes
#
# After each path, the script confirms that every resource provider, resource
# type, API version, location, and provider summary in the built-in manifests
# is registered in UCP. A same-version installation with missing records is
# reinstalled.
#
# Environment:
#   RADIUS_MANIFEST_DIR  Directory with the built-in provider manifests that
#                        match the CLI version (required).
#
# Requires: rad, kubectl, jq, yq (mikefarah)
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME

readonly UCP_PLANE_PATH="/apis/api.ucp.dev/v1alpha3/planes/radius/local"
readonly UCP_API_VERSION="2023-10-01-preview"
REGISTRATION_CHECK_ATTEMPTS="${REGISTRATION_CHECK_ATTEMPTS:-10}"
REGISTRATION_CHECK_INTERVAL_SECONDS="${REGISTRATION_CHECK_INTERVAL_SECONDS:-15}"

readonly REQUIRED_CHART_VALUES=(
    --set global.azureWorkloadIdentity.enabled=true
    --set global.aws.irsa.enabled=true
    --set database.enabled=false
    --set de.resources.requests.memory=256Mi
    --set de.resources.limits.memory=512Mi
)

usage() {
    echo "Usage: ${SCRIPT_NAME}"
    echo ""
    echo "Manages Radius control plane installation based on version comparison."
    echo "Requires rad, kubectl, jq, and yq in PATH, and RADIUS_MANIFEST_DIR set"
    echo "to the built-in manifest directory for the CLI version."
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

# Print one tab-separated record for each UCP resource that the initializer
# writes for the manifests in $1:
#   provider <ns>, location <ns> <name>, type <ns> <name>,
#   apiVersion <ns> <type> <version>, summary <ns>
# The initializer reads every non-SVG file and merges files by namespace. A
# later file's location wins, and "global" is the default.
list_expected_registrations() {
    local manifest_dir="$1"
    local -A locations=()
    local file namespace location type_name api_version count=0
    local type_names api_versions

    for file in "${manifest_dir}"/*; do
        [[ -f "${file}" ]] || continue
        [[ "${file,,}" == *.svg ]] && continue
        count=$((count + 1))

        namespace=$(yq -r '.namespace // ""' "${file}")
        if [[ -z "${namespace}" ]]; then
            echo "ERROR: Manifest ${file} has no namespace." >&2
            return 1
        fi

        location=$(yq -r '(.location // {}) | keys | .[0] // ""' "${file}")
        if [[ -n "${location}" ]]; then
            locations["${namespace}"]="${location}"
        elif [[ -z "${locations["${namespace}"]+set}" ]]; then
            locations["${namespace}"]="global"
        fi

        # Capture each query before the loop so that a yq failure stops the
        # enumeration. A failed process substitution does not fail the loop.
        if ! type_names=$(yq -r '(.types // {}) | keys | .[]' "${file}"); then
            echo "ERROR: Failed to read resource types from ${file}." >&2
            return 1
        fi
        # shellcheck disable=SC2016 # $t is a yq variable.
        if ! api_versions=$(yq -r '(.types // {}) | to_entries[] | .key as $t
            | (.value.apiVersions // {}) | keys | .[] | $t + "\t" + .' \
            "${file}"); then
            echo "ERROR: Failed to read API versions from ${file}." >&2
            return 1
        fi

        while IFS= read -r type_name; do
            [[ -n "${type_name}" ]] || continue
            printf 'type\t%s\t%s\n' "${namespace}" "${type_name}"
        done <<<"${type_names}"

        while IFS=$'\t' read -r type_name api_version; do
            [[ -n "${type_name}" ]] || continue
            printf 'apiVersion\t%s\t%s\t%s\n' \
                "${namespace}" "${type_name}" "${api_version}"
        done <<<"${api_versions}"
    done

    if ((count == 0)); then
        echo "ERROR: No manifests found in ${manifest_dir}." >&2
        return 1
    fi

    for namespace in "${!locations[@]}"; do
        printf 'provider\t%s\n' "${namespace}"
        printf 'location\t%s\t%s\n' "${namespace}" "${locations["${namespace}"]}"
        printf 'summary\t%s\n' "${namespace}"
    done
}

# GET a UCP resource through the Kubernetes API aggregation layer and print
# the response body.
# Returns: 0 = found, 1 = not found, 2 = query failed
get_ucp_resource() {
    local path="$1" err_file body status=0
    err_file=$(mktemp)
    body=$(kubectl get --raw \
        "${UCP_PLANE_PATH}${path}?api-version=${UCP_API_VERSION}" \
        2>"${err_file}") || status=$?

    if ((status == 0)); then
        rm -f "${err_file}"
        printf '%s' "${body}"
        return 0
    fi

    if grep -Fq "NotFound" "${err_file}"; then
        rm -f "${err_file}"
        return 1
    fi

    echo "ERROR: Failed to query UCP resource ${path}:" >&2
    cat "${err_file}" >&2
    rm -f "${err_file}"
    return 2
}

# Verify that every record from the built-in manifests is registered in UCP.
# Also verify that each provider summary lists all of its types, API versions,
# and location, and that each location maps all of its types and API versions.
# UCP routing uses the location mappings, so a provider entry alone does not
# make the built-in types usable.
# Returns: 0 = complete, 1 = records missing, 2 = query failed. A query
# failure has precedence, so an unknown state never starts a reinstall.
verify_manifest_registration_complete() {
    echo ""
    echo "Verifying built-in manifest registration in ${RADIUS_MANIFEST_DIR}..."

    local expected
    if ! expected=$(list_expected_registrations "${RADIUS_MANIFEST_DIR}"); then
        return 2
    fi

    local -A summaries=() location_bodies=()
    local -a missing=()
    local kind namespace name version path body status query_failed=0
    local rp_path

    while IFS=$'\t' read -r kind namespace name version; do
        rp_path="/providers/System.Resources/resourceproviders/${namespace}"
        case "${kind}" in
            provider) path="${rp_path}" ;;
            location) path="${rp_path}/locations/${name}" ;;
            type) path="${rp_path}/resourcetypes/${name}" ;;
            apiVersion)
                path="${rp_path}/resourcetypes/${name}/apiversions/${version}"
                ;;
            summary) path="/providers/${namespace}" ;;
            *)
                echo "ERROR: Unknown registration kind '${kind}'." >&2
                return 2
                ;;
        esac

        status=0
        body=$(get_ucp_resource "${path}") || status=$?
        if ((status == 2)); then
            query_failed=1
        elif ((status == 1)); then
            missing+=("${kind} ${path}")
        elif [[ "${kind}" == "summary" ]]; then
            summaries["${namespace}"]="${body}"
        elif [[ "${kind}" == "location" ]]; then
            location_bodies["${namespace}"]="${name}"$'\t'"${body}"
        fi
    done <<<"${expected}"

    # UCP matches the location's resource type names without regard to case.
    local location_name location_body
    while IFS=$'\t' read -r kind namespace name version; do
        [[ -n "${location_bodies["${namespace}"]+set}" ]] || continue
        location_name="${location_bodies["${namespace}"]%%$'\t'*}"
        location_body="${location_bodies["${namespace}"]#*$'\t'}"
        case "${kind}" in
            type)
                jq -e --arg t "${name}" \
                    '.properties.resourceTypes // {} | keys
                    | any(ascii_downcase == ($t | ascii_downcase))' \
                    <<<"${location_body}" >/dev/null ||
                    missing+=("location type ${namespace}/${location_name}/${name}")
                ;;
            apiVersion)
                jq -e --arg t "${name}" --arg v "${version}" \
                    '.properties.resourceTypes // {} | to_entries
                    | any((.key | ascii_downcase) == ($t | ascii_downcase)
                        and ((.value.apiVersions // {}) | has($v)))' \
                    <<<"${location_body}" >/dev/null ||
                    missing+=("location apiVersion ${namespace}/${location_name}/${name}@${version}")
                ;;
        esac
    done <<<"${expected}"

    while IFS=$'\t' read -r kind namespace name version; do
        [[ -n "${summaries["${namespace}"]+set}" ]] || continue
        case "${kind}" in
            location)
                jq -e --arg l "${name}" '.locations | has($l)' \
                    <<<"${summaries["${namespace}"]}" >/dev/null ||
                    missing+=("summary location ${namespace}/${name}")
                ;;
            type)
                jq -e --arg t "${name}" '.resourceTypes | has($t)' \
                    <<<"${summaries["${namespace}"]}" >/dev/null ||
                    missing+=("summary type ${namespace}/${name}")
                ;;
            apiVersion)
                jq -e --arg t "${name}" --arg v "${version}" \
                    '.resourceTypes[$t].apiVersions // {} | has($v)' \
                    <<<"${summaries["${namespace}"]}" >/dev/null ||
                    missing+=("summary apiVersion ${namespace}/${name}@${version}")
                ;;
        esac
    done <<<"${expected}"

    if ((${#missing[@]} > 0)); then
        echo "ERROR: ${#missing[@]} built-in registration records are missing:"
        printf '  %s\n' "${missing[@]}"
    fi

    if ((query_failed == 1)); then
        echo "ERROR: One or more UCP queries failed. Registration state is unknown."
        return 2
    fi

    if ((${#missing[@]} > 0)); then
        return 1
    fi

    echo "All built-in resource providers, types, API versions, and locations are registered."
    return 0
}

# Poll until the built-in manifests are fully registered. Use this after an
# install or upgrade because the UCP initializer runs after the pod starts.
wait_for_manifest_registration() {
    local attempt
    for ((attempt = 1; attempt <= REGISTRATION_CHECK_ATTEMPTS; attempt++)); do
        if verify_manifest_registration_complete; then
            return 0
        fi
        if ((attempt < REGISTRATION_CHECK_ATTEMPTS)); then
            echo "Registration incomplete (check ${attempt}/${REGISTRATION_CHECK_ATTEMPTS})." \
                "Retrying in ${REGISTRATION_CHECK_INTERVAL_SECONDS} seconds..."
            sleep "${REGISTRATION_CHECK_INTERVAL_SECONDS}"
        fi
    done

    echo "ERROR: Built-in resource types are not fully registered after" \
        "${REGISTRATION_CHECK_ATTEMPTS} checks."
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

aws_irsa_token_volumes_enabled() {
    local -a deployments=(ucp applications-rp dynamic-rp)
    local deployment volume_name

    for deployment in "${deployments[@]}"; do
        if ! volume_name=$(
            kubectl get deployment "${deployment}" -n radius-system \
                -o jsonpath='{.spec.template.spec.volumes[?(@.name=="aws-iam-token")].name}'
        ); then
            echo "Error: Failed to inspect deployment ${deployment} for AWS IRSA configuration." >&2
            return 2
        fi

        if [[ "${volume_name}" != "aws-iam-token" ]]; then
            return 1
        fi
    done

    return 0
}

upgrade_radius() {
    rad upgrade kubernetes "${REQUIRED_CHART_VALUES[@]}" "$@"
}

reconcile_required_chart_values() {
    local irsa_status=0
    aws_irsa_token_volumes_enabled || irsa_status=$?

    if [[ ${irsa_status} -eq 0 ]]; then
        echo "Required Radius chart values are already configured."
        return 0
    fi

    if [[ ${irsa_status} -ne 1 ]]; then
        return 1
    fi

    echo "AWS IRSA is not enabled on the current Radius installation. Reconciling chart values..."
    # The upgrade preflight rejects a same-version target. We already established
    # that this is an installed, matching-version control plane, so skip preflight
    # for this one-time Helm value reconciliation.
    if ! upgrade_radius --skip-preflight; then
        echo "Error: Failed to reconcile required Radius chart values." >&2
        return 1
    fi

    if ! aws_irsa_token_volumes_enabled; then
        echo "Error: AWS IRSA token volumes are still missing after reconciliation." >&2
        return 1
    fi

    echo "Required Radius chart values reconciled successfully."
}

# Install Radius on the cluster
install_radius() {
    echo "Installing Radius..."
    if ! rad install kubernetes "${REQUIRED_CHART_VALUES[@]}"; then
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

    if ! wait_for_manifest_registration; then
        exit 1
    fi

    save_skip_resources_list
}

require_prerequisites() {
    local cmd
    for cmd in rad kubectl jq yq; do
        if ! command -v "${cmd}" &>/dev/null; then
            echo "Error: ${cmd} not found in PATH" >&2
            exit 1
        fi
    done

    if [[ -z "${RADIUS_MANIFEST_DIR:-}" || ! -d "${RADIUS_MANIFEST_DIR}" ]]; then
        echo "Error: RADIUS_MANIFEST_DIR must name the built-in manifest" \
            "directory for the CLI version (got '${RADIUS_MANIFEST_DIR:-}')." >&2
        exit 1
    fi
}

main() {
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        usage
    fi

    require_prerequisites

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
        echo "Radius control plane version matches CLI version (${cli_version})."

        if ! reconcile_required_chart_values; then
            exit 1
        fi

        # Verify the full built-in registration. Retry once for both outcomes:
        # a query failure can be a transient connectivity issue, and missing
        # records can be an initializer that is still running after a restart.
        # Only reinstall when the second check also finds missing records.
        local check_result=0
        verify_manifest_registration_complete || check_result=$?

        if [[ ${check_result} -ne 0 ]]; then
            echo ""
            echo "Registration check did not pass. Retrying in 30 seconds..."
            sleep 30
            check_result=0
            verify_manifest_registration_complete || check_result=$?
        fi

        if [[ ${check_result} -eq 0 ]]; then
            save_skip_resources_list
        elif [[ ${check_result} -eq 1 ]]; then
            echo ""
            echo "Built-in registration is incomplete despite matching versions. Reinstalling Radius..."
            if ! rad uninstall kubernetes --purge --yes; then
                echo "Warning: Uninstall failed, continuing with install attempt..."
            fi
            install_radius
        else
            echo ""
            echo "ERROR: Unable to verify built-in registration after retry."
            echo "This may indicate a connectivity or authentication issue."
            exit 1
        fi
    else
        echo ""
        echo "Version mismatch detected. Attempting upgrade from ${cp_version} to ${cli_version}..."
        # There are scenarios when an upgrade may not be possible, and we are relying on the rad upgrade command to
        # detect and report an error, which will cause the workflow to fail. Manual intervention may be required in such cases.
        # Radius preserves stored Helm overrides. Explicit values ensure fresh
        # installs and upgrades use the same required LRT configuration.
        if ! upgrade_radius; then
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
        if ! wait_for_manifest_registration; then
            exit 1
        fi
        save_skip_resources_list
    fi

    echo "============================================================================"
    echo "Radius Control Plane Management Complete"
    echo "============================================================================"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
