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
# Tests for .github/scripts/authz-would-deny-check.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT="${SCRIPT_DIR}/authz-would-deny-check.sh"
readonly FIXTURES="${SCRIPT_DIR}/testdata/authz-would-deny"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
PASS=0
FAIL=0

cleanup() {
    rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

pass_test() {
    ((++PASS))
}

# Runs the script and sets STATUS and OUTPUT for the caller to assert on.
# An empty allowlist is used unless the caller passes --allowlist.
run_check() {
    STATUS=0
    OUTPUT="$(AUTHZ_WOULD_DENY_ALLOWLIST="${TEST_ROOT}/no-allowlist.txt" \
        PATH="${TEST_ROOT}/bin:${PATH}" bash "${SCRIPT}" "$@" 2>&1)" || STATUS=$?
}

assert_status() {
    if [[ "${STATUS}" -eq "$1" ]]; then
        pass_test
    else
        fail_test "expected exit status $1, got ${STATUS}. Output:"$'\n'"${OUTPUT}"
    fi
}

assert_output_contains() {
    if grep -Fq -- "$1" <<<"${OUTPUT}"; then
        pass_test
    else
        fail_test "expected output to contain '$1'. Output:"$'\n'"${OUTPUT}"
    fi
}

assert_output_not_contains() {
    if grep -Fq -- "$1" <<<"${OUTPUT}"; then
        fail_test "expected output not to contain '$1'. Output:"$'\n'"${OUTPUT}"
    else
        pass_test
    fi
}

# A kubectl stub that serves pod inventory and logs from fixture directories.
#   MOCK_PODS       jsonpath output for "get pods": "<pod>\t<container>=<restarts> ...", one pod per line.
#   MOCK_LOGS       directory with <pod>.log (current) and <pod>.<container>.previous.log files.
#   MOCK_CALLS      file that records every invocation.
mkdir -p "${TEST_ROOT}/bin"
cat >"${TEST_ROOT}/bin/kubectl" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "$*" >>"${MOCK_CALLS}"
case "$1" in
    get)
        if [[ "${MOCK_INVENTORY_FAILURE:-false}" == true ]]; then
            echo "mock: pod inventory unavailable" >&2
            exit 1
        fi
        printf '%b' "${MOCK_PODS}"
        ;;
    logs)
        pod="$2"
        container=""
        previous=false
        shift 2
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --container) container="$2"; shift 2 ;;
                --previous) previous=true; shift ;;
                *) shift ;;
            esac
        done
        if [[ "${previous}" == true ]]; then
            file="${MOCK_LOGS}/${pod}.${container}.previous.log"
        elif [[ -n "${container}" ]]; then
            file="${MOCK_LOGS}/${pod}.${container}.log"
        else
            file="${MOCK_LOGS}/${pod}.log"
        fi
        if [[ ! -f "${file}" ]]; then
            echo "Error from server (BadRequest): no logs for ${pod}" >&2
            exit 1
        fi
        cat "${file}"
        ;;
    *)
        echo "unexpected kubectl call: $*" >&2
        exit 1
        ;;
esac
MOCK
chmod +x "${TEST_ROOT}/bin/kubectl"
export MOCK_CALLS="${TEST_ROOT}/kubectl-calls.txt"

echo "Test: logs without would-deny lines pass"
run_check --logs-dir "${FIXTURES}/clean"
assert_status 0
assert_output_contains "No authorization would-deny log lines found"

echo "Test: authzWouldDeny=false is not reported"
assert_output_not_contains "would-deny: "

echo "Test: would-deny lines fail and name the pod"
run_check --logs-dir "${FIXTURES}/would-deny"
assert_status 1
assert_output_contains 'would-deny: applications-rp-5c6d7e8f9-fghij: {"severity":"info"'
assert_output_contains '"authzCode":"AuthorizationFailed"'
assert_output_contains "would-deny: dynamic-rp-6f7a8b9c0-klmno (previous): "
assert_output_contains '"authzCode": "InvalidClientIdentity"'
assert_output_contains "Found 2 authorization would-deny log line(s)"
assert_output_not_contains "ucp-7d9f8b6c4-abcde:"

echo "Test: allowlisted would-deny lines pass and cite the issue"
run_check --logs-dir "${FIXTURES}/would-deny" --allowlist "${FIXTURES}/allowlist-all.txt"
assert_status 0
assert_output_contains "allowed (https://github.com/radius-project/radius/issues/99901): applications-rp-5c6d7e8f9-fghij:"
assert_output_contains "allowed (radius-project/radius#99902): dynamic-rp-6f7a8b9c0-klmno (previous):"
assert_output_contains "No authorization would-deny log lines found (2 allowlisted)"

