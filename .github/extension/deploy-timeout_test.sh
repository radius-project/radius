#!/bin/bash

# Tests that the deploy workflow templates bound the `Run rad commands` step with
# a validated timeout. Run directly or via `make test-deploy-timeout`.
#
# Whole-file greps are deliberately avoided here. `timeout-minutes` nested under
# `with:` is valid YAML that GitHub silently treats as an undeclared composite
# action input, so a grep for the key alone would pass on exactly the mistake
# these tests exist to catch. Assertions are therefore scoped to a single step's
# body, with indentation derived from the file rather than hardcoded.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly AZURE_WF="${SCRIPT_DIR}/run-rad-commands-azure.yml"
readonly AWS_WF="${SCRIPT_DIR}/run-rad-commands-aws.yml"
readonly DEPLOY_STEP="Run rad commands"
readonly RESOLVE_STEP="Resolve deploy timeout"
readonly RESOLVE_ID="deploy-timeout"
readonly PUBLISH_STEP="Publish deployed graph and status"
readonly TEARDOWN_STEP="Teardown"
readonly TIMEOUT_VAR="RADIUS_DEPLOY_TIMEOUT_MINUTES"
readonly DEFAULT_TIMEOUT="30"
readonly MAX_TIMEOUT="330"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# step_body FILE STEP
#
# Emit the body of the named step with the step's own key indentation stripped,
# so a direct key of the step starts at column 0 and anything nested under it
# (`with:`, `env:`) stays indented. That keeps the assertions below independent
# of how deeply the workflow happens to be indented. Comment lines are dropped
# so prose mentioning a key cannot satisfy an assertion.
#
# Steps are located by `- name:`, which both templates use as the first key of
# every step. If that convention changes the lookup fails loudly rather than
# silently passing.
step_body() {
    local file="$1" step="$2"
    awk -v step="${step}" '
        # Start of the target step: remember how far its keys are indented.
        !inside && /^[[:space:]]*-[[:space:]]+name:[[:space:]]/ {
            line = $0
            sub(/^[[:space:]]*-[[:space:]]+name:[[:space:]]*/, "", line)
            gsub(/^["'\'']|["'\'']$/, "", line)
            if (line == step) {
                match($0, /^[[:space:]]*-[[:space:]]+/)
                keyindent = RLENGTH
                inside = 1
            }
            next
        }
        # A later list item indented less than the step keys ends the body.
        inside && /^[[:space:]]*-[[:space:]]/ {
            match($0, /^[[:space:]]*/)
            if (RLENGTH < keyindent) exit
        }
        inside && /^[[:space:]]*#/ { next }
        inside { print substr($0, keyindent + 1) }
    ' "${file}"
}

# step_names FILE -- every step name, in file order.
step_names() {
    awk '
        /^[[:space:]]*-[[:space:]]+name:[[:space:]]/ {
            line = $0
            sub(/^[[:space:]]*-[[:space:]]+name:[[:space:]]*/, "", line)
            gsub(/^["'\'']|["'\'']$/, "", line)
            print line
        }
    ' "$1"
}

# step_position FILE STEP -- 1-based position of the step among the file's step
# names, or empty when absent. Avoids `mapfile`/arrays so the script still runs
# on the bash 3.2 that ships with macOS.
step_position() {
    step_names "$1" | grep -n -x -F -- "$2" | head -n 1 | cut -d: -f1 || true
}

