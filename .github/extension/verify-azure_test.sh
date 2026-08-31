#!/bin/bash

# Tests the Azure subscription visibility retry by extracting and executing the
# workflow's real run block with deterministic Azure CLI and sleep stubs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly WORKFLOW_FILE="${SCRIPT_DIR}/verify-azure.yml"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly BODY_SCRIPT="${TEST_ROOT}/wait-for-subscription.sh"
readonly STUB_BIN="${TEST_ROOT}/bin"
readonly AZ_CALLS="${TEST_ROOT}/az-calls.log"
readonly SLEEP_CALLS="${TEST_ROOT}/sleep-calls.log"
readonly TEST_LOG="${TEST_ROOT}/test.log"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v jq >/dev/null 2>&1; then
    fail "jq is required; run 'make install-jq'"
fi
if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 is required to parse verify-azure.yml"
fi

extract_wait_step() {
    python3 - "${WORKFLOW_FILE}" "${BODY_SCRIPT}" <<'PYTHON'
import re
import sys

workflow_file, out_file = sys.argv[1], sys.argv[2]
lines = open(workflow_file, encoding="utf-8").read().splitlines()

names = [
    i for i, line in enumerate(lines)
    if re.match(r"^\s*-\s+name:\s+Wait for Azure subscription\s*$", line)
]
if len(names) != 1:
    sys.exit(
        "expected one 'Wait for Azure subscription' step, found %d" % len(names)
    )

run_at = None
step_indent = len(lines[names[0]]) - len(lines[names[0]].lstrip(" "))
for i in range(names[0] + 1, len(lines)):
    line = lines[i]
    indent = len(line) - len(line.lstrip(" "))
    if line.strip() and indent <= step_indent:
        break
    if re.match(r"^\s*run:\s*\|\s*$", line):
        run_at = i
        break

if run_at is None:
    sys.exit("wait step has no 'run: |' block")

body, base = [], None
for line in lines[run_at + 1:]:
    if not line.strip():
        body.append("")
        continue
    indent = len(line) - len(line.lstrip(" "))
    if base is None:
        base = indent
    if indent < base:
        break
    body.append(line[base:])

while body and not body[-1]:
    body.pop()
if not body:
    sys.exit("wait step run block is empty")

open(out_file, "w", encoding="utf-8").write("\n".join(body) + "\n")
PYTHON
}

assert_workflow_contract() {
    python3 - "${WORKFLOW_FILE}" <<'PYTHON'
import re
import sys

lines = open(sys.argv[1], encoding="utf-8").read().splitlines()


def named_step(name):
    pattern = re.compile(r"^\s*-\s+name:\s+" + re.escape(name) + r"\s*$")
    matches = [i for i, line in enumerate(lines) if pattern.match(line)]
    if len(matches) != 1:
        sys.exit("expected exactly one %s step" % name)

    start = matches[0]
    step_indent = len(lines[start]) - len(lines[start].lstrip(" "))
    block = []
    for line in lines[start + 1:]:
        indent = len(line) - len(line.lstrip(" "))
        if line.strip() and indent <= step_indent:
            break
        block.append(line)
    return block


login = "\n".join(named_step("Azure Login (OIDC)"))
if not re.search(
    r"^\s+allow-no-subscriptions:\s+true\s*$", login, re.MULTILINE
):
    sys.exit("Azure Login must set allow-no-subscriptions: true")
if re.search(r"^\s+subscription-id:", login, re.MULTILINE):
    sys.exit(
        "Azure Login must not select the subscription before RBAC propagation"
    )

wait = [line.strip() for line in named_step("Wait for Azure subscription")]
if wait.count("timeout-minutes: 12") != 1:
    sys.exit("wait step must set timeout-minutes: 12")
expected_env = (
    "AZURE_SUBSCRIPTION_ID: ${{ vars.AZURE_SUBSCRIPTION_ID }}"
)
if wait.count(expected_env) != 1:
    sys.exit("wait step must map AZURE_SUBSCRIPTION_ID from the environment")
PYTHON
}

write_stubs() {
    mkdir -p "${STUB_BIN}"
    cat >"${STUB_BIN}/az" <<'EOF'
#!/bin/bash
set -euo pipefail

printf '%s\n' "$*" >>"${AZ_CALLS}"

case "${1:-} ${2:-}" in
    "account list")
        call_count="$(grep -c '^account list ' "${AZ_CALLS}")"
        if [[ "${AZ_LIST_FAIL:-false}" == "true" ]]; then
            echo "Azure CLI subscription refresh failed" >&2
            exit 2
        fi
        if ((call_count < AZ_VISIBLE_ON_ATTEMPT)); then
            printf '[]\n'
        else
            printf '[{"id":"%s"}]\n' "${AZ_VISIBLE_SUBSCRIPTION_ID}"
        fi
        ;;
    "account set")
        ;;
    *)
        echo "unexpected az invocation: $*" >&2
        exit 3
        ;;
