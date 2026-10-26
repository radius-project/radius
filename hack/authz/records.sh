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
# Lists, shows, creates and revokes UCP execution records through the
# PROTOTYPE test hook. Calls go through the Kubernetes API server as the
# current kubeconfig user.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly RECORDS_PATH="/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/System.Authorization/executionRecords"

usage() {
    cat <<EOF
Usage: $(basename "$0") [list]
       $(basename "$0") get RECORD_ID
       $(basename "$0") create RECORD_ID FILE
       $(basename "$0") revoke RECORD_ID

Shows UCP execution records, the approvals that scope what a deployment may change.

  list     Print all records as JSON (default).
  get      Print one record.
  create   Create a record from a JSON file (PROTOTYPE test hook). The file has
           submitter, actions, targets, ttl and optional operations
           [{resourceId, action, assignedComponent, body}]. Prints the record
           and the created operation IDs.
  revoke   Close a record as revoked.
EOF
}

fail_usage() {
    usage >&2
    authz_die "$1"
}

command="${1:-list}"
case "${command}" in
    -h | --help)
        usage
        exit 0
        ;;
    list)
        [[ $# -le 1 ]] || fail_usage "list takes no arguments"
        authz_kubectl get --raw "${RECORDS_PATH}"
        ;;
    get)
        [[ $# -eq 2 ]] || fail_usage "get needs RECORD_ID"
        authz_kubectl get --raw "${RECORDS_PATH}/$2"
        ;;
    create)
        [[ $# -eq 3 ]] || fail_usage "create needs RECORD_ID and FILE"
        authz_kubectl replace --raw "${RECORDS_PATH}/$2" --filename "$3"
        ;;
    revoke)
        [[ $# -eq 2 ]] || fail_usage "revoke needs RECORD_ID"
        authz_kubectl delete --raw "${RECORDS_PATH}/$2"
        ;;
    *)
        fail_usage "unknown command '${command}'"
        ;;
esac
echo
