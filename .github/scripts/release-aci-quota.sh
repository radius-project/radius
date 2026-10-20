#!/bin/bash

set -euo pipefail

readonly ACI_API_VERSION="2024-11-01-preview"

usage() {
    echo "Usage: $0 <subscription-id> <resource-group>"
}

if (( $# != 2 )); then
    usage >&2
    exit 1
fi

readonly SUBSCRIPTION_ID="$1"
readonly RESOURCE_GROUP="$2"

if ! az group show \
    --subscription "${SUBSCRIPTION_ID}" \
    --name "${RESOURCE_GROUP}" \
    --output none; then
    echo "Warning: resource group ${RESOURCE_GROUP} is unavailable; skipping ACI cleanup." >&2
    exit 0
fi

if ! aci_rows=$(az resource list \
    --subscription "${SUBSCRIPTION_ID}" \
    --resource-group "${RESOURCE_GROUP}" \
    --query "[?starts_with(type, 'Microsoft.ContainerInstance/')].[id,type]" \
    --output tsv); then
    echo "Warning: failed to list Azure Container Instances resources in ${RESOURCE_GROUP}." >&2
    exit 0
fi

if [[ -z "${aci_rows}" ]]; then
    echo "No Azure Container Instances resources in ${RESOURCE_GROUP}."
    exit 0
fi

declare -a ngroup_ids=()
declare -a remaining_aci_ids=()

while IFS=$'\t' read -r id type; do
    [[ -z "${id}" ]] && continue

    if [[ "${type,,}" == "microsoft.containerinstance/ngroups" ]]; then
        ngroup_ids+=("${id}")
    else
        remaining_aci_ids+=("${id}")
    fi
done <<< "${aci_rows}"

delete_resources() {
    local resource_kind="$1"
    shift

    if (( $# == 0 )); then
        return
    fi

    echo "Deleting ${resource_kind} from ${RESOURCE_GROUP}:"
    printf '  %s\n' "$@"

    local id
    for id in "$@"; do
        (
            if ! az resource delete \
                --subscription "${SUBSCRIPTION_ID}" \
                --ids "${id}" \
                --api-version "${ACI_API_VERSION}" \
                --verbose; then
                echo "Warning: failed to delete ${id}." >&2
            fi
        ) &
    done
    wait
}

# nGroups hold the container scale-set quota and reference the profiles, so the
# two resource classes must be deleted in this order.
delete_resources "ACI nGroups" "${ngroup_ids[@]}"
delete_resources "remaining ACI resources" "${remaining_aci_ids[@]}"
