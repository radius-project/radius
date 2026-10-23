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
readonly SCRIPT="${SCRIPT_DIR}/validate-release-merge-group.sh"
readonly WORKFLOW="${SCRIPT_DIR}/../workflows/release-plan.yaml"

TEST_ROOT=""
WORKFLOW_SELECTOR=""
REPO=""
BASE_SHA=""
RELEASE_SHA=""
GROUP_BASE_SHA=""
GROUP_SHA=""
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

setup_repo() {
    local trusted_validator="${1:-false}"

    REPO="${TEST_ROOT}/repo"
    rm -rf "${REPO}"
    mkdir -p "${REPO}"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false
    printf 'base\n' >"${REPO}/base.txt"
    git -C "${REPO}" add base.txt
    if [[ "${trusted_validator}" == true ]]; then
        mkdir -p "${REPO}/.github/scripts"
        cp "${SCRIPT}" "${REPO}/.github/scripts/validate-release-merge-group.sh"
        git -C "${REPO}" add .github/scripts/validate-release-merge-group.sh
    fi
    git -C "${REPO}" commit -q -m "chore: initial"
    BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"

    git -C "${REPO}" checkout -q -b release-pr
    mkdir -p "${REPO}/docs/release-notes" \
        "${REPO}/.github/release-plans"
    printf 'supported: []\n' >"${REPO}/versions.yaml"
    printf '# Changelog\n' >"${REPO}/CHANGELOG.md"
    printf '# Release notes\n' \
        >"${REPO}/docs/release-notes/v0.61.0-rc.1.md"
    printf 'schemaVersion: 2\n' \
        >"${REPO}/.github/release-plans/v0.61.0-rc.1.yaml"
    git -C "${REPO}" add versions.yaml CHANGELOG.md docs/release-notes \
        .github/release-plans
    git -C "${REPO}" commit -q -m "chore(release): prepare v0.61.0-rc.1"
    RELEASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    cat >"${REPO}/candidates.json" <<EOF
[{"number":123,"head_sha":"${RELEASE_SHA}","files":[".github/release-plans/v0.61.0-rc.1.yaml","CHANGELOG.md","docs/release-notes/v0.61.0-rc.1.md","versions.yaml"]}]
EOF
}

create_squash_group() {
    local extra_change="$1"
    local advance_base="${2:-false}"

    git -C "${REPO}" checkout -q main
    git -C "${REPO}" reset -q --hard "${BASE_SHA}"
    if [[ "${advance_base}" == "true" ]]; then
        printf 'already on main\n' >"${REPO}/advanced-base.txt"
        git -C "${REPO}" add advanced-base.txt
        git -C "${REPO}" commit -q -m "fix: advance main before queueing"
    fi
    GROUP_BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" diff --binary "${BASE_SHA}" "${RELEASE_SHA}" |
        git -C "${REPO}" apply --index
    if [[ "${extra_change}" == "true" ]]; then
        printf 'other\n' >"${REPO}/other.txt"
        git -C "${REPO}" add other.txt
    fi
    git -C "${REPO}" commit -q -m "squash merge group"
    GROUP_SHA="$(git -C "${REPO}" rev-parse HEAD)"
}

run_validator() {
    local merge_group_sha="$1"
    local status

    pushd "${REPO}" >/dev/null
    set +e
    bash "${SCRIPT}" --candidates-file candidates.json \
        --merge-group-sha "${merge_group_sha}" --base-sha "${GROUP_BASE_SHA}" \
        --output-file selected.txt
    status=$?
    set -e
    popd >/dev/null
    return "${status}"
}

test_accepts_squash_release_only_group() {
    local group_sha
    create_squash_group false
    group_sha="${GROUP_SHA}"
    if git -C "${REPO}" merge-base --is-ancestor \
        "${RELEASE_SHA}" "${group_sha}"; then
        fail_test "fixture must not make the PR head an ancestor"
        return
    fi
    if ! run_validator "${group_sha}" >/dev/null; then
        fail_test "expected a squash release-only group to pass"
        return
    fi
    if [[ "$(<"${REPO}/selected.txt")" != "123" ]]; then
        fail_test "selector did not identify release PR #123"
        return
    fi
    ((++PASS))
}

