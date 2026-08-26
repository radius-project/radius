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
readonly SCRIPT="${SCRIPT_DIR}/prepare-release.sh"
readonly PRESERVE_SCRIPT="${SCRIPT_DIR}/preserve-release-note-sections.sh"

TEST_ROOT=""
REPO=""
PASS=0
FAIL=0
LAST_OUTPUT=""
LAST_STATUS=0

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

commit() {
    git -C "${REPO}" commit -q --allow-empty -m "$1"
}

setup_repo() {
    local version="$1"
    local previous_changelog_version="0.60.0"

    if [[ "${version}" == *-rc* ]]; then
        previous_changelog_version="0.59.0"
    fi

    REPO="${TEST_ROOT}/repo"
    rm -rf "${REPO}"
    mkdir -p "${REPO}/out" "${REPO}/docs/release-notes" \
        "${REPO}/.github/release-parity"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false
    printf "supported:\n  - channel: '0.60'\n    version: '%s'\n" \
        "${version}" > "${REPO}/versions.yaml"
    printf 'deprecated:\n  - channel: '\''0.59'\''\n    version: '\''v0.59.0'\''\n' \
        >> "${REPO}/versions.yaml"
    printf '[]\n' > "${REPO}/backports.json"
    cat > "${REPO}/CHANGELOG.md" << EOF
# Changelog

## [Unreleased]

## [${previous_changelog_version}] - 2026-08-19

Previous release.

[Unreleased]: https://example.test/compare/v${previous_changelog_version}...HEAD
[${previous_changelog_version}]: https://example.test/releases/v${previous_changelog_version}
EOF
    cat > "${REPO}/docs/release-notes/template.md" << 'EOF'
## Announcing Radius vX.Y.Z
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

## Highlights

<!-- CURATE HIGHLIGHTS -->

## Breaking changes

<!-- ADD ANY BREAKING CHANGES HERE, IF ANY -->

## New contributors

<!-- PASTE THE OUTPUT OF THE GENERATED CONTRIBUTOR LIST HERE -->

## Upgrading to Radius vX.Y.Z

<!-- CURATE UPGRADING -->

## Full changelog

<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->
EOF
    cat > "${REPO}/docs/release-notes/template_patch.md" << 'EOF'
## Radius vX.Y.Z

## Changelog

<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->
EOF
    printf '{"cliAssets":[{"name":"rad_linux_amd64"}]}\n' \
        > "${REPO}/.github/release-parity/targets.json"
    printf '[changelog]\n' > "${REPO}/cliff.toml"
    cat > "${REPO}/fake-git-cliff" << 'EOF'
#!/bin/bash
set -euo pipefail
output=""
tag=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output) output="$2"; shift 2 ;;
        --tag) tag="$2"; shift 2 ;;
        *) shift ;;
    esac
done
cat >"${output}" <<BODY
## [${tag#v}] - 2026-08-24

### Breaking changes

- Replace a legacy contract

### Fixed

- Fix release preparation

### New contributors

- @first made their first contribution
BODY
EOF
    chmod +x "${REPO}/fake-git-cliff"
    cat > "${REPO}/fake-range.sh" << 'EOF'
#!/bin/bash
printf '%s\n' 'HEAD~1..HEAD'
EOF
    chmod +x "${REPO}/fake-range.sh"
    git -C "${REPO}" add .
    commit "initial"
    git -C "${REPO}" tag v0.59.0
    commit "fix: release preparation"
}

run_prepare() {
    local release_type="$1"
    local channel="$2"

    set +e
    LAST_OUTPUT="$(
        cd "${REPO}" \
                     && GIT_CLIFF="${REPO}/fake-git-cliff" \
                CHANGELOG_RANGE_SCRIPT="${REPO}/fake-range.sh" \
                bash "${SCRIPT}" \
                --release-type "${release_type}" \
                --channel "${channel}" \
                --backports-file backports.json \
                --output-dir out \
                --release-date 2026-08-24 2>&1
    )"
    LAST_STATUS=$?
    set -e
}

