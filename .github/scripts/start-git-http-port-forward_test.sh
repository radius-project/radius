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

# Tests start-git-http-port-forward.sh with stub kubectl and curl commands, so
# no cluster or network access is needed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT="${SCRIPT_DIR}/start-git-http-port-forward.sh"

TEST_DIR="$(mktemp -d)"
readonly TEST_DIR
export TEST_DIR

cleanup() {
    local pid
    cat "${TEST_DIR}"/*.pid 2>/dev/null | while read -r pid; do
        kill "${pid}" 2>/dev/null || true
    done || true
    rm -rf "${TEST_DIR}"
}
trap cleanup EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

# Each stub kubectl records its own pid and its parent (the restart loop).
# KUBECTL_MODE=flaky becomes ready and then exits like a lost pod connection.
# KUBECTL_MODE=hang never becomes ready and runs until it is killed.
mkdir -p "${TEST_DIR}/bin"
cat >"${TEST_DIR}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
echo "$$" >>"${TEST_DIR}/kubectl.pid"
echo "${PPID}" >"${TEST_DIR}/loop.pid"
echo "$*" >>"${TEST_DIR}/kubectl-calls"
case "${KUBECTL_MODE}" in
    flaky)
        touch "${TEST_DIR}/ready"
        sleep 0.3
        echo "error: lost connection to pod"
        exit 1
        ;;
    hang)
        exec sleep 300
        ;;
esac
EOF
cat >"${TEST_DIR}/bin/curl" <<'EOF'
#!/usr/bin/env bash
[[ -f "${TEST_DIR}/ready" ]] || exit 7
EOF
chmod +x "${TEST_DIR}/bin/kubectl" "${TEST_DIR}/bin/curl"
PATH="${TEST_DIR}/bin:${PATH}"

reset_case() {
    rm -f "${TEST_DIR}"/*.pid "${TEST_DIR}/kubectl-calls" \
        "${TEST_DIR}/ready" "${TEST_DIR}/pf.log"
    export KUBECTL_MODE="$1"
}

call_count() {
    if [[ -f "${TEST_DIR}/kubectl-calls" ]]; then
        wc -l <"${TEST_DIR}/kubectl-calls" | tr -d ' '
    else
        echo 0
    fi
}

is_running() {
    kill -0 "$1" 2>/dev/null
}

wait_for_exit() {
    local pid=$1 i
    for ((i = 0; i < 50; i++)); do
        is_running "${pid}" || return 0
        sleep 0.1
    done
    return 1
}

# Restarts kubectl every time it exits and logs each restart.
reset_case flaky
"${SCRIPT}" "${TEST_DIR}/pf.log" ns svc 1234 3000 >/dev/null ||
    fail_test "startup failed while the forward was ready"
for ((i = 0; i < 50 && $(call_count) < 3; i++)); do
    sleep 0.1
done
(($(call_count) >= 3)) || fail_test "port-forward was not restarted"
grep -q -- "port-forward -n ns svc/svc 1234:3000" "${TEST_DIR}/kubectl-calls" ||
    fail_test "kubectl arguments"
grep -q "error: lost connection to pod" "${TEST_DIR}/pf.log" ||
    fail_test "kubectl output was not logged"
grep -q "port-forward exited with status 1, restarting" "${TEST_DIR}/pf.log" ||
    fail_test "restart was not logged"

# A TERM to the loop stops it and its active kubectl.
loop_pid=$(<"${TEST_DIR}/loop.pid")
kill "${loop_pid}"
wait_for_exit "${loop_pid}" || fail_test "loop ignored TERM"
calls=$(call_count)
sleep 1.5
(($(call_count) == calls)) || fail_test "kubectl restarted after TERM"

# A readiness timeout fails the step, prints the log, and stops the loop and
# its active kubectl instead of leaving them running.
reset_case hang
status=0
GIT_HTTP_PORT_FORWARD_TIMEOUT=1 "${SCRIPT}" "${TEST_DIR}/pf.log" \
    >/dev/null 2>"${TEST_DIR}/stderr" || status=$?
((status == 1)) || fail_test "timeout status was ${status}"
grep -q "Port forwarding failed after 1s" "${TEST_DIR}/stderr" ||
    fail_test "timeout message"
grep -q "starting port-forward" "${TEST_DIR}/stderr" ||
    fail_test "timeout did not print the log"
wait_for_exit "$(<"${TEST_DIR}/loop.pid")" ||
    fail_test "loop kept running after timeout"
while read -r pid; do
    wait_for_exit "${pid}" || fail_test "kubectl ${pid} kept running after timeout"
done <"${TEST_DIR}/kubectl.pid"
calls=$(call_count)
sleep 1.5
(($(call_count) == calls)) || fail_test "kubectl restarted after timeout"

echo "start-git-http-port-forward tests passed"
