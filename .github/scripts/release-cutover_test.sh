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
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly RELEASE_WORKFLOW="${REPO_ROOT}/.github/workflows/build-release.yaml"
readonly CLI_WORKFLOW="${REPO_ROOT}/.github/workflows/__build-cli.yaml"
readonly IMAGE_WORKFLOW="${REPO_ROOT}/.github/workflows/__build-images.yaml"
readonly HELM_WORKFLOW="${REPO_ROOT}/.github/workflows/__build-helm-chart.yaml"
readonly BICEP_TYPES_WORKFLOW="${REPO_ROOT}/.github/workflows/__build-bicep-types.yaml"
readonly CONFIG="${REPO_ROOT}/.goreleaser.yaml"
readonly ARTIFACTS_MAKEFILE="${REPO_ROOT}/build/artifacts.mk"
PASS=0
FAIL=0

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

assert_json_equal() {
    local actual="$1"
    local expected="$2"
    local description="$3"

    if ! jq -e -n \
        --argjson actual "${actual}" \
        --argjson expected "${expected}" \
        '$actual == $expected' > /dev/null; then
        fail_test "${description}"
        return 1
    fi
}

test_release_job_graph() {
    local actual_jobs
    local expected_jobs
    local actual_needs
    local expected_needs

    actual_jobs="$(yq -o=json '.jobs | keys | sort' "${RELEASE_WORKFLOW}")"
    expected_jobs='[
        "approve-publication",
        "attest-release",
        "build-and-push-bicep-types",
        "build-and-push-helm-chart",
        "build-and-push-remaining-images",
        "build-summary",
        "coordinate-release",
        "finalize-release",
        "goreleaser-release",
        "publish-legacy-bicep-types",
        "publish-radius-bicep-extension",
        "publish-sibling-releases",
        "release-preflight",
        "verify-release"
    ]'
    assert_json_equal "${actual_jobs}" "${expected_jobs}" \
        "tag workflow contains superseded jobs" || return

    actual_needs="$(yq -o=json '
        .jobs."finalize-release".needs | sort
    ' "${RELEASE_WORKFLOW}")"
    expected_needs='[
        "approve-publication",
        "attest-release",
        "build-and-push-bicep-types",
        "build-and-push-helm-chart",
        "build-and-push-remaining-images",
        "goreleaser-release",
        "release-preflight",
        "verify-release"
    ]'
    assert_json_equal "${actual_needs}" "${expected_needs}" \
        "finalization does not depend on every mandatory stage" || return
    ((++PASS))
}

