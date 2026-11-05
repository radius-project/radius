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
# Shows execution records. Placeholder until Stack B adds execution records.
# ============================================================================

set -euo pipefail

usage() {
    cat <<EOF
Usage: $(basename "$0") [RECORD_ID]

Shows UCP execution records, the approvals that scope what a deployment may
change. Execution records are added in Stack B; until then this prints a notice
and exits 0.

Planned output:
  Without RECORD_ID, one line per record:
    ID  STATUS  DEPLOYMENT  CREATED  EXPIRES  TARGETS
  With RECORD_ID, the record's status (active, closed, or expired), parent
  record, approved actions and targets, and its operations with their assigned
  component and inputHash.
EOF
}

case "${1:-}" in
    -h | --help)
        usage
        exit 0
        ;;
    -*)
        usage >&2
        echo "error: unknown option '$1'" >&2
        exit 2
        ;;
esac

if [[ $# -gt 1 ]]; then
    usage >&2
    echo "error: at most one record ID is allowed" >&2
    exit 2
fi

echo "execution records are added in Stack B"
