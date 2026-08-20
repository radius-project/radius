#!/bin/bash

# Tests the result-accumulator lifecycle in the run-rad-commands composite
# action: the outcome written by the EXIT trap must never claim success for a
# run that was killed rather than completed.
#
# A step timeout kills bash mid-command, so none of the failure branches that
# assign `command_failed` ever run - but the EXIT trap still fires and writes
# the accumulator as it stands. The accumulator must therefore be pessimistic:
# seeded to a non-success value and promoted to `succeeded` only after the
# command loop has actually completed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly ACTION_FILE="${SCRIPT_DIR}/action.yml"
readonly STEP_NAME="Run rad commands"
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly PROLOGUE="${TEST_ROOT}/prologue.sh"
readonly RESULT_FILE="${TEST_ROOT}/rad-commands-result.json"
readonly READY_FILE="${TEST_ROOT}/ready"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v jq >/dev/null 2>&1; then
    fail "jq is required; run 'make install-jq'"
fi

# Extract the accumulator prologue verbatim from the action - the seed, the
# write_result writer, and the EXIT trap - so the test exercises the shipped
# code rather than a copy of it that can drift. Scoped to the "Run rad
# commands" step so an unrelated step gaining the same anchor lines cannot
# silently redirect the extraction.
extract_prologue() {
    python3 - "${ACTION_FILE}" "${PROLOGUE}" "${STEP_NAME}" <<'PYTHON'
import re
import sys

action_file, out_file, step_name = sys.argv[1], sys.argv[2], sys.argv[3]
lines = open(action_file, encoding="utf-8").read().splitlines()

steps = [i for i, l in enumerate(lines)
         if re.match(r"^\s*-\s+name:\s+%s\s*$" % re.escape(step_name), l)]
if len(steps) != 1:
    sys.exit("expected exactly one '%s' step, found %d" % (step_name, len(steps)))

# Bound the search at the next step so the prologue can only come from this one.
start_line = steps[0]
step_indent = len(lines[start_line]) - len(lines[start_line].lstrip(" "))
end = len(lines)
for i, line in enumerate(lines[start_line + 1:], start=start_line + 1):
    if not line.strip():
        continue
    indent = len(line) - len(line.lstrip(" "))
    if indent <= step_indent and line.lstrip().startswith("- "):
        end = i
        break
step = lines[start_line:end]

seeds = [i for i, l in enumerate(step) if l.strip().startswith("RESULT_FILE=")]
if len(seeds) != 1:
    sys.exit("expected exactly one RESULT_FILE assignment in the step, "
             "found %d" % len(seeds))

base = len(step[seeds[0]]) - len(step[seeds[0]].lstrip(" "))
body = []
for line in step[seeds[0]:]:
    body.append(line[base:] if line.strip() else "")
    # The trap's handler has been renamed before (write_result -> cleanup), so
    # match any handler rather than pinning to one name.
    if re.match(r"^trap\s+\w+\s+EXIT$", line.strip()):
        break
else:
    sys.exit("did not find a 'trap <handler> EXIT' after the accumulator seed")

open(out_file, "w", encoding="utf-8").write("\n".join(body) + "\n")
PYTHON

    grep -qE '^trap[[:space:]]+[A-Za-z_][A-Za-z0-9_]*[[:space:]]+EXIT$' "${PROLOGUE}" ||
        fail "failed to extract the accumulator prologue from action.yml"

    # Keep the artifact - and the directory holding it - inside the sandbox
    # instead of the runner's /tmp/radius-output.
    sed -i -E \
        -e "s|^RESULT_FILE=.*|RESULT_FILE=${RESULT_FILE}|" \
        -e "s|^mkdir -p /tmp/radius-output\$|mkdir -p ${TEST_ROOT}|" \
        "${PROLOGUE}"
    grep -q "^RESULT_FILE=${RESULT_FILE}\$" "${PROLOGUE}" ||
        fail "failed to redirect RESULT_FILE into the test sandbox"
    if grep -q '/tmp/radius-output' "${PROLOGUE}"; then
        fail "the extracted prologue still writes to /tmp/radius-output"
    fi
}

result_outcome() {
    jq -r '.outcome' "${RESULT_FILE}"
}

