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

ARTIFACTS_FILE="${1:-dist/goreleaser/artifacts.json}"

fail() {
    echo "Error: $*" >&2
    exit 1
}

main() {
    local name
    local binary_path
    local checksum_path
    local checksum
    local actual

    command -v jq > /dev/null || fail "required command not found: jq"
    if ! command -v sha256sum > /dev/null; then
        fail "required command not found: sha256sum"
    fi
    if [[ ! -f "${ARTIFACTS_FILE}" ]]; then
        fail "artifacts file not found: ${ARTIFACTS_FILE}"
    fi

    while IFS= read -r name; do
        binary_path="$(jq -er --arg name "${name}" '
            [
                .[]
                | select(
                    .name == $name
                    and .type == "Binary"
                    and .extra.ID == "rad"
                )
            ]
            | select(length == 1)
            | .[0].path
        ' "${ARTIFACTS_FILE}")"
        checksum_path="$(jq -er \
            --arg name "${name}.sha256" \
            --arg binary_path "${binary_path}" '
            [
                .[]
                | select(
                    .name == $name
                    and .type == "Checksum"
                    and .extra.ChecksumOf == $binary_path
                )
            ]
            | select(length == 1)
            | .[0].path
        ' "${ARTIFACTS_FILE}")"
        if [[ ! -f "${binary_path}" ]]; then
            fail "binary not found: ${binary_path}"
        fi
        if [[ ! -f "${checksum_path}" ]]; then
            fail "checksum not found: ${checksum_path}"
        fi
        checksum="$(awk 'NR == 1 { print $1 }' "${checksum_path}")"
        if [[ ! "${checksum}" =~ ^[0-9a-f]{64}$ ]]; then
            fail "invalid checksum format: ${checksum_path}"
        fi
        actual="$(sha256sum "${binary_path}" | cut -d ' ' -f 1)"
        if [[ "${checksum}" != "${actual}" ]]; then
            fail "checksum mismatch for ${name}"
        fi
        printf '%s *%s\n' "${checksum}" "${name}" > "${checksum_path}"
    done < <(jq -r '
        .[]
        | select(
            .type == "Binary"
            and .extra.ID == "rad"
            and (.name | startswith("rad_"))
        )
        | .name
    ' "${ARTIFACTS_FILE}")
}

main "$@"
