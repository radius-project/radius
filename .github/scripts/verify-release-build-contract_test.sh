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
    printf 'version: 2\n' >"${REPO}/.goreleaser.yaml"
    printf '%s\n' 'on:' '  workflow_dispatch:' '  push:' '    tags: ["v*"]' \
        'jobs:' '  release-preflight: {}' '  goreleaser-release: {}' \
        '  finalize-release: {}' >"${WORKFLOWS}/build-release.yaml"
    # Both run on tag pushes without publishing a release.
    printf '%s\n' 'on:' '  push:' '    paths: [setup.yaml]' >"${WORKFLOWS}/setup.yml"
    printf '%s\n' 'on: [push, pull_request]' >"${WORKFLOWS}/lint.yaml"
}

# Commits the fixture and expects a pass, or a failure that contains $1.
expect() {
    local expected="$1"
    local description="$2"
    local revision="${3:-HEAD}"
    local output
    local status=0

    git -C "${REPO}" add -A
    git -C "${REPO}" commit -q --allow-empty -m "${description}"
    output="$(cd "${REPO}" && bash "${SCRIPT}" "${revision}" 2>&1)" || status=$?
    if [[ "${expected}" == "pass" && ${status} -eq 0 ]] ||
        [[ "${expected}" != "pass" && ${status} -ne 0 && "${output}" == *"${expected}"* ]]; then
        ((++PASS))
        return
    fi
    echo "  ASSERT FAILED: ${description}: ${output}"
    ((++FAIL))
}

main() {
    command -v yq >/dev/null || {
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

    printf '%s\n' 'on:' '  push:' '    branches: [main]' '    tags: ["v*"]' \
        >"${WORKFLOWS}/build.yaml"
    expect "also runs .github/workflows/build.yaml on tag pushes" \
        "legacy tag publisher beside the release build"

    write_pipeline
    printf '%s\n' 'on:' '  push:' '    tags-ignore: [nightly]' >"${WORKFLOWS}/publish.yaml"
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
