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

# Delete all test resources in queuemessages
kubectl delete queuemessages.ucp.dev -n radius-system --all

# Delete all test resources in resources without proxy resource.
resources=$(kubectl get resources.ucp.dev -n radius-system --no-headers -o custom-columns=":metadata.name")
for r in $resources
do
    if [[ $r == scope.local.* || $r == scope.aws.* || -z "$r" ]]; then
        echo "skip deletion: $r"
    else
        echo "delete resource: $r"
        kubectl delete resources.ucp.dev $r -n radius-system --ignore-not-found=true
    fi
done

# Delete all test namespaces.
namespaces=$(kubectl get namespace | grep -E '^corerp.*|^default-.*|^radiusfunctionaltestbucket.*|^radius-test.*|^kubernetes-cli.*|^dpsb-.*|^azstorage-workload.*|^dapr-serviceinvocation|^ms.+' | awk '{print $1}')
for ns in $namespaces
do
    if [ -z "$ns" ]; then
        break
    fi
    echo "deleting namespaces: $ns"
    kubectl delete namespace $ns --ignore-not-found=true
done
