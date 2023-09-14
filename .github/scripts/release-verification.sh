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
# (e.g. 0.24.0, 0.24.0-rc1)
RELEASE_VERSION_NUMBER=$1

if [[ -z "${RELEASE_VERSION_NUMBER}" ]]; then
    echo "Error: RELEASE_VERSION_NUMBER is not set."
    exit 1
fi

# EXPECTED_CLI_VERSION is the same as the RELEASE_VERSION_NUMBER
EXPECTED_CLI_VERSION=$RELEASE_VERSION_NUMBER

EXPECTED_TAG_VERSION=$RELEASE_VERSION_NUMBER
# if RELEASE_VERSION_NUMBER contains -rc, then it is a prerelease.
# In that case, we need to set expected tag version to the major.minor of the 
# release version number
if [[ $RELEASE_VERSION_NUMBER != *"rc"* ]]; then
    EXPECTED_TAG_VERSION=$(echo $RELEASE_VERSION_NUMBER | cut -d '.' -f 1,2)
fi

echo "RELEASE_VERSION_NUMBER: ${RELEASE_VERSION_NUMBER}"
echo "EXPECTED_CLI_VERSION: ${EXPECTED_CLI_VERSION}"
echo "EXPECTED_TAG_VERSION: ${EXPECTED_TAG_VERSION}"

curl https://get.radapp.dev/tools/rad/$EXPECTED_TAG_VERSION/linux-x64/rad --output rad
chmod +x ./rad

RELEASE_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.release')
VERSION_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.version')

if [[ "${RELEASE_FROM_RAD_VERSION}" != "${EXPECTED_CLI_VERSION}" ]]; then
    echo "Error: Release: ${RELEASE_FROM_RAD_VERSION} from rad version does not match the desired release: ${EXPECTED_CLI_VERSION}."
    exit 1
fi

if [[ "${VERSION_FROM_RAD_VERSION}" != "v${EXPECTED_CLI_VERSION}" ]]; then
    echo "Error: Version: ${VERSION_FROM_RAD_VERSION} from rad version does not match the desired version: v${EXPECTED_CLI_VERSION}."
    exit 1
fi

kind create cluster
./rad install kubernetes

EXPECTED_APPCORE_RP_IMAGE="radius.azurecr.io/applications-rp:${EXPECTED_TAG_VERSION}"
EXPECTED_UCP_IMAGE="radius.azurecr.io/ucpd:${EXPECTED_TAG_VERSION}"
EXPECTED_DE_IMAGE="radius.azurecr.io/deployment-engine:${EXPECTED_TAG_VERSION}"

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
