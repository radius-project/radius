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
        "controller concurrency must queue matching attempts"
    assert_json "${CONTROLLER}" '.permissions' '{}' \
        "controller must disable default GITHUB_TOKEN permissions"
    assert_json "${CONTROLLER}" '.jobs.validate.permissions' \
        '{"actions":"read","contents":"read","pull-requests":"read"}' \
        "controller validation must use a read-only GITHUB_TOKEN"
    assert_json "${ENTRY}" '.jobs.reconcile.permissions' \
        '{"actions":"write","contents":"read","pull-requests":"read"}' \
        "entry must grant only the scopes the controller uses"
}

test_approval_gate_and_bindings() {
    local gateway

    assert_yq "${CONTROLLER}" \
        '.jobs.approve.environment == "release" and
        ([.jobs.validate.steps[] |
        select((.with.script // "") |
            contains("verify-release-environment.mjs")) |
        .env.INPUT_ENVIRONMENT] | (length == 1 and .[0] == "release"))' \
        "validation must fail closed unless approval requires reviewers"
    assert_json "${CONTROLLER}" '.jobs.validate.outputs."de-source-commit"' \
        "\"\${{ steps.de-tag.outputs.commit }}\"" \
        "validation must record the Deployment Engine commit it showed"
    assert_json "${CONTROLLER}" \
        '.jobs."publish-deployment-engine".steps[] |
        select(.id == "de-tag") | .env.APPROVED_DE_COMMIT' \
        "\"\${{ needs.validate.outputs.de-source-commit }}\"" \
        "publication must reject a Deployment Engine tag moved after approval"
    assert_yq "${CONTROLLER}" \
        '.jobs."publish-deployment-engine".steps[] |
        select(.id == "staged") |
        ((has("continue-on-error") | not) and
        (.run | contains("--allow-absent")) and
        (.run | contains("--allow-stale") | not))' \
        "immutable image conflicts must fail rather than dispatch a replacement"
    assert_contains "${ENTRY}" \
        "if [[ \"\${RESOLVE_RESULT}\" == \"failure\" ]]" \
        "entry summary must suggest remediation only after a failure"
    for gateway in "${APPROVE}" "${RESUME}"; do
        assert_json "${gateway}" '.jobs.dispatch.if' \
            "\"github.repository == 'radius-project/radius'\"" \
            "$(basename "${gateway}") must not dispatch from forks"
    done
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
    assert_yq "${CONTROLLER}" \
        '.jobs."create-radius-tag"."timeout-minutes" == 12' \
        "Radius tag creation must allow five minutes for build discovery"
    assert_contains "${CONTROLLER}" \
        'verify-deployment-engine-tag.sh' \
        "controller must verify the Deployment Engine tag"
    assert_contains "${CONTROLLER}" \
        'verify-deployment-engine-image.sh' \
        "controller must verify the Deployment Engine destination"
    assert_contains "${CONTROLLER}" \
        'reconcile-release-controller-lock.mjs' \
        "controller must persist the Deployment Engine digest lock"
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
    assert_json "${ENTRY}" '.jobs.resolve.steps[0].with.ref' \
        "\"\${{ github.workflow_sha }}\"" \
        "resolver code must be pinned to the executing workflow commit"
    assert_json "${CONTROLLER}" \
        '[.jobs.validate, .jobs."publish-deployment-engine",
          .jobs."reconcile-siblings", .jobs."create-radius-tag"] |
          map(.steps[0].with.ref) | unique' \
        "[\"\${{ github.workflow_sha }}\"]" \
        "controller jobs must execute the same trusted code revision"
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
}

