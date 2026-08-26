#!/bin/bash

# Verifies the state-persistence guard shared by the restore-state and teardown
# composite actions, without a cluster or the rad CLI. The invariant: teardown
# only runs `rad shutdown` when `rad startup` restored state earlier in the same
# run, so a workflow that fails before startup cannot overwrite the durable state
# archive with blank/uninitialized state.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
readonly REPO_ROOT
readonly RESTORE_ACTION="${REPO_ROOT}/.github/extension/actions/restore-state/action.yml"
readonly TEARDOWN_ACTION="${REPO_ROOT}/.github/extension/actions/teardown/action.yml"
readonly MARKER='radius-state-restored'

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

# restore-state must write the marker, and only after `rad startup` succeeds.
grep -qF "${MARKER}" "${RESTORE_ACTION}" ||
    fail "restore-state must write the '${MARKER}' marker after rad startup"

startup_line="$(grep -n '^[[:space:]]*rad startup$' "${RESTORE_ACTION}" | head -n1 | cut -d: -f1)"
marker_line="$(grep -n "touch .*${MARKER}" "${RESTORE_ACTION}" | head -n1 | cut -d: -f1)"
[ -n "${startup_line}" ] || fail "restore-state must run 'rad startup'"
[ -n "${marker_line}" ] || fail "restore-state must 'touch' the marker"
[ "${marker_line}" -gt "${startup_line}" ] ||
    fail "the marker must be written after 'rad startup' (startup line ${startup_line}, marker line ${marker_line})"

# teardown must gate `rad shutdown` on the marker: check it, exit early when
# absent, and run shutdown only afterwards.
grep -qF "${MARKER}" "${TEARDOWN_ACTION}" ||
    fail "teardown must check the '${MARKER}' marker before rad shutdown"

guard_line="$(grep -n "! -f .*${MARKER}" "${TEARDOWN_ACTION}" | head -n1 | cut -d: -f1)"
exit_line="$(grep -n '^[[:space:]]*exit 0$' "${TEARDOWN_ACTION}" | head -n1 | cut -d: -f1)"
shutdown_line="$(grep -n '^[[:space:]]*rad shutdown$' "${TEARDOWN_ACTION}" | head -n1 | cut -d: -f1)"
[ -n "${guard_line}" ] || fail "teardown must guard on the absence of the marker file"
[ -n "${exit_line}" ] || fail "teardown must 'exit 0' when the marker is absent"
[ -n "${shutdown_line}" ] || fail "teardown must run 'rad shutdown'"
[ "${guard_line}" -lt "${exit_line}" ] ||
    fail "the marker guard must precede the early 'exit 0'"
[ "${exit_line}" -lt "${shutdown_line}" ] ||
    fail "the early 'exit 0' must precede 'rad shutdown' so shutdown is skipped when the marker is absent"

printf 'state-persistence guard tests passed\n'
