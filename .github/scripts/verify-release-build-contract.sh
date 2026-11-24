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
REQUIRE_SIBLING_RELEASES=false
FROZEN_FIRST_RC=false
BICEP_DUAL_PIPELINE=false
GHCR_ONLY=false
REQUIRE_CURRENT_BICEP=false

fail() {
    echo "Error: $*" >&2
    exit 1
}

reject() {
    fail "release commit ${COMMIT} $*; migrate its release branch to the current release pipeline first"
}

yq_true() {
    [[ "$(git show "${COMMIT}:$1" 2> /dev/null | yq "$2" 2> /dev/null)" == "true" ]]
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
    local bicep_promotion_condition="${promotion_condition}"
    local bicep_promotion_name="Promote verified Bicep aliases"
    local bicep_promotion_script="promote-bicep-types.mjs"
    if [[ "${GHCR_ONLY}" == true ]]; then
        bicep_promotion_condition="needs.release-preflight.outputs.ghcr-bicep == 'true'"
        bicep_promotion_name="Promote locked Bicep extensions"
        bicep_promotion_script="promote-release-bicep.mjs"
    elif [[ "${BICEP_DUAL_PIPELINE}" == true ]]; then
        bicep_promotion_condition="needs.release-preflight.outputs.ghcr-bicep == 'false' &&
            ${promotion_condition}"
    fi

    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' \
                                                            | jq -e \
            --arg approval "${approval_condition}" \
            --arg finalization "${finalization_condition}" \
            --arg coordination "${coordination_condition}" \
            --arg bicep_promotion "${bicep_promotion_condition}" \
            --arg bicep_name "${bicep_promotion_name}" \
            --arg bicep_script "${bicep_promotion_script}" \
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
        def promotion_step($name; $condition; check):
            [.steps[] | select(.name == $name)] as $steps |
            ($steps | length) == 1 and ($steps[0] |
                (.if | normalized) == ($condition | normalized) and
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
            ($steps | index($bicep_name)) and
        ($steps | index($bicep_name)) <
            ($steps | index("Publish and verify draft GitHub Release")) and
        ($finalize | promotion_step("Promote stable aliases"; $promotion;
            .run | contains("make promote-release-aliases"))) and
        ($finalize | promotion_step($bicep_name; $bicep_promotion;
            .env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json" and
            (.with.script | contains($bicep_script)))) and
        ($finalize.steps | any(
            .name == "Recheck outputs after approval" and
            .run == "make recheck-release-publication")) and
        ($finalize.steps | any(
            .name == "Recheck RC prerequisites after approval" and
            .env.INPUT_MODE == "check-rc"))
    ' > /dev/null \
                 || reject "does not enforce the publication gate in ${BUILD_WORKFLOW}"
}

verify_sibling_publication_gate() {
    if ! yq_true "${BUILD_WORKFLOW}" '.jobs | has("publish-sibling-releases")'; then
        [[ "${REQUIRE_SIBLING_RELEASES}" == false ]] \
                                                     || reject "has no publish-sibling-releases job"
        return
    fi
    local condition="!cancelled() && github.repository == 'radius-project/radius' &&
        startsWith(github.ref, 'refs/tags/v') &&
        needs.verify-release.result == 'success' &&
        (needs.finalize-release.result == 'success' ||
        needs.release-preflight.outputs.release-state == 'published')"
    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' \
                                                            | jq -e --arg condition "${condition}" '
            .jobs."publish-sibling-releases" as $job |
            ($job.if | gsub("\\s"; "")) == ($condition | gsub("\\s"; "")) and
            ($job.needs | sort) == ["finalize-release", "release-preflight", "verify-release"] and
            ($job."continue-on-error" // false) == false and
            $job.concurrency.group == "release-finalization" and
            $job.concurrency.queue == "max" and
            $job.concurrency."cancel-in-progress" == false and
            ([$job.steps[] | select(.name == "Reconcile sibling release entries")] |
                length == 1 and all(.[]; .if == null and
                    (."continue-on-error" // false) == false and
                    .env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json")) and
            (.jobs."build-summary".needs | index("publish-sibling-releases") != null)
        ' > /dev/null \
                     || reject "does not enforce the sibling publication gate"
}

verify_attestation_gate() {
    if ! yq_true "${BUILD_WORKFLOW}" '.jobs | has("attest-release")'; then
        [[ "${BICEP_DUAL_PIPELINE}" == false ]] ||
            reject "does not enforce the attestation gate"
        return 0
    fi
    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' | jq -e '
        def normalized: gsub("\\s"; "");
        .jobs as $jobs |
        $jobs."attest-release" as $attest |
        ($attest.needs | sort) == ["build-and-push-helm-chart",
            "build-and-push-remaining-images", "goreleaser-release", "release-preflight"] and
        ($attest.if | normalized) == ("github.repository == '\''radius-project/radius'\'' &&
            startsWith(github.ref, '\''refs/tags/v'\'') &&
            needs.release-preflight.outputs.release-state != '\''published'\''" | normalized) and
        ($attest."continue-on-error" // false) == false and
        $attest.permissions."id-token" == "write" and
        $attest.permissions.attestations == "write" and
        ([$attest.steps[] | select(.name == "Attest release provenance")] |
            length == 1 and all(.[]; .if == null and
            (."continue-on-error" // false) == false and
            (.uses | test("^actions/attest@[a-f0-9]{40}$")) and
            .with."subject-checksums" == "dist/attestation/subjects.txt")) and
        ($jobs."verify-release".needs | index("attest-release") != null) and
        ($jobs."finalize-release".needs | index("attest-release") != null) and
        ($jobs."build-summary".needs | index("attest-release") != null) and
        $jobs."verify-release".permissions.attestations == "read" and
        $jobs."finalize-release".permissions.attestations == "read" and
        ($jobs."verify-release".if | normalized) == ("!cancelled() &&
            needs.release-preflight.result == '\''success'\'' &&
            (needs.release-preflight.outputs.release-state == '\''published'\'' ||
            (needs.goreleaser-release.result == '\''success'\'' &&
            needs.build-and-push-remaining-images.result == '\''success'\'' &&
            needs.build-and-push-helm-chart.result == '\''success'\'' &&
            needs.build-and-push-bicep-types.result == '\''success'\'' &&
            needs.attest-release.result == '\''success'\''))" | normalized) and
        ([$jobs | to_entries[] | select(.key != "attest-release") |
            .value.permissions.attestations] | all(. != "write"))
    ' >/dev/null || reject "does not enforce the attestation gate"
}

verify_bicep_contract() {
    local targets=".github/release-parity/targets.json"
    local ghcr=false
    if [[ -n "${EXPECTED_TARGETS:-}" ]]; then
        git cat-file -e "${COMMIT}:${targets}" 2>/dev/null ||
            reject "has no source targets for the intended release contract"
        git show "${COMMIT}:${targets}" | jq -e --slurpfile expected "${EXPECTED_TARGETS}" \
            '. == $expected[0]' >/dev/null ||
            reject "source targets differ from the intended release contract"
    fi
    # Older release pipelines need no GHCR selector, but an explicit selector
    # must be known and must name the complete canonical target pair.
    if git cat-file -e "${COMMIT}:${targets}" 2>/dev/null; then
        git show "${COMMIT}:${targets}" | jq -e '
            if has("bicepExtensionsContract") then
                .bicepExtensionsContract == "ghcr-v1" and .ociArtifacts == [
                    {name:"aws-bicep-types",repository:"ghcr.io/radius-project/bicep-types-aws",
                     artifactType:"application/vnd.ms.bicep.provider.artifact"},
                    {name:"radius-bicep-types",repository:"ghcr.io/radius-project/bicep-types-radius",
                     artifactType:"application/vnd.ms.bicep.provider.artifact"}
                ]
            else
                (.ociArtifacts | length) > 0 and
                all(.ociArtifacts[]; .repository | startswith("biceptypes.azurecr.io/"))
            end
        ' >/dev/null || reject "has an invalid Bicep release contract"
        ghcr="$(git show "${COMMIT}:${targets}" | jq 'has("bicepExtensionsContract")')"
    fi
    # The GHCR publisher validates its tag using the validator at that same
    # commit. Older validators only understand the main squash or a backport.
    if [[ "${ghcr}" == true && "${FROZEN_FIRST_RC}" == true ]]; then
        git grep -q -F 'frozen product commit plus approved release metadata' \
            "${COMMIT}" -- .github/scripts/validate-release-controller-plan.sh ||
            reject "cannot validate a frozen first RC in its tagged Bicep publisher"
    fi
    if yq_true "${BUILD_WORKFLOW}" '.jobs | has("publish-radius-bicep-extension")'; then
        BICEP_DUAL_PIPELINE=true
        git cat-file -e "${COMMIT}:.github/workflows/__publish-release-bicep.yaml" 2>/dev/null ||
            reject "has no reusable Radius Bicep producer workflow"
    elif [[ "${ghcr}" == true ]]; then
        reject "does not enforce the publication gate for GHCR Bicep outputs"
    else
        return 0
    fi
    if yq_true "${BUILD_WORKFLOW}" '.jobs."finalize-release".steps |
        any_c(.name == "Promote locked Bicep extensions")'; then
        [[ "${ghcr}" == true ]] || reject "has an invalid Bicep release contract"
        verify_ghcr_only_bicep
        verify_ghcr_only_entrypoints
        GHCR_ONLY=true
        return
    fi
    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' | jq -e '
        def normalized: gsub("\\s"; "");
        def exact_if($condition):
            (.if | normalized) == ($condition | normalized) and
            (."continue-on-error" == null or ."continue-on-error" == false);
        def step($name; $condition):
            [.steps[] | select(.name == $name)] |
            length == 1 and all(.[]; exact_if($condition));
        .jobs as $jobs |
        $jobs."build-and-push-bicep-types" as $staging |
        ($staging.steps | map(.name)) as $stageSteps |
        $jobs."finalize-release" as $finalize |
        ($finalize.steps | map(.name)) as $steps |
        "needs.release-preflight.outputs.ghcr-bicep == '\''true'\''" as $ghcr |
        "needs.release-preflight.outputs.ghcr-bicep == '\''false'\'' &&
         env.UPDATE_RELEASE == '\''true'\'' &&
         steps.alias-policy.outputs.promote_channel == '\''true'\''" as $legacy |
        ($ghcr + " || (" + $legacy + ")") as $transport |
        ($finalize | step("Get Bicep promotion token"; $transport)) and
        ($finalize | step("Promote verified Bicep aliases"; $legacy)) and
        ($finalize | step("Promote and mirror locked Bicep extensions"; $ghcr)) and
        ([$finalize.steps[] | select(.uses // "" | contains("create-github-app-token"))] |
            length == 1 and all(.[]; .name == "Get Bicep promotion token" and exact_if($transport) and
                .with.owner == "azure-octo" and .with.repositories == "radius-publisher" and
                .with."permission-actions" == "read" and .with."permission-contents" == "write")) and
        $finalize.permissions."id-token" == null and
        ($finalize | tojson | test("BICEPTYPES_|AZURE_CLIENT_ID|AZURE_TENANT_ID|AZURE_SUBSCRIPTION_ID") | not) and
        ($finalize.steps | all(.[]; ((.run // "") | test("azure-oidc-login|az acr|oras cp.*azurecr")) | not)) and
        ($finalize.steps | all(.[]; ((.uses // "") | startswith("azure/")) | not)) and
        ($steps | index("Persist the verified release manifest")) <
            ($steps | index("Get Bicep promotion token")) and
        ($steps | index("Promote stable aliases")) <
            ($steps | index("Promote and mirror locked Bicep extensions")) and
        ($steps | index("Promote and mirror locked Bicep extensions")) <
            ($steps | index("Publish and verify draft GitHub Release")) and
        ($finalize.steps | any(.name == "Promote and mirror locked Bicep extensions" and
            (.with.script | contains("promote-release-bicep.mjs")) and
            .with."github-token" == "${{ steps.promotion-token.outputs.token }}" and
            .env.INPUT_LOCK_FILE == "dist/verification/assets/bicep-extension-lock.json" and
            .env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json" and
            .env.INPUT_PLAN_FILE == "dist/verification/plan.json" and
            .env.INPUT_JOB_DEADLINE == "${{ env.BICEP_JOB_DEADLINE }}" and
            .env.INPUT_PROMOTE_CHANNEL == "${{ steps.alias-policy.outputs.promote_channel }}" and
            .env.INPUT_PROMOTE_LATEST == "${{ steps.alias-policy.outputs.promote_latest }}")) and
        ($jobs."publish-legacy-bicep-types" | exact_if(
            "github.repository == '\''radius-project/radius'\'' &&
             startsWith(github.ref, '\''refs/tags/v'\'') &&
             needs.release-preflight.outputs.release-state != '\''published'\'' &&
             needs.release-preflight.outputs.ghcr-bicep == '\''false'\''")) and
        $jobs."publish-legacy-bicep-types".uses == "./.github/workflows/__build-bicep-types.yaml" and
        ($jobs."publish-radius-bicep-extension" | exact_if(
            "github.repository == '\''radius-project/radius'\'' &&
             startsWith(github.ref, '\''refs/tags/v'\'') &&
             needs.release-preflight.outputs.release-state != '\''published'\'' &&
             needs.release-preflight.outputs.ghcr-bicep == '\''true'\'' &&
             needs.release-preflight.outputs.bicep-locked != '\''true'\''")) and
        $jobs."publish-radius-bicep-extension".uses == "./.github/workflows/__publish-release-bicep.yaml" and
        ($jobs."build-and-push-bicep-types".needs |
            index("publish-legacy-bicep-types") != null and
            index("publish-radius-bicep-extension") != null) and
        ($jobs."build-and-push-remaining-images".steps | map(
            select(.run == "make docker-publish-bicep")) |
            length == 1 and all(.[]; .env.BICEP_EXTENSION_TAG == "${{ env.REL_VERSION }}" and
                .env.BICEP_RELEASE_PLAN == ".github/release-plans/${{ github.ref_name }}.yaml")) and
        ($jobs."build-and-push-bicep-types" | step(
            "Authenticate and retain immutable extension pair"; $ghcr)) and
        ($staging | step("Get immutable Bicep mirror token"; $ghcr)) and
        ($staging | step("Stage immutable ACR mirrors before verification"; $ghcr)) and
        ($staging.steps | any(.name == "Get immutable Bicep mirror token" and
            .with.owner == "azure-octo" and .with.repositories == "radius-publisher" and
            .with."permission-actions" == "read" and .with."permission-contents" == "write")) and
        $staging.permissions."id-token" == null and
        $staging.permissions.packages == "read" and
        ($staging.steps[0].name == "Record Bicep job deadline" and
            $finalize.steps[0].name == "Record Bicep job deadline") and
        ($staging | tojson | test("BICEPTYPES_|azure/login|azure-oidc-login|az acr|oras cp") | not) and
        ($stageSteps | index("Authenticate and retain immutable extension pair")) <
            ($stageSteps | index("Get immutable Bicep mirror token")) and
        ($stageSteps | index("Get immutable Bicep mirror token")) <
            ($stageSteps | index("Stage immutable ACR mirrors before verification")) and
        ($staging.steps | any(.name == "Stage immutable ACR mirrors before verification" and
            (.with.script | contains("stageMirrors({github, core, context,")) and
            (.with.script | contains("publisherGithub: getOctokit(process.env.PUBLISHER_TOKEN)")) and
            .env.PUBLISHER_TOKEN == "${{ steps.mirror-token.outputs.token }}" and
            .env.INPUT_JOB_DEADLINE == "${{ env.BICEP_JOB_DEADLINE }}" and
            .env.INPUT_DIRECTORY == "${{ runner.temp }}/bicep-mirror-staging"))
    ' >/dev/null || reject "does not enforce the publication gate for both Bicep contracts"
}

verify_ghcr_only_bicep() {
    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' | jq -e '
        def normalized: gsub("\\s"; "");
        def exact_if($condition):
            (.if | normalized) == ($condition | normalized) and
            (."continue-on-error" // false) == false;
        .jobs as $jobs |
        $jobs."build-and-push-bicep-types" as $pair |
        $jobs."finalize-release" as $finalize |
        "needs.release-preflight.outputs.ghcr-bicep == '\''true'\''" as $ghcr |
        ($jobs | has("publish-legacy-bicep-types") | not) and
        ($jobs."publish-radius-bicep-extension" | exact_if(
            "github.repository == '\''radius-project/radius'\'' &&
             startsWith(github.ref, '\''refs/tags/v'\'') &&
             needs.release-preflight.outputs.release-state != '\''published'\'' &&
             needs.release-preflight.outputs.ghcr-bicep == '\''true'\'' &&
             needs.release-preflight.outputs.bicep-locked != '\''true'\''")) and
        $jobs."publish-radius-bicep-extension".uses == "./.github/workflows/__publish-release-bicep.yaml" and
        ($pair.needs | sort) == ["publish-radius-bicep-extension", "release-preflight"] and
        ($pair | exact_if(
            "!cancelled() && needs.release-preflight.result == '\''success'\'' &&
             needs.release-preflight.outputs.release-state != '\''published'\'' &&
             needs.release-preflight.outputs.ghcr-bicep == '\''true'\'' &&
             (needs.publish-radius-bicep-extension.result == '\''success'\'' ||
              (needs.release-preflight.outputs.bicep-locked == '\''true'\'' &&
               needs.publish-radius-bicep-extension.result == '\''skipped'\''))")) and
        ($pair.steps | map(select(.name == "Authenticate and retain immutable extension pair")) |
            length == 1 and all(.[]; exact_if($ghcr) and
                (.with.script | contains("stage-release-bicep.mjs")) and
                .env.INPUT_OPERATION == "lock")) and
        $pair.permissions.packages == "read" and
        $finalize.steps[0].name == "Record Bicep job deadline" and
        ([$pair, $finalize] | all(.[];
            .permissions."id-token" == null and
            (tojson | test("BICEPTYPES_|RADIUS_PUBLISHER_BOT_|create-github-app-token|radius-publisher|stageMirrors|promote-bicep-types.mjs|azurecr|azure/login|azure-oidc-login|az acr") | not))) and
        ($finalize.steps | map(select(.name == "Promote locked Bicep extensions")) |
            length == 1 and all(.[]; exact_if($ghcr) and
                .with."github-token" == null and
                (.with.script | contains("promote-release-bicep.mjs")) and
                .env.INPUT_LOCK_FILE == "dist/verification/assets/bicep-extension-lock.json" and
                .env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json" and
                .env.INPUT_PLAN_FILE == "dist/verification/plan.json" and
                .env.INPUT_JOB_DEADLINE == "${{ env.BICEP_JOB_DEADLINE }}" and
                .env.INPUT_PROMOTE_CHANNEL == "${{ steps.alias-policy.outputs.promote_channel }}" and
                .env.INPUT_PROMOTE_LATEST == "${{ steps.alias-policy.outputs.promote_latest }}")) and
        ($jobs."build-and-push-remaining-images".steps | map(
            select(.run == "make docker-publish-bicep")) |
            length == 1 and all(.[]; .env.BICEP_EXTENSION_TAG == "${{ env.REL_VERSION }}" and
                .env.BICEP_RELEASE_PLAN == ".github/release-plans/${{ github.ref_name }}.yaml"))
    ' >/dev/null || reject "does not enforce the publication gate for GHCR-only Bicep"
}

verify_ghcr_only_entrypoints() {
    local path content
    for path in \
        .github/scripts/release-parity-manifest.sh \
        .github/scripts/collect-release-bicep.mjs \
        .github/scripts/promote-release-bicep.mjs; do
        content="$(git show "${COMMIT}:${path}")" ||
            reject "has no GHCR-only entry point ${path}"
        if grep -Eq 'verify-mirrors|verifyMirrors|bicepExtensionMirrors|stageMirrors|stageExtensionMirrors|releaseCopyRequests|promotionRequests|dispatchPromotion|monitor-remote-workflow|promote-bicep-types\.mjs|radius-publisher' <<<"${content}"; then
            reject "GHCR-only entry point ${path} still requires the retired Bicep ACR writer"
        fi
        case "${path}" in
            */release-parity-manifest.sh)
                grep -Fq 'release-bicep-extensions.mjs" verify' <<<"${content}" ||
                    reject "GHCR-only parity entry point does not verify the locked pair" ;;
            */collect-release-bicep.mjs)
                grep -Fq 'verifyBicepExtensionOutput(record, observed)' <<<"${content}" ||
                    reject "GHCR-only collector does not verify source-bound manifests" ;;
            */promote-release-bicep.mjs)
                if ! grep -Fq 'validatePublicationManifest(manifest, plan.version, sourceSha)' <<<"${content}" ||
                    ! grep -Fq 'receipt.ghcr = await promoteExtensions(' <<<"${content}"; then
                    reject "GHCR-only finalizer does not promote the approved pair"
                fi ;;
        esac
    done
}

verify_historical_bicep_resume_gate() {
    [[ "${GHCR_ONLY}" != true ]] || return 0
    local condition="github.repository == 'radius-project/radius' &&
        startsWith(github.ref, 'refs/tags/v') &&
        needs.release-preflight.outputs.release-state != 'published'"
    if [[ "${BICEP_DUAL_PIPELINE}" == true ]]; then
        condition="!cancelled() && needs.release-preflight.result == 'success' &&
            needs.release-preflight.outputs.release-state != 'published' &&
            (needs.publish-legacy-bicep-types.result == 'success' ||
             (needs.release-preflight.outputs.ghcr-bicep == 'true' &&
              (needs.publish-radius-bicep-extension.result == 'success' ||
               needs.release-preflight.outputs.bicep-locked == 'true')))"
    fi
    git show "${COMMIT}:${BUILD_WORKFLOW}" | yq -o=json '.' |
        jq -e --arg condition "${condition}" --argjson dual "${BICEP_DUAL_PIPELINE}" '
            .jobs as $jobs |
            $jobs."release-preflight".outputs."release-state" ==
                "${{ steps.release-state.outputs.result }}" and
            ([$jobs."release-preflight".steps[] | select(.id == "release-state")] |
                length == 1 and all(.[]; .if == null and
                    (."continue-on-error" // false) == false and
                    .with."result-encoding" == "string")) and
            (["build-and-push-bicep-types", "finalize-release"] +
                (if $dual then ["publish-radius-bicep-extension", "publish-legacy-bicep-types"] else [] end) |
                all(.[]; . as $name | $jobs[$name].needs |
                    type == "array" and index("release-preflight") != null)) and
            ($jobs."build-and-push-bicep-types" |
                (.if | gsub("\\s"; "")) == ($condition | gsub("\\s"; "")) and
                (."continue-on-error" // false) == false)
        ' >/dev/null || reject "does not enforce the published Bicep resume gate"
}

main() {
    local job name

    [[ $# -ge 1 ]] || fail "usage: verify-release-build-contract.sh <commit> [--require-sibling-releases] [--frozen-first-rc] [--expected-targets <json>] [--require-current-bicep]"
    local revision="$1"
    shift
    EXPECTED_TARGETS=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --require-sibling-releases) REQUIRE_SIBLING_RELEASES=true; shift ;;
            --frozen-first-rc) FROZEN_FIRST_RC=true; shift ;;
            --require-current-bicep) REQUIRE_CURRENT_BICEP=true; shift ;;
            --expected-targets)
                [[ $# -ge 2 && -f "$2" ]] || fail "expected targets JSON is required"
                EXPECTED_TARGETS="$2"; shift 2 ;;
            *) fail "unknown argument: $1" ;;
        esac
    done
    command -v yq > /dev/null || fail "required command not found: yq"
    command -v jq > /dev/null || fail "required command not found: jq"
    COMMIT="$(git rev-parse --verify --quiet "${revision}^{commit}")" \
                                                             || fail "release commit does not exist: ${revision}"

    git cat-file -e "${COMMIT}:.goreleaser.yaml" 2> /dev/null \
                                                             || reject "has no .goreleaser.yaml"
    git cat-file -e "${COMMIT}:${BUILD_WORKFLOW}" 2> /dev/null \
                                                              || reject "has no ${BUILD_WORKFLOW}"
    yq_true "${BUILD_WORKFLOW}" '[.on.push.tags[]? | select(. == "v*")] | length > 0' \
                                                                                      || reject "does not run ${BUILD_WORKFLOW} on v* tags"
    for job in "${REQUIRED_JOBS[@]}"; do
        yq_true "${BUILD_WORKFLOW}" ".jobs | has(\"${job}\")" \
                                                              || reject "has no ${job} job in ${BUILD_WORKFLOW}"
    done

    yq_true ".goreleaser.yaml" \
        '.release.draft == true and .release.make_latest == false' ||
        reject "does not stage a draft GoReleaser release"
    verify_bicep_contract
    if [[ "${REQUIRE_CURRENT_BICEP}" == true && "${GHCR_ONLY}" != true ]]; then
        fail "release commit ${COMMIT} requires the retired Bicep ACR writer; finish in-flight legacy and mirror-dependent releases before cutover, or use a reviewed new GHCR-only release source. Do not move existing tags or rewrite release locks."
    fi
    verify_publication_gate
    verify_historical_bicep_resume_gate
    verify_sibling_publication_gate
    verify_attestation_gate

    # Path-only push triggers also run for tags; only a tag filter marks a publisher.
    while IFS= read -r name; do
        [[ "${name}" =~ \.ya?ml$ && ".github/workflows/${name}" != "${BUILD_WORKFLOW}" ]] \
                                                                                          || continue
        if yq_true ".github/workflows/${name}" '(.on | tag) == "!!map" and
            ((.on.push // {}) | tag) == "!!map" and
            ((.on.push // {}) | (has("tags") or has("tags-ignore")))'; then
            reject "also runs .github/workflows/${name} on tag pushes"
        fi
    done < <(git ls-tree --name-only "${COMMIT}:.github/workflows")

    if [[ "${REQUIRE_BICEP_RECIPES:-false}" == true ]]; then
        yq_true "${BUILD_WORKFLOW}" '
            [.jobs."coordinate-release".steps[] |
                select(.id == "coordinate" and .if == null and
                    .env.RECIPE_TOKEN != null and
                    (."continue-on-error" // false) == false)] |
                length == 1' || reject "has no final Bicep recipe coordinator"
        if ! git grep -q -F 'publish-bicep-recipes.yaml' "${COMMIT}" -- \
            .github/scripts/coordinate-release.mjs; then
            reject "does not coordinate the Bicep recipe publisher"
        fi
    fi

    echo "Release commit ${COMMIT} starts ${BUILD_WORKFLOW} on its tag."
}

main "$@"
