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

RELEASE=$1
PLATFORM=$2

# If version is not provided, exit
if [[ -z "$RELEASE" ]]; then
    echo "Error: No release version provided. Please provide a valid semver (e.g. 0.1.0 or 0.1.0-rc1)."
    exit 1
fi

# If platform is not provided, exit
if [[ -z "$PLATFORM" ]]; then
    echo "Error: No platform provided. Please provide a valid platform (e.g. macos-x64 or linux-x64)."
    exit 1
fi

VERSION="v${RELEASE}"

curl https://get.radapp.dev/tools/rad/$RELEASE/$PLATFORM/rad --output rad
chmod +x ./rad

RELEASE_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.release')
VERSION_FROM_RAD_VERSION=$(./rad version -o json | jq -r '.version')

if [[ "${RELEASE_FROM_RAD_VERSION}" != "${RELEASE}" ]]; then
    echo "Error: Release version: ${RELEASE_FROM_RAD_VERSION} from rad version does not match the desired release version: ${RELEASE}."
    exit 1
fi

if [[ "${VERSION_FROM_RAD_VERSION}" != "${VERSION}" ]]; then
    echo "Error: Version: ${VERSION_FROM_RAD_VERSION} from rad version does not match the desired version: ${VERSION}."
    exit 1
fi

kind create cluster
rad install kubernetes

EXPECTED_APPCORE_RP_IMAGE="radius.azurecr.io/appcore-rp:${RELEASE}"
APPCORE_RP_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=appcore-rp | awk '/^.*Image:/ {print $2}')
UCP_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=ucp | awk '/^.*Image:/ {print $2}')
EXPECTED_UCP_IMAGE="radius.azurecr.io/ucpd:${RELEASE}"
DE_IMAGE=$(kubectl describe pods -n radius-system -l control-plane=bicep-de | awk '/^.*Image:/ {print $2}')
EXPECTED_DE_IMAGE="radius.azurecr.io/deployment-engine:${RELEASE}"

if [[ "${APPCORE_RP_IMAGE}" != "${EXPECTED_APPCORE_RP_IMAGE}" ]]; then
    echo "Error: Appcore RP image: ${APPCORE_RP_IMAGE} does not match the desired image: ${EXPECTED_APPCORE_RP_IMAGE}."
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
