#!/bin/bash

# Tests the result-accumulator lifecycle in the run-rad-commands composite
# action: the outcome written by the EXIT trap must never claim success for a
# run that exited before it completed.
#
# Any exit before the promotion bypasses the failure branches that assign
# `command_failed` - but the EXIT trap still fires and writes the accumulator as
# it stands. GitHub runs `shell: bash` steps as `bash --noprofile --norc -eo
# pipefail {0}`, so the reachable case is an `errexit` abort; a step timeout or
# a cancellation behaves the same way. The accumulator must therefore be
# pessimistic: seeded to a non-success value and promoted to `succeeded` only
# after the command loop has actually completed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# Overridable so the suite can be pointed at a mutated copy of the action to
# confirm the static assertions below actually bite.
ACTION_FILE="${ACTION_FILE:-${SCRIPT_DIR}/action.yml}"
readonly ACTION_FILE
readonly STEP_NAME="Run rad commands"
# The step sources this before the accumulator, and `cleanup` calls into it, so
# the extracted prologue needs it too. Sourced for real rather than stubbed, so
# a change to those helpers is exercised here instead of hidden behind a stub.
readonly PROGRESS_LIB="${SCRIPT_DIR}/../deploy-progress/progress.sh"
# The value the accumulator is seeded to, and the value the promotion is allowed
# to promote *from*. Asserted exactly rather than as "anything non-success":
# publish-deploy-status, the design note, and the promotion guard all name it.
readonly SEED_OUTCOME="interrupted"
readonly SEED_EXIT="1"
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly PROLOGUE="${TEST_ROOT}/prologue.sh"
readonly RESULT_FILE="${TEST_ROOT}/rad-commands-result.json"
readonly READY_FILE="${TEST_ROOT}/ready"
readonly VICTIM_LOG="${TEST_ROOT}/victim.log"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v jq >/dev/null 2>&1; then
    fail "jq is required; run 'make install-jq'"
fi
if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 is required to parse action.yml"
fi
[[ -f "${PROGRESS_LIB}" ]] ||
    fail "expected the deploy-progress helpers at ${PROGRESS_LIB}; the accumulator's EXIT trap calls into them"

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
# A run that exits mid-command must not publish success.
#
# Models an abnormal exit faithfully: the prologue runs, a command is in flight,
# and the shell dies without any failure branch executing - only the EXIT trap
# gets to speak. SIGTERM stands in for the whole class (step timeout, job
# cancellation); the `errexit` case is covered separately below. The subshell
# uses the same options GitHub gives a `shell: bash` step (`-e -o pipefail`).
# ---------------------------------------------------------------------------
# The victim's own output is not asserted on, so send it to a log rather than
# inheriting this script's stdout. SIGTERM reaps the subshell but not the
# `sleep` it is waiting on, and an orphan holding the pipe open would stall any
# caller that captures our output until the sleep expires.
# RUNNER_TEMP keeps the progress helpers' scratch files inside the sandbox.
ENVIRONMENT="aks-dev" RUNNER_TEMP="${TEST_ROOT}" bash -eo pipefail -c "
    source '${PROGRESS_LIB}'
    source '${PROLOGUE}'
    touch '${READY_FILE}'
    # Stands in for a long-running \`rad deploy\`.
    sleep 60
" >"${VICTIM_LOG}" 2>&1 &
victim=$!

# Kill only once the trap is installed, so the test cannot race ahead of the
# code it is exercising and mistake a never-started run for a killed one.
for _ in $(seq 1 100); do
    [[ -f "${READY_FILE}" ]] && break
    sleep 0.1
done
[[ -f "${READY_FILE}" ]] ||
    fail "the extracted prologue never finished installing the EXIT trap$(printf '\n%s' "$(cat "${VICTIM_LOG}" 2>/dev/null)")"

# The result file is written by the trap, not eagerly. If that ever changes,
# the assertions below would be testing a file the kill had no part in.
if [[ -f "${RESULT_FILE}" ]]; then
    fail "the result file must be written by the EXIT trap, not eagerly"
fi

kill -TERM "${victim}" 2>/dev/null || true
wait "${victim}" 2>/dev/null || true

[[ -f "${RESULT_FILE}" ]] ||
    fail "expected the EXIT trap to write a result file on termination"

