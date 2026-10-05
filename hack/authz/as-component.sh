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
# Impersonates a compromised Radius component: copies the component's mTLS
# certificate, key, and CA from its Secret into the rogue pod and runs curl
# with them.
#
# Exit codes: curl's exit code, 2 usage error, 3 the component's Secret does
# not exist or is incomplete.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly SECRET_KEYS=(tls.crt tls.key ca.crt)

usage() {
    cat <<EOF
Usage: $(basename "$0") [-n NAMESPACE] COMPONENT -- CURL_ARGS...

Runs curl from the rogue pod presenting COMPONENT's mTLS certificate, to act
as a compromised component. The certificate, key, and CA are copied from the
Secret "${AUTHZ_COMPONENT_SECRET_FORMAT//%s/COMPONENT}" in ${AUTHZ_RADIUS_NAMESPACE} into the
pod and passed to curl as --cert, --key, and --cacert. A leading "curl" in
CURL_ARGS is optional.

Components: ${AUTHZ_KNOWN_COMPONENTS[*]}

Options:
  -n, --namespace NAME  Namespace of the rogue pod (default: ${AUTHZ_ROGUE_NAMESPACE},
                        env: AUTHZ_ROGUE_NAMESPACE).
  -h, --help            Show this help.

Environment:
  AUTHZ_COMPONENT_SECRET_FORMAT  Secret name format, %s is the component
                                 (default: ${AUTHZ_COMPONENT_SECRET_FORMAT}).
  AUTHZ_KUBE_CONTEXT             kubeconfig context (default: ${AUTHZ_KUBE_CONTEXT}).

Example:
  $(basename "$0") dynamic-rp -- -s -o /dev/null -w '%{http_code}' \\
    'https://applications-rp.radius-system:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'

Component certificates are added in Stack A. Until then this reports which
Secret is missing and exits 3.
EOF
}

COMPONENT=""
CURL_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n | --namespace)
            [[ $# -ge 2 ]] || authz_die "$1 requires a value"
            AUTHZ_ROGUE_NAMESPACE="$2"
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        --)
            shift
            CURL_ARGS=("$@")
            break
            ;;
        -*)
            usage >&2
            authz_die "unknown option '$1'"
            ;;
        *)
            [[ -z "${COMPONENT}" ]] || authz_die "only one component is allowed, got '${COMPONENT}' and '$1'"
            COMPONENT="$1"
            shift
            ;;
    esac
done

[[ -n "${COMPONENT}" ]] || {
    usage >&2
    authz_die "a component is required"
}
((${#CURL_ARGS[@]} > 0)) || authz_die "no curl arguments given after --"

SECRET_NAME="$(authz_component_secret_name "${COMPONENT}")" || exit 2
readonly SECRET_NAME
readonly CERT_DIR="${AUTHZ_ROGUE_WORKDIR}/${COMPONENT}"

authz_require_tools kubectl

secret_missing() {
    cat >&2 <<EOF
Component certificate Secret not found: ${AUTHZ_RADIUS_NAMESPACE}/${SECRET_NAME}
$1

as-component.sh expects a Secret named "${SECRET_NAME}" in ${AUTHZ_RADIUS_NAMESPACE} with
the keys ${SECRET_KEYS[*]}. Per-component mTLS certificates are added in
Stack A of the internal component authorization design, so this Secret does
not exist on earlier builds. If Stack A uses different Secret names, set
AUTHZ_COMPONENT_SECRET_FORMAT (currently "${AUTHZ_COMPONENT_SECRET_FORMAT}", %s is the
component name).
EOF
    exit 3
}

if ! authz_kubectl get secret "${SECRET_NAME}" \
    --namespace "${AUTHZ_RADIUS_NAMESPACE}" >/dev/null 2>&1; then
    secret_missing "The Secret does not exist (or the current user cannot read it)."
fi

for key in "${SECRET_KEYS[@]}"; do
    value="$(authz_kubectl get secret "${SECRET_NAME}" \
        --namespace "${AUTHZ_RADIUS_NAMESPACE}" \
        --output "jsonpath={.data.${key//./\\.}}")"
    [[ -n "${value}" ]] || secret_missing "The Secret has no '${key}' key."
done

authz_ensure_rogue_pod

# shellcheck disable=SC2016 # Expanded by sh in the pod.
readonly WRITE_KEY_SCRIPT='umask 077 && mkdir -p "$1" && base64 -d >"$1/$2"'

# The data stays base64-encoded until it is inside the pod, so the private key
# is never written to the local disk.
for key in "${SECRET_KEYS[@]}"; do
    authz_kubectl get secret "${SECRET_NAME}" \
        --namespace "${AUTHZ_RADIUS_NAMESPACE}" \
        --output "jsonpath={.data.${key//./\\.}}" |
        authz_kubectl exec --stdin "${AUTHZ_ROGUE_POD}" \
            --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
            --container "${AUTHZ_ROGUE_CURL_CONTAINER}" -- \
            sh -c "${WRITE_KEY_SCRIPT}" sh "${CERT_DIR}" "${key}"
done

echo "Calling as ${COMPONENT} with ${AUTHZ_RADIUS_NAMESPACE}/${SECRET_NAME}" >&2
authz_rogue_curl "${CURL_ARGS[@]}" \
    --cert "${CERT_DIR}/tls.crt" \
    --key "${CERT_DIR}/tls.key" \
    --cacert "${CERT_DIR}/ca.crt"