result_exit_code() {
    jq -r '.exitCode' "${RESULT_FILE}"
}

# Locate a line in action.yml. Fails with a useful message rather than handing
# an empty operand to a later comparison if the pattern has been renamed away.
action_line() {
    local pattern="$1" picker="$2" line
    line="$(grep -n -e "${pattern}" "${ACTION_FILE}" | "${picker}" -1 | cut -d: -f1)" ||
        fail "no line in action.yml matches: ${pattern}"
    [[ -n "${line}" ]] || fail "no line in action.yml matches: ${pattern}"
    printf '%s' "${line}"
}

extract_prologue

# ---------------------------------------------------------------------------
# A run killed mid-command must not publish success.
#
# Models a step timeout faithfully: the prologue runs, a command is in flight,
# and the shell is killed by SIGTERM without any failure branch executing -
# only the EXIT trap gets to speak. The subshell uses the same options GitHub
# gives a `shell: bash` step (`-e -o pipefail`).
# ---------------------------------------------------------------------------
ENVIRONMENT="aks-dev" bash -eo pipefail -c "
    source '${PROLOGUE}'
    touch '${READY_FILE}'
    # Stands in for a long-running \`rad deploy\`.
    sleep 60
" &
victim=$!

# Kill only once the trap is installed, so the test cannot race ahead of the
# code it is exercising and mistake a never-started run for a killed one.
for _ in $(seq 1 100); do
    [[ -f "${READY_FILE}" ]] && break
    sleep 0.1
done
[[ -f "${READY_FILE}" ]] ||
    fail "the extracted prologue never finished installing the EXIT trap"

# The result file is written by the trap, not eagerly. If that ever changes,
# the assertions below would be testing a file the kill had no part in.
if [[ -f "${RESULT_FILE}" ]]; then
    fail "the result file must be written by the EXIT trap, not eagerly"
fi

kill -TERM "${victim}" 2>/dev/null || true
wait "${victim}" 2>/dev/null || true

[[ -f "${RESULT_FILE}" ]] ||
    fail "expected the EXIT trap to write a result file on termination"
outcome="$(result_outcome)"
[[ "${outcome}" != "succeeded" && "${outcome}" != "success" ]] ||
    fail "a killed run published '${outcome}'; the seed must be pessimistic"
[[ "$(result_exit_code)" != "0" ]] ||
    fail "a killed run published exitCode 0; the seeded exit code must be non-zero"

# publish-deploy-status maps any unrecognized outcome to `failed`, so the seed
# only has to be non-success - but it must not be `unknown`, which that action
# reserves for a missing result file and maps to the neutral `in_progress`.
[[ "${outcome}" != "unknown" ]] ||
    fail "the seed must not be 'unknown'; publish-deploy-status maps that to in_progress"

# ---------------------------------------------------------------------------
# Success must be earned, not seeded.
#
# Every failure branch exits before the promotion, so `succeeded` may be
# assigned exactly once and only after the command loop.
# ---------------------------------------------------------------------------
# `grep -c` exits 1 on zero matches, which under `set -e` would abort silently -
# report the count instead so a removed promotion names itself.
success_assignments="$(grep -c '^[[:space:]]*OVERALL_OUTCOME="succeeded"$' "${ACTION_FILE}" || true)"
[[ "${success_assignments}" == "1" ]] ||
    fail "expected exactly one OVERALL_OUTCOME=\"succeeded\" assignment, found ${success_assignments}"

seed_line="$(action_line '^[[:space:]]*OVERALL_OUTCOME=' head)"
success_line="$(action_line '^[[:space:]]*OVERALL_OUTCOME="succeeded"$' head)"
last_failure_line="$(action_line '^[[:space:]]*OVERALL_OUTCOME="command_failed"$' tail)"
[[ "${success_line}" != "${seed_line}" ]] ||
    fail "OVERALL_OUTCOME is seeded to 'succeeded'; a killed run would inherit it"
[[ "${success_line}" -gt "${last_failure_line}" ]] ||
    fail "OVERALL_OUTCOME=\"succeeded\" must be assigned after the command loop"

echo "run-rad-commands command outcome tests passed"
