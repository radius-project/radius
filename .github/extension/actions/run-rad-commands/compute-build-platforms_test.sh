#!/bin/bash

# Tests for compute-build-platforms.sh (the effective container build platform
# resolver) and for the workflow/action wiring that feeds it. Run directly or via
# `make test-build-platforms`.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT="${SCRIPT_DIR}/compute-build-platforms.sh"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
readonly REPO_ROOT
readonly AZURE_WF="${REPO_ROOT}/.github/extension/run-rad-commands-azure.yml"
readonly AWS_WF="${REPO_ROOT}/.github/extension/run-rad-commands-aws.yml"
readonly ACTION="${SCRIPT_DIR}/action.yml"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# assert_platforms MODE FALLBACK ARCHES EXPECTED
assert_platforms() {
    local mode="$1" fallback="$2" arches="$3" expected="$4"
    local actual
    actual="$(bash "${SCRIPT}" "${mode}" "${fallback}" "${arches}")"
    if [[ "${actual}" != "${expected}" ]]; then
        fail "compute('${mode}','${fallback}','${arches}') = '${actual}', expected '${expected}'"
    fi
}

DEFAULT="linux/amd64,linux/arm64"

# --- Feature disabled -------------------------------------------------------
# Empty mode and an unsubstituted placeholder both emit nothing so the recipe's
# default platforms apply (existing behavior preserved).
assert_platforms "" "" "" ""
assert_platforms "" "${DEFAULT}" "amd64" ""
assert_platforms "{{TARGET_CLUSTER_ARCH_MODE}}" "${DEFAULT}" "amd64 arm64" ""

# --- detect: single-arch cluster -> that single platform, no emulation ------
assert_platforms "detect" "${DEFAULT}" "amd64" "linux/amd64"
assert_platforms "detect" "${DEFAULT}" "arm64" "linux/arm64"
assert_platforms "detect" "${DEFAULT}" "x86_64" "linux/amd64"
assert_platforms "detect" "${DEFAULT}" "aarch64" "linux/arm64"
# Multiple nodes, all the same arch.
assert_platforms "detect" "${DEFAULT}" "amd64
amd64
amd64" "linux/amd64"

# --- detect: mixed / undetermined -> fallback -------------------------------
assert_platforms "detect" "${DEFAULT}" "amd64 arm64" "${DEFAULT}"
assert_platforms "detect" "${DEFAULT}" "" "${DEFAULT}"
# An unknown architecture is not classified confidently -> fallback.
assert_platforms "detect" "${DEFAULT}" "ppc64le" "${DEFAULT}"
# One known plus one unknown is still not confidently single-arch -> fallback.
assert_platforms "detect" "${DEFAULT}" "amd64 ppc64le" "${DEFAULT}"

# --- detect: custom / placeholder fallback ----------------------------------
assert_platforms "detect" "linux/arm64" "amd64 arm64" "linux/arm64"
assert_platforms "detect" "{{TARGET_CLUSTER_ARCH_FALLBACK_PLATFORMS}}" "amd64 arm64" "${DEFAULT}"

# --- Explicit override: honored verbatim, no detection ----------------------
assert_platforms "linux/amd64" "${DEFAULT}" "arm64" "linux/amd64"
assert_platforms "linux/amd64,linux/arm64" "${DEFAULT}" "" "${DEFAULT}"
# Whitespace and duplicates are normalized (trimmed, de-duplicated, sorted).
assert_platforms "linux/amd64, linux/amd64 , linux/arm64" "${DEFAULT}" "" "${DEFAULT}"

# --- Unrecognized mode -> fail safe to fallback, with a warning -------------
unrecognized_err="$(bash "${SCRIPT}" "bogus" "${DEFAULT}" "amd64" 2>&1 >/dev/null)"
assert_platforms "bogus" "${DEFAULT}" "amd64" "${DEFAULT}"
echo "${unrecognized_err}" | grep -q "unrecognized mode" ||
    fail "expected a warning on stderr for an unrecognized mode"

# --- Workflow / action wiring ------------------------------------------------
for wf in "${AZURE_WF}" "${AWS_WF}"; do
    grep -q "{{TARGET_CLUSTER_ARCH_MODE}}" "${wf}" ||
        fail "expected TARGET_CLUSTER_ARCH_MODE placeholder in $(basename "${wf}")"
    grep -q "{{TARGET_CLUSTER_ARCH_FALLBACK_PLATFORMS}}" "${wf}" ||
        fail "expected TARGET_CLUSTER_ARCH_FALLBACK_PLATFORMS placeholder in $(basename "${wf}")"
    grep -q "build-arch-mode:" "${wf}" ||
        fail "expected build-arch-mode wired to run-rad-commands in $(basename "${wf}")"
    grep -q "build-fallback-platforms:" "${wf}" ||
        fail "expected build-fallback-platforms wired to run-rad-commands in $(basename "${wf}")"
done

grep -q "build-arch-mode:" "${ACTION}" ||
    fail "expected build-arch-mode input declared in run-rad-commands action.yml"
grep -q "build-fallback-platforms:" "${ACTION}" ||
    fail "expected build-fallback-platforms input declared in run-rad-commands action.yml"
grep -q "compute-build-platforms.sh" "${ACTION}" ||
    fail "expected the detection step to invoke compute-build-platforms.sh"
grep -q "RADIUS_EFFECTIVE_BUILD_PLATFORMS" "${ACTION}" ||
    fail "expected the detection step to export RADIUS_EFFECTIVE_BUILD_PLATFORMS"

echo "compute-build-platforms tests passed"
