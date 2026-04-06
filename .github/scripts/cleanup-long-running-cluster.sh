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

SKIP_RESOURCE_FILE="${1:-}"
echo "cleaning up long-running cluster on Azure"
echo "Using skip-resource-list from: ${SKIP_RESOURCE_FILE}"

# Delete all test resources in queuemessages.
if kubectl get crd queuemessages.ucp.dev >/dev/null 2>&1; then
    echo "delete all resources in queuemessages.ucp.dev"
    kubectl delete queuemessages.ucp.dev -n radius-system --all --wait=false
fi

# Delete test resources in resources.ucp.dev while preserving resource provider
# infrastructure. Resource providers, resource types, API versions, and locations
# are essential for Radius to function (e.g., Applications.Core/environments).
# Deleting them causes "resource type not found" errors that break subsequent runs.
if kubectl get crd resources.ucp.dev >/dev/null 2>&1; then
    echo "delete test resources in resources.ucp.dev"

    # Build a list of resource provider infrastructure entries that must always be
    # preserved, regardless of whether the skip-delete-resources-list.txt exists.
    # These entries are identified by their ucp.dev/resource-type label.
    RP_INFRA_FILE="$(mktemp)"
    if ! kubectl get resources.ucp.dev -n radius-system --no-headers \
        -o custom-columns=":metadata.name" \
        -l "ucp.dev/resource-type in (system.resources_resourceproviders,system.resources_resourceproviders_resourcetypes,system.resources_resourceproviders_resourcetypes_apiversions,system.resources_resourceproviders_locations)" \
        > "${RP_INFRA_FILE}"; then
        echo "failed to build resource provider preserve list; aborting resources.ucp.dev cleanup" >&2
        rm -f "${RP_INFRA_FILE}"
        exit 1
    fi
    rp_count=$(wc -l < "${RP_INFRA_FILE}" | tr -d ' ')
    echo "found ${rp_count} resource provider infrastructure entries to preserve"

    resources=$(kubectl get resources.ucp.dev -n radius-system --no-headers -o custom-columns=":metadata.name")
    for r in ${resources}; do
        if [[ -z "${r}" ]]; then
            continue
        fi

        # Skip all scope entries (planes, resource groups, etc.)
        if [[ ${r} == scope.* ]]; then
            echo "skip deletion: ${r} (scope entry)"
            continue
        fi

        # Skip resource provider infrastructure entries (resource types, API versions, locations).
        # This is the primary safeguard that protects resource types even when the
        # skip-delete-resources-list.txt is missing due to a cache miss.
        if grep -qFx "${r}" "${RP_INFRA_FILE}" 2>/dev/null; then
            echo "skip deletion: ${r} (resource provider infrastructure)"
            continue
        fi

        # Skip resources listed in skip resource file
        if [[ -n "${SKIP_RESOURCE_FILE}" ]] && [[ -f "${SKIP_RESOURCE_FILE}" ]] && grep -qFx "${r}" "${SKIP_RESOURCE_FILE}"; then
            echo "skip deletion: ${r} (found in skip-resource-list ${SKIP_RESOURCE_FILE})"
            continue
        fi

        echo "deleting resource: ${r}"
        kubectl delete resources.ucp.dev "${r}" -n radius-system --ignore-not-found=true --wait=false
    done

    rm -f "${RP_INFRA_FILE}"
fi

# Delete all test namespaces.
echo "delete all test namespaces"
namespace_whitelist=(
    "aks-command"
    "azure-monitor"
    "azure-policy"
    "azure-workload-identity-system"
    "cert-manager"
    "cluster-autoscaler"
    "dapr-system"
    "default"
    "gatekeeper-system"
    "ingress-nginx"
    "istio-system"
    "keda"
    "kube-node-lease"
    "kube-public"
    "kube-system"
    "nginx-ingress"
    "radius-system"
    "secrets-store-csi-driver"
)
namespaces=$(kubectl get namespaces --no-headers -o custom-columns=":metadata.name")
for ns in ${namespaces}; do
    if [[ -z "${ns}" ]]; then
        break
    fi
    # shellcheck disable=SC2076
    if [[ " ${namespace_whitelist[*]} " =~ " ${ns} " ]]; then
        echo "skip deletion: ${ns}"
    else
        echo "deleting namespaces: ${ns}"
        kubectl delete namespace "${ns}" --ignore-not-found=true --wait=false
    fi
done
