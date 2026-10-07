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
# Runs .github/scripts/authz-would-deny-check.sh against the kit cluster.
# All logic lives in that script; this wrapper only selects the kubeconfig
# context so the check never reads logs from another cluster, and adds
# --require-dry-run when up.sh installed the cluster in dryRun mode.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly CHECK_SCRIPT="${AUTHZ_REPO_ROOT}/.github/scripts/authz-would-deny-check.sh"

TEMP_DIR=""
# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

usage() {
    cat <<EOF
Usage: $(basename "$0") [--current-context] [CHECK_ARGS...]

Fails if a Radius pod logged an authorization dry-run would-deny line. Wraps
.github/scripts/authz-would-deny-check.sh and runs it against the kubeconfig
context ${AUTHZ_KUBE_CONTEXT}.

When up.sh installed the cluster in dryRun mode (the ${AUTHZ_KIT_MODE_LABEL}
label on namespace ${AUTHZ_RADIUS_NAMESPACE}), --require-dry-run is added so
the check also fails unless ucp, applications-rp, dynamic-rp, and controller
all started in dryRun. For other or unknown modes it only scans for
would-deny lines; pass --require-dry-run to require dryRun anyway.

Options:
  --current-context  Use the current kubeconfig context instead.
  -h, --help         Show this help and the check's options.

Environment:
  AUTHZ_KUBE_CONTEXT  kubeconfig context (default: ${AUTHZ_KUBE_CONTEXT}).

CHECK_ARGS are passed to the check script. Its exit codes are returned:
0 no unexpected would-deny lines, 1 would-deny lines found, 2 usage,
collection, or dry-run verification error.
EOF
}

USE_CURRENT_CONTEXT=false
CHECK_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --current-context)
            USE_CURRENT_CONTEXT=true
            shift
            ;;
        -h | --help)
            usage
            echo
            echo "Check script options:"
            bash "${CHECK_SCRIPT}" --help
            exit 0
            ;;
        *)
            CHECK_ARGS+=("$1")
            shift
            ;;
    esac
done

[[ -f "${CHECK_SCRIPT}" ]] || authz_die "check script not found: ${CHECK_SCRIPT}"

if [[ "${USE_CURRENT_CONTEXT}" == false ]]; then
    authz_require_tools kubectl
    TEMP_DIR="$(mktemp -d)"
    if ! kubectl config view --minify --flatten \
        --context "${AUTHZ_KUBE_CONTEXT}" >"${TEMP_DIR}/kubeconfig"; then
        authz_die "kubeconfig context '${AUTHZ_KUBE_CONTEXT}' not found. Run hack/authz/up.sh, set AUTHZ_KUBE_CONTEXT, or pass --current-context."
    fi
    export KUBECONFIG="${TEMP_DIR}/kubeconfig"
fi

MODE_ARGS=()
mode="$(authz_recorded_mode)"
if [[ -n "${mode}" ]]; then
    echo "Kit cluster installed in authz mode ${mode}." >&2
else
    echo "Install mode unknown (no ${AUTHZ_KIT_MODE_LABEL} label on namespace ${AUTHZ_RADIUS_NAMESPACE}); pass --require-dry-run to require dryRun." >&2
fi
if [[ " ${CHECK_ARGS[*]-} " != *" --require-dry-run "* ]]; then
    while IFS= read -r arg; do
        MODE_ARGS+=("${arg}")
    done < <(authz_would_deny_mode_args "${mode}")
fi

status=0
bash "${CHECK_SCRIPT}" --namespace "${AUTHZ_RADIUS_NAMESPACE}" \
    ${MODE_ARGS[@]+"${MODE_ARGS[@]}"} ${CHECK_ARGS[@]+"${CHECK_ARGS[@]}"} || status=$?
exit "${status}"
