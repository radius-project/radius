#!/bin/bash

kubectl delete queuemessages.ucp.dev -n radius-system --all

resources=$(kubectl get resources.ucp.dev -n radius-system --no-headers -o custom-columns=":metadata.name")
for r in $resources
do
    if [[ $r == scope.local.* || $r == scope.aws.* ]]; then
        echo "skip deletion: $r"
    else
        echo "delete resource: $r"
        kubectl delete resources.ucp.dev $r -n radius-system
    fi
done

kubectl delete namespace $(kubectl get namespace | grep -E '^corerp.*|^default-.*|^radiusfunctionaltestbucket.*|^kubernetes-cli.*|^dpsb-.*|^azstorage-workload.*|^dapr-serviceinvocation|^ms.+' | awk '{print $1}')