assert_version() {
    local expected="$1"
    local actual

    if ((LAST_STATUS != 0)); then
        fail_test "expected success, got: ${LAST_OUTPUT}"
        return 1
    fi
    actual="$(cat "${REPO}/out/version.txt")"
    if [[ "${actual}" != "${expected}" ]]; then
        fail_test "got version ${actual}; expected ${expected}"
        return 1
    fi
    return 0
}

assert_file_contains() {
    local file="$1"
    local expected="$2"

    if ! grep -Fq "${expected}" "${file}"; then
        fail_test "${file} does not contain: ${expected}"
        return 1
    fi
    return 0
}

assert_yq_value() {
    local file="$1"
    local expression="$2"
    local expected="$3"
    local actual

    actual="$(yq -r "${expression}" "${file}")"
    if [[ "${actual}" != "${expected}" ]]; then
        fail_test "${file} query ${expression} returned ${actual}; expected ${expected}"
        return 1
    fi
    return 0
}

make_release_branch() {
    git -C "${REPO}" branch "release/$1"
}

test_first_rc() {
    setup_repo "v0.60.0"
    git -C "${REPO}" tag v0.60.0
    run_prepare rc 0.61
    assert_version "v0.61.0-rc.1" || return
    assert_yq_value "${REPO}/versions.yaml" '.supported[0].version' \
        'v0.61.0-rc.1' || return
    assert_file_contains "${REPO}/CHANGELOG.md" \
        '## [0.61.0-rc.1] - 2026-08-24' || return
    assert_file_contains "${REPO}/CHANGELOG.md" \
        '[Unreleased]: https://github.com/radius-project/radius/compare/v0.61.0-rc.1...HEAD' || return
    assert_file_contains \
        "${REPO}/docs/release-notes/v0.61.0-rc.1.md" \
        '@first made their first contribution' || return
    assert_file_contains "${REPO}/out/release-plan.yaml" \
        'version: v0.61.0-rc.1' || return
    assert_yq_value "${REPO}/out/release-plan.yaml" \
        '.expectedOutputs.cliAssets[0].name' 'rad_linux_amd64' || return
    assert_yq_value "${REPO}/out/release-plan.yaml" \
        '.source.releaseCommit' 'null' || return
    assert_yq_value "${REPO}/out/release-plan.yaml" \
        '.source.releaseCommitResolution' \
        'release PR squash commit on main' || return
    ((++PASS))
}

test_first_rc_preserves_support_window() {
    setup_repo "v0.60.0"
    yq -i '.supported += [{"channel": "0.59", "version": "v0.59.1"}]' \
        "${REPO}/versions.yaml"
    git -C "${REPO}" tag v0.60.0
    run_prepare rc 0.61
    assert_version "v0.61.0-rc.1" || return
    assert_yq_value "${REPO}/versions.yaml" '.supported | length' '2' || return
    assert_yq_value "${REPO}/versions.yaml" '.supported[1].channel' \
        '0.60' || return
    assert_yq_value "${REPO}/versions.yaml" '.deprecated[0].channel' \
        '0.59' || return
    ((++PASS))
}

test_first_rc_requires_latest_stable() {
    setup_repo "v0.60.0-rc.3"
    run_prepare rc 0.61
    if ((LAST_STATUS == 0)); then
        fail_test "expected a new channel to reject an unfinished current channel"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"must be stable"* ]]; then
        fail_test "new-channel failure did not explain stable requirement"
        return
    fi
    ((++PASS))
}

test_subsequent_rc() {
    setup_repo "v0.60.0-rc.2"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.1
    git -C "${REPO}" tag v0.60.0-rc.2
    run_prepare rc 0.60
    assert_version "v0.60.0-rc.3" || return
    ((++PASS))
}

test_subsequent_rc_rejects_stale_metadata() {
    setup_repo "v0.60.0-rc.1"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.1
    git -C "${REPO}" tag v0.60.0-rc.2
    run_prepare rc 0.60
    if ((LAST_STATUS == 0)); then
        fail_test "expected stale RC metadata to fail"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"does not match the highest tag"* ]]; then
        fail_test "stale metadata failure was unclear: ${LAST_OUTPUT}"
        return
    fi
    ((++PASS))
}