test_rejects_group_with_extra_changes() {
    local group_sha
    create_squash_group true
    group_sha="${GROUP_SHA}"
    if run_validator "${group_sha}" >/dev/null 2>&1; then
        fail_test "expected a batched merge group to fail"
        return
    fi
    ((++PASS))
}

test_selects_release_pr_on_advanced_base() {
    local group_sha
    create_squash_group false true
    group_sha="${GROUP_SHA}"
    if [[ "$(git -C "${REPO}" rev-parse "${RELEASE_SHA}^{tree}")" == "$(git -C "${REPO}" rev-parse "${group_sha}^{tree}")" ]]; then
        fail_test "fixture must produce different complete trees"
        return
    fi
    if ! run_validator "${group_sha}" >/dev/null; then
        fail_test "expected selector to identify the release PR on a newer base"
        return
    fi
    ((++PASS))
}

test_accepts_group_without_release_pr() {
    local file="${1:-other.txt}"
    local expects_plan="${2:-false}"
    local group_sha output status=0

    git -C "${REPO}" checkout -q main
    git -C "${REPO}" reset -q --hard "${BASE_SHA}"
    mkdir -p "$(dirname "${REPO}/${file}")"
    printf 'changed\n' > "${REPO}/${file}"
    git -C "${REPO}" add "${file}"
    git -C "${REPO}" commit -q -m "fix: unrelated change"
    printf '[]\n' > "${REPO}/candidates.json"
    GROUP_BASE_SHA="${BASE_SHA}"
    group_sha="$(git -C "${REPO}" rev-parse HEAD)"
    output="$(run_validator "${group_sha}" 2>&1)" || status=$?
    if [[ "${expects_plan}" == "true" ]]; then
        if [[ "${status}" == 0 || "${output}" != *"release metadata changed without a matching release plan"* ]]; then
            fail_test "expected unplanned metadata ${file} to fail: ${output}"
            return
        fi
    else
        if [[ "${status}" != 0 || -s "${REPO}/selected.txt" ]]; then
            fail_test "expected ordinary change ${file} to pass without a plan: ${output}"
            return
        fi
    fi
    ((++PASS))
}

run_workflow_selector() {
    local output_dir="${TEST_ROOT}/workflow"
    local status=0

    rm -rf "${output_dir}"
    mkdir -p "${output_dir}"
    cp "${REPO}/candidates.json" "${output_dir}/release-pr-candidates.json"
    : > "${output_dir}/github-output"
    pushd "${REPO}" > /dev/null
    BASE_SHA="${GROUP_BASE_SHA}" GITHUB_SHA="${GROUP_SHA}" \
        RUNNER_TEMP="${output_dir}" GITHUB_OUTPUT="${output_dir}/github-output" \
        bash -e -c "${WORKFLOW_SELECTOR}" || status=$?
    popd > /dev/null
    return "${status}"
}

test_workflow_bootstrap() {
    local file="$1"
    local expects_failure="$2"
    local output status=0

    setup_repo
    git -C "${REPO}" checkout -q main
    mkdir -p "$(dirname "${REPO}/${file}")" "${REPO}/.github/scripts"
    printf 'changed\n' > "${REPO}/${file}"
    printf '#!/bin/bash\ntouch queued-validator-executed\nexit 99\n' \
        > "${REPO}/.github/scripts/validate-release-merge-group.sh"
    git -C "${REPO}" add "${file}" .github/scripts/validate-release-merge-group.sh
    git -C "${REPO}" commit -q -m "ci: introduce release validation"
    GROUP_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    GROUP_BASE_SHA="${BASE_SHA}"
    git -C "${REPO}" checkout -q --detach "${GROUP_BASE_SHA}"
    printf '[]\n' > "${REPO}/candidates.json"

    output="$(run_workflow_selector 2>&1)" || status=$?
    if [[ -e "${REPO}/queued-validator-executed" ]]; then
        fail_test "bootstrap executed a validator from the queued PR"
        return
    fi
    if [[ "${expects_failure}" == true ]]; then
        if [[ "${status}" == 0 || "${output}" != *"trusted release validator is installed on main"* ]]; then
            fail_test "bootstrap must reject release metadata ${file}: ${output}"
            return
        fi
    elif [[ "${status}" != 0 ]] || ! grep -Fxq 'number=' "${TEST_ROOT}/workflow/github-output"; then
        fail_test "bootstrap must accept ordinary changes ${file}: ${output}"
        return
    fi
    ((++PASS))
}

