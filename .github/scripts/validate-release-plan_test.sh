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
readonly SCRIPT="${SCRIPT_DIR}/validate-release-plan.sh"

TEST_ROOT=""
REPO=""
BASE_SHA=""
HEAD_DIR=""
EXPECTED_DIR=""
EXPECTED_PLAN=""
EXPECTED_BACKPORTS=""
FAKE_PREPARE=""
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

write_plan() {
    local product_commit="$1"

    cat > "${EXPECTED_PLAN}" << EOF
schemaVersion: 1
version: v0.61.0-rc.1
releaseType: rc
channel: "0.61"
releaseDate: 2026-08-24
chartVersion: 0.61.0-rc.1
source:
  productRef: HEAD
  productCommit: ${product_commit}
  releaseCommit: null
  releaseCommitResolution: release PR squash commit on main
releaseBranch: release/0.61
previousVersion: v0.60.0
expectedOutputs:
  repository: radius-project/radius
includedBackports: []
EOF
}

wrap_body() {
    {
        echo '## Release plan'
        echo
        echo '<!-- radius-release-plan:start -->'
        echo '```yaml'
        cat "${EXPECTED_PLAN}"
        echo '```'
        echo '<!-- radius-release-plan:end -->'
    } > "${REPO}/body.md"
}

setup_repo() {
    REPO="${TEST_ROOT}/repo"
    HEAD_DIR="${TEST_ROOT}/head"
    EXPECTED_DIR="${TEST_ROOT}/expected"
    EXPECTED_PLAN="${TEST_ROOT}/expected-plan.yaml"
    EXPECTED_BACKPORTS="${TEST_ROOT}/expected-backports.json"
    FAKE_PREPARE="${TEST_ROOT}/fake-prepare.sh"
    rm -rf "${REPO}" "${HEAD_DIR}" "${EXPECTED_DIR}"
    mkdir -p "${REPO}"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false
    cat > "${REPO}/versions.yaml" << 'EOF'
supported:
  - channel: '0.60'
    version: 'v0.60.0'
deprecated: []
EOF
    git -C "${REPO}" add versions.yaml
    git -C "${REPO}" commit -q -m "chore: initial"
    git -C "${REPO}" tag v0.60.0
    BASE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
    write_plan "${BASE_SHA}"
    wrap_body
    printf '%s\n' \
        '["CHANGELOG.md","docs/release-notes/v0.61.0-rc.1.md","versions.yaml"]' \
        > "${REPO}/files.json"

    mkdir -p "${EXPECTED_DIR}/docs/release-notes"
    cat > "${EXPECTED_DIR}/versions.yaml" << 'EOF'
supported:
  - channel: "0.61"
    version: v0.61.0-rc.1
deprecated:
  - channel: '0.60'
    version: 'v0.60.0'
EOF
    cat > "${EXPECTED_DIR}/CHANGELOG.md" << 'EOF'
# Changelog

## [Unreleased]

## [0.61.0-rc.1] - 2026-08-24

### Fixed

- Fix release preparation
EOF
    cat > "${EXPECTED_DIR}/docs/release-notes/v0.61.0-rc.1.md" << 'EOF'
## Announcing Radius v0.61.0-rc.1

## Highlights

<!-- CURATE HIGHLIGHTS -->

## Upgrading to Radius v0.61.0-rc.1

<!-- CURATE UPGRADING -->

## Full changelog

### Fixed

- Fix release preparation
EOF
    cp -R "${EXPECTED_DIR}/." "${HEAD_DIR}/"
    printf '[]\n' > "${EXPECTED_BACKPORTS}"
    cat > "${FAKE_PREPARE}" << 'EOF'
#!/bin/bash
set -euo pipefail
output_dir=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir) output_dir="$2"; shift 2 ;;
        *) shift ;;
    esac
done
mkdir -p "${output_dir}" docs/release-notes
cp "${EXPECTED_RELEASE_DIR}/versions.yaml" versions.yaml
cp "${EXPECTED_RELEASE_DIR}/CHANGELOG.md" CHANGELOG.md
cp "${EXPECTED_RELEASE_DIR}/docs/release-notes/v0.61.0-rc.1.md" \
    docs/release-notes/v0.61.0-rc.1.md
cp "${EXPECTED_PLAN_FILE}" "${output_dir}/release-plan.yaml"
EOF
    chmod +x "${FAKE_PREPARE}"
}

