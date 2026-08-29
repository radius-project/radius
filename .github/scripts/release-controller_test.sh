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
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly ROOT
readonly WORKFLOWS="${ROOT}/.github/workflows"
readonly CONTROLLER="${WORKFLOWS}/__release-controller.yaml"
readonly DISPATCHER="${WORKFLOWS}/__dispatch-release-controller.yaml"
readonly ENTRY="${WORKFLOWS}/release-controller.yaml"
readonly APPROVE="${WORKFLOWS}/approve-release.yaml"
readonly RESUME="${WORKFLOWS}/resume-release.yaml"

PASS=0
FAIL=0

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

assert_yq() {
    local file="$1"
    local expression="$2"
    local message="$3"

    if [[ "$(yq -r "${expression}" "${file}")" != "true" ]]; then
        fail_test "${message}"
        return
    fi
    ((++PASS))
}

assert_json() {
    local file="$1"
    local expression="$2"
    local expected="$3"
    local message="$4"
    local actual

    actual="$(yq -o=json -I=0 "${expression}" "${file}" | tr -d '\r\n')"
    if [[ "${actual}" != "${expected}" ]]; then
        fail_test "${message}: got ${actual}"
        return
    fi
    ((++PASS))
}

assert_contains() {
    local file="$1"
    local expected="$2"
    local message="$3"

    if ! grep -Fq -- "${expected}" "${file}"; then
        fail_test "${message}"
        return
    fi
    ((++PASS))
}

assert_not_contains() {
    local file="$1"
    local unexpected="$2"
    local message="$3"

    if grep -Fq -- "${unexpected}" "${file}"; then
        fail_test "${message}"
        return
    fi
    ((++PASS))
}

test_entry_triggers() {
    assert_json "${ENTRY}" '.on.pull_request_target.types' \
        '["closed"]' \
        "controller must start from merged pull requests"
    assert_json "${ENTRY}" '.on.pull_request_target.branches' \
        '["main","release/*"]' \
        "controller must observe release PR and metadata backport merges"
    assert_json "${ENTRY}" '.on.repository_dispatch.types' \
        '["release-controller.approve","release-controller.resume"]' \
        "controller must accept only approved default-branch dispatches"
    assert_yq "${APPROVE}" \
        '.on.workflow_dispatch.inputs.version.required == true and
        .on.workflow_dispatch.inputs."source-commit".required == true' \
        "Approve Release must require version and source"
    assert_yq "${RESUME}" \
        '.on.workflow_dispatch.inputs.version.required == true and
        .on.workflow_dispatch.inputs."source-commit".required == true' \
        "Resume Release must require version and source"
}

test_shared_controller_contract() {
    assert_yq "${ENTRY}" \
        '.jobs.reconcile.uses ==
        "./.github/workflows/__release-controller.yaml"' \
        "default-branch entry must call the shared controller"
    assert_yq "${APPROVE}" \
        '.jobs.dispatch.uses ==
        "./.github/workflows/__dispatch-release-controller.yaml"' \
        "Approve Release must call the secretless dispatcher"
    assert_yq "${RESUME}" \
        '.jobs.dispatch.uses ==
        "./.github/workflows/__dispatch-release-controller.yaml"' \
        "Resume Release must call the secretless dispatcher"
    assert_yq "${CONTROLLER}" \
        ".concurrency.queue == \"max\" and
        .concurrency.group ==
        \"release-\${{ inputs.version }}-\${{ inputs.source-commit }}\"" \
        "controller concurrency must preserve every matching attempt"
    assert_json "${CONTROLLER}" '.permissions' '{}' \
        "controller must disable default GITHUB_TOKEN permissions"
    assert_json "${CONTROLLER}" '.jobs.validate.permissions' \
        '{"contents":"read","pull-requests":"read"}' \
        "controller validation must use a read-only GITHUB_TOKEN"
}