test_workflow_uses_existing_trusted_validator() {
    local output

    setup_repo true
    create_squash_group false
    git -C "${REPO}" checkout -q --detach "${GROUP_BASE_SHA}"
    if ! output="$(run_workflow_selector 2>&1)"; then
        fail_test "installed trusted validator should select the release PR: ${output}"
        return
    fi
    if ! grep -Fxq 'number=123' "${TEST_ROOT}/workflow/github-output"; then
        fail_test "workflow must use the installed validator rather than bypassing release validation"
        return
    fi
    ((++PASS))
}

test_rc_cleanup() {
    local scenario="$1"
    local expected="$2"
    local output status=0

    setup_repo true
    git -C "${REPO}" checkout -q main
    cat >"${REPO}/CHANGELOG.md" <<'EOF'
# Changelog

Keep this introduction.

## [Unreleased]

Keep these unreleased notes.

## [0.62.0-rc.2] - 2026-10-23

### Fixed

- Second candidate.

## [0.62.0-rc.1] - 2026-10-22

### Added

- First candidate.

## [0.60.0] - 2026-08-19

### Added

- Keep stable history.

Keep the release history footer.

[Unreleased]: https://github.com/radius-project/radius/compare/v0.62.0-rc.2...HEAD
[0.62.0-rc.2]: https://github.com/radius-project/radius/compare/v0.61.1...v0.62.0-rc.2
[0.62.0-rc.1]: https://github.com/radius-project/radius/compare/v0.61.1...v0.62.0-rc.1
[0.60.0]: https://github.com/radius-project/radius/releases/tag/v0.60.0
EOF
    case "${scenario}" in
        single-rc)
            sed -i '/^## \[0.62.0-rc.1\]/,/^## \[0.60.0\]/{ /^## \[0.60.0\]/!d; }; /^\[0.62.0-rc.1\]:/d' "${REPO}/CHANGELOG.md"
            ;;
        ambiguous-baseline)
            sed -i '/^\[0.62.0-rc.1\]:/s/v0.61.1/v0.61.0/' "${REPO}/CHANGELOG.md"
            ;;
        duplicate-definition)
            grep '^\[0.62.0-rc.1\]:' "${REPO}/CHANGELOG.md" >>"${REPO}/duplicate"
            cat "${REPO}/duplicate" >>"${REPO}/CHANGELOG.md"
            ;;
        padded-definition)
            printf '[ Unreleased ]: duplicate\n' >>"${REPO}/CHANGELOG.md"
            ;;
        duplicate-heading)
            sed -i '/^## \[0.62.0-rc.1\]/a ## [0.62.0-rc.1] - 2026-10-22' "${REPO}/CHANGELOG.md"
            ;;
        missing-definition)
            sed -i '/^\[0.62.0-rc.1\]:/d' "${REPO}/CHANGELOG.md"
            ;;
        malformed-reference)
            sed -i '/^\[0.62.0-rc.1\]:/s/$/ extra/' "${REPO}/CHANGELOG.md"
            ;;
        wrong-unreleased)
            sed -i '/^\[Unreleased\]:/s/rc.2/rc.1/' "${REPO}/CHANGELOG.md"
            ;;
        unknown-rc)
            printf '[0.62.0-rc.3]: unknown\n' >>"${REPO}/CHANGELOG.md"
            ;;
        malformed-heading)
            sed -i 's/^## \[0.62.0-rc.1\].*/## [0.62.0-rc.1]/' "${REPO}/CHANGELOG.md"
            ;;
        interleaved-stable)
            sed -i '/^## \[0.62.0-rc.1\]/i ## [0.61.0] - 2026-09-19' "${REPO}/CHANGELOG.md"
            ;;
        mixed-rc-version)
            sed -i 's/0.62.0-rc.1/0.61.0-rc.1/g' "${REPO}/CHANGELOG.md"
            ;;
        crlf-base)
            sed -i 's/$/\r/' "${REPO}/CHANGELOG.md"
            ;;
    esac
    git -C "${REPO}" add CHANGELOG.md
    git -C "${REPO}" commit -q -m "test: trusted RC history"
    GROUP_BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    sed -e '/^## \[0.62.0-rc.2\]/,/^## \[0.60.0\]/{ /^## \[0.60.0\]/!d; }' \
        -e '/^\[0.62.0-rc.[12]\]:/d' \
        -e '/^\[Unreleased\]:/s/v0.62.0-rc.2/v0.61.1/' \
        "${REPO}/CHANGELOG.md" >"${REPO}/cleaned"
    mv "${REPO}/cleaned" "${REPO}/CHANGELOG.md"
    case "${scenario}" in
        stable-content) sed -i 's/Keep stable history/Alter stable history/' "${REPO}/CHANGELOG.md" ;;
        stable-link) sed -i '/^\[0.60.0\]:/s/v0.60.0/v0.61.1/' "${REPO}/CHANGELOG.md" ;;
        stable-deletion) sed -i '/^- Keep stable history/d' "${REPO}/CHANGELOG.md" ;;
        prose) sed -i 's/Keep this introduction/Alter this introduction/' "${REPO}/CHANGELOG.md" ;;
        unreleased-prose) sed -i 's/Keep these unreleased notes/Alter these unreleased notes/' "${REPO}/CHANGELOG.md" ;;
        footer) sed -i 's/Keep the release history footer/Alter the release history footer/' "${REPO}/CHANGELOG.md" ;;
        wrong-baseline) sed -i '/^\[Unreleased\]:/s/v0.61.1/v0.61.0/' "${REPO}/CHANGELOG.md" ;;
        missing-unreleased) sed -i '/^\[Unreleased\]:/d' "${REPO}/CHANGELOG.md" ;;
        retained-definition) git -C "${REPO}" show "${GROUP_BASE_SHA}:CHANGELOG.md" | grep '^\[0.62.0-rc.1\]:' >>"${REPO}/CHANGELOG.md" ;;
        no-newline) truncate -s -1 "${REPO}/CHANGELOG.md" ;;
        mode) chmod +x "${REPO}/CHANGELOG.md" ;;
        deleted) rm "${REPO}/CHANGELOG.md" ;;
        renamed)
            mv "${REPO}/CHANGELOG.md" "${REPO}/RENAMED.md"
            git -C "${REPO}" add RENAMED.md
            ;;
        symlink)
            rm "${REPO}/CHANGELOG.md"
            ln -s base.txt "${REPO}/CHANGELOG.md"
            ;;
        no-cleanup)
            git -C "${REPO}" show "${GROUP_BASE_SHA}:CHANGELOG.md" >"${REPO}/CHANGELOG.md"
            printf '\n' >>"${REPO}/CHANGELOG.md"
            ;;
        link-only)
            git -C "${REPO}" show "${GROUP_BASE_SHA}:CHANGELOG.md" |
                sed '/^\[Unreleased\]:/s/v0.62.0-rc.2/v0.61.1/' >"${REPO}/CHANGELOG.md"
            ;;
        partial-cleanup)
            git -C "${REPO}" show "${GROUP_BASE_SHA}:CHANGELOG.md" |
                sed -e '/^## \[0.62.0-rc.2\]/,/^## \[0.62.0-rc.1\]/{ /^## \[0.62.0-rc.1\]/!d; }' \
                    -e '/^\[0.62.0-rc.2\]:/d' \
                    -e '/^\[Unreleased\]:/s/rc.2/rc.1/' >"${REPO}/CHANGELOG.md"
            ;;
        version | plan | note | deleted-metadata | renamed-metadata)
            mkdir -p "${REPO}/.github/release-plans" "${REPO}/docs/release-notes"
            case "${scenario}" in
                version)
                    printf 'supported: []\n' >"${REPO}/versions.yaml"
                    git -C "${REPO}" add versions.yaml
                    ;;
                plan)
                    printf 'schemaVersion: 2\n' >"${REPO}/.github/release-plans/v0.62.0.yaml"
                    git -C "${REPO}" add .github/release-plans
                    ;;
                note)
                    printf 'notes\n' >"${REPO}/docs/release-notes/v0.62.0.md"
                    git -C "${REPO}" add docs/release-notes
                    ;;
                deleted-metadata | renamed-metadata)
                    # The removed metadata must exist in the comparison base.
                    git -C "${REPO}" show release-pr:versions.yaml >"${REPO}/versions.yaml"
                    git -C "${REPO}" add versions.yaml
                    git -C "${REPO}" commit -q -m "test: recorded versions"
                    GROUP_BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
                    if [[ "${scenario}" == renamed-metadata ]]; then
                        git -C "${REPO}" mv versions.yaml ordinary.yaml
                    else
                        git -C "${REPO}" rm -q versions.yaml
                    fi
                    ;;
            esac
            ;;
        tooling)
            printf 'tooling\n' >"${REPO}/base.txt"
            git -C "${REPO}" add base.txt
            ;;
        untrusted-validator)
            printf '#!/bin/bash\ntouch queued-validator-executed\nexit 99\n' \
                >"${REPO}/.github/scripts/validate-release-merge-group.sh"
            git -C "${REPO}" add .github/scripts/validate-release-merge-group.sh
            ;;
    esac
    git -C "${REPO}" add -u
    git -C "${REPO}" commit -q -m "test: ${scenario} cleanup"
    GROUP_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" checkout -q --detach "${GROUP_BASE_SHA}"
    printf '[]\n' >"${REPO}/candidates.json"
    output="$(run_workflow_selector 2>&1)" || status=$?
    if [[ -e "${REPO}/queued-validator-executed" ]]; then
        fail_test "cleanup executed the queued validator"
        return
    fi
    if [[ "${expected}" == pass ]]; then
        if [[ "${status}" != 0 ]] || ! grep -Fxq 'number=' "${TEST_ROOT}/workflow/github-output"; then
            fail_test "${scenario} cleanup should pass without selecting a release: ${output}"
            return
        fi
    elif [[ "${status}" == 0 || "${output}" != *"release metadata changed without a matching release plan"* ]]; then
        fail_test "${scenario} cleanup should fail closed: ${output}"
        return
    fi
    ((++PASS))
}

