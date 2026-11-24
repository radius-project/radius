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
readonly SCRIPT="${SCRIPT_DIR}/verify-release-build-contract.sh"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly ROOT

REPO=""
WORKFLOWS=""
PASS=0
FAIL=0

cleanup() {
    if [[ -n "${REPO}" && -d "${REPO}" ]]; then
        rm -rf "${REPO}"
    fi
}
trap cleanup EXIT

write_pipeline() {
    rm -rf "${REPO}/.github" "${REPO}/.goreleaser.yaml"
    mkdir -p "${WORKFLOWS}"
    cp "${ROOT}/.goreleaser.yaml" "${REPO}/.goreleaser.yaml"
    cp "${ROOT}/.github/workflows/build-release.yaml" "${WORKFLOWS}/build-release.yaml"
    cp "${ROOT}/.github/workflows/__publish-release-bicep.yaml" "${WORKFLOWS}/__publish-release-bicep.yaml"
    mkdir -p "${REPO}/.github/release-parity"
    cp "${ROOT}/.github/release-parity/targets.json" "${REPO}/.github/release-parity/targets.json"
    mkdir -p "${REPO}/.github/scripts"
    cp "${ROOT}/.github/scripts/release-parity-manifest.sh" \
        "${ROOT}/.github/scripts/collect-release-bicep.mjs" \
        "${ROOT}/.github/scripts/promote-release-bicep.mjs" "${REPO}/.github/scripts/"
    # Both run on tag pushes without publishing a release.
    printf '%s\n' 'on:' '  push:' '    paths: [setup.yaml]' > "${WORKFLOWS}/setup.yml"
    printf '%s\n' 'on: [push, pull_request]' > "${WORKFLOWS}/lint.yaml"
}

use_legacy_contract() {
    yq -i -o=json '
        del(.bicepExtensionsContract) |
        .ociArtifacts[0].repository = "biceptypes.azurecr.io/aws" |
        .ociArtifacts[1].repository = "biceptypes.azurecr.io/radius"
    ' "${REPO}/.github/release-parity/targets.json"
    yq -i '
        .jobs."build-and-push-bicep-types" = {
            "uses": "./.github/workflows/__build-bicep-types.yaml",
            "needs": ["release-preflight"],
            "if": "github.repository == '\''radius-project/radius'\'' && startsWith(github.ref, '\''refs/tags/v'\'') && needs.release-preflight.outputs.release-state != '\''published'\''"
        } |
        del(.jobs."publish-radius-bicep-extension") |
        (.jobs."finalize-release".steps[] | select(.name == "Promote locked Bicep extensions")) |= (
            .name = "Promote verified Bicep aliases" |
            .if = "env.UPDATE_RELEASE == '\''true'\'' && steps.alias-policy.outputs.promote_channel == '\''true'\''" |
            .with.script = "await import(\"./promote-bicep-types.mjs\")"
        )
    ' "${WORKFLOWS}/build-release.yaml"
}

# Commits the fixture and expects a pass, or a failure that contains $1.
expect() {
    local expected="$1"
    local description="$2"
    local revision="${3:-HEAD}"
    local option="${4:-}"
    local output
    local status=0
    local -a args=("${revision}")
    [[ -z "${option}" ]] || args+=("${option}")
    [[ -z "${TEST_EXPECTED_TARGETS:-}" ]] || args+=(--expected-targets "${TEST_EXPECTED_TARGETS}")

    git -C "${REPO}" add -A
    git -C "${REPO}" commit -q --allow-empty -m "${description}"
    output="$(cd "${REPO}" && bash "${SCRIPT}" "${args[@]}" 2>&1)" || status=$?
    if [[ "${expected}" == "pass" && ${status} -eq 0 ]] \
                                                        || [[ "${expected}" != "pass" && ${status} -ne 0 && "${output}" == *"${expected}"* ]]; then
        ((++PASS))
        return
    fi
    echo "  ASSERT FAILED: ${description}: ${output}"
    ((++FAIL))
}

