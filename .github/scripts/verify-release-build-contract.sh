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

# A pushed tag runs the tagged commit's workflows, not those on main.
set -euo pipefail

readonly BUILD_WORKFLOW=".github/workflows/build-release.yaml"
readonly -a REQUIRED_JOBS=(
    release-preflight goreleaser-release
    build-and-push-remaining-images build-and-push-helm-chart
    build-and-push-bicep-types finalize-release
    verify-release approve-publication coordinate-release
)
COMMIT=""

fail() {
    echo "Error: $*" >&2
    exit 1
}

reject() {
    fail "release commit ${COMMIT} $*; migrate its release branch to the current release pipeline first"
}

yq_true() {
    [[ "$(git show "${COMMIT}:$1" 2>/dev/null | yq "$2" 2>/dev/null)" == "true" ]]
}

verify_publication_gate() {
    # Compare the gate expressions after whitespace normalization, rather than
    # accepting a guard mentioned inside an expression that can bypass it.
    local approval_condition="!contains(github.ref_name, '-') &&
        needs.release-preflight.outputs.release-state != 'published'"
    local finalization_condition="!cancelled() &&
        github.repository == 'radius-project/radius' &&
        startsWith(github.ref, 'refs/tags/v') &&
        needs.release-preflight.outputs.release-state != 'published' &&
        needs.verify-release.result == 'success' &&
        (needs.approve-publication.result == 'success' ||
        (contains(github.ref_name, '-rc.') &&
        needs.approve-publication.result == 'skipped'))"
    local coordination_condition="!cancelled() &&
        needs.verify-release.result == 'success' &&
        (needs.finalize-release.result == 'success' ||
        needs.release-preflight.outputs.release-state == 'published')"
    local promotion_condition="env.UPDATE_RELEASE == 'true' &&
        steps.alias-policy.outputs.promote_channel == 'true'"

    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' |
        jq -e \
            --arg approval "${approval_condition}" \
            --arg finalization "${finalization_condition}" \
            --arg coordination "${coordination_condition}" \
            --arg promotion "${promotion_condition}" '
        def normalized: gsub("\\s"; "");
        def depends_on($required):
            (.needs | type) == "array" and
            (($required - .needs) | length) == 0;
        def mandatory:
            .if == null and
            (."continue-on-error" == null or ."continue-on-error" == false);
        def required_step($name):
            [.steps[] | select(.name == $name)] as $steps |
            ($steps | length) == 1 and ($steps[0] | mandatory);
        def promotion_step($name; check):
            [.steps[] | select(.name == $name)] as $steps |
            ($steps | length) == 1 and ($steps[0] |
                (.if | normalized) == ($promotion | normalized) and
                (."continue-on-error" == null or ."continue-on-error" == false) and
                check);
        .jobs as $jobs |
        $jobs."verify-release" as $verify |
        $jobs."approve-publication" as $approve |
        $jobs."finalize-release" as $finalize |
        $jobs."coordinate-release" as $coordinate |
        ($finalize.steps | map(.name)) as $steps |
        ($verify | depends_on([
            "release-preflight", "goreleaser-release",
            "build-and-push-remaining-images", "build-and-push-helm-chart",
            "build-and-push-bicep-types"])) and
        ($approve | depends_on(["release-preflight", "verify-release"])) and
        ($finalize | depends_on([
            "release-preflight", "goreleaser-release",
            "build-and-push-remaining-images", "build-and-push-helm-chart",
            "build-and-push-bicep-types", "verify-release",
            "approve-publication"])) and
        ($coordinate | depends_on([
            "release-preflight", "verify-release", "finalize-release"])) and
        ([$verify, $approve, $finalize, $coordinate] |
            all(.[]; ."continue-on-error" == null or
                ."continue-on-error" == false)) and
        $approve.environment == "release" and
        ($approve.if | normalized) == ($approval | normalized) and
        ($finalize.if | normalized) == ($finalization | normalized) and
        ($coordinate.if | normalized) == ($coordination | normalized) and
        ([$verify.steps[] |
            select(.run == "make verify-release-publication")] |
            length == 1 and all(.[]; mandatory)) and
        ($finalize |
            required_step("Recheck outputs after approval") and
            required_step("Recheck RC prerequisites after approval") and
            required_step("Select release aliases") and
            required_step("Persist the verified release manifest") and
            required_step("Publish and verify draft GitHub Release")) and
        ($steps | index("Recheck outputs after approval")) <
            ($steps | index("Recheck RC prerequisites after approval")) and
        ($steps | index("Recheck RC prerequisites after approval")) <
            ($steps | index("Select release aliases")) and
        ($steps | index("Select release aliases")) <
            ($steps | index("Persist the verified release manifest")) and
        ($steps | index("Persist the verified release manifest")) <
            ($steps | index("Promote stable aliases")) and
        ($steps | index("Promote stable aliases")) <
            ($steps | index("Promote verified Bicep aliases")) and
        ($steps | index("Promote verified Bicep aliases")) <
            ($steps | index("Publish and verify draft GitHub Release")) and
        ($finalize | promotion_step("Promote stable aliases";
            .run | contains("make promote-release-aliases"))) and
        ($finalize | promotion_step("Promote verified Bicep aliases";
            .env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json" and
            (.with.script | contains("promote-bicep-types.mjs")))) and
        ($finalize.steps | any(
            .name == "Recheck outputs after approval" and
            .run == "make recheck-release-publication")) and
        ($finalize.steps | any(
            .name == "Recheck RC prerequisites after approval" and
            .env.INPUT_MODE == "check-rc"))
    ' >/dev/null ||
        reject "does not enforce the publication gate in ${BUILD_WORKFLOW}"
}

