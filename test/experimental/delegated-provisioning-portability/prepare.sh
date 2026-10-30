#!/bin/bash
# Prepare separate portability fixtures; preserve the original Kubernetes demo.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
readonly ROOT
readonly RUN=/home/brhamilt/delegated-provisioning-deebec8c-run
export PATH=/home/brhamilt/.local/bin:/usr/local/bin:/usr/bin:/bin
export HOME="${RUN}/home"
export KUBECONFIG="${RUN}/kubeconfig"
mkdir -p "${RUN}/portability/downloads"
bicep build "${ROOT}/test/experimental/delegated-provisioning-portability/app.bicep" \
    --outfile "${RUN}/portability/app.json"
bicep build "${ROOT}/test/experimental/delegated-provisioning-portability/aca-bicep/container-app.bicep" \
    --outfile "${RUN}/portability/aca-bicep-template.json"
cd "${RUN}/portability/downloads"
curl -fsSL --retry 3 \
    https://releases.hashicorp.com/terraform-provider-azurerm/4.50.0/terraform-provider-azurerm_4.50.0_linux_amd64.zip \
    -o azurerm.zip
curl -fsSL --retry 3 \
    https://releases.hashicorp.com/terraform-provider-azurerm/4.50.0/terraform-provider-azurerm_4.50.0_SHA256SUMS \
    -o SHA256SUMS
expected="$(grep ' terraform-provider-azurerm_4.50.0_linux_amd64.zip$' \
    SHA256SUMS | cut -d ' ' -f 1)"
printf '%s  azurerm.zip\n' "${expected}" | sha256sum -c -
mkdir -p "${RUN}/portability/providers/registry.terraform.io/hashicorp/azurerm/4.50.0/linux_amd64"
python3 -m zipfile -e azurerm.zip \
    "${RUN}/portability/providers/registry.terraform.io/hashicorp/azurerm/4.50.0/linux_amd64"
find "${RUN}/portability/providers" -name 'terraform-provider-*' \
    -type f -exec chmod a+rx {} \;
docker image inspect python:3.12-alpine >/dev/null 2>&1 ||
    docker pull python:3.12-alpine
docker save --platform linux/amd64 python:3.12-alpine | docker exec -i \
    delegated-deebec8c-control-plane ctr -n k8s.io images import \
    --platform linux/amd64 -