main() {
    local job dependency mutation contract
    command -v yq > /dev/null || {
        echo "yq is required" >&2
        exit 1
    }
    REPO="$(mktemp -d "${TMPDIR:-/tmp}/release-build-contract-test-XXXXXX")"
    WORKFLOWS="${REPO}/.github/workflows"
    git -C "${REPO}" init -q -b main
    git -C "${REPO}" config user.name "Radius Test"
    git -C "${REPO}" config user.email "test@example.com"
    git -C "${REPO}" config commit.gpgsign false

    write_pipeline
    expect pass "current release pipeline beside non-publishing push workflows"
    local intended="${REPO}/intended-targets.json"
    cp "${ROOT}/.github/release-parity/targets.json" "${intended}"
    TEST_EXPECTED_TARGETS="${intended}" expect pass "matching intended targets"
    use_legacy_contract
    TEST_EXPECTED_TARGETS="${intended}" expect "source targets differ" "legacy source rejects intended GHCR plan"
    cp "${REPO}/.github/release-parity/targets.json" "${intended}"
    TEST_EXPECTED_TARGETS="${intended}" expect pass "same legacy source accepts matching legacy plan"
    TEST_EXPECTED_TARGETS="${intended}" expect "retired Bicep ACR writer" \
        "matching selector-free source cannot start new publication" HEAD --require-current-bicep
    write_pipeline
    TEST_EXPECTED_TARGETS="${intended}" expect "source targets differ" \
        "GHCR-only source rejects selector-free approved plan before publication" HEAD --require-current-bicep
    # shellcheck disable=SC2016 # Literal workflow expressions under test.
    for mutation in \
        '.jobs."build-and-push-bicep-types".needs = []' \
        'del(.jobs."release-preflight".outputs."release-state")' \
        '.jobs."release-preflight".outputs."release-state" = "${{ steps.other.outputs.result }}"' \
        '(.jobs."release-preflight".steps[] | select(.id == "release-state")).if = "false"'; do
        write_pipeline
        use_legacy_contract
        yq -i "${mutation}" "${WORKFLOWS}/build-release.yaml"
        expect "published Bicep resume gate" "historical resume rejects missing state wiring: ${mutation}"
    done
    local script
    for script in release-parity-manifest.sh collect-release-bicep.mjs promote-release-bicep.mjs; do
        write_pipeline
        case "${script}" in
            release-parity-manifest.sh)
                # shellcheck disable=SC2016 # Literal tagged script, never executed.
                printf '\nnode "${SCRIPT_DIR}/collect-release-bicep.mjs" verify-mirrors\n' >>"${REPO}/.github/scripts/${script}" ;;
            collect-release-bicep.mjs)
                printf '\nexport async function verifyMirrors() { throw new Error("old mirror verifier"); }\n' >>"${REPO}/.github/scripts/${script}" ;;
            promote-release-bicep.mjs)
                printf '\nimport { promotionRequests } from "./promote-bicep-types.mjs";\n' >>"${REPO}/.github/scripts/${script}" ;;
        esac
        expect "still requires the retired Bicep ACR writer" \
            "new workflow with mirror-dependent ${script} rejects before tagging" HEAD --require-current-bicep
        write_pipeline
        rm "${REPO}/.github/scripts/${script}"
        expect "has no GHCR-only entry point" "partial backport missing ${script}" HEAD --require-current-bicep
    done
    write_pipeline
    rm "${WORKFLOWS}/__publish-release-bicep.yaml"
    expect "has no reusable Radius Bicep producer workflow" "missing reusable workflow rejects partial backport"
    write_pipeline
    expect pass "new preparations require sibling publication" HEAD --require-sibling-releases
    expect pass "new mutation requires GHCR-only publication" HEAD --require-current-bicep

    write_pipeline
    yq -i 'del(.jobs."publish-sibling-releases") |
        .jobs."build-summary".needs |= map(select(. != "publish-sibling-releases"))' \
        "${WORKFLOWS}/build-release.yaml"
    expect pass "legacy published plans retain their original build contract"
    expect "has no publish-sibling-releases job" \
        "new preparations reject a branch without sibling publication" HEAD --require-sibling-releases

    for contract in ghcr legacy; do
        write_pipeline
        [[ "${contract}" != legacy ]] || use_legacy_contract
        expect pass "${contract} pipeline requires gated sibling publication" HEAD --require-sibling-releases
        for mutation in \
            'del(.jobs."attest-release")' \
            '.jobs."attest-release".needs = []' \
            '.jobs."attest-release".if = "always()"' \
            '.jobs."attest-release"."continue-on-error" = true' \
            '.jobs."attest-release".permissions."id-token" = "none"' \
            '.jobs."goreleaser-release".permissions.attestations = "write"' \
            '.jobs."verify-release".needs |= map(select(. != "attest-release"))' \
            '.jobs."verify-release".if += " || true"' \
            '.jobs."verify-release".permissions.attestations = "none"' \
            '.jobs."finalize-release".permissions.attestations = "none"' \
            '.jobs."finalize-release".needs |= map(select(. != "attest-release"))' \
            '.jobs."build-summary".needs |= map(select(. != "attest-release"))' \
            '(.jobs."attest-release".steps[] | select(.name == "Attest release provenance")).if = "false"' \
            '(.jobs."attest-release".steps[] | select(.name == "Attest release provenance"))."continue-on-error" = true'; do
            [[ "${contract}" != legacy || "${mutation}" != 'del(.jobs."attest-release")' ]] || continue
            write_pipeline
            [[ "${contract}" != legacy ]] || use_legacy_contract
            yq -i "${mutation}" "${WORKFLOWS}/build-release.yaml"
            expect "does not enforce the attestation gate" \
                "${contract} weakened attestation gate: ${mutation}" HEAD --require-sibling-releases
        done
        for mutation in \
            '.jobs."publish-sibling-releases".if = "always()"' \
            '.jobs."publish-sibling-releases".if += " || true"' \
            '.jobs."publish-sibling-releases".needs = []' \
            '.jobs."publish-sibling-releases"."continue-on-error" = true' \
            '.jobs."publish-sibling-releases".concurrency.group = "unserialized"' \
            '.jobs."publish-sibling-releases".concurrency."cancel-in-progress" = true' \
            '.jobs."build-summary".needs |= map(select(. != "publish-sibling-releases"))' \
            '(.jobs."publish-sibling-releases".steps[] | select(.name == "Reconcile sibling release entries")).env.INPUT_MANIFEST_FILE = "unverified.json"' \
            '(.jobs."publish-sibling-releases".steps[] | select(.name == "Reconcile sibling release entries")).if = "false"' \
            '(.jobs."publish-sibling-releases".steps[] | select(.name == "Reconcile sibling release entries"))."continue-on-error" = true'; do
            write_pipeline
            [[ "${contract}" != legacy ]] || use_legacy_contract
            yq -i "${mutation}" "${WORKFLOWS}/build-release.yaml"
            expect "does not enforce the sibling publication gate" \
                "${contract} weakened sibling gate: ${mutation}" HEAD --require-sibling-releases
        done
    done

    for selector in '""' 'null' '"ghcr-v2"'; do
        write_pipeline
        SELECTOR="${selector}" yq -i -o=json \
            '.bicepExtensionsContract = (strenv(SELECTOR) | from_json)' \
            "${REPO}/.github/release-parity/targets.json"
        expect "invalid Bicep release contract" "reject unknown or empty selector ${selector}"
    done

    write_pipeline
    yq -i -o=json 'del(.bicepExtensionsContract)' "${REPO}/.github/release-parity/targets.json"
    expect "invalid Bicep release contract" "GHCR outputs without selector"

    write_pipeline
    use_legacy_contract
    expect pass "historical inspection preserves selector-free ACR contracts"
    expect "requires the retired Bicep ACR writer" \
        "new mutation cannot use a historical ACR pipeline" HEAD --require-current-bicep
    yq -i '.jobs."build-and-push-bicep-types".if += " || true"' "${WORKFLOWS}/build-release.yaml"
    expect "does not enforce the published Bicep resume gate" \
        "historical published recovery must never reach the ACR writer"

    for job in verify-release approve-publication coordinate-release; do
        write_pipeline
        JOB="${job}" yq -i 'del(.jobs[strenv(JOB)])' \
            "${WORKFLOWS}/build-release.yaml"
        expect "has no ${job} job" "pipeline missing ${job}"
    done

    for dependency in \
        verify-release:goreleaser-release \
        verify-release:build-and-push-remaining-images \
        verify-release:build-and-push-helm-chart \
        verify-release:build-and-push-bicep-types \
        approve-publication:verify-release \
        finalize-release:verify-release \
        finalize-release:approve-publication \
        coordinate-release:finalize-release; do
        write_pipeline
        JOB="${dependency%%:*}" DEPENDENCY="${dependency#*:}" yq -i '
            .jobs[strenv(JOB)].needs |=
                map(select(. != strenv(DEPENDENCY)))
        ' "${WORKFLOWS}/build-release.yaml"
        expect "does not enforce the publication gate" \
            "pipeline missing dependency ${dependency}"
    done

    for mutation in \
        '.jobs."verify-release"."continue-on-error" = true' \
        '(.jobs."verify-release".steps[] | select(.run == "make verify-release-publication"))."continue-on-error" = true' \
        '(.jobs."verify-release".steps[] | select(.run == "make verify-release-publication")).if = "false"' \
        '.jobs."approve-publication".environment = "unprotected"' \
        '.jobs."approve-publication".if = "false"' \
        '.jobs."finalize-release".if = "always()"' \
        '.jobs."finalize-release".if += " || true"' \
        '(.jobs."finalize-release".steps[] | select(.run == "make recheck-release-publication"))."continue-on-error" = true' \
        '(.jobs."finalize-release".steps[] | select(.run == "make recheck-release-publication")).if = "false"' \
        '.jobs."finalize-release".steps |= map(select(.name != "Recheck outputs after approval"))' \
        '.jobs."finalize-release".steps |= reverse' \
        '.jobs."finalize-release".steps |= map(select(.name != "Promote stable aliases"))' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote stable aliases")).if = "false"' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote stable aliases"))."continue-on-error" = true' \
        '.jobs."finalize-release".steps |= map(select(.name != "Promote locked Bicep extensions"))' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote locked Bicep extensions")).if = "false"' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote locked Bicep extensions"))."continue-on-error" = true' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote locked Bicep extensions")).if += " || true"' \
        '.jobs."finalize-release".permissions."id-token" = "write"' \
        '.jobs."finalize-release".env.BICEPTYPES_CLIENT_ID = "forbidden"' \
        '.jobs."finalize-release".steps += [{"name":"Unexpected Azure login","run":"az acr login --name biceptypes"}]' \
        '.jobs."finalize-release".steps += [{"name":"Private token","uses":"actions/create-github-app-token@abc"}]' \
        '.jobs."publish-legacy-bicep-types".if += " || true"' \
        '.jobs."publish-radius-bicep-extension".if = "true"' \
        '(.jobs."build-and-push-remaining-images".steps[] | select(.run == "make docker-publish-bicep")).env.BICEP_RELEASE_PLAN = ""' \
        '(.jobs."build-and-push-remaining-images".steps[] | select(.run == "make docker-publish-bicep")).env.BICEP_EXTENSION_TAG = "latest"' \
        '(.jobs."build-and-push-bicep-types".steps[] | select(.name == "Authenticate and retain immutable extension pair")).if = "false"' \
        '.jobs."build-and-push-bicep-types".if += " || true"' \
        '.jobs."build-and-push-bicep-types".steps += [{"name":"Retired mirror","with":{"script":"await stageMirrors({github, core})"}}]' \
        '.jobs."build-and-push-bicep-types".permissions."id-token" = "write"' \
        '.jobs."build-and-push-bicep-types".permissions.packages = "write"' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote locked Bicep extensions")).env.INPUT_JOB_DEADLINE = ""' \
        '.jobs."finalize-release".steps |= map(select(.name != "Record Bicep job deadline"))' \
        '(.jobs."build-and-push-bicep-types".steps[] | select(.name == "Authenticate and retain immutable extension pair")).with.script = "await finalize({github,core,context})"' \
        '(.jobs."finalize-release".steps[] | select(.name == "Recheck RC prerequisites after approval")).if = "false"' \
        '.jobs."coordinate-release".if = "false"'; do
        write_pipeline
        yq -i "${mutation}" "${WORKFLOWS}/build-release.yaml"
        expect "does not enforce the publication gate" \
            "weakened gate: ${mutation}"
    done

    write_pipeline
    yq -i '.release.draft = false' "${REPO}/.goreleaser.yaml"
    expect "does not stage a draft GoReleaser release" \
        "GoReleaser publishes before the gate"

    write_pipeline
    printf '%s\n' 'on:' '  push:' '    branches: [main]' '    tags: ["v*"]' \
        > "${WORKFLOWS}/build.yaml"
    expect "also runs .github/workflows/build.yaml on tag pushes" \
        "legacy tag publisher beside the release build"

    write_pipeline
    printf '%s\n' 'on:' '  push:' '    tags-ignore: [nightly]' > "${WORKFLOWS}/publish.yaml"
    expect "also runs .github/workflows/publish.yaml on tag pushes" \
        "publisher selected by tags-ignore"

    write_pipeline
    yq -i 'del(.jobs."finalize-release") | .jobs."publish-release" = {}' \
        "${WORKFLOWS}/build-release.yaml"
    expect "has no finalize-release job" "older release job graph (release/0.61)"

    write_pipeline
    yq -i '.on.push = {"branches": ["main"]}' "${WORKFLOWS}/build-release.yaml"
    expect "does not run .github/workflows/build-release.yaml on v* tags" \
        "release build without a tag trigger"

    write_pipeline
    rm "${REPO}/.goreleaser.yaml"
    expect "has no .goreleaser.yaml" "missing GoReleaser configuration"

    write_pipeline
    rm "${WORKFLOWS}/build-release.yaml"
    expect "has no .github/workflows/build-release.yaml" \
        "missing release build (release/0.60)"

    expect "release commit does not exist" "unknown release commit" refs/heads/missing

    write_pipeline
    mkdir -p "${REPO}/.github/scripts"
    cp "${ROOT}/.github/scripts/coordinate-release.mjs" "${REPO}/.github/scripts/"
    REQUIRE_BICEP_RECIPES=true expect pass "new finals require the recipe coordinator" HEAD --require-sibling-releases

    rm "${REPO}/.github/scripts/coordinate-release.mjs"
    REQUIRE_BICEP_RECIPES=true expect "does not coordinate the Bicep recipe publisher" \
        "new final cannot use an old coordination script"
    yq -i 'del(.jobs."coordinate-release".steps[].env.RECIPE_TOKEN)' \
        "${WORKFLOWS}/build-release.yaml"
    expect pass "existing plans need no recipe coordinator"
    REQUIRE_BICEP_RECIPES=true expect "has no final Bicep recipe coordinator" \
        "new final cannot use an old tagged workflow"

    if ((FAIL > 0)); then
        echo "Release build contract tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Release build contract tests passed (${PASS} tests)"
}

main "$@"
