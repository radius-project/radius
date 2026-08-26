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
        "build-and-push-bicep-types",
        "build-and-push-helm-chart",
        "build-and-push-remaining-images",
        "build-summary",
        "finalize-release",
        "goreleaser-release",
        "release-preflight"
    ]'
    assert_json_equal "${actual_jobs}" "${expected_jobs}" \
        "tag workflow contains superseded jobs" || return

    actual_needs="$(yq -o=json '
        .jobs."finalize-release".needs | sort
    ' "${RELEASE_WORKFLOW}")"
    expected_needs='[
        "build-and-push-bicep-types",
        "build-and-push-helm-chart",
        "build-and-push-remaining-images",
        "goreleaser-release",
        "release-preflight"
    ]'
    assert_json_equal "${actual_needs}" "${expected_needs}" \
        "finalization does not depend on every mandatory stage" || return
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
    ((++PASS))
}

test_finalization_is_digest_locked() {
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

test_old_release_paths_are_deleted() {
    if [[ -e "${REPO_ROOT}/.github/workflows/__publish-release.yaml" ]] \
                                                                        || [[ -e "${REPO_ROOT}/.github/scripts/verify-goreleaser-shadow.sh" ]] \
                                                                            || grep -Fq 'goreleaser-shadow' "${RELEASE_WORKFLOW}"; then
        fail_test "superseded tag release paths are still present"
        return
    fi
    ((++PASS))
}

test_release_resume_contract() {
    local published_guards
    local retry_count
    local immutable_count

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
    if [[ "${published_guards}" != "5" ]]; then
        fail_test "published releases can re-enter mutating jobs"
        return
    fi
    retry_count="$(grep -Fc 'retries: 5' "${RELEASE_WORKFLOW}")"
    if ((retry_count < 8)); then
        fail_test "release API operations do not use bounded retries"
        return
    fi
    if [[ "$(grep -Fc 'overwrite: true' "${RELEASE_WORKFLOW}")" != "3" ]]; then
        fail_test "same-run lock artifacts are not resumable"
        return
    fi
    if [[ "$(grep -Fc 'assert-images-absent' "${RELEASE_WORKFLOW}")" != "2" ]]; then
        fail_test "unlocked immutable images are not guarded before pushes"
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

main() {
    command -v jq > /dev/null
    command -v yq > /dev/null

    test_release_job_graph
    test_privileged_jobs_require_preflight
    test_rejects_malformed_release_tag
    test_goreleaser_stages_prepared_draft
    test_finalization_is_digest_locked
    test_main_publishes_only_edge
    test_old_release_paths_are_deleted
    test_release_resume_contract

    if ((FAIL > 0)); then
        echo "release cutover tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "release cutover tests passed (${PASS} tests)"
}

main "$@"
