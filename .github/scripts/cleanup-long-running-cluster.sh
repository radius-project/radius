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
readonly RESOURCE_DELETE_BATCH_SIZE=100
declare -a resource_delete_batch=()

delete_resource_batch() {
    if (( ${#resource_delete_batch[@]} == 0 )); then
        return
    fi

    echo "deleting ${#resource_delete_batch[@]} resources.ucp.dev entries"
    kubectl delete resources.ucp.dev "${resource_delete_batch[@]}" \
        -n radius-system --ignore-not-found=true --wait=false
    resource_delete_batch=()
}

queue_resource_deletion() {
    resource_delete_batch+=("$1")
    if (( ${#resource_delete_batch[@]} >= RESOURCE_DELETE_BATCH_SIZE )); then
        delete_resource_batch
    fi
}

echo "cleaning up long-running cluster on Azure"
echo "Using skip-resource-list from: $SKIP_RESOURCE_FILE"

# Delete all test resources in queuemessages.
if kubectl get crd queuemessages.ucp.dev >/dev/null 2>&1; then
    echo "delete all resources in queuemessages.ucp.dev"
    kubectl delete queuemessages.ucp.dev -n radius-system --all --wait=false
fi

# Testing deletion of deployment.apps.

# Delete all test resources in resources without proxy resource.
if kubectl get crd resources.ucp.dev >/dev/null 2>&1; then
    if [[ -n "$SKIP_RESOURCE_FILE" && -f "$SKIP_RESOURCE_FILE" ]]; then
        echo "delete resources in resources.ucp.dev except entries in skip-resource-list"
    else
        echo "no skip-resource-list available; delete only scope.* resources in resources.ucp.dev"
    fi
    resources=$(kubectl get resources.ucp.dev -n radius-system --no-headers -o custom-columns=":metadata.name")
    for r in $resources; do
        if [[ -z "$r" ]]; then
            continue
        fi

        # Always skip built-in plane scopes.
        if [[ $r == scope.local.* || $r == scope.aws.* ]]; then
            echo "skip deletion: $r"
            continue
        fi

        # If a skip-resource file is available, use it to protect system resources.
        if [ -n "$SKIP_RESOURCE_FILE" ] && [ -f "$SKIP_RESOURCE_FILE" ]; then
            if grep -F -x -q -- "$r" "$SKIP_RESOURCE_FILE"; then
                echo "skip deletion: $r (found in skip-resource-list $SKIP_RESOURCE_FILE)"
            else
                queue_resource_deletion "$r"
            fi
            continue
        fi

        # No skip-resource file: only delete scope entries (test resource groups).
        # Non-scope resources (resource.*) may include system-critical entries
        # that must not be deleted without a valid skip list.
        if [[ $r == scope.* ]]; then
            queue_resource_deletion "$r"
        else
            echo "skip deletion: $r (no skip list available, preserving non-scope resource)"
        fi
    done
    delete_resource_batch
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
for ns in $namespaces; do
    if [ -z "$ns" ]; then
        break
    fi
    # shellcheck disable=SC2076
    if [[ " ${namespace_whitelist[*]} " =~ " ${ns} " ]]; then
        echo "skip deletion: $ns"
    else
        echo "deleting namespaces: $ns"
        kubectl delete namespace "$ns" --ignore-not-found=true --wait=false
    fi
done