test_subsequent_rc_rejects_divergent_branch() {
    setup_repo "v0.60.0-rc.1"
    git -C "${REPO}" branch release/0.60 v0.59.0
    git -C "${REPO}" tag v0.60.0-rc.1
    run_prepare rc 0.60
    if ((LAST_STATUS == 0)); then
        fail_test "expected a branch missing the current RC to fail"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"not reachable"* ]]; then
        fail_test "divergent branch failure was unclear: ${LAST_OUTPUT}"
        return
    fi
    ((++PASS))
}

test_final() {
    setup_repo "v0.60.0-rc.3"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.3
    run_prepare final 0.60
    assert_version "v0.60.0" || return
    assert_file_contains "${REPO}/out/release-plan.yaml" \
        'releaseType: final' || return
    assert_file_contains "${REPO}/out/requires-backport.txt" 'true' || return
    assert_file_contains "${REPO}/docs/release-notes/v0.60.0.md" \
        '## Upgrading to Radius v0.60.0' || return
    ((++PASS))
}

test_final_rejects_unvalidated_branch_tip() {
    setup_repo "v0.60.0-rc.3"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.3
    git -C "${REPO}" checkout -q release/0.60
    commit "fix: unvalidated release branch change"
    git -C "${REPO}" checkout -q main
    run_prepare final 0.60
    if ((LAST_STATUS == 0)); then
        fail_test "expected final preparation to reject an advanced branch"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"validate another RC"* ]]; then
        fail_test "final failure did not require another RC: ${LAST_OUTPUT}"
        return
    fi
    ((++PASS))
}

test_final_rejects_out_of_policy_rc_number() {
    setup_repo "v0.60.0-rc.0"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.0
    run_prepare final 0.60
    if ((LAST_STATUS == 0)); then
        fail_test "expected rc.0 to be rejected as a final release base"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"has no RC"* ]]; then
        fail_test "out-of-policy RC failure was unclear: ${LAST_OUTPUT}"
        return
    fi
    ((++PASS))
}

test_version_only_does_not_mutate_files() {
    local before
    local output_dir="${TEST_ROOT}/version-only-output"

    setup_repo "v0.60.0"
    git -C "${REPO}" tag v0.60.0
    before="$(git -C "${REPO}" status --porcelain)"
    set +e
    LAST_OUTPUT="$(
        cd "${REPO}" && bash "${SCRIPT}" --release-type rc \
            --channel 0.61 --output-dir "${output_dir}" --version-only \
            --release-date 2026-08-24 2>&1
    )"
    LAST_STATUS=$?
    set -e
    if ((LAST_STATUS != 0)); then
        fail_test "expected version-only success, got: ${LAST_OUTPUT}"
        return
    fi
    if [[ "$(< "${output_dir}/version.txt")" != "v0.61.0-rc.1" ]]; then
        fail_test "version-only mode selected the wrong version"
        return
    fi
    if [[ "$(git -C "${REPO}" status --porcelain)" != "${before}" ]]; then
        fail_test "version-only mode changed repository files"
        return
    fi
    ((++PASS))
}

test_patch() {
    setup_repo "v0.60.2"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.2
    git -C "${REPO}" tag v0.61.0
    run_prepare patch 0.60
    assert_version "v0.60.3" || return
    assert_file_contains "${REPO}/CHANGELOG.md" \
        'compare/v0.60.2...v0.60.3' || return
    ((++PASS))
}

test_unmerged_backport_fails() {
    setup_repo "v0.60.0-rc.1"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.0-rc.1
    cat > "${REPO}/backports.json" << 'EOF'
[{"source_pr":123,"backport_merged":false}]
EOF
    run_prepare rc 0.60
    if ((LAST_STATUS == 0)); then
        fail_test "expected an unmerged selected backport to fail"
        return
    fi
    if [[ "${LAST_OUTPUT}" != *"#123"* ]]; then
        fail_test "failure did not identify the missing backport: ${LAST_OUTPUT}"
        return
    fi
    ((++PASS))
}

