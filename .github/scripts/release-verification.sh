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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=.github/scripts/release-version.sh
source "${SCRIPT_DIR}/release-version.sh"

# Configuration
readonly NAMESPACE="radius-system"
readonly GITHUB_ORG="radius-project"
readonly GITHUB_REPO="radius"

# Parse arguments: version [os] [arch]
RELEASE_VERSION_NUMBER="${1:-}"
OS="${2:-linux}"
ARCH="${3:-amd64}"
CLI_PATH="${RELEASE_VERIFY_CLI:-}"
CHART_PATH="${RELEASE_VERIFY_CHART:-./deploy/Chart}"
MANIFEST_FILE="${RELEASE_VERIFY_MANIFEST:-}"
TEMP_DIR=""
CLUSTER_NAME="radius-verification-$$"
CLUSTER_CREATED=false
INSTALL_ARGS=(install kubernetes --skip-contour-install)

# Cleanup function to remove temporary files and cluster
cleanup() {
    if [[ "${CLUSTER_CREATED}" == "true" ]]; then
        kind delete cluster --name "${CLUSTER_NAME}" || true
    fi
    if [[ -n "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}

# Set up cleanup trap
trap cleanup EXIT

# Validates prerequisites and environment
validate_prerequisites() {
    # Check for required commands
    local required_commands=("kubectl" "kind" "curl" "jq" "helm")
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
    pod_name=$(kubectl get pods --no-headers -n "$NAMESPACE" -o custom-columns=":metadata.name" |
        grep "^${pod_prefix}" |
        head -n 1)

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

    helm upgrade radius "${CHART_PATH}" \
        --namespace radius-system \
        --reuse-values \
        --set preupgrade.enabled=true \
        --set preupgrade.checks.version=false \
        --set preupgrade.targetVersion="${EXPECTED_CLI_VERSION}" \
        --wait --timeout 5m

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

# Validate version format. Historical rcN releases remain verifiable.
if ! is_radius_release_version "${RELEASE_VERSION_NUMBER}"; then
    echo "Error: Invalid version format. Expected format: X.Y.Z or X.Y.Z-rc.N" >&2
    exit 1
fi

validate_prerequisites
TEMP_DIR="$(mktemp -d)"
export KUBECONFIG="${TEMP_DIR}/kubeconfig"

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

if [[ -z "${CLI_PATH}" ]]; then
    CLI_PATH="${TEMP_DIR}/rad"
    curl --fail --silent --show-error --location --retry 5 \
        --max-time 120 "${DOWNLOAD_URL}" -o "${CLI_PATH}"
elif [[ -z "${MANIFEST_FILE}" || ! -f "${CHART_PATH}" ]]; then
    echo "Staged installation requires a manifest and downloaded chart." >&2
    exit 1
fi
if [[ -n "${MANIFEST_FILE}" ]]; then
    jq '.checks.installation = "pending"' "${MANIFEST_FILE}" \
        > "${TEMP_DIR}/pending.json"
    mv "${TEMP_DIR}/pending.json" "${MANIFEST_FILE}"
    binary_digest="$(sha256sum "${CLI_PATH}" | cut -d ' ' -f 1)"
    jq -e --arg tag "v${RELEASE_VERSION_NUMBER}" \
        --arg name "${RADIUS_CLI_ARTIFACT}" --arg digest "${binary_digest}" '
        .tag == $tag and
        ([.checks | to_entries[] | select(.key != "installation") |
          .value] | all(. == "verified")) and
        any(.observed.cli.assets[]; .name == $name and .sha256 == $digest)
    ' "${MANIFEST_FILE}" > /dev/null
    chart_digest="sha256:$(sha256sum "${CHART_PATH}" | cut -d ' ' -f 1)"
    jq -e --arg digest "${chart_digest}" '
        [.observed.helm.manifest.layers[] |
         select(.mediaType == "application/vnd.cncf.helm.chart.content.v1.tar+gzip") |
         .digest] == [$digest]
    ' "${MANIFEST_FILE}" > /dev/null
    INSTALL_ARGS+=(--chart "${CHART_PATH}")
    for component in rp:applications-rp controller:controller ucp:ucpd \
        dynamicrp:dynamic-rp de:deployment-engine dashboard:dashboard \
        bicep:bicep preupgrade:pre-upgrade; do
        image="$(jq -er --arg name "${component#*:}" '
            .observed.images[] | select(.name == $name) |
            .reference + "@" + .digest
        ' "${MANIFEST_FILE}")"
        INSTALL_ARGS+=(--set "${component%%:*}.image=${image}")
    done
fi
chmod +x "${CLI_PATH}"

RAD_VERSION_JSON=$("${CLI_PATH}" version --cli -o json)
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
CLUSTER_CREATED=true
if ! kind create cluster --name "${CLUSTER_NAME}" \
    --kubeconfig "${KUBECONFIG}" --wait 120s; then
    echo "Error: Failed to create kind cluster" >&2
    exit 1
fi

echo "Installing Radius..."
if ! "${CLI_PATH}" "${INSTALL_ARGS[@]}"; then
    echo "Error: Failed to install Radius" >&2
    exit 1
fi
kubectl wait --for=condition=Available deployment --all \
    --namespace "${NAMESPACE}" --timeout=300s

expected_image() {
    if [[ -n "${MANIFEST_FILE}" ]]; then
        jq -er --arg name "$1" '
            .observed.images[] | select(.name == $name) |
            .reference + "@" + .digest
        ' "${MANIFEST_FILE}"
    else
        printf 'ghcr.io/radius-project/%s:%s\n' "$1" "${EXPECTED_TAG_VERSION}"
    fi
}

# Verify all pod images
echo "Verifying pod images..."
verify_pod_image "applications-rp" "$(expected_image applications-rp)" "Applications RP"
verify_pod_image "bicep-de" "$(expected_image deployment-engine)" "Deployment Engine"
verify_pod_image "controller" "$(expected_image controller)" "Controller"
verify_pod_image "dashboard" "$(expected_image dashboard)" "Dashboard"
verify_pod_image "dynamic-rp" "$(expected_image dynamic-rp)" "Dynamic RP"
verify_pod_image "ucp" "$(expected_image ucpd)" "UCP"
verify_pre_upgrade_image "$(expected_image pre-upgrade)"
if [[ -n "${MANIFEST_FILE}" ]]; then
    kubectl get pods -n "${NAMESPACE}" -o json |
        jq -e --arg image "$(expected_image bicep)" '
            any(.items[].spec.initContainers[]?; .image == $image)
        ' > /dev/null
    jq '.checks.installation = "verified"' "${MANIFEST_FILE}" \
        > "${TEMP_DIR}/manifest.json"
    mv "${TEMP_DIR}/manifest.json" "${MANIFEST_FILE}"
fi

echo "============================================================================"
echo "Release verification successful."
echo "============================================================================"