echo "Test: lines not covered by the allowlist still fail"
run_check --logs-dir "${FIXTURES}/would-deny" --allowlist "${FIXTURES}/allowlist-partial.txt"
assert_status 1
assert_output_contains "allowed (radius-project/radius#99902): dynamic-rp-6f7a8b9c0-klmno (previous):"
assert_output_contains "would-deny: applications-rp-5c6d7e8f9-fghij:"
assert_output_contains "Found 1 authorization would-deny log line(s)"

echo "Test: AUTHZ_WOULD_DENY_ALLOWLIST selects the allowlist"
STATUS=0
OUTPUT="$(AUTHZ_WOULD_DENY_ALLOWLIST="${FIXTURES}/allowlist-all.txt" \
    bash "${SCRIPT}" --logs-dir "${FIXTURES}/would-deny" 2>&1)" || STATUS=$?
assert_status 0

echo "Test: the default allowlist has no entries"
STATUS=0
OUTPUT="$(env -u AUTHZ_WOULD_DENY_ALLOWLIST bash "${SCRIPT}" --logs-dir "${FIXTURES}/would-deny" 2>&1)" || STATUS=$?
assert_status 1
assert_output_contains "Found 2 authorization would-deny log line(s)"

echo "Test: allowlist entries must reference an issue"
run_check --logs-dir "${FIXTURES}/clean" --allowlist "${FIXTURES}/allowlist-missing-issue.txt"
assert_status 2
assert_output_contains "allowlist-missing-issue.txt:1: entry must start with an issue reference"

echo "Test: allowlist entries must have a pattern"
run_check --logs-dir "${FIXTURES}/clean" --allowlist "${FIXTURES}/allowlist-missing-pattern.txt"
assert_status 2
assert_output_contains "allowlist-missing-pattern.txt:1: entry is missing a pattern"

echo "Test: allowlist patterns must be valid regular expressions"
run_check --logs-dir "${FIXTURES}/clean" --allowlist "${FIXTURES}/allowlist-invalid-regex.txt"
assert_status 2
assert_output_contains "allowlist-invalid-regex.txt:1: invalid pattern"

echo "Test: an explicit allowlist that does not exist is an error"
run_check --logs-dir "${FIXTURES}/clean" --allowlist "${TEST_ROOT}/missing.txt"
assert_status 2
assert_output_contains "allowlist not found"

echo "Test: a missing logs directory is an error"
run_check --logs-dir "${TEST_ROOT}/missing-dir"
assert_status 2
assert_output_contains "logs directory not found"

echo "Test: unknown arguments are rejected"
run_check --bogus
assert_status 2
assert_output_contains "unknown argument: --bogus"

echo "Test: cluster mode reads current and previous container logs"
MOCK_DIR="${TEST_ROOT}/cluster-logs"
mkdir -p "${MOCK_DIR}"
cp "${FIXTURES}/clean/ucp-7d9f8b6c4-abcde.log" "${MOCK_DIR}/ucp-7d9f8b6c4-abcde.log"
cp "${FIXTURES}/would-deny/applications-rp-5c6d7e8f9-fghij.log" "${MOCK_DIR}/applications-rp-5c6d7e8f9-fghij.log"
cp "${FIXTURES}/clean/ucp-7d9f8b6c4-abcde.log" "${MOCK_DIR}/dynamic-rp-6f7a8b9c0-klmno.log"
cp "${FIXTURES}/would-deny/dynamic-rp-6f7a8b9c0-klmno.previous.log" \
    "${MOCK_DIR}/dynamic-rp-6f7a8b9c0-klmno.dynamic-rp.previous.log"
: >"${MOCK_CALLS}"
export MOCK_LOGS="${MOCK_DIR}"
export MOCK_PODS='ucp-7d9f8b6c4-abcde\tucp=0 \napplications-rp-5c6d7e8f9-fghij\tapplications-rp=0 \ndynamic-rp-6f7a8b9c0-klmno\tdynamic-rp=2 sidecar=0 \n'
run_check --namespace custom-ns
assert_status 1
assert_output_contains "would-deny: applications-rp-5c6d7e8f9-fghij: "
assert_output_contains "would-deny: dynamic-rp-6f7a8b9c0-klmno (previous): "
assert_output_contains "Found 2 authorization would-deny log line(s)"
if grep -Fxq "logs dynamic-rp-6f7a8b9c0-klmno --namespace custom-ns --container dynamic-rp --previous" "${MOCK_CALLS}"; then
    pass_test