main() {
    local job name

    [[ $# -eq 1 ]] || fail "usage: verify-release-build-contract.sh <commit>"
    command -v yq >/dev/null || fail "required command not found: yq"
    command -v jq >/dev/null || fail "required command not found: jq"
    COMMIT="$(git rev-parse --verify --quiet "$1^{commit}")" ||
        fail "release commit does not exist: $1"

    git cat-file -e "${COMMIT}:.goreleaser.yaml" 2>/dev/null ||
        reject "has no .goreleaser.yaml"
    git cat-file -e "${COMMIT}:${BUILD_WORKFLOW}" 2>/dev/null ||
        reject "has no ${BUILD_WORKFLOW}"
    yq_true "${BUILD_WORKFLOW}" '[.on.push.tags[]? | select(. == "v*")] | length > 0' ||
        reject "does not run ${BUILD_WORKFLOW} on v* tags"
    for job in "${REQUIRED_JOBS[@]}"; do
        yq_true "${BUILD_WORKFLOW}" ".jobs | has(\"${job}\")" ||
            reject "has no ${job} job in ${BUILD_WORKFLOW}"
    done

    yq_true ".goreleaser.yaml" \
        '.release.draft == true and .release.make_latest == false' ||
        reject "does not stage a draft GoReleaser release"
    verify_publication_gate

    # Path-only push triggers also run for tags; only a tag filter marks a publisher.
    while IFS= read -r name; do
        [[ "${name}" =~ \.ya?ml$ && ".github/workflows/${name}" != "${BUILD_WORKFLOW}" ]] ||
            continue
        if yq_true ".github/workflows/${name}" '(.on | tag) == "!!map" and
            ((.on.push // {}) | tag) == "!!map" and
            ((.on.push // {}) | (has("tags") or has("tags-ignore")))'; then
            reject "also runs .github/workflows/${name} on tag pushes"
        fi
    done < <(git ls-tree --name-only "${COMMIT}:.github/workflows")

    echo "Release commit ${COMMIT} starts ${BUILD_WORKFLOW} on its tag."
}

main "$@"
