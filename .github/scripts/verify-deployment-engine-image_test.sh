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
readonly SCRIPT="${SCRIPT_DIR}/verify-deployment-engine-image.sh"
readonly DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

TEST_ROOT=""
PASS=0
FAIL=0

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

write_fake_docker() {
    mkdir -p "${TEST_ROOT}/bin"
    cat >"${TEST_ROOT}/bin/docker" <<EOF
#!/bin/bash
set -euo pipefail
if [[ "\${DE_IMAGE_MODE:-complete}" == "missing" ]]; then
    echo "manifest unknown" >&2
    exit 1
fi
variant='{"os":"linux","architecture":"arm","variant":"v7"}'
if [[ "\${DE_IMAGE_MODE:-complete}" == "bad-platforms" ]]; then
    variant='{"os":"linux","architecture":"arm64"}'
fi
cat <<JSON
{"manifest":{"digest":"${DIGEST}","manifests":[
  {"platform":{"os":"linux","architecture":"amd64"}},
  {"platform":{"os":"linux","architecture":"arm64"}},
  {"platform":\${variant}}
]},"image":{"linux/amd64":{"config":{"Labels":{
    "org.opencontainers.image.ref":"refs/tags/v0.61.0-rc.1",
    "org.opencontainers.image.revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}},
    "linux/arm64":{"config":{"Labels":{
    "org.opencontainers.image.ref":"refs/tags/v0.61.0-rc.1",
    "org.opencontainers.image.revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}},
    "linux/arm/v7":{"config":{"Labels":{
    "org.opencontainers.image.ref":"refs/tags/v0.61.0-rc.1",
    "org.opencontainers.image.revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}}
JSON
EOF
    chmod +x "${TEST_ROOT}/bin/docker"
}

run_verifier() {
    local mode="$1"
    shift

    DE_IMAGE_MODE="${mode}" RELEASE_RETRY_ATTEMPTS=1 \
        RELEASE_RETRY_NO_SLEEP=true PATH="${TEST_ROOT}/bin:${PATH}" \
        bash "${SCRIPT}" --tag 0.61.0-rc.1 \
        --signed-tag v0.61.0-rc.1 \
        --source-commit aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
        --output "${TEST_ROOT}/image.json" "$@"
}

main() {
    local state

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/de-image-test-XXXXXX")"
    state="${TEST_ROOT}/state.txt"
    write_fake_docker
    if ! run_verifier complete --expected-digest "${DIGEST}" >/dev/null; then
        fail_test "valid Deployment Engine image was rejected"
    elif [[ "$(jq -r .digest "${TEST_ROOT}/image.json")" != "${DIGEST}" ]]; then
        fail_test "verified image digest was not recorded"
    else
        ((++PASS))
    fi

    if ! run_verifier missing --allow-absent --state-output "${state}" \
        >/dev/null; then
        fail_test "confirmed missing image was not accepted in preflight"
    elif [[ "$(<"${state}")" != "absent" ]]; then
        fail_test "missing image state was not recorded"
    else
        ((++PASS))
    fi

    if run_verifier complete --expected-digest \
        "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" \
        >/dev/null 2>&1; then
        fail_test "moved Deployment Engine image was accepted"
    else
        ((++PASS))
    fi

    if run_verifier bad-platforms >/dev/null 2>&1; then
        fail_test "wrong Deployment Engine platform set was accepted"
    else
        ((++PASS))
    fi

    if ((FAIL > 0)); then
        echo "Deployment Engine image tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Deployment Engine image tests passed (${PASS} tests)"
}

main "$@"
