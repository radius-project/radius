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
readonly SCRIPT="${SCRIPT_DIR}/validate-release-controller-plan.sh"

TEST_ROOT=""
REPO=""
ORIGIN=""
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

pass_test() {
    ((++PASS))
}

init_fixture() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/controller-plan-test-XXXXXX")"
    ORIGIN="${TEST_ROOT}/origin.git"
    REPO="${TEST_ROOT}/repo"
    git init --quiet --bare "${ORIGIN}"
    git clone --quiet "${ORIGIN}" "${REPO}"
    git -C "${REPO}" config user.name "Release Test"
    git -C "${REPO}" config user.email "release-test@example.com"
    git -C "${REPO}" config commit.gpgsign false
}

write_initial_files() {
    mkdir -p "${REPO}/docs/release-notes" \
        "${REPO}/.github/release-plans"
    cat >"${REPO}/versions.yaml" <<'EOF'
supported:
  - channel: '0.60'
    version: 'v0.60.0'
deprecated: []
EOF
    printf '# Changelog\n' >"${REPO}/CHANGELOG.md"
    git -C "${REPO}" add .
    git -C "${REPO}" commit --quiet -m "Initial release state"
    git -C "${REPO}" branch -M main
    git -C "${REPO}" push --quiet -u origin main
}

write_release_files() {
    local version="$1"
    local channel="$2"

    cat >"${REPO}/versions.yaml" <<EOF
supported:
  - channel: '${channel}'
    version: '${version}'
deprecated: []
EOF
    printf '# Changelog\n\n## [%s]\n' "${version#v}" \
        >"${REPO}/CHANGELOG.md"
    printf '# Radius %s\n' "${version}" \
        >"${REPO}/docs/release-notes/${version}.md"
}

write_plan() {
    local plan_file="$1"
    local version="$2"
    local release_type="$3"
    local channel="$4"
    local product_ref="$5"
    local product_commit="$6"
    local resolution="$7"

    cat >"${plan_file}" <<EOF
schemaVersion: 2
version: ${version}
releaseType: ${release_type}
channel: "${channel}"
releaseDate: "2026-08-28"
chartVersion: ${version#v}
source:
  productRef: ${product_ref}
  productCommit: ${product_commit}
  releaseCommit: null
  releaseCommitResolution: ${resolution}
releaseBranch: release/${channel}
releasePlanPath: .github/release-plans/${version}.yaml
previousVersion: v0.60.0
siblingRepositories: [{name: recipes, repository: radius-project/recipes,
        sourceRef: main, sourceCommit: "1111111111111111111111111111111111111111"},
    {name: dashboard, repository: radius-project/dashboard,
        sourceRef: main, sourceCommit: "2222222222222222222222222222222222222222"},
    {name: bicep-types-aws, repository: radius-project/bicep-types-aws,
        sourceRef: main, sourceCommit: "3333333333333333333333333333333333333333"}]
expectedOutputs:
  repository: radius-project/radius
includedBackports: []
EOF
}

run_validator() {
    local plan_file="$1"
    local version="$2"
    local source_pr_commit="$3"
    local release_commit="$4"
    local trigger="$5"
    local output_dir="$6"

    set +e
    LAST_OUTPUT="$({
        cd "${REPO}"
        bash "${SCRIPT}" --plan-file "${plan_file}" \
            --version "${version}" \
            --source-pr-commit "${source_pr_commit}" \
            --release-commit "${release_commit}" \
            --trigger "${trigger}" --output-dir "${output_dir}"
    } 2>&1)"
    LAST_STATUS=$?
    set -e
}

assert_success() {
    if ((LAST_STATUS != 0)); then
        fail_test "expected success: ${LAST_OUTPUT}"
        return 1
    fi
}

