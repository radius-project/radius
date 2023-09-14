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

set -ex

# RELEASE_VERSION_NUMBER is the Radius release version number
# (e.g. 0.24, 0.24.0, 0.24.0-rc1)
RELEASE_VERSION_NUMBER=$1

if [[ -z "${RELEASE_VERSION_NUMBER}" ]]; then
    echo "Error: RELEASE_VERSION_NUMBER is not set."
    exit 1
fi

echo "RELEASE_VERSION_NUMBER: ${RELEASE_VERSION_NUMBER}"

curl https://get.radapp.dev/tools/rad/$RELEASE_VERSION_NUMBER/linux-x64/rad --output rad
chmod +x ./rad

RELEASE_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.release')
VERSION_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.version')

if [[ "${RELEASE_FROM_RAD_VERSION}" != "${RELEASE_VERSION_NUMBER}" ]]; then
    echo "Error: Release: ${RELEASE_FROM_RAD_VERSION} from rad version does not match the desired release: ${RELEASE_VERSION_NUMBER}."
    exit 1
fi

if [[ "${VERSION_FROM_RAD_VERSION}" != "v${RELEASE_VERSION_NUMBER}" ]]; then
    echo "Error: Version: ${VERSION_FROM_RAD_VERSION} from rad version does not match the desired version: v${RELEASE_VERSION_NUMBER}."
    exit 1
fi

kind create cluster
./rad install kubernetes

EXPECTED_APPCORE_RP_IMAGE="radius.azurecr.io/applications-rp:${RELEASE_VERSION_NUMBER}"
EXPECTED_UCP_IMAGE="radius.azurecr.io/ucpd:${RELEASE_VERSION_NUMBER}"
EXPECTED_DE_IMAGE="radius.azurecr.io/deployment-engine:${RELEASE_VERSION_NUMBER}"

APPCORE_RP_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=applications-rp | awk '/^.*Image:/ {print $2}')
UCP_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=ucp | awk '/^.*Image:/ {print $2}')
DE_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=bicep-de | awk '/^.*Image:/ {print $2}')

if [[ "${APPCORE_RP_IMAGE}" != "${EXPECTED_APPCORE_RP_IMAGE}" ]]; then
    echo "Error: Applications RP image: ${APPCORE_RP_IMAGE} does not match the desired image: ${EXPECTED_APPCORE_RP_IMAGE}."
    exit 1
fi

if [[ "${UCP_IMAGE}" != "${EXPECTED_UCP_IMAGE}" ]]; then
    echo "Error: UCP image: ${UCP_IMAGE} does not match the desired image: ${EXPECTED_UCP_IMAGE}."
    exit 1
fi

if [[ "${DE_IMAGE}" != "${EXPECTED_DE_IMAGE}" ]]; then
    echo "Error: DE image: ${DE_IMAGE} does not match the desired image: ${EXPECTED_DE_IMAGE}."
    exit 1
fi

echo "Release verification successful."
