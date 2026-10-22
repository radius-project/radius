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
readonly COMMIT_A="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
readonly COMMIT_B="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
readonly ANNOTATED_REF='{"object":{"type":"tag","sha":"tag-object"}}'
readonly LIGHTWEIGHT_REF="{\"object\":{\"type\":\"commit\",\"sha\":\"${COMMIT_A}\"}}"

TEST_ROOT=""
PASS=0
FAIL=0
OUTPUT=""
STATUS=0

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

# Each test describes the repository through FAKE_* variables: the tag ref
# (empty when absent), the error GitHub returns for a tag it cannot read, the
# tag object, how a commit compares with main and whether GitHub verified it,
# and the commit an absent tag's base ref resolves to.
write_fake_gh() {
    cat >"${TEST_ROOT}/gh" <<'EOF'
#!/bin/bash
set -euo pipefail
case "$*" in
    *"/git/ref/tags/"*)
        if [[ -z "${FAKE_REF:-}" ]]; then
            printf 'gh: %s\n' "${FAKE_REF_ERROR:-Not Found (HTTP 404)}" >&2
            exit 1
        fi
        printf '%s\n' "${FAKE_REF}"
        ;;
    *"/git/tags/"*) printf '%s\n' "${FAKE_TAG_OBJECT}" ;;
    *"/compare/"*)
        printf '%s\n' "$2" >>"${FAKE_COMPARISONS}"
        printf '{"status":"%s","base_commit":{"commit":{"verification":{"verified":%s}}}}\n' \
            "${FAKE_COMPARE:-ahead}" "${FAKE_VERIFIED:-true}"
        ;;
    *"/commits/"*)
        if [[ -z "${FAKE_BASE_COMMIT:-}" ]]; then
            echo "gh: Not Found (HTTP 404)" >&2
            exit 1
        fi
        printf '{"sha":"%s"}\n' "${FAKE_BASE_COMMIT}"
        ;;
    *) printf '%s\n' 'azure-octo/deployment-engine' ;;
esac
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

tag_object() {
    local verified="$1"
    local reason="$2"
    local name="${3:-v0.61.0-rc.1}"
    local target_type="${4:-commit}"

    printf '{"tag":"%s","verification":{"verified":%s,"reason":"%s"},"object":{"type":"%s","sha":"%s"}}' \
        "${name}" "${verified}" "${reason}" "${target_type}" "${COMMIT_A}"
}

repository() {
    write_fake_gh
    export FAKE_REF="$1"
    export FAKE_TAG_OBJECT="${2:-}"
    export FAKE_COMPARE="${3:-ahead}"
    export FAKE_BASE_COMMIT="${4:-}"
    export FAKE_COMPARISONS="${TEST_ROOT}/comparisons"
    unset FAKE_REF_ERROR FAKE_VERIFIED
    : >"${FAKE_COMPARISONS}"
    : >"${TEST_ROOT}/outputs"
}

run_script() {
    set +e
    OUTPUT="$(GITHUB_OUTPUT="${TEST_ROOT}/outputs" GH="${TEST_ROOT}/gh" \
        bash "${SCRIPT}" "$@" 2>&1)"
    STATUS=$?
    set -e
}

expect_outputs() {
    local commit="$1"
    local state="$2"
    local message="$3"

    if ((STATUS != 0)); then
        fail_test "${message}: ${OUTPUT}"
        return 1
    fi
    if [[ "$(<"${TEST_ROOT}/outputs")" != "commit=${commit}"$'\n'"state=${state}" ]]; then
        fail_test "${message}: unexpected outputs $(<"${TEST_ROOT}/outputs")"
        return 1
    fi
}

expect_failure() {
    local expected="$1"
    local message="$2"

    if ((STATUS == 0)); then
        fail_test "${message}"
        return 1
    fi
    if [[ "${OUTPUT}" != *"${expected}"* ]]; then
        fail_test "${message}: ${OUTPUT}"
        return 1
    fi
}

test_accepts_verified_annotated_tag() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid)"
    run_script v0.61.0-rc.1
    expect_outputs "${COMMIT_A}" signed "verified annotated tag" || return
    ((++PASS))
}

test_signed_tag_may_name_a_commit_off_main() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid)" diverged
    run_script v0.61.0-rc.1
    expect_outputs "${COMMIT_A}" signed \
        "a maintainer-signed hotfix tag must not require main" || return
    if [[ -s "${TEST_ROOT}/comparisons" ]]; then
        fail_test "a signed tag must not be compared with main"
        return
    fi
    ((++PASS))
}