else
    fail_test "expected previous logs to be read for the restarted container. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
fi
if grep -Fq -- "--container sidecar --previous" "${MOCK_CALLS}"; then
    fail_test "previous logs must only be read for restarted containers"
else
    pass_test
fi

echo "Test: cluster mode passes when no pod logged a would-deny line"
rm "${MOCK_DIR}/applications-rp-5c6d7e8f9-fghij.log" "${MOCK_DIR}/dynamic-rp-6f7a8b9c0-klmno.dynamic-rp.previous.log"
cp "${FIXTURES}/clean/applications-rp-5c6d7e8f9-fghij.log" "${MOCK_DIR}/applications-rp-5c6d7e8f9-fghij.log"
export MOCK_PODS='ucp-7d9f8b6c4-abcde\tucp=0 \napplications-rp-5c6d7e8f9-fghij\tapplications-rp=0 \n'
run_check
assert_status 0
assert_output_contains "No authorization would-deny log lines found"
if grep -Fq "get pods --namespace radius-system" "${MOCK_CALLS}"; then
    pass_test
else
    fail_test "expected radius-system to be the default namespace"
fi

echo "Test: one readable pod cannot hide another pod's log failure"
export MOCK_PODS='ucp-7d9f8b6c4-abcde\tucp=0 \npending-pod\tpending=0 \n'
run_check
assert_status 2
assert_output_contains "could not read logs for pod pending-pod"
assert_output_not_contains "No authorization would-deny"

echo "Test: cluster mode fails when no pod logs can be read"
export MOCK_PODS='pending-pod\tpending=0 \n'
run_check
assert_status 2
assert_output_contains "could not read logs for pod pending-pod"

echo "Test: missing previous logs fail even when current logs are readable"
export MOCK_PODS='ucp-7d9f8b6c4-abcde\tucp=1 \n'
run_check
assert_status 2
assert_output_contains "could not read previous logs for container ucp"

echo "Test: failure to list pods cannot pass"
export MOCK_INVENTORY_FAILURE=true
run_check
assert_status 2
assert_output_contains "could not list pods"
unset MOCK_INVENTORY_FAILURE

echo "Test: cluster mode fails when the namespace has no pods"
export MOCK_PODS=''
run_check
assert_status 2
assert_output_contains "no pods found in namespace radius-system"

echo "Test: --cluster combines cluster logs with --logs-dir"
export MOCK_PODS='ucp-7d9f8b6c4-abcde\tucp=0 \n'
: >"${MOCK_CALLS}"
run_check --cluster --logs-dir "${FIXTURES}/would-deny"
assert_status 1
assert_output_contains "Found 2 authorization would-deny log line(s)"
if grep -Fq "get pods" "${MOCK_CALLS}"; then
    pass_test
else
    fail_test "expected --cluster to read pod logs"
fi

echo "Test: empty directories cannot pass as scanned evidence"
mkdir -p "${TEST_ROOT}/empty"
run_check --logs-dir "${TEST_ROOT}/empty"
assert_status 2
assert_output_contains "no log files found"

# Inject I/O failures rather than relying on chmod (which root can bypass).
export REAL_FIND
REAL_FIND="$(command -v find)"
export REAL_GREP
REAL_GREP="$(command -v grep)"
cat >"${TEST_ROOT}/bin/find" <<'MOCK'
#!/bin/bash
set -euo pipefail
"${REAL_FIND}" "$@"
echo "mock: directory traversal failed" >&2
exit 1
MOCK
chmod +x "${TEST_ROOT}/bin/find"
echo "Test: partial find output cannot hide a traversal failure"
run_check --logs-dir "${FIXTURES}/clean"
assert_status 2
assert_output_contains "could not enumerate logs"
assert_output_not_contains "No authorization would-deny"
rm "${TEST_ROOT}/bin/find"

cat >"${TEST_ROOT}/bin/sort" <<'MOCK'
#!/bin/bash
echo "mock: sort failed" >&2
exit 2
MOCK
chmod +x "${TEST_ROOT}/bin/sort"
echo "Test: sort failure cannot produce an empty successful scan"
run_check --logs-dir "${FIXTURES}/clean"
assert_status 2
assert_output_contains "could not sort logs"
rm "${TEST_ROOT}/bin/sort"

cat >"${TEST_ROOT}/bin/grep" <<'MOCK'
#!/bin/bash
set -euo pipefail
if [[ "$*" == *authzWouldDeny* ]]; then
    echo "mock: log read failed" >&2
    exit 2