test_publication_gate() {
    if ! yq -o=json '.jobs' "${RELEASE_WORKFLOW}" | jq -e '
        # GitHub lists a draft release only for push access, so verification
        # of the staged draft needs contents write; it must still not publish.
        ."verify-release".permissions.contents == "write" and
        ."verify-release".permissions.packages == "read" and
        (."verify-release".steps | any(.run == "make verify-release-publication")) and
        ."approve-publication".environment == "release" and
        (."approve-publication".if | contains("!contains(github.ref_name")) and
        (."finalize-release".if | contains("needs.verify-release.result == '\''success'\''")) and
        (."finalize-release".if | contains("needs.approve-publication.result")) and
        (."finalize-release".steps | map(.name) |
          index("Recheck outputs after approval") < index("Select release aliases")) and
        ."finalize-release".permissions.deployments == "read" and
        (."finalize-release".steps | map(.name) |
          index("Recheck outputs after approval") < index("Recheck RC prerequisites after approval") and
          index("Recheck RC prerequisites after approval") < index("Select release aliases")) and
        (."finalize-release".steps | any(.env.INPUT_MODE == "check-rc")) and
        (."finalize-release".steps | any(.env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json")) and
        (."finalize-release".steps | map(.name) |
          index("Persist the verified release manifest") < index("Promote verified Bicep aliases") and
          index("Promote verified Bicep aliases") < index("Publish and verify draft GitHub Release")) and
        (."finalize-release".steps | any(
          .name == "Promote verified Bicep aliases" and
          (.if | contains("promote_channel")) and
          (."continue-on-error" // false) == false)) and
        (."finalize-release".steps | map(.name) |
          index("Recheck outputs after approval") < index("Authenticate tagged finalizer for ACR compatibility") and
          index("Promote and mirror locked Bicep extensions") < index("Publish and verify draft GitHub Release")) and
        ."finalize-release".permissions."id-token" == "write" and
        (."finalize-release".steps | any(
          .name == "Authenticate tagged finalizer for ACR compatibility" and
          .run == "node .github/scripts/azure-oidc-login.mjs" and
          .env.AZURE_CLIENT_ID == "${{ secrets.BICEPTYPES_CLIENT_ID }}" and
          .env.AZURE_TENANT_ID == "${{ secrets.BICEPTYPES_TENANT_ID }}" and
          .env.AZURE_SUBSCRIPTION_ID == "${{ secrets.BICEPTYPES_SUBSCRIPTION_ID }}")) and
        ."publish-radius-bicep-extension".uses == "./.github/workflows/__publish-release-bicep.yaml" and
        (."publish-legacy-bicep-types".if | contains("ghcr-bicep == '\''false'\''")) and
        (."publish-radius-bicep-extension".if | contains("ghcr-bicep == '\''true'\''")) and
        ([."release-preflight", ."build-and-push-bicep-types"] |
          all(.steps | all((.uses // "" | contains("create-github-app-token")) | not))) and
        (."finalize-release".steps | map(select(
          .name == "Get Bicep promotion token" or .name == "Promote verified Bicep aliases")) |
          length == 2 and all(.[]; .if | contains("ghcr-bicep == '\''false'\''"))) and
        (."finalize-release".steps | any(.env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json"))
    ' >/dev/null; then
        fail_test "failed verification or missing approval can reach publication"
        return
    fi
    if ! yq -o=json '.jobs."build-and-push-aws-bicep-types"' \
        "${BICEP_TYPES_WORKFLOW}" | jq -e '
        (.if | contains("refs/tags/v")) and
        ."timeout-minutes" > 45 and
        (.steps | any(.env.INPUT_EVENT_TYPE == "bicep-types-aws" and
            (.env.INPUT_CLIENT_PAYLOAD | contains("steps.source.outputs.sha"))))
    ' > /dev/null; then
        fail_test "publication does not await the planned immutable AWS types"
        return
    fi
    if ! yq -o=json '.jobs."build-and-push-helm-chart".steps' \
        "${HELM_WORKFLOW}" | jq -e '
        (map(.name) | index("Pin external chart images") as $pin |
            $pin != null and $pin < index("Package Helm chart") and
            $pin < index("Push helm chart to GHCR")) and
        any(.[]; .name == "Pin external chart images" and
            (.if | contains("refs/tags/v")) and
            (.run | contains("--names dashboard")) and
            (.run | contains("--names deployment-engine")) and
            (.run | contains("--expected-digest")) and
            (.run | contains("--source-sha")))
    ' > /dev/null; then
        fail_test "chart publication can precede verified external version tags"
        return
    fi
    ((++PASS))
}

test_privileged_jobs_require_preflight() {
    local job
    local needs

    for job in \
        goreleaser-release \
        build-and-push-remaining-images \
        build-and-push-helm-chart \
        build-and-push-bicep-types \
        attest-release \
        publish-legacy-bicep-types \
        publish-radius-bicep-extension \
        finalize-release; do
        needs="$(JOB="${job}" yq -o=json '
            .jobs[strenv(JOB)].needs
        ' "${RELEASE_WORKFLOW}")"
        if ! jq -e 'index("release-preflight") != null' \
            <<< "${needs}" > /dev/null; then
            fail_test "${job} can bypass release preflight"
            return
        fi
    done
    ((++PASS))
}

test_rejects_malformed_release_tag() {
    # shellcheck source=.github/scripts/release-version.sh
    source "${REPO_ROOT}/.github/scripts/release-version.sh"

    if is_radius_release_version "not-semver"; then
        fail_test "malformed v-prefixed tags pass the release preflight"
        return
    fi
    ((++PASS))
}

test_goreleaser_stages_prepared_draft() {
    # shellcheck disable=SC2016 # Literal Make expression under test.
    if ! grep -Fq 'make goreleaser-release' "${RELEASE_WORKFLOW}" \
                                                                  || ! grep -Fq -- '--release-notes "$(GORELEASER_RELEASE_NOTES)"' \
            "${ARTIFACTS_MAKEFILE}"; then
        fail_test "GoReleaser does not consume the prepared release notes"
        return
    fi
    if ! yq -e '
        .release.draft == true
        and .release.use_existing_draft == true
        and .release.replace_existing_artifacts == true
        and .release.make_latest == false
    ' "${CONFIG}" > /dev/null; then
        fail_test "GoReleaser release settings bypass finalization"
        return
    fi
    # A rerun with locked images runs GoReleaser with --skip=docker, so the
    # verifier after the release must receive --skip-images like the snapshot.
    # shellcheck disable=SC2016 # Literal Make expression under test.
    if [[ "$(grep -Fc 'verify-goreleaser-snapshot.sh $(GORELEASER_VERIFY_ARGS)' \
        "${ARTIFACTS_MAKEFILE}")" != "2" ]]; then
        fail_test "the release target verifies images that --skip=docker never built"
        return
    fi
    ((++PASS))
}

test_finalization_is_digest_locked() {
    if ! yq -o=json '.jobs."finalize-release".concurrency' \
        "${RELEASE_WORKFLOW}" | jq -e '
        .group == "release-finalization" and
        .queue == "max" and
        ."cancel-in-progress" == false
    ' > /dev/null; then
        fail_test "shared release aliases are not serialized across versions"
        return
    fi
    if ! yq -o=json '.jobs."finalize-release".steps' \
        "${RELEASE_WORKFLOW}" | jq -e '
        any(.[]; .id == "alias-policy" and .env.INPUT_MODE == "select-aliases") and
        any(.[]; .name == "Promote stable aliases" and
            (.if | contains("promote_channel")) and
            .env.RELEASE_PROMOTE_LATEST == "${{ steps.alias-policy.outputs.promote_latest }}") and
        any(.[]; .env.INPUT_MAKE_LATEST == "${{ steps.alias-policy.outputs.promote_latest }}")
    ' > /dev/null; then
        fail_test "finalization can regress newer channel or latest aliases"
        return
    fi
    if ! grep -Fq 'make capture-release-image-digests' \
        "${RELEASE_WORKFLOW}" \
                              || ! grep -Fq 'make release-cli-oci' "${RELEASE_WORKFLOW}" \
                                                                || ! grep -Fq 'make promote-release-aliases' "${RELEASE_WORKFLOW}"; then
        fail_test "finalization is not based on immutable digest locks"
        return
    fi
    if ! grep -Fq "if: env.UPDATE_RELEASE == 'true'" \
        "${RELEASE_WORKFLOW}"; then
        fail_test "prereleases can reach mutable alias promotion"
        return
    fi
    ((++PASS))
}

test_main_publishes_only_edge() {
    local path

    for path in "${CLI_WORKFLOW}" "${IMAGE_WORKFLOW}"; do
        if grep -Eq '(:latest|DOCKER_TAG_VERSION:[[:space:]]*latest|docker-multi-arch-tag)' \
            "${path}"; then
            fail_test "main publication still writes latest in ${path}"
            return
        fi
    done
    if ! grep -Fq ':edge"' "${CLI_WORKFLOW}" \
                                             || ! grep -Fq 'DOCKER_TAG_VERSION: edge' "${IMAGE_WORKFLOW}"; then
        fail_test "main publication does not write edge directly"
        return
    fi
    ((++PASS))
}

test_only_main_synchronizes_edge_docs() {
    local expression
    expression="$(yq -r '.jobs.reference-docs.env.PUBLISH_DOCS' \
        "${REPO_ROOT}/.github/workflows/publish-docs.yaml")"
    # shellcheck disable=SC2016 # Literal GitHub expression under test.
    if [[ "${expression}" != '${{ github.ref == '\''refs/heads/main'\'' && (github.event_name == '\''push'\'' || (github.event_name == '\''workflow_dispatch'\'' && github.event.inputs.open_pull_request == '\''true'\'')) }}' ]]; then
        fail_test "release branches or manual non-main runs can synchronize edge docs"
        return
    fi
    if ! yq -o=json '.' "${REPO_ROOT}/.github/workflows/publish-docs.yaml" \
                                                                           | jq -e '
            (.on.push.branches | index("release/*") != null) and
            (.jobs."reference-docs".steps |
                all(.[] | select(.id == "app-token" or .id == "bot-details" or
                    .name == "Sync reference docs to docs repository");
                    .if == "env.PUBLISH_DOCS == '\''true'\''"))
        ' > /dev/null; then
        fail_test "release previews or main-only docs write gates changed"
        return
    fi
    ((++PASS))
}

test_sibling_release_publication_is_gated_and_resumable() {
    if ! yq -o=json '.jobs' "${RELEASE_WORKFLOW}" | jq -e '
        ."publish-sibling-releases" as $job |
        ($job.needs | sort) == ["finalize-release", "release-preflight", "verify-release"] and
        ($job.if | contains("needs.verify-release.result == '\''success'\''")) and
        ($job.if | contains("needs.finalize-release.result == '\''success'\''")) and
        ($job.if | contains("needs.release-preflight.outputs.release-state == '\''published'\''")) and
        $job.concurrency.group == "release-finalization" and
        $job.concurrency.queue == "max" and
        $job.permissions.contents == "read" and
        ($job.steps | [.[] | select(.id == "sibling-token" or .id == "engine-token")] |
            length == 2 and all(.[]; .with."permission-workflows" == "write")) and
        ($job.steps | any(.env.INPUT_MANIFEST_FILE == "dist/verification/release-manifest.json")) and
        (.["build-summary"].needs | index("publish-sibling-releases") != null)
    ' > /dev/null; then
        fail_test "sibling GitHub Releases bypass publication, serialization or resume reporting"
        return
    fi
    ((++PASS))
}

test_old_release_paths_are_deleted() {
    if [[ -e "${REPO_ROOT}/.github/workflows/__publish-release.yaml" ]] \
                                                                        || [[ -e "${REPO_ROOT}/.github/scripts/verify-goreleaser-shadow.sh" ]] \
                                                                            || [[ -e "${REPO_ROOT}/.github/scripts/image-payload-manifest/main.go" ]] \
                                                                               || grep -Fq 'goreleaser-shadow' "${RELEASE_WORKFLOW}"; then
        fail_test "superseded tag release paths are still present"
        return
    fi
    ((++PASS))
}

test_final_cleanup_contract() {
    local file workflow
    for file in get_release_version.py release-get-version.sh; do
        if [[ -e "${SCRIPT_DIR}/${file}" ]]; then
            fail_test "obsolete release helper remains: ${file}"
            return
        fi
    done
    for file in release-verification.yaml goreleaser-snapshot.yaml; do
        if [[ -e "${REPO_ROOT}/.github/workflows/${file}" ]]; then
            fail_test "obsolete workflow remains: ${file}"
            return
        fi
    done
    if grep -Fq 'docker-multi-arch' "${REPO_ROOT}/build/docker.mk"; then
        fail_test "legacy multi-architecture build targets remain"
        return
    fi
    if ! yq -o=json '.jobs."build-and-push-remaining-images".strategy.matrix.image' \
        "${RELEASE_WORKFLOW}" | jq -e '. == ["bicep"]' > /dev/null \
                                                                  || ! jq -e 'all(.images[]; .category != "test")' \
            "${REPO_ROOT}/.github/release-parity/targets.json" > /dev/null; then
        fail_test "test images are still official release outputs"
        return
    fi
    for workflow in build-main.yaml build-validation.yaml; do
        if ! yq -o=json '.jobs' "${REPO_ROOT}/.github/workflows/${workflow}" | jq -e '
            ."build-snapshot".uses == "./.github/workflows/__build-snapshot.yaml" and
            (."build-and-push-cli".needs | index("build-snapshot") != null) and
            (."build-and-push-images".needs | index("build-snapshot") != null) and
            (."build-check".needs | index("build-snapshot") != null and index("build-and-push-images") != null)
        ' > /dev/null; then
            fail_test "${workflow} bypasses the shared snapshot or its required check"
            return
        fi
    done
    for workflow in "${CLI_WORKFLOW}" "${IMAGE_WORKFLOW}"; do
        if ! yq -o=json '.jobs' "${workflow}" | jq -e '
            ([to_entries[] | select(.key != "publish-edge") | .value.permissions.packages] | all(. == null)) and
            (."publish-edge".if | contains("refs/heads/main") and contains("radius-project/radius")) and
            (."publish-edge".steps | any(.id == "current-main" and (.with.script | contains("getBranch"))))
        ' > /dev/null; then
            fail_test "snapshot exports gained registry permissions or edge lost its main-head guard"
            return
        fi
    done
    if ! yq -o=json '.jobs."build-and-push-bicep-types".steps' "${BICEP_TYPES_WORKFLOW}" | jq -e '
        (map(.id) | index("current-main")) as $check |
        (map(.id) | index("monitor-remote-workflow")) as $dispatch |
        $check != null and $dispatch != null and $check < $dispatch and
        (.[$check].if | contains("refs/heads/main")) and
        (.[$check].with.script | contains("getBranch")) and
        (.[$dispatch].if | contains("steps.current-main.outputs.current"))
    ' > /dev/null; then
        fail_test "edge Bicep types dispatch lost its main-head guard"
        return
    fi
    if ! yq -o=json '.jobs."build-and-push-images".steps' \
        "${IMAGE_WORKFLOW}" | jq -e \
        --arg name "bicep-image-\${{ steps.release-metadata.outputs.REL_VERSION }}" '
        map(select((.uses // "") | startswith("actions/upload-artifact@")) | .with) |
        any(.[]; .name == $name and
            .path == "./dist/images/bicep.tar" and
            ."retention-days" == 1 and
            ."if-no-files-found" == "error" and .overwrite == true) and
        all(.[]; .path == "./dist/images/bicep.tar" or .path == "dist/metrics/")
    ' > /dev/null; then
        fail_test "snapshot image exports must retain only the Bicep tar and metrics"
        return
    fi
    ((++PASS))
}

test_test_image_tags_are_attempt_scoped() {
    if ! yq -o=json '.jobs' "${REPO_ROOT}/.github/workflows/functional-test-cloud.yaml" | jq -e '
        (.build.env.REL_VERSION | startswith("test-") and contains("github.run_id") and contains("github.run_attempt")) and
        .build.outputs.REL_VERSION == "${{ steps.test-image-version.outputs.REL_VERSION }}" and
        .tests.env.REL_VERSION == "${{ needs.build.outputs.REL_VERSION }}" and
        .tests.env.BICEP_RECIPE_TAG_VERSION == .tests.env.REL_VERSION
    ' > /dev/null; then
        fail_test "cloud test images can reuse tags across build attempts"
        return
    fi
    if ! yq -o=json '.jobs.build.steps' "${REPO_ROOT}/.github/workflows/functional-test-noncloud.yaml" | jq -e '
        any(.[]; .id == "gen-id" and (.run | contains("REL_VERSION=test-") and contains("${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}")))
    ' > /dev/null; then
        fail_test "noncloud test images do not use attempt-specific tags"
        return
    fi
    if ! yq -o=json '.jobs.tests.steps' "${REPO_ROOT}/.github/workflows/long-running-azure.yaml" | jq -e '
        (map(.run // "" | contains("make docker-build-magpiego docker-push-magpiego")) | index(true)) as $build |
        (map(.name) | index("Run functional tests")) as $test |
        $build != null and $test != null and $build < $test and
        (.[$build]."working-directory" | contains("checkout-release-codebase")) and
        .[$build].env.DOCKER_REGISTRY == .[$test].env.DOCKER_REGISTRY and
        .[$build].env.DOCKER_TAG_VERSION == .[$test].env.REL_VERSION and
        (.[$test].env.REL_VERSION | contains("steps.gen-id.outputs.TEST_IMAGE_TAG")) and
        any(.[]; .id == "gen-id" and (.run | contains("TEST_IMAGE_TAG=test-") and contains("${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}")))
    ' > /dev/null; then
        fail_test "long-running tests do not build their own attempt-scoped Magpie image"
        return
    fi
    ((++PASS))
}

test_release_resume_contract() {
    local published_guards
    local retry_count
    local immutable_count
    local restage_gates

    # shellcheck disable=SC2016 # Literal workflow expressions under test.
    for marker in \
        production-image-intent.json \
        core-release-lock.json \
        '${{ matrix.image }}-image-digests.json' \
        '${{ matrix.image }}-image-intent.json' \
        production-image-digests.json \
        release-cli-oci.json \
        release-image-digests.json; do
        if ! grep -Fq "${marker}" "${RELEASE_WORKFLOW}"; then
            fail_test "release workflow has no durable ${marker} contract"
            return
        fi
    done
    published_guards="$(grep -Fc \
        "needs.release-preflight.outputs.release-state != 'published'" \
        "${RELEASE_WORKFLOW}")"
    if [[ "${published_guards}" != "9" ]]; then
        fail_test "published releases can re-enter mutating jobs"
        return
    fi
    retry_count="$(grep -Fc 'retries: 5' "${RELEASE_WORKFLOW}")"
    if ((retry_count < 8)); then
        fail_test "release API operations do not use bounded retries"
        return
    fi
    if [[ "$(grep -Fc 'overwrite: true' "${RELEASE_WORKFLOW}")" != "4" ]]; then
        fail_test "same-run lock artifacts are not resumable"
        return
    fi
    if [[ "$(grep -Fc 'assert-images-absent' "${RELEASE_WORKFLOW}")" != "2" ]]; then
        fail_test "unlocked immutable images are not guarded before pushes"
        return
    fi
    restage_gates="$(grep -Fc "outputs.state == 'absent'" \
        "${RELEASE_WORKFLOW}")"
    if [[ "${restage_gates}" != "2" ]]; then
        fail_test "a partial image set cannot be re-staged"
        return
    fi
    # shellcheck disable=SC2016 # Literal workflow expression under test.
    if ! grep -Fq 'group: build-release-${{ github.ref }}' \
        "${RELEASE_WORKFLOW}"; then
        fail_test "release concurrency is not serialized by immutable tag name"
        return
    fi
    immutable_count="$(grep -Fc 'INPUT_IMMUTABLE: "true"' \
        "${RELEASE_WORKFLOW}")"
    if ((immutable_count < 7)); then
        fail_test "source intents and digest locks are not immutable"
        return
    fi
    ((++PASS))
}

test_draft_locks_are_reloaded_on_every_staging_attempt() {
    if ! yq -o=json '.jobs' "${RELEASE_WORKFLOW}" | jq -e '
        ."release-preflight".permissions.contents == "write" and
        (."goreleaser-release".steps |
            (map(.name) | index("Ensure source-bound draft release")) as $draft |
            [.[] | select(.id == "image-lock-download" or
                .id == "cli-lock-download")] as $downloads |
            ($downloads | length) == 2 and
            all($downloads[];
                .if == null and
                .env.INPUT_MODE == "download" and
                .env.INPUT_OPTIONAL == "true") and
            $draft != null and
            (map(.id) | index("image-lock-download")) > $draft and
            (map(.id) | index("cli-lock-download")) > $draft)
    ' > /dev/null; then
        fail_test "staging retries cannot recover locks when the original preflight reported absent"
        return
    fi
    ((++PASS))
}

test_bicep_image_uses_staged_extension_versions() {
    if ! yq -o=json '.jobs."build-and-push-remaining-images".steps' \
        "${RELEASE_WORKFLOW}" | jq -e '
        any(.[]; .run == "make docker-publish-bicep" and
            .env.BICEP_EXTENSION_TAG == "${{ env.REL_VERSION }}" and
            .env.BICEP_RELEASE_PLAN == ".github/release-plans/${{ github.ref_name }}.yaml")
    ' >/dev/null; then
        fail_test "release Bicep image still depends on unpublished channel aliases"
        return
    fi
    # shellcheck disable=SC2016 # Literal Make variable under test.
    if ! grep -Fq 'generate-bicepconfig.sh $(BICEP_EXTENSION_TAG)' \
        "${REPO_ROOT}/build/build.mk"; then
        fail_test "Bicep image build ignores its immutable extension tag"
        return
    fi
    if ! (
        directory="$(mktemp -d "${TMPDIR:-/tmp}/bicep-config-test-XXXXXX")"
        trap 'rm -rf "${directory}"' EXIT
        for tag in edge 0.61.0 0.61.2 0.61.0-rc.1; do
            expected="${tag}"
            bash "${REPO_ROOT}/build/scripts/generate-bicepconfig.sh" \
                "${tag}" "${directory}"
            jq -e --arg tag "${expected}" '
                .experimentalFeaturesEnabled.ociEnabled == true and
                .extensions.radius == ("br:ghcr.io/radius-project/bicep-types-radius:" + $tag) and
                .extensions.aws == ("br:ghcr.io/radius-project/bicep-types-aws:" + $tag)
            ' "${directory}/bicepconfig.json" > /dev/null || exit 1
        done
    ); then
        fail_test "Bicep extension configuration changed its tag contract"
        return
    fi
    ((++PASS))
}

main() {
    command -v jq > /dev/null
    command -v yq > /dev/null

    test_release_job_graph
    test_publication_gate
    test_privileged_jobs_require_preflight
    test_rejects_malformed_release_tag
    test_goreleaser_stages_prepared_draft
    test_finalization_is_digest_locked
    test_main_publishes_only_edge
    test_only_main_synchronizes_edge_docs
    test_sibling_release_publication_is_gated_and_resumable
    test_old_release_paths_are_deleted
    test_final_cleanup_contract
    test_test_image_tags_are_attempt_scoped
    test_release_resume_contract
    test_draft_locks_are_reloaded_on_every_staging_attempt
    test_bicep_image_uses_staged_extension_versions

    if ((FAIL > 0)); then
        echo "release cutover tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "release cutover tests passed (${PASS} tests)"
}

main "$@"