run_validator() {
    local status

    pushd "${REPO}" > /dev/null
    set +e
    PREPARE_RELEASE_SCRIPT="${FAKE_PREPARE}" \
        EXPECTED_BACKPORTS_FILE="${EXPECTED_BACKPORTS}" \
        EXPECTED_RELEASE_DIR="${EXPECTED_DIR}" \
        EXPECTED_PLAN_FILE="${EXPECTED_PLAN}" \
        bash "${SCRIPT}" --body-file body.md --files-file files.json \
        --base-sha "${BASE_SHA}" --head-dir "${HEAD_DIR}" \
        --repository radius-project/radius
    status=$?
    set -e
    popd > /dev/null
    return "${status}"
}

test_accepts_generated_plan() {
    if ! run_validator > /dev/null; then
        fail_test "expected the generated plan to pass"
        return
    fi
    ((++PASS))
}

test_accepts_curated_note_sections() {
    sed -i 's/<!-- CURATE HIGHLIGHTS -->/A curated highlight./' \
        "${HEAD_DIR}/docs/release-notes/v0.61.0-rc.1.md"
    sed -i 's/<!-- CURATE UPGRADING -->/Run the documented upgrade command./' \
        "${HEAD_DIR}/docs/release-notes/v0.61.0-rc.1.md"
    if ! run_validator > /dev/null; then
        fail_test "expected curated note sections to pass"
        return
    fi
    ((++PASS))
}

test_rejects_product_commit_drift() {
    sed -i "s/${BASE_SHA}/$(printf 'f%.0s' {1..40})/" "${REPO}/body.md"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected product commit drift to fail"
        return
    fi
    ((++PASS))
}

test_rejects_unexpected_file() {
    jq '. + ["unrelated.txt"]' "${REPO}/files.json" \
        > "${REPO}/files.json.tmp"
    mv "${REPO}/files.json.tmp" "${REPO}/files.json"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected an unrelated changed file to fail"
        return
    fi
    ((++PASS))
}

test_rejects_tampered_versions() {
    yq -i '.supported[0].version = "v9.9.9"' "${HEAD_DIR}/versions.yaml"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected tampered versions.yaml to fail"
        return
    fi
    ((++PASS))
}

test_rejects_tampered_changelog() {
    printf '\n- Unplanned entry\n' >> "${HEAD_DIR}/CHANGELOG.md"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected a tampered changelog to fail"
        return
    fi
    ((++PASS))
}

test_rejects_tampered_generated_notes() {
    sed -i 's/Fix release preparation/Replace generated content/' \
        "${HEAD_DIR}/docs/release-notes/v0.61.0-rc.1.md"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected generated note changes to fail"
        return
    fi
    ((++PASS))
}

test_rejects_tampered_output_contract() {
    sed -i 's|radius-project/radius|other/repository|' "${REPO}/body.md"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected a tampered output contract to fail"
        return
    fi
    ((++PASS))
}

test_rejects_tampered_backports() {
    sed -i 's/includedBackports: \[\]/includedBackports: [{source_pr: 1, backport_merged: true}]/' \
        "${REPO}/body.md"
    if run_validator > /dev/null 2>&1; then
        fail_test "expected a tampered backport list to fail"
        return
    fi
    ((++PASS))
}

test_rejects_live_backport_state_drift() {
    cat > "${EXPECTED_BACKPORTS}" << 'EOF'
[{"source_pr":123,"backport_merged":true}]
EOF
    if run_validator > /dev/null 2>&1; then
        fail_test "expected live backport state drift to fail"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/release-plan-fixture-XXXXXX")"

    setup_repo
    test_accepts_generated_plan
    setup_repo
    test_accepts_curated_note_sections
    setup_repo
    test_rejects_product_commit_drift
    setup_repo
    test_rejects_unexpected_file
    setup_repo
    test_rejects_tampered_versions
    setup_repo
    test_rejects_tampered_changelog
    setup_repo
    test_rejects_tampered_generated_notes
    setup_repo
    test_rejects_tampered_output_contract
    setup_repo
    test_rejects_tampered_backports
    setup_repo
    test_rejects_live_backport_state_drift

    if ((FAIL > 0)); then
        echo "release plan tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "release plan tests passed (${PASS} tests)"
}

main "$@"
