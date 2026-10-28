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
    # Both run on tag pushes without publishing a release.
    printf '%s\n' 'on:' '  push:' '    paths: [setup.yaml]' > "${WORKFLOWS}/setup.yml"
    printf '%s\n' 'on: [push, pull_request]' > "${WORKFLOWS}/lint.yaml"
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
    local job dependency mutation
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
    expect pass "new preparations require sibling publication" HEAD --require-sibling-releases

    write_pipeline
    yq -i 'del(.jobs."publish-sibling-releases") |
        .jobs."build-summary".needs |= map(select(. != "publish-sibling-releases"))' \
        "${WORKFLOWS}/build-release.yaml"
    expect pass "legacy published plans retain their original build contract"
    expect "has no publish-sibling-releases job" \
        "new preparations reject a branch without sibling publication" HEAD --require-sibling-releases

    for mutation in \
        '.jobs."publish-sibling-releases".if = "always()"' \
        '.jobs."publish-sibling-releases".needs = []' \
        '.jobs."publish-sibling-releases"."continue-on-error" = true' \
        '.jobs."publish-sibling-releases".concurrency.group = "unserialized"' \
        '(.jobs."publish-sibling-releases".steps[] | select(.name == "Reconcile sibling release entries")).if = "false"' \
        '(.jobs."publish-sibling-releases".steps[] | select(.name == "Reconcile sibling release entries"))."continue-on-error" = true'; do
        write_pipeline
        yq -i "${mutation}" "${WORKFLOWS}/build-release.yaml"
        expect "does not enforce the sibling publication gate" \
            "weakened sibling gate: ${mutation}"
    done

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
        '.jobs."finalize-release".steps |= map(select(.name != "Promote verified Bicep aliases"))' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote verified Bicep aliases")).if = "false"' \
        '(.jobs."finalize-release".steps[] | select(.name == "Promote verified Bicep aliases"))."continue-on-error" = true' \
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

    if ((FAIL > 0)); then
        echo "Release build contract tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi
    echo "Release build contract tests passed (${PASS} tests)"
}

main "$@"
