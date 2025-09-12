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

# Configuration
readonly NAMESPACE="radius-system"
readonly GITHUB_ORG="radius-project"
readonly GITHUB_REPO="radius"

# Parse arguments: version [os] [arch]
RELEASE_VERSION_NUMBER="${1:-}"
OS="${2:-linux}"
ARCH="${3:-amd64}"

# Cleanup function to remove temporary files and cluster
cleanup() {
    if [[ -f "./rad" ]]; then
        echo "Deleting downloaded ./rad binary..."
        rm -f ./rad
    fi
    if kind get clusters 2>/dev/null | grep -q "kind"; then
        echo "Deleting kind cluster..."
        kind delete cluster || true
    fi
}

# Set up cleanup trap
trap cleanup EXIT ERR

# Validates prerequisites and environment
validate_prerequisites() {
    # Check for required commands
    local required_commands=("kubectl" "kind" "curl" "jq")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            echo "Error: Required command '$cmd' is not installed or not in PATH" >&2
            exit 1
        fi
    done
}

# Retrieves the base image for a given pod name prefix (without the unique identifier suffix)
# Usage: get_pod_base_image <pod_name_prefix>
get_pod_base_image() {
    local pod_prefix="$1"
    local pod_name
    pod_name=$(kubectl get pods --no-headers -n "$NAMESPACE" -o custom-columns=":metadata.name" \
        | grep "^${pod_prefix}" \
        | head -n 1)
    
    if [[ -z "$pod_name" ]]; then
        echo "Error: No pod found with prefix '$pod_prefix' in namespace '$NAMESPACE'" >&2
        return 1
    fi
    
    kubectl get pod -n "$NAMESPACE" "$pod_name" -o jsonpath="{.spec.containers[*].image}"
}

# Verifies that a pod's image matches the expected image
# Usage: verify_pod_image <pod_prefix> <expected_image> <component_name>
verify_pod_image() {
    local pod_prefix="$1"
    local expected_image="$2"
    local component_name="$3"
    
    local actual_image
    actual_image=$(get_pod_base_image "$pod_prefix")
    
    if [[ "$actual_image" != "$expected_image" ]]; then
        echo "Error: $component_name image: $actual_image does not match the desired image: $expected_image." >&2
        exit 1
    fi
    
    echo "$component_name image verified: $actual_image"
}

# This function verifies the status of the pre-upgrade image.
# Note: The pre-upgrade image runs as a Kubernetes Job, so the verification process differs from standard deployments.
# This verification checks the container image used in the pre-upgrade job.
verify_pre_upgrade_image() {
    local expected_image="$1"
    
    # expect error - ignore and continue. We only want to trigger the pre-upgrade container to run as a job so that we 
    # can verify the container image used in the job.
    helm upgrade radius ./deploy/Chart \
        --namespace radius-system \
        --set global.imageTag="${EXPECTED_TAG_VERSION}" \
        --set preupgrade.enabled=true \
        --set preupgrade.targetVersion="${EXPECTED_CLI_VERSION}" \
        --wait 2>/dev/null || true

    # Extract the "image" field from the pre-upgrade job
    PRE_UPGRADE_IMAGE=$(kubectl get job pre-upgrade -n radius-system -o json | jq -r '.spec.template.spec.containers[0].image')

    if [[ "$PRE_UPGRADE_IMAGE" != "$expected_image" ]]; then
        echo "Error: Pre-upgrade image: $PRE_UPGRADE_IMAGE does not match the desired image: $expected_image." >&2
        exit 1
    else
        echo "Pre-upgrade image verified: $PRE_UPGRADE_IMAGE"
    fi
}

if [[ -z "$RELEASE_VERSION_NUMBER" ]]; then
    echo "Error: RELEASE_VERSION_NUMBER is not set." >&2
    echo "Usage: $0 <version> [os] [arch]" >&2
    echo "  version: Release version (e.g., 0.24.0)" >&2
    echo "  os:      linux (default) or darwin" >&2
    echo "  arch:    amd64 (default) or arm64" >&2
    echo "Examples:" >&2
    echo "  $0 0.24.0              # Linux AMD64" >&2
    echo "  $0 0.24.0 darwin       # macOS AMD64" >&2
    echo "  $0 0.24.0 darwin arm64 # macOS ARM64" >&2
    exit 1
fi