test_publisher_uses_prepared_notes_for_every_release() {
    local makefile="${SCRIPT_DIR}/../../build/artifacts.mk"

    if grep -Fq -- '--generate-notes' "${makefile}"; then
        fail_test "release publisher still bypasses canonical prepared notes"
        return
    fi
    # shellcheck disable=SC2016 # Literal Make expression under test.
    if ! grep -Fq -- \
        '--release-notes "$(GORELEASER_RELEASE_NOTES)"' "${makefile}"; then
        fail_test "GoReleaser does not use canonical prepared notes"
        return
    fi
    ((++PASS))
}

test_first_rc_rerun_preserves_curated_sections() {
    local existing="${TEST_ROOT}/existing-notes.md"
    local notes="${REPO}/docs/release-notes/v0.61.0-rc.1.md"
    local first_commit second_commit

    setup_repo "v0.60.0"
    git -C "${REPO}" tag v0.60.0
    run_prepare rc 0.61
    [[ "${LAST_STATUS}" == "0" ]] || {
        fail_test "first preparation failed: ${LAST_OUTPUT}"
        return
    }
    sed -i 's/<!-- CURATE HIGHLIGHTS -->/Curated highlight./' "${notes}"
    sed -i 's/<!-- CURATE UPGRADING -->/Curated upgrade guidance./' "${notes}"
    cp "${notes}" "${existing}"
    first_commit="$(yq -r '.source.productCommit' "${REPO}/out/release-plan.yaml")"

    git -C "${REPO}" checkout -- versions.yaml CHANGELOG.md
    rm -rf "${REPO}/out" "${notes}"
    git -C "${REPO}" commit -q --allow-empty -m "fix: advance main"
    run_prepare rc 0.61
    [[ "${LAST_STATUS}" == "0" ]] || {
        fail_test "second preparation failed: ${LAST_OUTPUT}"
        return
    }
    bash "${PRESERVE_SCRIPT}" "${notes}" "${existing}"
    second_commit="$(yq -r '.source.productCommit' "${REPO}/out/release-plan.yaml")"
    if [[ "${first_commit}" == "${second_commit}" ]]; then
        fail_test "rerun did not update the planned product commit"
        return
    fi
    assert_file_contains "${notes}" 'Curated highlight.' || return
    assert_file_contains "${notes}" 'Curated upgrade guidance.' || return
    assert_file_contains "${notes}" 'Fix release preparation' || return
    ((++PASS))
}

test_patch_rerun_needs_no_curated_sections() {
    local notes="${REPO}/docs/release-notes/v0.60.3.md"

    setup_repo "v0.60.2"
    make_release_branch 0.60
    git -C "${REPO}" tag v0.60.2
    run_prepare patch 0.60
    [[ "${LAST_STATUS}" == "0" ]] || {
        fail_test "first patch preparation failed: ${LAST_OUTPUT}"
        return
    }
    git -C "${REPO}" checkout -- versions.yaml CHANGELOG.md
    rm -rf "${REPO}/out" "${notes}"
    run_prepare patch 0.60
    [[ "${LAST_STATUS}" == "0" ]] || {
        fail_test "patch regeneration failed: ${LAST_OUTPUT}"
        return
    }
    if grep -Eq '^## (Highlights|Upgrading to Radius )' "${notes}"; then
        fail_test "patch notes unexpectedly require curated sections"
        return
    fi
    assert_file_contains "${notes}" 'Fix release preparation' || return
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/prepare-release-test-XXXXXX")"

    test_first_rc
    test_first_rc_preserves_support_window
    test_first_rc_requires_latest_stable
    test_subsequent_rc
    test_subsequent_rc_rejects_stale_metadata
    test_subsequent_rc_rejects_divergent_branch
    test_final
    test_final_rejects_unvalidated_branch_tip
    test_final_rejects_out_of_policy_rc_number
    test_version_only_does_not_mutate_files
    test_patch
    test_unmerged_backport_fails
    test_publisher_uses_prepared_notes_for_every_release
    test_first_rc_rerun_preserves_curated_sections
    test_patch_rerun_needs_no_curated_sections

    if ((FAIL > 0)); then
        echo "prepare release tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "prepare release tests passed (${PASS} tests)"
}

main "$@"