assert_failure_contains() {
    local expected="$1"

    if ((LAST_STATUS == 0)); then
        fail_test "expected failure containing: ${expected}"
        return 1
    fi
    if [[ "${LAST_OUTPUT}" != *"${expected}"* ]]; then
        fail_test "missing '${expected}' in: ${LAST_OUTPUT}"
        return 1
    fi
}

test_first_rc_resolves_squash_commit() {
    local base source plan output

    base="$(git -C "${REPO}" rev-parse HEAD)"
    write_release_files v0.61.0-rc.1 0.61
    plan="${REPO}/.github/release-plans/v0.61.0-rc.1.yaml"
    write_plan "${plan}" v0.61.0-rc.1 rc 0.61 HEAD "${base}" \
        "release PR squash commit on main"
    git -C "${REPO}" add .
    git -C "${REPO}" commit --quiet \
        -m "chore(release): prepare v0.61.0-rc.1"
    source="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" push --quiet origin main
    output="${TEST_ROOT}/first-rc-output"

    run_validator "${plan}" v0.61.0-rc.1 "${source}" "${source}" \
        release-pr "${output}"
    assert_success || return
    [[ "$(<"${output}/ready.txt")" == "true" ]] || {
        fail_test "first RC was not ready"
        return
    }
    [[ "$(yq -r '.source.releaseCommit' \
        "${output}/release-plan.yaml")" == "${source}" ]] || {
        fail_test "resolved plan did not record the release commit"
        return
    }
    [[ "$(<"${output}/de-image-tag.txt")" == "0.61.0-rc.1" ]] || {
        fail_test "first RC selected the wrong Deployment Engine tag"
        return
    }
    [[ "$(<"${output}/recipes-commit.txt")" == "1111111111111111111111111111111111111111" ]] || {
        fail_test "controller did not emit the frozen recipes commit"
        return
    }
    pass_test
}

test_rejects_requested_version_mismatch() {
    local plan source

    plan="${REPO}/.github/release-plans/v0.61.0-rc.1.yaml"
    source="$(git -C "${REPO}" rev-parse main)"
    run_validator "${plan}" v0.61.0-rc.2 "${source}" "${source}" \
        dispatch "${TEST_ROOT}/mismatch-output"
    assert_failure_contains "does not match plan" || return
    pass_test
}

test_rejects_conflicting_radius_tag() {
    local plan source base

    plan="${REPO}/.github/release-plans/v0.61.0-rc.1.yaml"
    source="$(git -C "${REPO}" rev-parse main)"
    base="$(git -C "${REPO}" rev-parse "${source}^")"
    git -C "${REPO}" push --quiet origin \
        "${base}:refs/tags/v0.61.0-rc.1"
    run_validator "${plan}" v0.61.0-rc.1 "${source}" "${source}" \
        dispatch "${TEST_ROOT}/tag-conflict-output"
    git -C "${REPO}" push --quiet origin :refs/tags/v0.61.0-rc.1
    assert_failure_contains "points at" || return
    pass_test
}

create_existing_channel_release() {
    local product_commit source_commit backport_commit plan

    git -C "${REPO}" checkout --quiet -b release/0.61 main
    mkdir -p "${REPO}/.github/workflows"
    printf 'product fix\n' >"${REPO}/product.txt"
    printf 'name: legacy-release\n' \
        >"${REPO}/.github/workflows/release.yaml"
    git -C "${REPO}" add product.txt .github/workflows/release.yaml
    git -C "${REPO}" commit --quiet -m "fix: stabilize release"
    product_commit="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" push --quiet -u origin release/0.61

    git -C "${REPO}" checkout --quiet main
    write_release_files v0.61.0 0.61
    plan="${REPO}/.github/release-plans/v0.61.0.yaml"
    write_plan "${plan}" v0.61.0 final 0.61 \
        refs/remotes/origin/release/0.61 "${product_commit}" \
        "generated release backport commit on release/0.61"
    git -C "${REPO}" add .
    git -C "${REPO}" commit --quiet \
        -m "chore(release): prepare v0.61.0"
    source_commit="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" push --quiet origin main

    git -C "${REPO}" checkout --quiet release/0.61
    git -C "${REPO}" cherry-pick --quiet --no-commit "${source_commit}"
    git -C "${REPO}" rm --quiet .github/workflows/release.yaml
    git -C "${REPO}" commit --quiet \
        -m "chore(release): prepare v0.61.0" \
        -m "(cherry picked from commit ${source_commit})"
    backport_commit="$(git -C "${REPO}" rev-parse HEAD)"
    git -C "${REPO}" push --quiet origin release/0.61
    git -C "${REPO}" fetch --quiet origin \
        '+refs/heads/release/0.61:refs/remotes/origin/release/0.61'
    printf '%s\n%s\n' "${source_commit}" "${backport_commit}" \
        >"${TEST_ROOT}/existing-commits.txt"
}

