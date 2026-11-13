#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
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

# PROTOTYPE: applies an empty DeploymentTemplate in NAMESPACE that targets
# RESOURCE_GROUP, so you can see the controller's namespace mapping check.
# Usage: deployment-template.sh NAMESPACE RESOURCE_GROUP

set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 NAMESPACE RESOURCE_GROUP" >&2
    exit 2
fi

namespace="$1"
group="$2"
context="kind-${AUTHZ_CLUSTER_NAME:-radius-authz}"
name="dt-$(echo "${group}" | tr '[:upper:]' '[:lower:]')"
scope="/planes/radius/local/resourceGroups/${group}"

kubectl --context "${context}" create namespace "${namespace}" --dry-run=client -o yaml |
    kubectl --context "${context}" apply -f - >/dev/null

kubectl --context "${context}" apply -f - <<YAML
apiVersion: radapp.io/v1alpha3
kind: DeploymentTemplate
metadata:
  name: ${name}
  namespace: ${namespace}
spec:
  repository: prototype
  template: '{"\$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#","contentVersion":"1.0.0.0","resources":{}}'
  parameters: {}
  providerConfig: '{"radius":{"type":"radius","value":{"scope":"${scope}"}},"deployments":{"type":"Microsoft.Resources","value":{"scope":"${scope}"}}}'
YAML