# Validate version format
if [[ ! "$RELEASE_VERSION_NUMBER" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
    echo "Error: Invalid version format. Expected format: X.Y.Z or X.Y.Z-rcN" >&2
    exit 1
fi

validate_prerequisites

readonly RADIUS_CLI_ARTIFACT="rad_${OS}_${ARCH}"
readonly DOWNLOAD_BASE="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
readonly DOWNLOAD_URL="${DOWNLOAD_BASE}/v${RELEASE_VERSION_NUMBER}/${RADIUS_CLI_ARTIFACT}"

# EXPECTED_CLI_VERSION is the same as the RELEASE_VERSION_NUMBER
EXPECTED_CLI_VERSION=$RELEASE_VERSION_NUMBER

EXPECTED_TAG_VERSION="$RELEASE_VERSION_NUMBER"
# if RELEASE_VERSION_NUMBER contains -rc, then it is a prerelease.
# In that case, we need to set expected tag version to the major.minor of the
# release version number
if [[ "$RELEASE_VERSION_NUMBER" != *"rc"* ]]; then
    EXPECTED_TAG_VERSION=$(echo "$RELEASE_VERSION_NUMBER" | cut -d '.' -f 1,2)
fi

echo "RELEASE_VERSION_NUMBER: ${RELEASE_VERSION_NUMBER}"
echo "OS: ${OS}"
echo "ARCH: ${ARCH}"
echo "EXPECTED_CLI_VERSION: ${EXPECTED_CLI_VERSION}"
echo "EXPECTED_TAG_VERSION: ${EXPECTED_TAG_VERSION}"

echo "Downloading ${DOWNLOAD_URL}"
if ! curl -sSL "${DOWNLOAD_URL}" -o rad; then
    echo "Error: Failed to download rad CLI from ${DOWNLOAD_URL}" >&2
    exit 1
fi
chmod +x ./rad

RAD_VERSION_JSON=$(./rad version --cli -o json)
echo "rad version output: $RAD_VERSION_JSON"

RELEASE_FROM_RAD_VERSION=$(echo "$RAD_VERSION_JSON" | jq -r '.release')
VERSION_FROM_RAD_VERSION=$(echo "$RAD_VERSION_JSON" | jq -r '.version')

if [[ "${RELEASE_FROM_RAD_VERSION}" != "${EXPECTED_CLI_VERSION}" ]]; then
    echo "Error: Release: ${RELEASE_FROM_RAD_VERSION} from rad version does not match the desired release: ${EXPECTED_CLI_VERSION}." >&2
    exit 1
fi

if [[ "${VERSION_FROM_RAD_VERSION}" != "v${EXPECTED_CLI_VERSION}" ]]; then
    echo "Error: Version: ${VERSION_FROM_RAD_VERSION} from rad version does not match the desired version: v${EXPECTED_CLI_VERSION}." >&2
    exit 1
fi

echo "Creating kind cluster..."
if ! kind create cluster; then
    echo "Error: Failed to create kind cluster" >&2
    exit 1
fi

echo "Installing Radius..."
if ! ./rad install kubernetes --skip-contour-install; then
    echo "Error: Failed to install Radius" >&2
    exit 1
fi

EXPECTED_APPCORE_RP_IMAGE="ghcr.io/radius-project/applications-rp:${EXPECTED_TAG_VERSION}"
EXPECTED_DE_IMAGE="ghcr.io/radius-project/deployment-engine:${EXPECTED_TAG_VERSION}"
EXPECTED_CONTROLLER_IMAGE="ghcr.io/radius-project/controller:${EXPECTED_TAG_VERSION}"
EXPECTED_DASHBOARD_IMAGE="ghcr.io/radius-project/dashboard:${EXPECTED_TAG_VERSION}"
EXPECTED_DYNAMIC_RP_IMAGE="ghcr.io/radius-project/dynamic-rp:${EXPECTED_TAG_VERSION}"
EXPECTED_UCP_IMAGE="ghcr.io/radius-project/ucpd:${EXPECTED_TAG_VERSION}"
EXPECTED_PRE_UPGRADE_IMAGE="ghcr.io/radius-project/pre-upgrade:${EXPECTED_TAG_VERSION}"

# Verify all pod images
echo "Verifying pod images..."
verify_pod_image "applications-rp" "$EXPECTED_APPCORE_RP_IMAGE" "Applications RP"
verify_pod_image "bicep-de" "$EXPECTED_DE_IMAGE" "Deployment Engine"
verify_pod_image "controller" "$EXPECTED_CONTROLLER_IMAGE" "Controller"
verify_pod_image "dashboard" "$EXPECTED_DASHBOARD_IMAGE" "Dashboard"
verify_pod_image "dynamic-rp" "$EXPECTED_DYNAMIC_RP_IMAGE" "Dynamic RP"
verify_pod_image "ucp" "$EXPECTED_UCP_IMAGE" "UCP"
verify_pre_upgrade_image "$EXPECTED_PRE_UPGRADE_IMAGE"

echo "============================================================================"
echo "Release verification successful."
echo "============================================================================"