test_stage_order() {
    assert_json "${CONTROLLER}" \
        '.jobs."publish-deployment-engine".needs' \
        '["validate","approve"]' \
        "Deployment Engine must publish after validation and approval"
    assert_json "${CONTROLLER}" '.jobs."reconcile-siblings".needs' \
        '["validate","publish-deployment-engine"]' \
        "sibling reconciliation must follow Deployment Engine"
    assert_json "${CONTROLLER}" '.jobs."create-radius-tag".needs' \
        '["validate","publish-deployment-engine","reconcile-siblings"]' \
        "Radius tag creation must be the last mutation stage"
    assert_contains "${CONTROLLER}" \
        'verify-deployment-engine-tag.sh' \
        "controller must hard-block on the signed Deployment Engine tag"
    assert_contains "${CONTROLLER}" \
        'monitor-remote-workflow.mjs' \
        "controller must use correlated Deployment Engine publication"
    assert_contains "${CONTROLLER}" \
        'verify-deployment-engine-image.sh' \
        "controller must verify the Deployment Engine destination"
    assert_contains "${CONTROLLER}" \
        'reconcile-release-controller-lock.mjs' \
        "controller must persist the Deployment Engine digest lock"
    assert_contains "${CONTROLLER}" \
        'INPUT_DISPATCH_IF_MISSING:' \
        "valid unlocked Deployment Engine state must use query-only monitoring"
    assert_contains "${CONTROLLER}" \
        "steps.existing-image.outputs.state == 'complete'" \
        "valid unlocked Deployment Engine state must suppress dispatch"
    assert_contains "${CONTROLLER}" \
        'steps.existing-image.outputs.digest' \
        "valid unlocked Deployment Engine state must be locked by digest"
    assert_contains "${CONTROLLER}" \
        ".github/release-plans/\${VERSION}.yaml" \
        "controller must read the immutable committed plan"
    assert_contains "${CONTROLLER}" \
        'steps.plan.outputs.recipes-commit' \
        "controller must use the frozen recipes commit"
    assert_yq "${CONTROLLER}" \
        '.jobs.summary.if == "always()"' \
        "controller summary must run after failures"
    assert_contains "${CONTROLLER}" \
        'gh workflow run resume-release.yaml --ref main' \
        "controller summary must provide the exact resume workflow"
    assert_yq "${ENTRY}" \
        '.jobs.summary.if | contains("always()")' \
        "controller entry summary must run after resolution failures"
    assert_contains "${DISPATCHER}" 'if: always()' \
        "manual gateway summary must run after dispatch failures"
    assert_contains "${CONTROLLER}" \
        "radius \"\${VERSION}\" \"\${RELEASE_BRANCH}\" \"\${SOURCE_COMMIT}\"" \
        "Radius reconciliation must use the exact approved source"
}

test_identity_and_cleanup() {
    assert_not_contains "${CONTROLLER}" 'persist-credentials: true' \
        "controller must not persist checkout credentials"
    assert_contains "${CONTROLLER}" 'gh auth setup-git' \
        "controller must configure short-lived App git credentials"
    assert_not_contains "${APPROVE}" 'RADIUS_RELEASE_BOT' \
        "Approve Release must not receive release App credentials"
    assert_not_contains "${RESUME}" 'RADIUS_RELEASE_BOT' \
        "Resume Release must not receive release App credentials"
    assert_not_contains "${DISPATCHER}" 'RADIUS_RELEASE_BOT' \
        "manual dispatcher must not receive release App credentials"
    if [[ -e "${WORKFLOWS}/release.yaml" ]]; then
        fail_test "legacy versions.yaml release workflow still exists"
    else
        ((++PASS))
    fi
    assert_not_contains "${ROOT}/.github/ghalint.yaml" \
        '.github/workflows/release.yaml' \
        "deleted release workflow still has a lint exception"
    assert_not_contains "${WORKFLOWS}/__changes.yml" \
        '.github/workflows/release.yaml' \
        "deleted release workflow remains in change classification"
    assert_contains "${ROOT}/.github/scripts/create-release-backport.sh" \
        'git rm -q .github/workflows/release.yaml' \
        "metadata backports must remove legacy release-branch workflows"
}

main() {
    command -v yq >/dev/null || {
        echo "yq is required" >&2
        exit 1
    }
    test_entry_triggers
    test_shared_controller_contract
    test_stage_order
    test_identity_and_cleanup

    if ((FAIL > 0)); then
        echo "Release controller tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Release controller tests passed (${PASS} tests)"
}

main "$@"
