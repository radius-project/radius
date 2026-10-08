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
TEST_ROOT=""

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

main() {
    local binary
    local checksum
    local artifacts
    local hash
    local hash_tool
    local command_name
    local command_dir
    local bash_command
    local tested_tools=0

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-checksum-test-XXXXXX")"
    binary="${TEST_ROOT}/rad_linux_amd64"
    checksum="${binary}.sha256"
    artifacts="${TEST_ROOT}/artifacts.json"
    printf 'abc' > "${binary}"
    hash="ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
    bash_command="$(command -v bash)"
    cat > "${artifacts}" << EOF
[
  {
    "name":"rad_linux_amd64",
    "type":"Binary",
    "path":"${binary}",
    "extra":{"ID":"rad"}
  },
  {
    "name":"rad_linux_amd64.sha256",
    "type":"Checksum",
    "path":"${checksum}",
    "extra":{"ChecksumOf":"${binary}"}
  }
]
EOF

    for hash_tool in shasum openssl sha256sum; do
        if ! command -v "${hash_tool}" > /dev/null; then
            echo "Skipping unavailable checksum tool: ${hash_tool}"
            continue
        fi
        command_dir="${TEST_ROOT}/${hash_tool}"
        mkdir -p "${command_dir}"
        for command_name in jq awk cut "${hash_tool}"; do
            ln -s "$(command -v "${command_name}")" \
                "${command_dir}/${command_name}"
        done

        printf '%s' "${hash}" > "${checksum}"
        PATH="${command_dir}" "${bash_command}" \
            "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}"
        [[ "$(cat "${checksum}")" == "${hash} *rad_linux_amd64" ]]
        PATH="${command_dir}" "${bash_command}" \
            "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}"
        [[ "$(cat "${checksum}")" == "${hash} *rad_linux_amd64" ]]

        printf '%064d' 0 > "${checksum}"
        if PATH="${command_dir}" "${bash_command}" \
            "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}" \
            > "${TEST_ROOT}/error" 2>&1; then
            echo "normalizer accepted a checksum mismatch with ${hash_tool}" >&2
            exit 1
        fi
        grep -Fq 'checksum mismatch for rad_linux_amd64' "${TEST_ROOT}/error"
        [[ "$(cat "${checksum}")" == "$(printf '%064d' 0)" ]]
        ((++tested_tools))
        echo "Checksum normalization passed with ${hash_tool}"
    done

    if ((tested_tools == 0)); then
        echo "checksum tests require sha256sum, shasum, or openssl" >&2
        exit 1
    fi

    command_dir="${TEST_ROOT}/no-hash-tool"
    mkdir -p "${command_dir}"
    ln -s "$(command -v jq)" "${command_dir}/jq"
    printf '%s' "${hash}" > "${checksum}"
    if PATH="${command_dir}" "${bash_command}" \
        "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}" \
        > "${TEST_ROOT}/error" 2>&1; then
        echo "normalizer accepted missing checksum tools" >&2
        exit 1
    fi
    grep -Fq 'required command not found: one of sha256sum shasum openssl' \
        "${TEST_ROOT}/error"
    [[ "$(cat "${checksum}")" == "${hash}" ]]
    echo "release checksum normalization tests passed"
}

main "$@"
