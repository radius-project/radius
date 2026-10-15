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
# Runs a rogue workload pod (rogue-client + openssl) and sends requests from it, to
# check what an unprivileged workload in the cluster can reach.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    cat <<EOF
Usage: $(basename "$0") [-n NAMESPACE] COMMAND [-- ARGS...]

Runs a rogue workload pod (${AUTHZ_ROGUE_POD}) and sends requests from it.

Commands:
  up                  Create the pod if needed and wait until it is ready.
  exec -- ARGS...     Run /rogue-client request ARGS in the pod.
  openssl -- ARGS...  Run openssl ARGS in the pod, for example s_client.
  down                Delete the pod and wait until it is gone.

Options:
  -n, --namespace NAME  Namespace for the pod (default: ${AUTHZ_ROGUE_NAMESPACE},
                        env: AUTHZ_ROGUE_NAMESPACE).
  -h, --help            Show this help.

Environment:
  AUTHZ_KUBE_CONTEXT  kubeconfig context (default: ${AUTHZ_KUBE_CONTEXT}).
  AUTHZ_ROGUE_IMAGE   Override the image recorded by up.sh in radius-authz-kit.

Request flags (before the URL): -X, -H (repeatable), --data, --cert, --key,
--cacert, -k, --max-time (duration, e.g. 30s). Curl-only flags are not supported.
Output: response body followed by radius-rogue-status:<HTTP code>.
Exit codes: 0 HTTP response (including 4xx/5xx), 1 transport failure, 2 usage.

Example:
  $(basename "$0") exec -- \\
    'http://applications-rp.radius-system:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'
EOF
}

COMMAND=""
ARGS=()

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
            ARGS=("$@")
            break
            ;;
        up | exec | openssl | down)
            [[ -z "${COMMAND}" ]] || authz_die "only one command is allowed"
            COMMAND="$1"
            shift
            ;;
        *)
            usage >&2
            authz_die "unknown argument '$1'"
            ;;
    esac
done

[[ -n "${COMMAND}" ]] || {
    usage >&2
    authz_die "a command is required"
}

authz_require_tools kubectl

case "${COMMAND}" in
    up)
        authz_ensure_rogue_pod
        echo "Rogue pod ${AUTHZ_ROGUE_NAMESPACE}/${AUTHZ_ROGUE_POD} is ready."
        ;;
    exec)
        ((${#ARGS[@]} > 0)) || authz_die "no request arguments given after --"
        authz_ensure_rogue_pod
        authz_rogue_request "${ARGS[@]}"
        ;;
    openssl)
        ((${#ARGS[@]} > 0)) || authz_die "no openssl arguments given after --"
        authz_ensure_rogue_pod
        authz_kubectl exec --stdin "${AUTHZ_ROGUE_POD}" \
            --namespace "${AUTHZ_ROGUE_NAMESPACE}" \
            --container "${AUTHZ_ROGUE_OPENSSL_CONTAINER}" -- openssl "${ARGS[@]}"
        ;;
    down)
        authz_kubectl delete pod "${AUTHZ_ROGUE_POD}" \
            --namespace "${AUTHZ_ROGUE_NAMESPACE}" --ignore-not-found --wait=true
        ;;
esac
