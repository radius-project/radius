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
# Impersonates a compromised Radius component: requests a ServiceAccount token
# for the component's ServiceAccount with the receiver as the audience (as the
# kubelet does for projected token volumes), and runs rogue-client with it as a
# bearer token. The token and the public Radius CA are copied into the rogue
# pod; rogue-client verifies the receiver's SPIFFE ID before sending the token.
# With -k in the request arguments, the token is sent as a header without
# verifying the server.
#
# Exit codes: 0 HTTP response, 1 transport failure, 2 usage error, 3 the
# ServiceAccount or the radius-ca Secret does not exist.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    cat <<EOF
Usage: $(basename "$0") [-n NAMESPACE] [-r RECEIVER] COMPONENT -- REQUEST_ARGS...

Runs /rogue-client request with a 10-minute ServiceAccount token for COMPONENT,
to act as a compromised component. The token is requested with
'kubectl create token COMPONENT --audience RECEIVER' in ${AUTHZ_RADIUS_NAMESPACE}.
rogue-client reads it from a file and sends it only after verifying the
server's Radius SPIFFE ID with the public CA from the radius-ca Secret, so
REQUEST_ARGS must not include --cert, --key, or --cacert. If REQUEST_ARGS
include -k, the server is not verified and the token is sent as a header.
Use rogue.sh --help for request flags, output format, and exit codes.
All flags must precede the URL.

Components: ${AUTHZ_KNOWN_COMPONENTS[*]}

Options:
  -n, --namespace NAME  Namespace of the rogue pod (default: ${AUTHZ_ROGUE_NAMESPACE},
                        env: AUTHZ_ROGUE_NAMESPACE).
  -r, --receiver NAME   Token audience (default: the first DNS label of the URL host).
  -h, --help            Show this help.

Environment:
  AUTHZ_KUBE_CONTEXT    kubeconfig context (default: ${AUTHZ_KUBE_CONTEXT}).

Example:
  $(basename "$0") dynamic-rp -- \\
    'https://applications-rp.radius-system.svc:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'
EOF
}

COMPONENT=""
RECEIVER=""
REQUEST_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n | --namespace)
            [[ $# -ge 2 ]] || authz_die "$1 requires a value"
            AUTHZ_ROGUE_NAMESPACE="$2"
            shift 2
            ;;
        -r | --receiver)
            [[ $# -ge 2 ]] || authz_die "$1 requires a value"
            RECEIVER="$2"
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        --)
            shift
            REQUEST_ARGS=("$@")
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
((${#REQUEST_ARGS[@]} > 0)) || authz_die "no request arguments given after --"
[[ "${COMPONENT}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] ||
    authz_die "invalid component name '${COMPONENT}': expected a name such as ${AUTHZ_KNOWN_COMPONENTS[*]}"

if [[ -z "${RECEIVER}" ]]; then
    url="${REQUEST_ARGS[${#REQUEST_ARGS[@]} - 1]}"
    host="${url#*://}"
    host="${host%%[:/?]*}"
    RECEIVER="${host%%.*}"
fi
[[ "${RECEIVER}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] ||
    authz_die "could not infer the receiver from the URL; pass --receiver"
readonly COMPONENT RECEIVER
readonly RADIUS_DIR="${AUTHZ_ROGUE_WORKDIR}/radius"

authz_require_tools kubectl

missing() {
    echo "$1" >&2
    exit 3
}

account="$(authz_kubectl get serviceaccount "${COMPONENT}" \
    --namespace "${AUTHZ_RADIUS_NAMESPACE}" --ignore-not-found --output name)"
[[ -n "${account}" ]] || missing "ServiceAccount not found: ${AUTHZ_RADIUS_NAMESPACE}/${COMPONENT}"
ca_secret="$(authz_kubectl get secret radius-ca \
    --namespace "${AUTHZ_RADIUS_NAMESPACE}" --ignore-not-found --output name)"
[[ -n "${ca_secret}" ]] ||
    missing "Secret not found: ${AUTHZ_RADIUS_NAMESPACE}/radius-ca (is internal authorization off?)"

authz_ensure_rogue_pod

# Keeps the CA only if it is a complete PEM block, because kubectl exec can drop stdin data.
# shellcheck disable=SC2016 # Expanded by sh in the pod.
readonly WRITE_CA_SCRIPT='mkdir -p "$1" && base64 -d >"$1/ca.crt.tmp" && grep -q -- "-----END" "$1/ca.crt.tmp" && mv "$1/ca.crt.tmp" "$1/ca.crt"'
for attempt in 1 2 3; do
    authz_kubectl get secret radius-ca --namespace "${AUTHZ_RADIUS_NAMESPACE}" \
        --output 'jsonpath={.data.ca\.crt}' |
        authz_kubectl exec --stdin "${AUTHZ_ROGUE_POD}" \
            --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
            --container "${AUTHZ_ROGUE_OPENSSL_CONTAINER}" -- \
            sh -c "${WRITE_CA_SCRIPT}" sh "${RADIUS_DIR}" && break
    ((attempt < 3)) || authz_die "could not copy the Radius CA into the rogue pod"
done

token="$(authz_kubectl create token "${COMPONENT}" \
    --namespace "${AUTHZ_RADIUS_NAMESPACE}" --audience "${RECEIVER}" --duration 10m)"

insecure=false
for arg in "${REQUEST_ARGS[@]}"; do
    [[ "${arg}" != -k ]] || insecure=true
done

echo "Calling ${RECEIVER} as ${COMPONENT} with a ServiceAccount token" >&2
if [[ "${insecure}" == true ]]; then
    # -k skips server verification, for example on the aggregation port, which does not
    # serve a Radius SPIFFE certificate. Send the token directly.
    authz_rogue_request -H "Authorization: Bearer ${token}" "${REQUEST_ARGS[@]}"
    exit
fi

# Like the projected token volume, the token is a file the client reads: the rogue-client
# Radius transport verifies the receiver's SPIFFE ID before it sends the token.
# shellcheck disable=SC2016 # Expanded by sh in the pod.
readonly WRITE_TOKEN_SCRIPT='mkdir -p "$1" && cat >"$1/token.tmp" && grep -q "\..*\." "$1/token.tmp" && mv "$1/token.tmp" "$1/token"'
for attempt in 1 2 3; do
    printf '%s' "${token}" |
        authz_kubectl exec --stdin "${AUTHZ_ROGUE_POD}" \
            --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
            --container "${AUTHZ_ROGUE_OPENSSL_CONTAINER}" -- \
            sh -c "${WRITE_TOKEN_SCRIPT}" sh "${RADIUS_DIR}/tokens/${RECEIVER}" && break
    ((attempt < 3)) || authz_die "could not copy the token into the rogue pod"
done
authz_rogue_request --radius-receiver "${RECEIVER}" --radius-dir "${RADIUS_DIR}" \
    --radius-namespace "${AUTHZ_RADIUS_NAMESPACE}" "${REQUEST_ARGS[@]}"