fi
exec "${REAL_GREP}" "$@"
MOCK
chmod +x "${TEST_ROOT}/bin/grep"
echo "Test: grep errors are not treated as no matches"
run_check --logs-dir "${FIXTURES}/clean"
assert_status 2
assert_output_contains "could not scan log file"
assert_output_not_contains "No authorization would-deny"
rm "${TEST_ROOT}/bin/grep"

echo "Test: all four live components must report dryRun"
export MOCK_PODS=''
for component in ucp applications-rp dynamic-rp controller; do
    MOCK_PODS+="${component}-pod"$'\t'"${component}=0 "$'\n'
    printf '{"message":"authz mode=dryRun","authzMode":"dryRun"}\n' \
        >"${MOCK_DIR}/${component}-pod.${component}.log"
    cp "${MOCK_DIR}/${component}-pod.${component}.log" \
        "${MOCK_DIR}/${component}-pod.log"
done
run_check --require-dry-run
assert_status 0
assert_output_contains "Verified dryRun startup for all four Radius components"

echo "Test: startup verification does not bypass would-deny detection"
run_check --require-dry-run --logs-dir "${FIXTURES}/would-deny"
assert_status 1
assert_output_contains "Found 2 authorization would-deny"

echo "Test: unreadable component startup logs fail"
mv "${MOCK_DIR}/controller-pod.controller.log" "${MOCK_DIR}/controller-startup"
run_check --require-dry-run
assert_status 2
assert_output_contains "could not read current logs for controller-pod/controller"
mv "${MOCK_DIR}/controller-startup" "${MOCK_DIR}/controller-pod.controller.log"

echo "Test: the Make target requires live dryRun evidence"
STATUS=0
OUTPUT="$(PATH="${TEST_ROOT}/bin:${PATH}" \
    make --no-print-directory -C "${SCRIPT_DIR}/../.." \
    authz-would-deny-check 2>&1)" || STATUS=$?
assert_status 0
assert_output_contains "Verified dryRun startup for all four Radius components"

for mode in off enforce dryRunUnexpected; do
    echo "Test: controller startup mode ${mode} fails"
    printf '{"message":"authz mode=%s","authzMode":"%s"}\n' \
        "${mode}" "${mode}" >"${MOCK_DIR}/controller-pod.controller.log"
    run_check --require-dry-run --logs-dir "${FIXTURES}/clean" \
        --allowlist "${FIXTURES}/allowlist-all.txt"
    assert_status 2
    assert_output_contains "controller-pod/controller did not start in dryRun"
done

echo "Test: previous and saved dryRun logs cannot replace current evidence"
cp "${MOCK_DIR}/controller-pod.log" \
    "${MOCK_DIR}/controller-pod.controller.previous.log"
MOCK_PODS="${MOCK_PODS/controller=0/controller=1}"
printf 'No startup message\n' >"${MOCK_DIR}/controller-pod.controller.log"
run_check --require-dry-run --logs-dir "${MOCK_DIR}"
assert_status 2
assert_output_contains "missing authz startup log for controller-pod/controller"

echo "Test: console startup format is accepted"
printf 'INFO controller authz mode=dryRun {"authzMode": "dryRun"}\n' \
    >"${MOCK_DIR}/controller-pod.controller.log"
run_check --require-dry-run
assert_status 0

echo "Test: a healthy replica cannot hide another replica in off mode"
MOCK_PODS+='controller-other'$'\tcontroller=0 \n'
cp "${MOCK_DIR}/controller-pod.log" "${MOCK_DIR}/controller-other.log"
printf 'INFO controller authz mode=off {"authzMode": "off"}\n' \
    >"${MOCK_DIR}/controller-other.controller.log"
run_check --require-dry-run
assert_status 2
assert_output_contains "controller-other/controller did not start in dryRun"

echo "Test: repeated ucp replicas cannot count as four different components"
MOCK_PODS=''
for replica in 1 2 3 4; do
    MOCK_PODS+="ucp-${replica}"$'\tucp=0 \n'
    cp "${MOCK_DIR}/ucp-pod.log" "${MOCK_DIR}/ucp-${replica}.log"
    cp "${MOCK_DIR}/ucp-pod.log" "${MOCK_DIR}/ucp-${replica}.ucp.log"
done
run_check --require-dry-run
assert_status 2
assert_output_contains "missing Radius component: applications-rp"

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"
if [[ "${FAIL}" -gt 0 ]]; then
    exit 1
fi
