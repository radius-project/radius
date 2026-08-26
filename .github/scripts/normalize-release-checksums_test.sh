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

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-checksum-test-XXXXXX")"
    binary="${TEST_ROOT}/rad_linux_amd64"
    checksum="${binary}.sha256"
    artifacts="${TEST_ROOT}/artifacts.json"
    printf 'radius\n' > "${binary}"
    hash="$(sha256sum "${binary}" | cut -d ' ' -f 1)"
    printf '%s' "${hash}" > "${checksum}"
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

    bash "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}"
    [[ "$(cat "${checksum}")" == "${hash} *rad_linux_amd64" ]]
    bash "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}"
    [[ "$(cat "${checksum}")" == "${hash} *rad_linux_amd64" ]]

    printf '%064d' 0 > "${checksum}"
    if bash "${SCRIPT_DIR}/normalize-release-checksums.sh" "${artifacts}" \
        > /dev/null 2>&1; then
        echo "normalizer accepted a checksum mismatch" >&2
        exit 1
    fi
    echo "release checksum normalization tests passed"
}

main "$@"
