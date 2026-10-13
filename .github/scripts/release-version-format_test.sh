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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly VALIDATOR="${SCRIPT_DIR}/validate_semver.py"
readonly TAG_PARSER="${SCRIPT_DIR}/get_release_version.py"
readonly RUNBOOK="${SCRIPT_DIR}/../../docs/contributing/contributing-releases/README.md"
# shellcheck source=.github/scripts/release-version.sh
source "${SCRIPT_DIR}/release-version.sh"

if [[ -z "${PYTHON:-}" ]]; then
    if command -v python3 > /dev/null; then
        PYTHON="python3"
    else
        PYTHON="python"
    fi
fi
readonly PYTHON

TEMP_DIR=""

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_valid() {
    local version="$1"

    if ! "${PYTHON}" "${VALIDATOR}" "${version}" > /dev/null; then
        fail_test "expected valid SemVer: ${version}"
    fi
}

assert_invalid() {
    local version="$1"

    if "${PYTHON}" "${VALIDATOR}" "${version}" > /dev/null 2>&1; then
        fail_test "expected invalid SemVer: ${version}"
    fi
}

assert_tag_parser() {
    local version="$1"
    local environment_file="${TEMP_DIR}/github-env"

    : > "${environment_file}"
    GITHUB_REF="refs/tags/v${version}" GITHUB_ENV="${environment_file}" \
        "${PYTHON}" "${TAG_PARSER}" > /dev/null

    if ! grep -Fxq "REL_VERSION=${version}" "${environment_file}"; then
        fail_test "tag parser did not preserve release version ${version}"
    fi
    if ! grep -Fxq "REL_CHANNEL=${version}" "${environment_file}"; then
        fail_test "tag parser did not preserve release channel ${version}"
    fi
    if ! grep -Fxq "CHART_VERSION=${version}" "${environment_file}"; then
        fail_test "tag parser did not preserve chart version ${version}"
    fi
}

assert_radius_valid() {
    local version="$1"

    if ! is_radius_release_version "${version}"; then
        fail_test "expected valid Radius release version: ${version}"
    fi
}

assert_radius_invalid() {
    local version="$1"

    if is_radius_release_version "${version}"; then
        fail_test "expected invalid Radius release version: ${version}"
    fi
}

assert_policy_callers() {
    local script

    for script in \
        release-get-version.sh \
        release-verification.sh \
        checkout-release-codebase.sh; do
        if ! grep -Fq "source \"\${SCRIPT_DIR}/release-version.sh\"" \
            "${SCRIPT_DIR}/${script}"; then
            fail_test "${script} does not use the shared release-version policy"
        fi
    done
}

assert_runbook_uses_dotted_rc() {
    local legacy_lines
    local version

    legacy_lines="$(grep -nE 'rc[0-9]' "${RUNBOOK}" || true)"
    if [[ -n "${legacy_lines}" ]]; then
        fail_test "release runbook shows legacy RC identifiers:"$'\n'"${legacy_lines}"
    fi

    while read -r version; do
        assert_radius_valid "${version#v}"
        if is_legacy_rc_version "${version#v}"; then
            fail_test "release runbook example uses a legacy RC version: ${version}"
        fi
    done < <(grep -oE "version: 'v[0-9][^']*'" "${RUNBOOK}" | grep -oE "v[0-9][^']*")
}

main() {
    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-version-format-test-XXXXXX")"

    assert_valid "0.61.0"
    assert_valid "0.61.0-rc.1"
    assert_valid "0.61.0-rc1"
    assert_valid "0.61.0-beta.1"
    assert_valid "0.61.0-rc.0"
    assert_valid "0.61.0-rc.2+build.7"
    assert_invalid "v0.61.0-rc.1"
    assert_invalid "0.61.0-rc.01"
    assert_invalid "0.61.0-rc..1"
    assert_invalid "0.61.0-rc_1"

    assert_tag_parser "0.61.0-rc.1"
    assert_tag_parser "0.60.0-rc3"

    assert_radius_valid "0.61.0"
    assert_radius_valid "0.61.0-rc.1"
    assert_radius_valid "0.60.0-rc3"
    assert_radius_invalid "v0.61.0-rc.1"
    assert_radius_invalid "0.61.0-beta.1"
    assert_radius_invalid "0.61.0-rc.0"
    assert_radius_invalid "0.61.0-rc0"
    assert_radius_invalid "0.61.0-rc.01"
    if [[ "$(canonical_radius_rc_version "0.60.0-rc3")" != "0.60.0-rc.3" ]]; then
        fail_test "legacy RC canonicalization failed"
    fi
    assert_policy_callers
    assert_runbook_uses_dotted_rc

    echo "release version format tests passed (24 tests)"
}

main "$@"