test_frozen_output_contract() {
    local job

    assert_json "${CONTROLLER}" '.jobs.validate.outputs."plan-artifact-id"' \
        "\"\${{ steps.upload-plan.outputs.artifact-id }}\"" \
        "validation must identify its immutable plan artifact"
    assert_yq "${CONTROLLER}" \
        '.jobs.validate.steps[] | select(.id == "upload-plan") |
        .with.path | contains("/release-controller/release-targets.json")' \
        "plan artifact must carry the approved output contract"
    assert_yq "${CONTROLLER}" \
        ".jobs.validate.steps[] | select(.id == \"upload-plan\") |
        .with.name | contains(\"\${{ github.run_attempt }}\")" \
        "reruns must retain earlier attempts instead of replacing their artifacts"
    assert_json "${CONTROLLER}" \
        '[.jobs."publish-deployment-engine", .jobs."create-radius-tag"] |
        [.[].steps[] | select(.run // "" |
        contains("verify-deployment-engine-image.sh")) |
        .env.GORELEASER_PARITY_TARGETS]' \
        "[\"\${{ runner.temp }}/release-controller/release-targets.json\",\"\${{ runner.temp }}/release-controller/release-targets.json\",\"\${{ runner.temp }}/release-controller/release-targets.json\"]" \
        "every image verification must use the approved output contract"
    for job in publish-deployment-engine create-radius-tag; do
        assert_json "${CONTROLLER}" \
            ".jobs.\"${job}\".steps[] |
            select(.name == \"Download approved release contract\") |
            [(.uses | test(\"^actions/download-artifact@[0-9a-fA-F]{40}\$\")),
            .with.artifact-ids, .with.path, .with.merge-multiple]" \
            "[true,\"\${{ needs.validate.outputs.plan-artifact-id }}\",\"\${{ runner.temp }}/release-controller\",true]" \
            "${job} must use SHA-pinned actions/download-artifact to download the exact validated artifact into the expected path"
    done
}

test_release_build_contract() {
    assert_json "${CONTROLLER}" \
        '[.jobs.validate.steps[] | select((.with.script // "") |
        contains("verifyReleaseBuildSource")) |
        [.if, .env.INPUT_SOURCE_SHA, .env.REQUIRE_BICEP_RECIPES]]' \
        "[[\"steps.plan.outputs.ready == 'true'\",\"\${{ inputs.source-commit }}\",\"\${{ steps.plan.outputs.recipe-source-commit != '' }}\"]]" \
        "validation must check the exact release commit and require recipes only for pinned plans"
    assert_contains "${WORKFLOWS}/prepare-release.yaml" \
        'verify-release-build-contract.sh' \
        "Prepare Release must reject an unmigrated release branch before opening a pull request"
    assert_contains "${WORKFLOWS}/prepare-release.yaml" \
        '--expected-targets .github/release-parity/targets.json' \
        "preparation must bind intended targets to the source contract"
    assert_contains "${ROOT}/.github/scripts/resolve-release-controller.mjs" \
        '"--expected-targets"' \
        "controller must bind approved targets before mutation"
    assert_contains "${ROOT}/.github/scripts/resolve-release-controller.mjs" \
        'writeFile(targetsFile, JSON.stringify(plan.expectedOutputs))' \
        "controller targets must come from the approved plan"
}

publish_step_index() {
    NAME="$1" yq -r '.jobs."publish-deployment-engine".steps | to_entries |
        .[] | select(.value.name == strenv(NAME)) | .key' "${CONTROLLER}"
}

test_deployment_engine_tag_automation() {
    local create verify wait inspect workflow

    # yq binds "and" more loosely than "|", so each combined condition after a
    # pipe is parenthesized to evaluate against the selected step.
    export ABSENT_IF="needs.validate.outputs.de-tag-state == 'absent'"
    assert_json "${CONTROLLER}" '.jobs.validate.outputs."de-tag-state"' \
        "\"\${{ steps.de-tag.outputs.state }}\"" \
        "validation must expose whether the Deployment Engine tag exists"
    assert_yq "${CONTROLLER}" \
        ".jobs.validate.steps[] | select(.id == \"de-tag\") |
        ((.run | contains(\"args=(--allow-absent main \\\"\${VERSION}\\\")\")) and
        (.run | contains(\"--allow-absent \\\"\${RELEASE_CANDIDATE}\\\"\")) and
        (.run | contains(\"\${RC_DE_COMMIT:?\")) and
        .env.RELEASE_TYPE == \"\${{ steps.plan.outputs.release-type }}\" and
        .env.RELEASE_CANDIDATE ==
            \"\${{ steps.plan.outputs.previous-version }}\" and
        .env.RC_DE_COMMIT ==
            \"\${{ steps.rc-lock.outputs.de-source-commit }}\")" \
        "validation must plan an absent tag at main or the commit the last RC locked"
    for workflow in "${CONTROLLER}" "${WORKFLOWS}/prepare-release.yaml"; do
        assert_yq "${workflow}" \
            '[.jobs[].steps[] | select(.id == "rc-lock")] |
            ((length == 1) and (.[0].if | contains("final")) and
            .[0].env.INPUT_MODE == "release-candidate" and
            (.[0].with.script |
                contains("reconcile-release-controller-lock.mjs")))' \
            "$(basename "${workflow}") must bind a final to its RC's trusted lock"
        assert_yq "${workflow}" \
            '[.jobs[].steps[] | select(.with.repositories == "deployment-engine" and
                .with.owner != "radius-project")] | length == 0' \
            "$(basename "${workflow}") must not request the old installation"
    done
    local spec job id access
    for spec in 'prepare-release.yaml prepare de-token read' \
        '__release-controller.yaml validate de-read-token read' \
        '__release-controller.yaml publish-deployment-engine de-read-token read' \
        '__release-controller.yaml publish-deployment-engine de-write-token write' \
        'build-release.yaml publish-sibling-releases engine-token write'; do
        read -r workflow job id access <<< "${spec}"
        assert_json "${WORKFLOWS}/${workflow}" \
            "[.jobs.\"${job}\".steps[] | select(.id == \"${id}\") |
              [.with.owner, .with.repositories, .with.\"client-id\", .with.\"private-key\", .with.\"permission-contents\"]]" \
            "[[\"radius-project\",\"deployment-engine\",\"\${{ secrets.RADIUS_PUBLISHER_BOT_CLIENT_ID }}\",\"\${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}\",\"${access}\"]]" \
            "${job}.${id} must request the scoped publisher App token"
    done
    assert_yq "${WORKFLOWS}/prepare-release.yaml" \
        ".jobs.prepare.steps[] |
        select(.name == \"Verify or plan Deployment Engine tag\") |
        ((.run | contains(\"args=(--allow-absent main \\\"\${VERSION}\\\")\")) and
        (.run | contains(\"--allow-absent \\\"\${RELEASE_CANDIDATE}\\\"\")) and
        (.run | contains(\"\${RC_DE_COMMIT:?\")) and
        .env.RC_DE_COMMIT ==
            \"\${{ steps.rc-lock.outputs.de-source-commit }}\")" \
        "Prepare Release must check the controller's tag plan before the pull request"
    assert_yq "${CONTROLLER}" \
        '[.jobs[].steps[] | select(.with."permission-contents" == "write" and
            .with.repositories == "deployment-engine")] |
        ((length == 1) and .[0].if == strenv(ABSENT_IF))' \
        "only an approved absent tag may mint a Deployment Engine write token"
    assert_yq "${CONTROLLER}" \
        ".jobs.\"publish-deployment-engine\".steps[] |
        select(.name == \"Create approved Deployment Engine tag\") |
        (.if == strenv(ABSENT_IF) and
        .env.RELEASE_RECONCILE_TAG_ONLY == \"true\" and
        .env.APPROVED_DE_COMMIT ==
            \"\${{ needs.validate.outputs.de-source-commit }}\" and
        .env.APP_SLUG == \"\${{ steps.de-write-token.outputs.app-slug }}\" and
        (.run | contains(\"== \\\"radius-publisher\\\"\")) and
        (.run | contains(\"release-create-tag-and-branch.sh\")))" \
        "only the Radius Publisher App may create the approved Deployment Engine tag"
    assert_yq "${CONTROLLER}" \
        '.jobs."publish-deployment-engine".steps[] | select(.id == "de-tag") |
        (.run | contains("--allow-absent") | not)' \
        "the tag must exist at the approved commit before publication"
    assert_yq "${CONTROLLER}" \
        ".jobs.\"publish-deployment-engine\".steps[] |
        select(.name == \"Wait for the Deployment Engine tag build\") |
        .if == \"steps.de-lock.outputs.exists != 'true'\"" \
        "every unlocked attempt, including a resume after tag creation, must wait for the tag build"
    assert_yq "${CONTROLLER}" \
        '.jobs."publish-deployment-engine"."timeout-minutes" == 40' \
        "publication must allow time for the Deployment Engine tag build"
    create="$(publish_step_index "Create approved Deployment Engine tag")"
    verify="$(publish_step_index "Reverify approved Deployment Engine tag")"
    wait="$(publish_step_index "Wait for the Deployment Engine tag build")"
    inspect="$(publish_step_index "Verify published Deployment Engine image")"
    if [[ -z "${create}" || -z "${verify}" || -z "${wait}" || -z "${inspect}" ]] ||
        ((create >= verify || verify >= wait || wait >= inspect)); then
        fail_test "tag creation, verification, and build wait must precede image inspection"
    else
        ((++PASS))
    fi
}

# A resumed attempt can find the tag that an interrupted attempt pushed while
# its build is still running. It must keep polling until the image is staged
# instead of dispatching a publisher that cannot find the source image yet.
test_resume_waits_for_running_tag_build() {
    if ! (
        root="$(mktemp -d "${TMPDIR:-/tmp}/de-wait-test-XXXXXX")"
        trap 'rm -rf "${root}"' EXIT
        mkdir -p "${root}/.github/scripts"
        cat >"${root}/.github/scripts/verify-deployment-engine-image.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
checks=$(($(cat "${CHECKS}" 2>/dev/null || echo 0) + 1))
echo "${checks}" >"${CHECKS}"
while [[ "$1" != "--state-output" ]]; do shift; done
if ((checks < 3)); then echo absent; else echo complete; fi >"$2"
echo '{"digest":"sha256:test"}' >"${RUNNER_TEMP}/de-image-wait.json"
EOF
        script="$(yq -r '.jobs."publish-deployment-engine".steps[] |
            select(.name == "Wait for the Deployment Engine tag build") |
            .run' "${CONTROLLER}")"
        # shellcheck disable=SC2329 # Invoked by the exported step script.
        sleep() { :; }
        export -f sleep
        cd "${root}" || exit 1
        CHECKS="${root}/checks" RUNNER_TEMP="${root}" GITHUB_OUTPUT="${root}/outputs" \
            DE_IMAGE_TAG=0.62.0-rc.1 RELEASE_VERSION=v0.62.0-rc.1 \
            DE_SOURCE_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
            bash -euo pipefail -c "${script}" >/dev/null || exit 1
        [[ "$(<"${root}/checks")" == "3" && "$(<"${root}/outputs")" == "digest=sha256:test" ]]
    ); then
        fail_test "a resumed attempt must poll until the tag build stages the image"
        return
    fi
    ((++PASS))
}

# Runs the validate job's Radius pin step against a bicep-types-aws checkout
# whose radius gitlink is $1, or that has no radius path when $1 is empty.
run_radius_pin_step() {
    local pinned="$1"
    local product="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    local root checkout script status=0

    script="$(yq -r '.jobs.validate.steps[] |
        select(.run // "" | contains("verify-bicep-types-aws-radius-pin.sh")) |
        .run' "${CONTROLLER}")"
    [[ -n "${script}" ]] || {
        echo "the validate job has no Radius pin step"
        return 1
    }
    root="$(mktemp -d "${TMPDIR:-/tmp}/radius-pin-test-XXXXXX")"
    checkout="${root}/controller-siblings/bicep-types-aws"
    git init -q -b main "${checkout}"
    git -C "${checkout}" config user.name "Radius Test"
    git -C "${checkout}" config user.email "test@example.com"
    git -C "${checkout}" config commit.gpgsign false
    printf 'state\n' > "${checkout}/state.txt"
    git -C "${checkout}" add state.txt
    if [[ -n "${pinned}" ]]; then
        git -C "${checkout}" update-index --add \
            --cacheinfo "160000,${pinned},radius"
    fi
    git -C "${checkout}" commit -q -m "Planned state"
    printf 'source:\n  productCommit: %s\n' "${product}" \
        > "${root}/release-plan.yaml"
    (
        cd "${root}" &&
            GITHUB_WORKSPACE="${ROOT}" RUNNER_TEMP="${root}" \
                BICEP_TYPES_AWS_COMMIT="$(git -C "${checkout}" rev-parse HEAD)" \
                bash -euo pipefail -c "${script}" 2>&1
    ) || status=$?
    rm -rf "${root}"
    return "${status}"
}

validate_step_index() {
    yq -r ".jobs.validate.steps | to_entries | map(select($1)) |
        .[0].key // -1" "${CONTROLLER}"
}

# A failed release plan check alone can't block tagging, because merging the
# release PR doesn't require it. The controller checks the frozen commit again
# before approval, so every mutating job, which needs validate, never starts.
test_controller_checks_radius_pin() {
    local matching="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    local other="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    local output check checkout

    check="$(validate_step_index \
        '.value.run // "" | contains("verify-bicep-types-aws-radius-pin.sh")')"
    checkout="$(validate_step_index \
        '.value.name == "Checkout planned bicep-types-aws source"')"
    if ((check < 0 || check < checkout)); then
        fail_test "validation must check the frozen bicep-types-aws Radius pin after checking it out"
    else
        ((++PASS))
    fi
    assert_json "${CONTROLLER}" \
        ".jobs.validate.steps[${check}].if" \
        "\"steps.plan.outputs.ready == 'true'\"" \
        "the Radius pin check must run whenever the controller can mutate"
    if output="$(run_radius_pin_step "${other}")" ||
        [[ "${output}" != *"pins Radius ${other}, but this release ships Radius ${matching}"* ]]; then
        fail_test "a mismatched bicep-types-aws Radius pin must stop the controller before approval: ${output}"
    else
        ((++PASS))
    fi
    if ! output="$(run_radius_pin_step "${matching}")"; then
        fail_test "a matching bicep-types-aws Radius pin must pass: ${output}"
    else
        ((++PASS))
    fi
    if ! output="$(run_radius_pin_step "")" ||
        [[ "${output}" != *"::notice::"*"has no radius submodule"* ]]; then
        fail_test "bicep-types-aws without a radius submodule must pass with a notice: ${output}"
    else
        ((++PASS))
    fi
}

test_bicep_evidence_boundary() {
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps |
        map(select(.env.INPUT_OPERATION == "read" or .env.INPUT_OPERATION == "select") |
            .env.INPUT_RELEASE_APP_SLUG)' \
        "[\"\${{ steps.release-token.outputs.app-slug }}\",\"\${{ steps.release-token.outputs.app-slug }}\"]" \
        "state readers must bind the configured release App identity"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.env.INPUT_OPERATION == "persist") | .env.INPUT_RELEASE_APP_SLUG' \
        "\"\${{ steps.bicep-state-token.outputs.app-slug }}\"" \
        "state writer readback must bind its configured App identity"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.id == "aws-read") |
        [.with.repositories, .with."permission-actions",
          .with."permission-contents", .with."permission-metadata"]' \
        '["bicep-types-aws","read","read","read"]' \
        "AWS evidence token must be a repository-scoped read token"
    assert_yq "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps |
        map(select(.env.INPUT_OPERATION != null or
          .id == "aws-read" or .id == "bicep-state-token")) |
        all_c(.if | contains("ghcr-bicep == '\''true'\''"))' \
        "legacy plans must not collect or persist new Bicep evidence"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.id == "bicep-state-token") |
        [.with.repositories, .with."permission-contents"]' \
        '["radius","write"]' \
        "only an explicit Radius token may persist Bicep evidence"
    assert_contains "${ROOT}/.github/scripts/collect-release-bicep.mjs" \
        'automation/bicep-release-state-' \
        "Bicep evidence must use a separate state ref"
    assert_not_contains "${ROOT}/.github/scripts/collect-release-bicep.mjs" \
        'automation/release-state-' \
        "Bicep persistence must not alter Deployment Engine state"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.id == "aws-publication") |
        [.with."github-token", .env.AWS_DISPATCH_TOKEN,
          .env.BICEP_STATE_TOKEN]' \
        "[\"\${{ steps.aws-read.outputs.token }}\",\"\${{ steps.release-token.outputs.token }}\",\"\${{ steps.bicep-state-token.outputs.token }}\"]" \
        "AWS reads, dispatch and Radius marker use separate scoped clients"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.id == "release-token") |
        [.with.repositories | contains("bicep-types-aws")]' \
        '[true]' \
        "existing sibling token must include the AWS dispatch repository"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps |
        map(select(.env.INPUT_OPERATION != null) |
          .env.INPUT_OPERATION)' \
        '["read","select","authenticate","snapshot","persist"]' \
        "raw source/controller binding must precede evidence persistence"
    assert_json "${CONTROLLER}" \
        '.jobs."reconcile-siblings".steps[] |
        select(.env.INPUT_OPERATION == "snapshot") |
        .with."github-token"' \
        "\"\${{ steps.aws-read.outputs.token }}\"" \
        "original generation validation must retain read-only AWS authority"
}

main() {
    command -v yq >/dev/null || {
        echo "yq is required" >&2
        exit 1
    }
    test_entry_triggers
    test_shared_controller_contract
    test_approval_gate_and_bindings
    test_frozen_output_contract
    test_stage_order
    test_controller_checks_radius_pin
    test_release_build_contract
    test_deployment_engine_tag_automation
    test_resume_waits_for_running_tag_build
    test_identity_and_cleanup
    test_bicep_evidence_boundary

    if ((FAIL > 0)); then
        echo "Release controller tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Release controller tests passed (${PASS} tests)"
}

main "$@"