test_release_pr_waits_for_backport() {
    local source backport output

    read -r source <"${TEST_ROOT}/existing-commits.txt"
    backport="$(sed -n '2p' "${TEST_ROOT}/existing-commits.txt")"
    output="${TEST_ROOT}/waiting-output"
    run_validator "${REPO}/.github/release-plans/v0.61.0.yaml" v0.61.0 \
        "${source}" "${source}" release-pr "${output}"
    assert_success || return
    [[ "$(<"${output}/ready.txt")" == "false" ]] || {
        fail_test "existing-channel release PR did not wait for backport"
        return
    }
    [[ "$(yq -r '.source.releaseCommit' \
        "${output}/release-plan.yaml")" == "null" ]] || {
        fail_test "waiting plan resolved its release commit too early"
        return
    }
    [[ -n "${backport}" ]] || {
        fail_test "backport fixture was not created"
        return
    }
    pass_test
}

test_backport_resolves_release_commit() {
    local source backport output

    read -r source <"${TEST_ROOT}/existing-commits.txt"
    backport="$(sed -n '2p' "${TEST_ROOT}/existing-commits.txt")"
    output="${TEST_ROOT}/backport-output"
    run_validator "${REPO}/.github/release-plans/v0.61.0.yaml" v0.61.0 \
        "${source}" "${backport}" release-backport "${output}"
    assert_success || return
    [[ "$(<"${output}/ready.txt")" == "true" ]] || {
        fail_test "release backport was not ready"
        return
    }
    [[ "$(<"${output}/de-image-tag.txt")" == "0.61" ]] || {
        fail_test "stable release selected the wrong Deployment Engine tag"
        return
    }
    pass_test
}

test_rejects_release_branch_advance() {
    local source backport

    read -r source <"${TEST_ROOT}/existing-commits.txt"
    backport="$(sed -n '2p' "${TEST_ROOT}/existing-commits.txt")"
    printf 'later change\n' >"${REPO}/later.txt"
    git -C "${REPO}" add later.txt
    git -C "${REPO}" commit --quiet -m "fix: advance release branch"
    git -C "${REPO}" push --quiet origin release/0.61
    git -C "${REPO}" fetch --quiet origin \
        '+refs/heads/release/0.61:refs/remotes/origin/release/0.61'
    run_validator "${REPO}/.github/release-plans/v0.61.0.yaml" v0.61.0 \
        "${source}" "${backport}" dispatch \
        "${TEST_ROOT}/advanced-output"
    assert_failure_contains "expected ${backport}" || return
    pass_test
}

main() {
    command -v yq >/dev/null || {
        echo "yq is required" >&2
        exit 1
    }
    init_fixture
    write_initial_files
    test_first_rc_resolves_squash_commit
    test_rejects_requested_version_mismatch
    test_rejects_conflicting_radius_tag
    create_existing_channel_release
    test_release_pr_waits_for_backport
    test_backport_resolves_release_commit
    test_rejects_release_branch_advance

    if ((FAIL > 0)); then
        echo "Controller plan tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Controller plan tests passed (${PASS} tests)"
}

main "$@"