test_accepts_unsigned_annotated_tag_on_main() {
    local status

    for status in ahead identical; do
        repository "${ANNOTATED_REF}" "$(tag_object false unsigned)" "${status}"
        run_script v0.61.0-rc.1
        expect_outputs "${COMMIT_A}" unsigned \
            "unsigned annotated tag on main (${status})" || return
    done
    ((++PASS))
}

test_accepts_lightweight_tag_on_main() {
    repository "${LIGHTWEIGHT_REF}"
    run_script v0.61.0-rc.1
    expect_outputs "${COMMIT_A}" unsigned \
        "controller-created lightweight tag on main" || return
    if [[ "$(<"${TEST_ROOT}/comparisons")" != *"/compare/${COMMIT_A}...main"* ]]; then
        fail_test "unsigned tag commit was not compared with main"
        return
    fi
    ((++PASS))
}

test_rejects_unsigned_tag_off_main() {
    local ref status

    for ref in "${ANNOTATED_REF}" "${LIGHTWEIGHT_REF}"; do
        for status in behind diverged; do
            repository "${ref}" "$(tag_object false unsigned)" "${status}"
            run_script v0.61.0-rc.1
            expect_failure "is not on main" \
                "unsigned tag off main (${status}) must fail" || return
        done
    done
    ((++PASS))
}

test_rejects_unverified_signature() {
    repository "${ANNOTATED_REF}" "$(tag_object false unknown_key)"
    run_script v0.61.0-rc.1
    expect_failure "unverified signature (unknown_key)" \
        "a signature GitHub cannot verify must fail" || return
    ((++PASS))
}

test_rejects_non_commit_tag_target() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid v0.61.0 tree)"
    run_script v0.61.0
    expect_failure "does not target a commit" \
        "a non-commit tag target must fail" || return
    ((++PASS))
}

test_rejects_mismatched_signed_tag_name() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid v0.60.0)"
    run_script v0.61.0-rc.1
    expect_failure "names v0.60.0" \
        "a mismatched tag object name must fail" || return
    ((++PASS))
}

test_accepts_tag_at_approved_commit() {
    repository "${LIGHTWEIGHT_REF}"
    run_script v0.61.0-rc.1 "${COMMIT_A}"
    expect_outputs "${COMMIT_A}" unsigned \
        "a tag at the approved commit" || return
    ((++PASS))
}

test_rejects_tag_moved_after_approval() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid)"
    run_script v0.61.0-rc.1 "${COMMIT_B}"
    expect_failure "is at ${COMMIT_A}, not the expected ${COMMIT_B}" \
        "a tag that moved after approval must fail" || return
    ((++PASS))
}

test_rejects_malformed_approved_commit() {
    repository "${ANNOTATED_REF}" "$(tag_object true valid)"
    run_script v0.61.0-rc.1 aaaa
    expect_failure "full commit SHA" \
        "a malformed approved commit must fail" || return
    ((++PASS))
}

test_missing_tag_without_plan_prints_recovery_command() {
    repository ""
    run_script v0.61.0-rc.1
    expect_failure "git tag -s v0.61.0-rc.1" \
        "a missing tag without a plan must fail with the signing command" ||
        return
    if [[ "${OUTPUT}" != *"git push origin v0.61.0-rc.1"* ||
        "${OUTPUT}" != *"release controller creates it after approval"* ]]; then
        fail_test "missing-tag error did not explain both recovery paths"
        return
    fi
    ((++PASS))
}

test_absent_tag_plans_base_commit() {
    local base

    for base in main v0.61.0-rc.2 v0.60.0-rc1; do
        repository "" "" ahead "${COMMIT_B}"
        run_script --allow-absent "${base}" v0.61.0
        expect_outputs "${COMMIT_B}" absent \
            "absent tag must plan the ${base} commit" || return
        if [[ "${OUTPUT}" != *"will create it at ${COMMIT_B} (${base})"* ]]; then
            fail_test "absent plan did not name ${base}: ${OUTPUT}"
            return
        fi
    done
    ((++PASS))
}

test_absent_plan_must_be_on_main() {
    repository "" "" diverged "${COMMIT_B}"
    run_script --allow-absent v0.61.0-rc.2 v0.61.0
    expect_failure "is not on main" \
        "an absent tag must not be planned off main" || return
    ((++PASS))
}

test_absent_plan_requires_resolvable_base() {
    repository ""
    run_script --allow-absent v0.61.0-rc.2 v0.61.0
    expect_failure "could not resolve Deployment Engine v0.61.0-rc.2" \
        "an unresolvable base ref must fail" || return
    ((++PASS))
}