for wf in "${AZURE_WF}" "${AWS_WF}"; do
    name="$(basename "${wf}")"

    # --- Ordering: validate first, clean up after the bounded step -------------
    resolve_at="$(step_position "${wf}" "${RESOLVE_STEP}")"
    deploy_at="$(step_position "${wf}" "${DEPLOY_STEP}")"
    publish_at="$(step_position "${wf}" "${PUBLISH_STEP}")"
    teardown_at="$(step_position "${wf}" "${TEARDOWN_STEP}")"

    [[ -n "${resolve_at}" ]] || fail "no '${RESOLVE_STEP}' step in ${name}"
    [[ -n "${deploy_at}" ]] || fail "no '${DEPLOY_STEP}' step in ${name}"
    [[ -n "${publish_at}" ]] || fail "no '${PUBLISH_STEP}' step in ${name}"
    [[ -n "${teardown_at}" ]] || fail "no '${TEARDOWN_STEP}' step in ${name}"

    [[ "${resolve_at}" -lt "${deploy_at}" ]] ||
        fail "'${RESOLVE_STEP}' must precede '${DEPLOY_STEP}' in ${name}; its output feeds the timeout"

    # The timeout is bound to the step rather than the job precisely so these two
    # still run after it fires. If they ever move ahead of the deploy step that
    # guarantee is gone, so pin the order.
    [[ "${deploy_at}" -lt "${publish_at}" && "${publish_at}" -lt "${teardown_at}" ]] ||
        fail "expected '${DEPLOY_STEP}' -> '${PUBLISH_STEP}' -> '${TEARDOWN_STEP}' order in ${name}"

    teardown_body="$(step_body "${wf}" "${TEARDOWN_STEP}")"
    grep -qE '^if: always\(\)' <<<"${teardown_body}" ||
        fail "'${TEARDOWN_STEP}' in ${name} must keep 'if: always()' so a timed-out deploy still persists state"

    # --- The timeout is bound to the step, not the job -------------------------
    deploy_body="$(step_body "${wf}" "${DEPLOY_STEP}")"
    [[ -n "${deploy_body}" ]] || fail "could not read the '${DEPLOY_STEP}' step body in ${name}"

    grep -qE '^uses: .*/run-rad-commands@' <<<"${deploy_body}" ||
        fail "'${DEPLOY_STEP}' in ${name} does not use the run-rad-commands action"

    # Indented relative to the step's own keys means it landed under `with:`,
    # where GitHub treats it as an undeclared action input and ignores it.
    if grep -qE '^[[:space:]]+timeout-minutes:' <<<"${deploy_body}"; then
        fail "'timeout-minutes' is nested under another key in the '${DEPLOY_STEP}' step of ${name}; it must be a direct key of the step or it is silently ignored"
    fi

    timeout_line="$(grep -E '^timeout-minutes:' <<<"${deploy_body}" || true)"
    [[ -n "${timeout_line}" ]] ||
        fail "expected 'timeout-minutes' as a direct key of the '${DEPLOY_STEP}' step in ${name}"

    # Match the whole expression rather than its parts, so a rearrangement that
    # merely happens to contain the right substrings cannot pass.
    expected_timeout="timeout-minutes: \${{ fromJSON(steps.${RESOLVE_ID}.outputs.minutes) }}"
    [[ "${timeout_line}" == "${expected_timeout}" ]] ||
        fail "unexpected timeout expression in ${name}: got '${timeout_line}', want '${expected_timeout}'"

    # A job-level timeout cancels the job. The always() teardown still runs, but
    # the `!cancelled()` publish step is skipped, so the run loses its graph and
    # status. Verified empirically, not assumed.
    if grep -qE '^ {4}timeout-minutes:' "${wf}"; then
        fail "job-level 'timeout-minutes' in ${name} cancels the job and skips the '!cancelled()' publish step; bound the deploy step instead"
    fi

    # --- The value reaching the timeout is validated ---------------------------
    # The runner applies a step timeout only when it evaluates to more than zero,
    # and treats an expression it cannot evaluate as no timeout at all. So a zero,
    # negative, malformed, or over-large value has to be rejected up front rather
    # than passed through.
    resolve_body="$(step_body "${wf}" "${RESOLVE_STEP}")"
    [[ -n "${resolve_body}" ]] || fail "could not read the '${RESOLVE_STEP}' step body in ${name}"

    grep -qE "^id: ${RESOLVE_ID}\$" <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must declare 'id: ${RESOLVE_ID}' for the timeout expression to reference it"
    # Assert the mapping itself, not just the variable name: the name also appears
    # in the error messages, so a bare name match still passes when the `env:`
    # mapping is dropped and the override is silently ignored.
    expected_env="REQUESTED: \${{ vars.${TIMEOUT_VAR} }}"
    grep -qF -- "${expected_env}" <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must map the override with '${expected_env}' or the ${TIMEOUT_VAR} variable is ignored"
    # Assert the write to GITHUB_OUTPUT, not any 'minutes=' assignment: the local
    # arithmetic assignment also matches, so a looser check passes when the output
    # write is deleted, leaving the timeout expression empty and the step unbounded.
    # shellcheck disable=SC2016  # the literal text is the assertion; no expansion wanted
    grep -qF -- 'echo "minutes=${minutes}" >> "$GITHUB_OUTPUT"' <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must write the resolved value to GITHUB_OUTPUT; without it the timeout expression evaluates to empty and the runner applies no timeout"
    expected_default="requested=\"\${REQUESTED:-${DEFAULT_TIMEOUT}}\""
    grep -qF -- "${expected_default}" <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must default with '${expected_default}' when ${TIMEOUT_VAR} is unset or empty"
    # Assert the comparisons themselves, not just the numbers: the bounds also
    # appear in the error messages, so a looser match passes when a check is
    # deleted but its message left behind.
    grep -qE "minutes < 1" <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must reject values below 1; the runner ignores a step timeout that is not greater than zero"
    grep -qE "minutes > ${MAX_TIMEOUT}" <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must reject values above ${MAX_TIMEOUT} so the step stays inside the job budget"
    grep -qE '\$\(\(10#' <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must normalize to base 10; a value like '030' is otherwise read as octal and rejected by fromJSON"
    grep -qE '\^\[0-9\]\{1,4\}\$' <<<"${resolve_body}" ||
        fail "'${RESOLVE_STEP}' in ${name} must reject non-numeric values before the arithmetic comparison"
done

echo "deploy timeout tests passed"
