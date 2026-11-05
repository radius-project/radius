#!/usr/bin/env bash

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

# Tests install-git-http-backend.sh with stub kubectl and envsubst commands, so
# no cluster is needed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT="${SCRIPT_DIR}/install-git-http-backend.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
export TEST_DIR
trap 'rm -rf "${TEST_DIR}"' EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

# DEPLOYMENT=absent|present|error controls the existence lookup.
# TERMINATING lists pods the stub reports as Terminating.
mkdir -p "${TEST_DIR}/bin"
cat >"${TEST_DIR}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"${TEST_DIR}/calls"
case "$*" in
    *"get deployment git-http-backend"*)
        case "${DEPLOYMENT}" in
            present) echo "deployment.apps/git-http-backend" ;;
            error) echo "error: Unauthorized" >&2; exit 1 ;;
        esac
        ;;
    *"get pods"*) printf '%s' "${TERMINATING:-}" ;;
    *"--dry-run=client"*) echo "apiVersion: v1" ;;
    *"apply -f -"*) cat >/dev/null ;;
esac
EOF
cat >"${TEST_DIR}/bin/envsubst" <<'EOF'
#!/usr/bin/env bash
cat
EOF
chmod +x "${TEST_DIR}/bin/kubectl" "${TEST_DIR}/bin/envsubst"
PATH="${TEST_DIR}/bin:${PATH}"

run_case() {
    : >"${TEST_DIR}/calls"
    export DEPLOYMENT="$1" TERMINATING="${2:-}"
    status=0
    "${SCRIPT}" user pass >/dev/null 2>&1 || status=$?
}

called() {
    grep -q -- "$1" "${TEST_DIR}/calls"
}

# A new Deployment already has the current config, so it is not restarted.
run_case absent
((status == 0)) || fail_test "new Deployment status ${status}"
! called "rollout restart" || fail_test "restarted a new Deployment"
called "rollout status" || fail_test "did not wait for rollout"
! called "wait --for=delete" || fail_test "waited with no Terminating pods"

# An existing Deployment is restarted, and its old pods must be deleted before
# the script returns.
run_case present "pod/old-a pod/old-b "
((status == 0)) || fail_test "existing Deployment status ${status}"
called "rollout restart deployment/git-http-backend" ||
    fail_test "did not restart an existing Deployment"
called "wait --for=delete --timeout=120s pod/old-a pod/old-b" ||
    fail_test "did not wait for Terminating pods"
restart_line=$(grep -n "rollout restart" "${TEST_DIR}/calls" | cut -d: -f1)
wait_line=$(grep -n "wait --for=delete" "${TEST_DIR}/calls" | cut -d: -f1)
((restart_line < wait_line)) || fail_test "waited before restarting"

# A failed lookup stops the install instead of skipping a needed restart.
run_case error
((status != 0)) || fail_test "lookup failure was ignored"
! called "apply -f -" || fail_test "applied after lookup failure"

echo "install-git-http-backend tests passed"