main() {
    local file scenario

    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-merge-group-XXXXXX")"
    WORKFLOW_SELECTOR="$(yq -r '.jobs."validate-merge-group".steps[] | select(.id == "select") | .run' "${WORKFLOW}")"

    setup_repo
    test_accepts_squash_release_only_group
    setup_repo
    test_rejects_group_with_extra_changes
    setup_repo
    test_selects_release_pr_on_advanced_base
    for file in other.txt docs/release-notes/README.md \
        docs/release-notes/template.md docs/release-notes/template_patch.md; do
        setup_repo
        test_accepts_group_without_release_pr "${file}"
    done
    for file in CHANGELOG.md versions.yaml docs/release-notes/v0.61.0.md \
        docs/release-notes/v0.61.0-rc.1.md docs/release-notes/v0.61.0-rc1.md \
        .github/release-plans/v0.61.0-rc.1.yaml; do
        setup_repo
        test_accepts_group_without_release_pr "${file}" true
    done
    for file in other.txt docs/release-notes/README.md \
        docs/release-notes/template.md docs/release-notes/template_patch.md; do
        test_workflow_bootstrap "${file}" false
    done
    for file in CHANGELOG.md versions.yaml docs/release-notes/v0.61.0.md \
        docs/release-notes/v0.61.0-rc.1.md docs/release-notes/v0.61.0-rc1.md \
        .github/release-plans/v0.61.0-rc.1.yaml; do
        test_workflow_bootstrap "${file}" true
    done
    test_workflow_uses_existing_trusted_validator
    for scenario in cleanup single-rc tooling untrusted-validator; do
        test_rc_cleanup "${scenario}" pass
    done
    for scenario in stable-content stable-link stable-deletion prose \
        unreleased-prose footer wrong-baseline missing-unreleased \
        retained-definition no-newline mode deleted renamed symlink no-cleanup \
        partial-cleanup link-only \
        version plan note deleted-metadata renamed-metadata \
        ambiguous-baseline duplicate-definition padded-definition duplicate-heading \
        missing-definition malformed-reference wrong-unreleased unknown-rc \
        malformed-heading interleaved-stable mixed-rc-version crlf-base; do
        test_rc_cleanup "${scenario}" fail
    done

    if ((FAIL > 0)); then
        echo "release merge-group tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "release merge-group tests passed (${PASS} tests)"
}

main "$@"