# Assert the seed exactly. `interrupted` is not an arbitrary non-success value:
# publish-deploy-status maps it to `failed` through its `*)` arm, it must not be
# `unknown` (which that action reserves for a missing result file and maps to
# the neutral `in_progress`), and the promotion in action.yml is guarded to fire
# only from this exact value.
outcome="$(result_outcome)"
[[ "${outcome}" == "${SEED_OUTCOME}" ]] ||
    fail "a killed run published '${outcome}'; expected the seed '${SEED_OUTCOME}'"
exit_code="$(result_exit_code)"
[[ "${exit_code}" == "${SEED_EXIT}" ]] ||
    fail "a killed run published exitCode '${exit_code}'; expected '${SEED_EXIT}'"

# ---------------------------------------------------------------------------
# An `errexit` abort must not publish success either.
#
# This is the reachable case today: GitHub runs `shell: bash` steps as
# `bash --noprofile --norc -eo pipefail {0}`, so any unguarded non-zero command
# ends the step mid-run. No failure branch assigns an outcome, and the trap
# still fires.
# ---------------------------------------------------------------------------
rm -f "${RESULT_FILE}"
ENVIRONMENT="aks-dev" RUNNER_TEMP="${TEST_ROOT}" bash -eo pipefail -c "
    source '${PROGRESS_LIB}'
    source '${PROLOGUE}'
    # Stands in for any unguarded command that returns non-zero.
    false
    # Unreachable under errexit; present so a broken -e would be visible.
    OVERALL_OUTCOME=\"succeeded\"
" && fail "the errexit run exited 0; the shell must abort on the failed command"

[[ -f "${RESULT_FILE}" ]] ||
    fail "expected the EXIT trap to write a result file on an errexit abort"
outcome="$(result_outcome)"
[[ "${outcome}" == "${SEED_OUTCOME}" ]] ||
    fail "an errexit abort published '${outcome}'; expected the seed '${SEED_OUTCOME}'"
exit_code="$(result_exit_code)"
[[ "${exit_code}" == "${SEED_EXIT}" ]] ||
    fail "an errexit abort published exitCode '${exit_code}'; expected '${SEED_EXIT}'"

# The seed must be the value the prologue actually assigns, and only that value,
# so the assertions above cannot drift from the shipped code.
seed_assignments="$(grep -c "^OVERALL_OUTCOME=\"${SEED_OUTCOME}\"$" "${PROLOGUE}" || true)"
[[ "${seed_assignments}" == "1" ]] ||
    fail "expected exactly one OVERALL_OUTCOME=\"${SEED_OUTCOME}\" seed in the prologue, found ${seed_assignments}"

# ---------------------------------------------------------------------------
# Success must be earned, not seeded - and the promotion must be guarded.
#
# The failure branches all exit before the promotion today, but the promotion
# does not get to depend on that: it is guarded so it can only ever fire from
# the seed. Assert the guard rather than trying to prove reachability by
# scanning the source, which indentation cannot establish in shell.
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
    fail "OVERALL_OUTCOME is seeded to 'succeeded'; an interrupted run would inherit it"
[[ "${success_line}" -gt "${last_failure_line}" ]] ||
    fail "OVERALL_OUTCOME=\"succeeded\" must be assigned after the command loop"

# The promotion fires only from the seed, so an outcome recorded by a failure
# path that did not exit survives instead of being overwritten.
guard_line="$(action_line "^[[:space:]]*if \[ \"\$OVERALL_OUTCOME\" = \"${SEED_OUTCOME}\" \]; then\$" tail)"
[[ "$((success_line - guard_line))" == "1" ]] ||
    fail "OVERALL_OUTCOME=\"succeeded\" (line ${success_line}) must be guarded by the seed test on the line above it (found the guard on line ${guard_line})"

# Exit on the accumulator, so the step result cannot disagree with the artifact.
# shellcheck disable=SC2016  # matching the literal text `exit "$OVERALL_EXIT"`
exit_line="$(action_line '^[[:space:]]*exit "\$OVERALL_EXIT"$' tail)"
[[ "${exit_line}" -gt "${success_line}" ]] ||
    fail "the run block must end with 'exit \"\$OVERALL_EXIT\"' after the promotion"

echo "run-rad-commands command outcome tests passed"
