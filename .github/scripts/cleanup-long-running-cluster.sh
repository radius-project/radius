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

set -e

SKIP_RESOURCE_FILE="${1:-}"
echo "cleaning up long-running cluster on Azure"
echo "Using skip-resource-list from: $SKIP_RESOURCE_FILE"

# Delete all test resources in queuemessages.
if kubectl get crd queuemessages.ucp.dev >/dev/null 2>&1; then
    echo "delete all resources in queuemessages.ucp.dev"
    kubectl delete queuemessages.ucp.dev -n radius-system --all
fi

# Testing deletion of deployment.apps.

# Delete all test resources in resources without proxy resource.
if kubectl get crd resources.ucp.dev >/dev/null 2>&1; then
    echo "delete all resources in resources.ucp.dev"
    resources=$(kubectl get resources.ucp.dev -n radius-system --no-headers -o custom-columns=":metadata.name")
    for r in $resources; do
        # Skip resources if they're either scope.* or listed in skip resource file
        if [[ $r == scope.local.* || $r == scope.aws.* || -z "$r" ]]; then
            echo "skip deletion: $r"
        elif [ -n "$SKIP_RESOURCE_FILE" ] && [ -f "$SKIP_RESOURCE_FILE" ] && grep -q "$r" "$SKIP_RESOURCE_FILE"; then
            echo "Skip deletion: $r (found in skip-resource-list $SKIP_RESOURCE_FILE)"    
        else
            echo "delete resource: $r"
            kubectl delete resources.ucp.dev $r -n radius-system --ignore-not-found=true
        fi
    done
fi

# Delete all test namespaces.
echo "delete all test namespaces"
namespaces=$(kubectl get namespace |
    grep -E '^kubernetes-interop-tutorial.*|^corerp.*|^test.*|^default-.*|^radiusfunctionaltestbucket.*|^radius-test.*|^kubernetes-cli.*|^dpsb-.*|^dsrp-.*|^azstorage-workload.*|^dapr-serviceinvocation|^daprrp-rs-.*|^dapr-sts-.*|^mynamespace.*|^demo.*|^tutorial-demo.*|^ms.+' |
    awk '{print $1}')
for ns in $namespaces; do
    if [ -z "$ns" ]; then
        break
    fi
    echo "deleting namespaces: $ns"
    kubectl delete namespace $ns --ignore-not-found=true
done