test_only_a_confirmed_missing_tag_is_planned() {
    local error

    for error in "Forbidden (HTTP 403)" "API rate limit exceeded (HTTP 429)" \
        "Service Unavailable (HTTP 503)" "connection reset by peer"; do
        repository "" "" ahead "${COMMIT_B}"
        export FAKE_REF_ERROR="${error}"
        run_script --allow-absent main v0.61.0-rc.1
        expect_failure "could not look up Deployment Engine tag v0.61.0-rc.1" \
            "a failed tag lookup (${error}) must not plan the tag" || return
        if [[ -s "${TEST_ROOT}/outputs" || "${OUTPUT}" == *"will create it"* ]]; then
            fail_test "a failed tag lookup (${error}) planned the tag"
            return
        fi
    done
    ((++PASS))
}

test_unsigned_or_planned_tag_requires_verified_commit() {
    repository "${LIGHTWEIGHT_REF}"
    export FAKE_VERIFIED=false
    run_script v0.61.0-rc.1
    expect_failure "does not have a GitHub-verified signature" \
        "an unsigned tag must name a GitHub-verified commit" || return

    repository "" "" ahead "${COMMIT_B}"
    export FAKE_VERIFIED=false
    run_script --allow-absent main v0.61.0-rc.1
    expect_failure "does not have a GitHub-verified signature" \
        "an absent tag must not be planned at an unverified commit" || return
    if [[ -s "${TEST_ROOT}/outputs" ]]; then
        fail_test "an unverified commit was planned"
        return
    fi
    ((++PASS))
}

test_final_plan_uses_release_candidate_lock() {
    repository "" "" ahead "${COMMIT_A}"
    run_script --allow-absent v0.61.0-rc.2 v0.61.0 "${COMMIT_A}"
    expect_outputs "${COMMIT_A}" absent \
        "a final must plan the commit its release candidate locked" || return
    ((++PASS))
}

test_final_plan_rejects_moved_release_candidate_tag() {
    repository "" "" ahead "${COMMIT_B}"
    run_script --allow-absent v0.61.0-rc.2 v0.61.0 "${COMMIT_A}"
    expect_failure \
        "v0.61.0-rc.2 resolves to ${COMMIT_B}, not the expected ${COMMIT_A}" \
        "a release candidate tag moved after its lock must fail" || return
    if [[ -s "${TEST_ROOT}/outputs" ]]; then
        fail_test "a moved release candidate tag was planned"
        return
    fi
    ((++PASS))
}

test_rejects_invalid_base_ref() {
    local base

    for base in release/0.61 HEAD "${COMMIT_B}" v0.61; do
        repository "" "" ahead "${COMMIT_B}"
        run_script --allow-absent "${base}" v0.61.0
        expect_failure "base ref must be main or a release tag" \
            "base ref ${base} must be rejected" || return
    done
    ((++PASS))
}

test_existing_tag_is_verified_even_when_absence_is_allowed() {
    repository "${ANNOTATED_REF}" "$(tag_object false unsigned)" diverged \
        "${COMMIT_B}"
    run_script --allow-absent main v0.61.0-rc.1
    expect_failure "is not on main" \
        "an existing tag must be verified instead of replaced by the plan" ||
        return
    ((++PASS))
}

test_unreadable_repository_is_not_reported_as_missing_tag() {
    write_unreadable_fake_gh
    run_script --allow-absent main v0.61.0-rc.1
    expect_failure "cannot read azure-octo/deployment-engine" \
        "an unreadable repository must fail as an access problem" || return
    if [[ "${OUTPUT}" == *"git tag -s"* || "${OUTPUT}" == *"is absent"* ]]; then
        fail_test "unreadable repository was reported as a missing tag"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/de-tag-test-XXXXXX")"

    test_accepts_verified_annotated_tag
    test_signed_tag_may_name_a_commit_off_main
    test_accepts_unsigned_annotated_tag_on_main
    test_accepts_lightweight_tag_on_main
    test_rejects_unsigned_tag_off_main
    test_rejects_unverified_signature
    test_rejects_non_commit_tag_target
    test_rejects_mismatched_signed_tag_name
    test_accepts_tag_at_approved_commit
    test_rejects_tag_moved_after_approval
    test_rejects_malformed_approved_commit
    test_missing_tag_without_plan_prints_recovery_command
    test_absent_tag_plans_base_commit
    test_absent_plan_must_be_on_main
    test_absent_plan_requires_resolvable_base
    test_only_a_confirmed_missing_tag_is_planned
    test_unsigned_or_planned_tag_requires_verified_commit
    test_final_plan_uses_release_candidate_lock
    test_final_plan_rejects_moved_release_candidate_tag
    test_rejects_invalid_base_ref
    test_existing_tag_is_verified_even_when_absence_is_allowed
    test_unreadable_repository_is_not_reported_as_missing_tag

    if ((FAIL > 0)); then
        echo "Deployment Engine tag tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "Deployment Engine tag tests passed (${PASS} tests)"
}

main "$@"