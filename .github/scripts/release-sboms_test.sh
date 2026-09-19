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
readonly WORKFLOWS_DIR="${REPO_ROOT}/.github/workflows"
readonly RELEASE_WORKFLOW="${WORKFLOWS_DIR}/build-release.yaml"
readonly SNAPSHOT_WORKFLOW="${WORKFLOWS_DIR}/goreleaser-snapshot.yaml"
readonly TOOLS_MANIFEST="${REPO_ROOT}/build/tools.yaml"
readonly TOOLS_MAKEFILE="${REPO_ROOT}/build/tools.generated.mk"
readonly RELEASE_DOCS_DIR="${REPO_ROOT}/docs/contributing"
readonly RELEASE_DOC="${RELEASE_DOCS_DIR}/contributing-releases/README.md"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

verify_tool_pin() {
    local syft
    local version

    syft="$(yq -o=json '.tools[] | select(.name == "syft")' \
        "${TOOLS_MANIFEST}")"
    jq -e '
        .makePrefix == "SYFT"
        and .source.type == "github-release"
        and .source.repository == "anchore/syft"
        and .checksumSource.type == "github-release-file"
        and .checksumSource.format == "standard"
        and (.platforms | keys | sort)
            == ["darwin_amd64", "darwin_arm64",
                "linux_amd64", "linux_arm64"]
        and all(.platforms[].checksum; test("^[0-9a-f]{64}$"))
    ' <<< "${syft}" > /dev/null || fail "Syft tool pin is incomplete"
    version="$(jq -r '.version' <<< "${syft}")"
    if ! grep -Fqx "SYFT_VERSION ?= ${version}" "${TOOLS_MAKEFILE}"; then
        fail "generated Syft version is not synchronized"
    fi
}

verify_workflow_wiring() {
    local asset_verifiers
    local image_verifiers
    local metadata_uploads

    if ! grep -Fq 'install-goreleaser install-syft' \
        "${SNAPSHOT_WORKFLOW}"; then
        fail "snapshot workflow does not install Syft"
    fi
    if ! grep -Fq 'make install-goreleaser install-syft' \
        "${RELEASE_WORKFLOW}"; then
        fail "release workflow does not install Syft"
    fi
    asset_verifiers="$(grep -Fc 'INPUT_MODE: verify-sboms' \
        "${RELEASE_WORKFLOW}")"
    if [[ "${asset_verifiers}" != "2" ]]; then
        fail "draft and published CLI SBOM verification is incomplete"
    fi
    image_verifiers="$(grep -Fc -- '--sboms' "${RELEASE_WORKFLOW}")"
    if [[ "${image_verifiers}" != "3" ]]; then
        fail "image SBOM verification is missing from a release path"
    fi
    metadata_uploads="$({
        grep -F 'dist/goreleaser/*.sbom.json' "${SNAPSHOT_WORKFLOW}"
        grep -F 'dist/goreleaser/*.sbom.json' "${RELEASE_WORKFLOW}"
    } | wc -l)"
    if [[ "${metadata_uploads}" != "2" ]]; then
        fail "SBOMs are missing from retained GoReleaser metadata"
    fi
}

verify_documentation() {
    if ! grep -Fq 'rad_linux_amd64.sbom.json' "${RELEASE_DOC}"; then
        fail "CLI SBOM discovery is not documented"
    fi
    if ! grep -Fq '{{ json (index .SBOM "linux/amd64").SPDX }}' \
        "${RELEASE_DOC}"; then
        fail "image SBOM discovery is not documented"
    fi
    if ! grep -Fq 'SPDX 2' "${RELEASE_DOC}"; then
        fail "SBOM format is not documented"
    fi
}

main() {
    command -v jq > /dev/null
    command -v yq > /dev/null

    bash "${SCRIPT_DIR}/verify-goreleaser-snapshot.sh" --config-only \
        > /dev/null
    yq -e '
        (.checksum.ids | length) == 1
        and .checksum.ids[0] == "rad"
        and .sboms[0].id == "rad-sbom"
    ' "${REPO_ROOT}/.goreleaser.yaml" > /dev/null || {
        fail "SBOMs can leak into the binary checksum pipeline"
    }
    verify_tool_pin
    verify_workflow_wiring
    verify_documentation
    echo "release SBOM contract tests passed"
}

main "$@"
