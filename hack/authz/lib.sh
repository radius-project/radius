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

# ============================================================================
# Shared settings and helpers for the manual authorization test kit.
#
# Sourced by the scripts in hack/authz. Settings in UPPER_CASE can be
# overridden with the environment variable of the same name.
# ============================================================================

# Variables here are used by the scripts that source this file.
# shellcheck disable=SC2034

AUTHZ_KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUTHZ_REPO_ROOT="$(cd "${AUTHZ_KIT_DIR}/../.." && pwd)"

AUTHZ_CLUSTER_NAME="${AUTHZ_CLUSTER_NAME:-radius-authz}"
AUTHZ_KUBE_CONTEXT="${AUTHZ_KUBE_CONTEXT:-kind-${AUTHZ_CLUSTER_NAME}}"
AUTHZ_RADIUS_NAMESPACE="${AUTHZ_RADIUS_NAMESPACE:-radius-system}"
AUTHZ_REGISTRY_NAME="${AUTHZ_REGISTRY_NAME:-radius-authz-registry}"
# 5001 rather than 5000, which macOS uses for AirPlay Receiver.
AUTHZ_REGISTRY_PORT="${AUTHZ_REGISTRY_PORT:-5001}"
AUTHZ_ROGUE_NAMESPACE="${AUTHZ_ROGUE_NAMESPACE:-default}"

# Name of the Secret in AUTHZ_RADIUS_NAMESPACE that holds a component's mTLS
# certificate (tls.crt, tls.key, ca.crt). "%s" is replaced by the component
# name. Stack A creates these Secrets and sets the final naming here.
AUTHZ_COMPONENT_SECRET_FORMAT="${AUTHZ_COMPONENT_SECRET_FORMAT:-%s-mtls}"

# Label that up.sh sets on AUTHZ_RADIUS_NAMESPACE to record the installed mode.
AUTHZ_KIT_MODE_LABEL="authz-kit.radius.dev/mode"

# Must match metadata.name and the container names in rogue-pod.yaml.
AUTHZ_ROGUE_POD="radius-authz-rogue"
AUTHZ_ROGUE_CURL_CONTAINER="curl"
AUTHZ_ROGUE_OPENSSL_CONTAINER="openssl"
AUTHZ_ROGUE_WORKDIR="/work"

# Components that call each other inside the control plane today.
AUTHZ_KNOWN_COMPONENTS=(ucp applications-rp dynamic-rp controller bicep-de)

authz_die() {
    echo "error: $*" >&2
    exit 2
}

authz_kubectl() {
    kubectl --context "${AUTHZ_KUBE_CONTEXT}" "$@"
}

# Fails with a list of every required tool that is not on PATH.
authz_require_tools() {
    local missing=()
    local tool
    for tool in "$@"; do
        command -v "${tool}" >/dev/null || missing+=("${tool}")
    done
    if ((${#missing[@]} > 0)); then
        authz_die "missing required tools: ${missing[*]} (see hack/authz/README.md)"
    fi
}

# Prints the Helm values for an authorization mode, one key=value per line.
# Both keys are always set so a reinstall can switch modes in either direction.
authz_mode_helm_values() {
    case "${1:-}" in
        off)
            printf '%s\n' global.rbac.enabled=false global.rbac.dryRun=false
            ;;
        dryRun)
            printf '%s\n' global.rbac.enabled=false global.rbac.dryRun=true
            ;;
        enforce)
            printf '%s\n' global.rbac.enabled=true global.rbac.dryRun=false
            ;;
        *)
            echo "error: unknown authz mode '${1:-}': expected off, dryRun, or enforce" >&2
            return 2
            ;;
    esac
}

# Prints the "rad install kubernetes" --set values for a mode, registry, and
# image tag, one per line. Images not built in this repository are pinned to
# their public location, as in the radius-install-custom skill.
authz_install_set_values() {
    local mode="${1:-}"
    local registry="${2:-}"
    local tag="${3:-}"

    if [[ -z "${registry}" || -z "${tag}" ]]; then
        echo "error: registry and image tag are required" >&2
        return 2
    fi

    authz_mode_helm_values "${mode}" >/dev/null || return
    printf '%s\n' \
        "global.imageRegistry=${registry}" \
        "global.imageTag=${tag}" \
        "de.image=ghcr.io/radius-project/deployment-engine" \
        "de.tag=latest" \
        "dashboard.image=ghcr.io/radius-project/dashboard" \
        "dashboard.tag=latest"
    authz_mode_helm_values "${mode}"
}

# Records the installed authorization mode as a label on the Radius namespace.
authz_record_mode() {
    local mode="${1:-}"
    authz_mode_helm_values "${mode}" >/dev/null || return
    authz_kubectl label namespace "${AUTHZ_RADIUS_NAMESPACE}" \
        "${AUTHZ_KIT_MODE_LABEL}=${mode}" --overwrite >/dev/null
}

# Prints the authorization mode recorded by up.sh, or nothing when it is
# unknown. Uses the current kubeconfig context.
authz_recorded_mode() {
    kubectl get namespace "${AUTHZ_RADIUS_NAMESPACE}" \
        -o "jsonpath={.metadata.labels.${AUTHZ_KIT_MODE_LABEL//./\\.}}" 2>/dev/null || true
}

# Prints the extra would-deny check arguments for an installed mode: a dryRun
# install must prove every component started in dryRun.
authz_would_deny_mode_args() {
    if [[ "${1:-}" == "dryRun" ]]; then
        echo "--require-dry-run"
    fi
}

# Prints the name of the Secret expected to hold a component's certificate.
authz_component_secret_name() {
    local component="${1:-}"
    if [[ ! "${component}" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]]; then
        echo "error: invalid component name '${component}': expected a name such as ${AUTHZ_KNOWN_COMPONENTS[*]}" >&2
        return 2
    fi
    if [[ "${AUTHZ_COMPONENT_SECRET_FORMAT}" != *%s* ]]; then
        echo "error: AUTHZ_COMPONENT_SECRET_FORMAT must contain %s, got '${AUTHZ_COMPONENT_SECRET_FORMAT}'" >&2
        return 2
    fi
    # shellcheck disable=SC2059 # The format string is the configurable Secret naming.
    printf "${AUTHZ_COMPONENT_SECRET_FORMAT}\n" "${component}"
}

# Creates the rogue pod if it does not exist and waits for it to be ready.
authz_ensure_rogue_pod() {
    if ! authz_kubectl get pod "${AUTHZ_ROGUE_POD}" \
        --namespace "${AUTHZ_ROGUE_NAMESPACE}" >/dev/null 2>&1; then
        echo "Creating rogue pod ${AUTHZ_ROGUE_NAMESPACE}/${AUTHZ_ROGUE_POD}" >&2
        authz_kubectl apply --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
            --filename "${AUTHZ_KIT_DIR}/rogue-pod.yaml" >&2
    fi
    authz_kubectl wait pod "${AUTHZ_ROGUE_POD}" \
        --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
        --for condition=Ready --timeout 120s >/dev/null
}

# Runs curl in the rogue pod. A leading "curl" argument is accepted and dropped
# so both "-- -s URL" and "-- curl -s URL" work.
authz_rogue_curl() {
    if [[ "${1:-}" == "curl" ]]; then
        shift
    fi
    (($# > 0)) || authz_die "no curl arguments given after --"
    authz_kubectl exec "${AUTHZ_ROGUE_POD}" \
        --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
        --container "${AUTHZ_ROGUE_CURL_CONTAINER}" -- curl "$@"
}
