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

TARGET_FILE="${1:-}"
EXISTING_FILE="${2:-}"
TEMP_DIR=""

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

fail() {
    echo "Error: $*" >&2
    exit 1
}

preserve_section() {
    local pattern="$1"
    local name="$2"
    local section_file="${TEMP_DIR}/${name}.md"
    local output_file="${TEMP_DIR}/${name}-output.md"
    local target_count existing_count

    target_count="$(grep -Ec "${pattern}" "${TARGET_FILE}")"
    existing_count="$(grep -Ec "${pattern}" "${EXISTING_FILE}")"
    if [[ "${target_count}" != "1" || "${existing_count}" != "1" ]]; then
        fail "${name} heading must appear exactly once in both files"
    fi

    awk -v pattern="${pattern}" '
        $0 ~ pattern { active = 1; next }
        active && /^## / { exit }
        active { print }
    ' "${EXISTING_FILE}" > "${section_file}"

    awk -v pattern="${pattern}" -v section="${section_file}" '
        $0 ~ pattern {
            print
            while ((getline line < section) > 0) { print line }
            close(section)
            replacing = 1
            next
        }
        replacing && /^## / { replacing = 0 }
        replacing { next }
        { print }
    ' "${TARGET_FILE}" > "${output_file}"
    mv "${output_file}" "${TARGET_FILE}"
}

main() {
    [[ -f "${TARGET_FILE}" ]] || fail "generated release note not found"
    [[ -f "${EXISTING_FILE}" ]] || fail "existing release note not found"
    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-note-sections-XXXXXX")"

    preserve_section '^## Highlights$' highlights
    preserve_section '^## Upgrading to Radius ' upgrading
}

main "$@"
