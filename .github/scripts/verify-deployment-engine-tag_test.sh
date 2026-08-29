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
readonly SCRIPT="${SCRIPT_DIR}/verify-deployment-engine-tag.sh"

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

write_fake_gh() {
    local type="$1"
    local verified="$2"
    local target_type="${3:-commit}"
    local target_sha="${4:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
    local signed_tag="${5:-v0.61.0-rc.1}"

    cat >"${TEST_ROOT}/gh" <<EOF
#!/bin/bash
set -euo pipefail
if [[ "\$*" == *"git/ref/tags/"* ]]; then
    printf '%s\n' '{"object":{"type":"${type}","sha":"tag-object"}}'
else
    printf '%s\n' '{"tag":"${signed_tag}","verification":{"verified":${verified}},"object":{"type":"${target_type}","sha":"${target_sha}"}}'
fi
EOF
    chmod +x "${TEST_ROOT}/gh"
}

write_missing_fake_gh() {
    cat >"${TEST_ROOT}/gh" <<'EOF'
#!/bin/bash
if [[ "$*" == *"git/ref/tags/"* ]]; then
    exit 1
fi
printf '%s\n' 'azure-octo/deployment-engine'
EOF
    chmod +x "${TEST_ROOT}/gh"
}

write_unreadable_fake_gh() {
    cat > "${TEST_ROOT}/gh" << 'EOF'
#!/bin/bash
echo "gh: Not Found (HTTP 404)" >&2
exit 1
EOF
    chmod +x "${TEST_ROOT}/gh"
}

test_accepts_verified_annotated_tag() {
    local outputs="${TEST_ROOT}/outputs"

    write_fake_gh tag true
    if ! GITHUB_OUTPUT="${outputs}" GH="${TEST_ROOT}/gh" \
        bash "${SCRIPT}" v0.61.0-rc.1 \
        >/dev/null; then
        fail_test "expected a verified annotated tag to pass"
        return
    fi
    if [[ "$(<"${outputs}")" != "commit=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ]]; then
        fail_test "verified tag target commit was not emitted"
        return
    fi
    ((++PASS))
}

test_rejects_lightweight_tag() {
    write_fake_gh commit true
    if GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0 >/dev/null 2>&1; then
        fail_test "expected a lightweight tag to fail"
        return
    fi
    ((++PASS))
}

test_rejects_unverified_tag() {
    write_fake_gh tag false
    if GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0 >/dev/null 2>&1; then
        fail_test "expected an unverified tag to fail"
        return
    fi
    ((++PASS))
}

test_rejects_non_commit_tag_target() {
    write_fake_gh tag true tree aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
        v0.61.0
    if GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0 \
        >/dev/null 2>&1; then
        fail_test "expected a non-commit tag target to fail"
        return
    fi
    ((++PASS))
}

test_rejects_mismatched_signed_tag_name() {
    write_fake_gh tag true commit \
        aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa v0.60.0
    if GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0-rc.1 \
        >/dev/null 2>&1; then
        fail_test "expected a mismatched signed tag name to fail"
        return
    fi
    ((++PASS))
}

test_missing_tag_prints_recovery_command() {
    local output

    write_missing_fake_gh
    set +e
    output="$(GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0-rc.1 2>&1)"
    local status=$?
    set -e
    if ((status == 0)); then
        fail_test "expected a missing tag to fail"
        return
    fi
    if [[ "${output}" != *"git tag -s v0.61.0-rc.1"* ||
        "${output}" != *"git push origin v0.61.0-rc.1"* ]]; then
        fail_test "missing-tag error did not include the recovery command"
        return
    fi
    ((++PASS))
}

test_unreadable_repository_is_not_reported_as_missing_tag() {
    local output

    write_unreadable_fake_gh
    set +e
    output="$(GH="${TEST_ROOT}/gh" bash "${SCRIPT}" v0.61.0-rc.1 2>&1)"
    local status=$?
    set -e
    if ((status == 0)); then
        fail_test "expected an unreadable repository to fail"
        return
    fi
    if [[ "${output}" != *"cannot read azure-octo/deployment-engine"* ]]; then
        fail_test "unreadable repository was not reported as an access problem"
        return
    fi
    if [[ "${output}" == *"git tag -s"* ]]; then
        fail_test "unreadable repository was reported as a missing tag"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/de-tag-test-XXXXXX")"

    test_accepts_verified_annotated_tag
    test_rejects_lightweight_tag
    test_rejects_unverified_tag
    test_rejects_non_commit_tag_target
    test_rejects_mismatched_signed_tag_name
    test_missing_tag_prints_recovery_command
    test_unreadable_repository_is_not_reported_as_missing_tag

    if ((FAIL > 0)); then
        echo "Deployment Engine tag tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "Deployment Engine tag tests passed (${PASS} tests)"
}

main "$@"
