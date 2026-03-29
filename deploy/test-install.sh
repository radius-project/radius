#!/bin/bash

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

# ============================================================================
# Integration tests for deploy/install.sh
#
# Exercises every flag and environment variable documented in --help.
# Each scenario installs into an isolated temporary directory and validates
# that the rad CLI binary was placed there and is functional.
#
# Usage:
#   ./deploy/test-install.sh
#   ./deploy/test-install.sh --version 0.40.0   # override pinned version
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly SCRIPT_DIR
readonly INSTALLER="${SCRIPT_DIR}/install.sh"

# A known-good release used for pinned-version tests.
PINNED_VERSION="0.40.0"

usage() {
    echo "Usage: $0 [--version <VERSION>]"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
    --version)
        if [[ $# -lt 2 || -z "${2}" ]]; then
            echo "Error: --version requires a value" >&2
            usage >&2
            exit 1
        fi
        PINNED_VERSION="$2"
        shift 2
        ;;
    *)
        echo "Unknown option: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
done
readonly PINNED_VERSION

# Test tracking
TEST_ROOT=""
PASS=0
FAIL=0
declare -a FAILURES=()

# ── Helpers ──────────────────────────────────────────────────────────────────

# Track whether $HOME/.rad existed before tests so we only clean up
# what we created (rad bicep download writes there).
RAD_HOME_EXISTED=false

setup() {
    TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/rad-test-XXXXXX")
    if [[ -d "${HOME}/.rad" ]]; then
        RAD_HOME_EXISTED=true
    fi
}

cleanup() {
    # Restore permissions on any dirs we may have made non-writable,
    # otherwise rm -rf cannot remove them.
    if [[ -d "${TEST_ROOT:-}" ]]; then
        find "${TEST_ROOT}" -type d -exec chmod u+rwx {} + 2>/dev/null || true
        rm -rf "${TEST_ROOT}"
    fi

    # Remove $HOME/.rad if the tests created it.
    if [[ "${RAD_HOME_EXISTED}" == "false" && -d "${HOME}/.rad" ]]; then
        rm -rf "${HOME}/.rad"
    fi
}
trap cleanup EXIT

# Create a unique install directory for a test scenario.
make_test_dir() {
    local name="$1"
    local dir="${TEST_ROOT}/${name}"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Assert rad binary exists, is executable, and can report its version.
assert_rad_installed() {
    local dir="$1"
    local rad="${dir}/rad"
    local output=""

    if [[ ! -f "${rad}" ]]; then
        echo "  ASSERT FAILED: rad binary not found in ${dir}"
        return 1
    fi
    if [[ ! -x "${rad}" ]]; then
        echo "  ASSERT FAILED: rad binary is not executable"
        return 1
    fi
    if ! output=$("${rad}" version --cli 2>&1); then
        echo "  ASSERT FAILED: 'rad version --cli' exited non-zero"
        echo "${output}"
        return 1
    fi
    return 0
}

# Assert that command output contains a substring.
assert_contains() {
    local output="$1"
    local substring="$2"

    if [[ "${output}" != *"${substring}"* ]]; then
        echo "  ASSERT FAILED: output does not contain '${substring}'"
        echo "  Output was: ${output:0:200}"
        return 1
    fi
    return 0
}

# Assert that a command exits with non-zero.
assert_fails() {
    local output=""

    echo "  CMD: $*"
    if output=$("$@" 2>&1); then
        echo "  ASSERT FAILED: expected non-zero exit from: $*"
        if [[ -n "${output}" ]]; then
            echo "${output}"
        fi
        return 1
    fi
    return 0
}

# Run the installer, log the command, and store output in LAST_OUTPUT.
LAST_OUTPUT=""
run_installer() {
    local status=0

    echo "  CMD: $*"
    if LAST_OUTPUT=$("$@" 2>&1); then
        status=0
    else
        status=$?
    fi
    echo "${LAST_OUTPUT}"
    return "${status}"
}

# Run a named test function and track results.
run_test() {
    local name="$1"
    local func="$2"

    echo "────────────────────────────────────────────────────────────────"
    echo "TEST: ${name}"
    echo "────────────────────────────────────────────────────────────────"

    if ${func}; then
        echo "  ✓ PASSED"
        ((++PASS))
    else
        echo "  ✗ FAILED"
        ((++FAIL))
        FAILURES+=("${name}")
    fi
    echo ""
}

# ── Test Scenarios ───────────────────────────────────────────────────────────

test_help_long_flag() {
    run_installer "${INSTALLER}" --help
    assert_contains "${LAST_OUTPUT}" "Usage"
    assert_contains "${LAST_OUTPUT}" "--version"
    assert_contains "${LAST_OUTPUT}" "--install-dir"
    assert_contains "${LAST_OUTPUT}" "--include-rc"
}

test_help_short_flag() {
    run_installer "${INSTALLER}" -h
    assert_contains "${LAST_OUTPUT}" "Usage"
}

test_default_install() {
    local dir
    dir=$(make_test_dir "default")
    run_installer "${INSTALLER}" --install-dir "${dir}"
    assert_rad_installed "${dir}"
}

test_specific_version_long_flag() {
    local dir
    dir=$(make_test_dir "version-long")
    run_installer "${INSTALLER}" --version "${PINNED_VERSION}" \
        --install-dir "${dir}"
    assert_rad_installed "${dir}"

    local actual
    actual=$("${dir}/rad" version --cli 2>&1)
    assert_contains "${actual}" "${PINNED_VERSION}"
}

test_specific_version_short_flags() {
    local dir
    dir=$(make_test_dir "version-short")
    run_installer "${INSTALLER}" -v "${PINNED_VERSION}" -d "${dir}"
    assert_rad_installed "${dir}"

    local actual
    actual=$("${dir}/rad" version --cli 2>&1)
    assert_contains "${actual}" "${PINNED_VERSION}"
}

test_include_rc_long_flag() {
    local dir
    dir=$(make_test_dir "rc-long")
    run_installer "${INSTALLER}" --include-rc --install-dir "${dir}"
    assert_rad_installed "${dir}"
}

test_include_rc_short_flag() {
    local dir
    dir=$(make_test_dir "rc-short")
    run_installer "${INSTALLER}" -rc -d "${dir}"
    assert_rad_installed "${dir}"
}

test_install_dir_env_var() {
    local dir
    dir=$(make_test_dir "env-install-dir")
    INSTALL_DIR="${dir}" run_installer "${INSTALLER}"
    assert_rad_installed "${dir}"
}

test_include_rc_env_var() {
    local dir
    dir=$(make_test_dir "env-include-rc")
    INCLUDE_RC=true run_installer "${INSTALLER}" --install-dir "${dir}"
    assert_rad_installed "${dir}"
}

test_radius_install_dir_compat() {
    local dir
    dir=$(make_test_dir "radius-install-dir")
    RADIUS_INSTALL_DIR="${dir}" run_installer "${INSTALLER}"
    assert_rad_installed "${dir}"
}

test_reinstall_over_existing() {
    local dir
    dir=$(make_test_dir "reinstall")

    # First install
    run_installer "${INSTALLER}" --version "${PINNED_VERSION}" \
        --install-dir "${dir}"

    # Second install — should mention "Reinstalling"
    run_installer "${INSTALLER}" --version "${PINNED_VERSION}" \
        --install-dir "${dir}"
    assert_contains "${LAST_OUTPUT}" "Reinstalling"
    assert_rad_installed "${dir}"
}

test_legacy_positional_version() {
    local dir
    dir=$(make_test_dir "positional")
    run_installer "${INSTALLER}" --install-dir "${dir}" "${PINNED_VERSION}"
    assert_rad_installed "${dir}"

    local actual
    actual=$("${dir}/rad" version --cli 2>&1)
    assert_contains "${actual}" "${PINNED_VERSION}"
}

test_unknown_flag_fails() {
    assert_fails "${INSTALLER}" --bogus
}

test_path_hint_shown() {
    local dir
    dir=$(make_test_dir "path-hint")
    run_installer "${INSTALLER}" --version "${PINNED_VERSION}" \
        --install-dir "${dir}"
    assert_rad_installed "${dir}"
    assert_contains "${LAST_OUTPUT}" "not in your"
}

# ── Non-privileged Environment Tests ────────────────────────────────────────

test_default_dir_nonroot() {
    # When not root and no INSTALL_DIR is set, the installer should
    # default to $HOME/.local/bin (not /usr/local/bin).
    if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
        echo "  SKIP: test requires non-root user"
        return 0
    fi

    local fake_home
    fake_home=$(make_test_dir "fakehome")

    # Pre-create the expected default directory so needsSudo() sees a
    # writable dir and doesn't try to escalate.
    mkdir -p "${fake_home}/.local/bin"

    HOME="${fake_home}" run_installer "${INSTALLER}" \
        --version "${PINNED_VERSION}"
    assert_rad_installed "${fake_home}/.local/bin"
}

test_nonwritable_dir_no_sudo_fails() {
    # Installing to a non-writable directory when sudo is unavailable
    # should fail with "requires root privileges".
    if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
        echo "  SKIP: test requires non-root user"
        return 0
    fi

    # Both the target AND its parent must be non-writable so needsSudo()
    # returns true (it checks both).
    local parent
    parent=$(make_test_dir "noperm-parent")
    local dir="${parent}/bin"
    mkdir -p "${dir}"
    chmod 555 "${dir}" "${parent}"

    # Build a PATH containing only the tools the installer needs,
    # notably excluding sudo.
    local bin_dir
    bin_dir=$(make_test_dir "nosudo-bin")
    local tool tool_path
    for tool in bash curl wget chmod cp rm mkdir mktemp uname id \
        tr grep awk sed dirname basename cat ln; do
        tool_path=$(command -v "${tool}" 2>/dev/null) || continue
        ln -sf "${tool_path}" "${bin_dir}/"
    done

    local status=0
    echo "  CMD: PATH=<no-sudo> ${INSTALLER} --version ${PINNED_VERSION} --install-dir ${dir}"
    if LAST_OUTPUT=$(PATH="${bin_dir}" "${INSTALLER}" --version "${PINNED_VERSION}" \
        --install-dir "${dir}" 2>&1); then
        status=0
    else
        status=$?
    fi
    echo "${LAST_OUTPUT}"

    chmod 755 "${parent}" "${dir}"

    if [[ "${status}" -eq 0 ]]; then
        echo "  ASSERT FAILED: expected non-zero exit"
        return 1
    fi
    assert_contains "${LAST_OUTPUT}" "requires root privileges"
}

# ── Main ─────────────────────────────────────────────────────────────────────

echo "============================================================================"
echo "Radius CLI Installer Integration Tests"
echo "============================================================================"
echo "Installer: ${INSTALLER}"
echo "Pinned version: ${PINNED_VERSION}"
echo ""

setup

run_test "help (--help)" test_help_long_flag
run_test "help (-h)" test_help_short_flag
run_test "default install (latest stable)" test_default_install
run_test "specific version (--version)" test_specific_version_long_flag
run_test "short flags (-v, -d)" test_specific_version_short_flags
run_test "include RC (--include-rc)" test_include_rc_long_flag
run_test "include RC (-rc)" test_include_rc_short_flag
run_test "INSTALL_DIR env var" test_install_dir_env_var
run_test "INCLUDE_RC env var" test_include_rc_env_var
run_test "RADIUS_INSTALL_DIR backward compat" test_radius_install_dir_compat
run_test "reinstall over existing binary" test_reinstall_over_existing
run_test "legacy positional version arg" test_legacy_positional_version
run_test "unknown flag exits non-zero" test_unknown_flag_fails
run_test "PATH hint shown for non-PATH dir" test_path_hint_shown
run_test "default dir is ~/.local/bin for non-root" test_default_dir_nonroot
run_test "non-writable dir without sudo fails" test_nonwritable_dir_no_sudo_fails

echo "============================================================================"
echo "Results: ${PASS} passed, ${FAIL} failed"
echo "============================================================================"

if ((FAIL > 0)); then
    echo ""
    echo "Failed tests:"
    for name in "${FAILURES[@]}"; do
        echo "  - ${name}"
    done
    exit 1
fi