esac
EOF
    chmod +x "${STUB_BIN}/az"

    cat >"${STUB_BIN}/sleep" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$1" >>"${SLEEP_CALLS}"
EOF
    chmod +x "${STUB_BIN}/sleep"
}

RUN_EXIT=0
TEST_SUBSCRIPTION_ID="ABCDEF00-1111-2222-3333-444455556666"

run_wait_step() {
    : >"${AZ_CALLS}"
    : >"${SLEEP_CALLS}"

    set +e
    (
        export AZ_CALLS SLEEP_CALLS
        export AZURE_SUBSCRIPTION_ID="${TEST_SUBSCRIPTION_ID}"
        PATH="${STUB_BIN}:${PATH}"
        export PATH
        bash "${BODY_SCRIPT}"
    ) >"${TEST_LOG}" 2>&1
    RUN_EXIT=$?
    set -e
}

assert_exit() {
    ((RUN_EXIT == $2)) ||
        fail "$1: expected exit $2, got ${RUN_EXIT}
$(cat "${TEST_LOG}")"
}

assert_count() {
    local expected="$1"
    local pattern="$2"
    local file="$3"
    local actual
    actual="$(grep -c -- "${pattern}" "${file}" || true)"
    [[ "${actual}" == "${expected}" ]] ||
        fail "expected ${expected} matches for '${pattern}', got ${actual}
$(cat "${file}")"
}

assert_log_contains() {
    grep -qF -- "$1" "${TEST_LOG}" ||
        fail "expected output to contain '$1'
$(cat "${TEST_LOG}")"
}

reset_scenario() {
    export AZ_LIST_FAIL=false
    export AZ_VISIBLE_ON_ATTEMPT=1
    export AZ_VISIBLE_SUBSCRIPTION_ID="abcdef00-1111-2222-3333-444455556666"
    TEST_SUBSCRIPTION_ID="ABCDEF00-1111-2222-3333-444455556666"
}

extract_wait_step
assert_workflow_contract
write_stubs

# The common case checks immediately, compares IDs case-insensitively, and
# selects the configured value without sleeping.
reset_scenario
run_wait_step
assert_exit "immediate visibility" 0
assert_count 1 '^account list --refresh --only-show-errors --output json$' \
    "${AZ_CALLS}"
assert_count 1 \
    '^account set --subscription ABCDEF00-1111-2222-3333-444455556666$' \
    "${AZ_CALLS}"
assert_count 0 '.' "${SLEEP_CALLS}"

# A missing subscription variable fails before querying Azure.
reset_scenario
TEST_SUBSCRIPTION_ID=""
run_wait_step
assert_exit "missing subscription ID" 1
assert_count 0 '.' "${AZ_CALLS}"
assert_log_contains "AZURE_SUBSCRIPTION_ID is not configured"

# A malformed subscription ID fails before querying Azure or sleeping.
reset_scenario
TEST_SUBSCRIPTION_ID="abcdef00-1111-2222-3333"
run_wait_step
assert_exit "malformed subscription ID" 1
assert_count 0 '.' "${AZ_CALLS}"
assert_count 0 '.' "${SLEEP_CALLS}"
assert_log_contains "AZURE_SUBSCRIPTION_ID must be a valid GUID"

# Delayed visibility uses bounded exponential backoff before selecting.
reset_scenario
export AZ_VISIBLE_ON_ATTEMPT=4
run_wait_step
assert_exit "delayed visibility" 0
assert_count 4 '^account list ' "${AZ_CALLS}"
[[ "$(cat "${SLEEP_CALLS}")" == $'5\n10\n20' ]] ||
    fail "expected 5/10/20 second backoff, got:
$(cat "${SLEEP_CALLS}")"

# A different visible subscription must never be selected.
reset_scenario
export AZ_VISIBLE_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000000"
run_wait_step
assert_exit "subscription timeout" 1
assert_count 23 '^account list ' "${AZ_CALLS}"
assert_count 0 '^account set ' "${AZ_CALLS}"
assert_count 22 '.' "${SLEEP_CALLS}"
[[ "$(tail -1 "${SLEEP_CALLS}")" == "30" ]] ||
    fail "expected backoff to cap at 30 seconds"
sleep_total=0
while read -r sleep_seconds; do
    sleep_total=$((sleep_total + sleep_seconds))
done <"${SLEEP_CALLS}"
((sleep_total >= 600)) ||
    fail "expected retry delays to cover 10 minutes, got ${sleep_total}s"
assert_log_contains "Azure OIDC authentication succeeded"
assert_log_contains "Subscription discovery may be failing"
assert_log_contains "Azure RBAC may still be propagating"

# Azure CLI errors are not mistaken for propagation and fail immediately.
reset_scenario
export AZ_LIST_FAIL=true
run_wait_step
assert_exit "Azure CLI refresh failure" 2
assert_count 1 '^account list ' "${AZ_CALLS}"
assert_count 0 '.' "${SLEEP_CALLS}"
assert_log_contains "Azure CLI subscription refresh failed"

echo "Azure verification workflow tests passed"
