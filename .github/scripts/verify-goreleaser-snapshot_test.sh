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

# Exercises verify-goreleaser-snapshot.sh against deterministic fixtures that
# mirror the metadata GoReleaser writes for a snapshot: one binary and one
# checksum sidecar per CLI asset, and one loaded image per production platform.
# The real .goreleaser.yaml and targets.json are used so the test also proves
# that the committed configuration satisfies the parity contract.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly TARGETS="${REPO_ROOT}/.github/release-parity/targets.json"
readonly VERIFIER="${SCRIPT_DIR}/verify-goreleaser-snapshot.sh"
readonly REGISTRY="ghcr.io/radius-project"
readonly SNAPSHOT_VERSION="0.0.0-snapshot-0123abcd"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly DIST="${TEST_ROOT}/dist"
readonly ENTRIES="${TEST_ROOT}/entries.jsonl"

fail() {
    echo "Error: $*" >&2
    exit 1
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d ' ' -f 1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d ' ' -f 1
    else
        openssl dgst -sha256 "$1" | awk '{ print $NF }'
    fi
}

# Fixture knobs. Each one breaks a single contract the verifier must catch.
OMIT_ASSET=""          # drop this CLI asset from the metadata and the dist dir
CORRUPT_CHECKSUM=""    # write a wrong hash into this asset's sidecar
EXTRA_TARGET="false"   # add a rad build for a platform outside the contract
OMIT_PLATFORM=""       # <image>:<platform> to leave out of the built images
WITHOUT_IMAGES="false" # emulate a snapshot that ran with --skip=docker
OMIT_SBOM=""           # drop this CLI asset's SBOM from the metadata
INVALID_SBOM=""        # write a document that is not SPDX for this CLI asset

# The smallest document that satisfies the verifier's SPDX checks.
write_sbom() {
    local asset="$1"
    local path="$2"

    jq -n --arg asset "${asset}" '
        {
            spdxVersion: "SPDX-2.3",
            SPDXID: "SPDXRef-DOCUMENT",
            dataLicense: "CC0-1.0",
            name: $asset,
            documentNamespace: ("https://radius.example/spdx/" + $asset),
            creationInfo: {
                created: "2026-09-10T00:00:00Z",
                creators: ["Tool: syft-1.0.0"]
            },
            packages: [{SPDXID: "SPDXRef-Package-rad", name: "rad"}],
            relationships: []
        }
    ' >"${path}"
}

write_cli_fixture() {
    local asset os arch goarm
    local binary_dir binary_path hash sidecar sbom

    while IFS=$'\t' read -r asset os arch; do
        [[ "${asset}" != "${OMIT_ASSET}" ]] || continue
        goarm=""
        [[ "${arch}" != "arm" ]] || goarm="7"
        binary_dir="${DIST}/${asset}"
        mkdir -p "${binary_dir}"
        binary_path="${binary_dir}/rad"
        [[ "${os}" != "windows" ]] || binary_path="${binary_dir}/rad.exe"
        printf 'fake %s binary\n' "${asset}" >"${binary_path}"

        hash="$(sha256_file "${binary_path}")"
        [[ "${asset}" != "${CORRUPT_CHECKSUM}" ]] || hash="$(printf '%064d' 0)"
        # GoReleaser writes a split checksum as the bare hash without a newline.
        sidecar="${DIST}/${asset}.sha256"
        printf '%s' "${hash}" >"${sidecar}"

        jq -n -c --arg name "${asset}" --arg path "${binary_path}" \
            --arg goos "${os}" --arg goarch "${arch}" --arg goarm "${goarm}" '
            {
                name: $name,
                path: $path,
                goos: $goos,
                goarch: $goarch,
                type: "Binary",
                extra: {Binary: "rad", ID: "rad"}
            }
            | if $goarm != "" then .goarm = $goarm else . end
        ' >>"${ENTRIES}"
        jq -n -c --arg name "${asset}.sha256" --arg path "${sidecar}" \
            --arg checksum_of "${binary_path}" '
            {
                name: $name,
                path: $path,
                type: "Checksum",
                extra: {ChecksumOf: $checksum_of}
            }
        ' >>"${ENTRIES}"

        [[ "${asset}" != "${OMIT_SBOM}" ]] || continue
        sbom="${DIST}/${asset}.sbom.json"
        if [[ "${asset}" == "${INVALID_SBOM}" ]]; then
            printf '{"name":"%s"}\n' "${asset}" >"${sbom}"
        else
            write_sbom "${asset}" "${sbom}"
        fi
        jq -n -c --arg name "${asset}.sbom.json" --arg path "${sbom}" '
            {
                name: $name,
                path: $path,
                type: "SBOM",
                extra: {ID: "rad-sbom"}
            }
        ' >>"${ENTRIES}"
    done < <(jq -r '.cliAssets[] | [.name, .os, .arch] | @tsv' "${TARGETS}")

    if [[ "${EXTRA_TARGET}" == "true" ]]; then
        mkdir -p "${DIST}/rad_freebsd_amd64"
        printf 'fake freebsd binary\n' >"${DIST}/rad_freebsd_amd64/rad"
        jq -n -c --arg path "${DIST}/rad_freebsd_amd64/rad" '
            {
                name: "rad_freebsd_amd64",
                path: $path,
                goos: "freebsd",
                goarch: "amd64",
                type: "Binary",
                extra: {Binary: "rad", ID: "rad"}
            }
        ' >>"${ENTRIES}"
    fi
}

write_image_fixture() {
    local image platform suffix name

    [[ "${WITHOUT_IMAGES}" != "true" ]] || return 0

    while IFS=$'\t' read -r image platform; do
        [[ "${image}:${platform}" != "${OMIT_PLATFORM}" ]] || continue
        # Snapshot tags carry the platform without the "linux/" prefix or
        # slashes, for example 0.0.0-snapshot-0123abcd-armv7.
        suffix="${platform#linux/}"
        suffix="${suffix//\//}"
        name="${REGISTRY}/${image}:${SNAPSHOT_VERSION}-${suffix}"
        jq -n -c --arg name "${name}" --arg id "${image}" \
            --arg platform "${platform}" '
            {
                name: $name,
                path: $name,
                type: "Docker Image",
                extra: {Digest: "sha256:0123", ID: $id, Platforms: [$platform]}
            }
        ' >>"${ENTRIES}"
    done < <(jq -r '
        .images[]
        | select(.category == "production")
        | .name as $name
        | .requiredPlatforms[]
        | [$name, .]
        | @tsv
    ' "${TARGETS}")
}

write_fixture() {
    rm -rf "${DIST}"
    mkdir -p "${DIST}"
    : >"${ENTRIES}"
    write_cli_fixture
    write_image_fixture
    jq -s '.' "${ENTRIES}" >"${DIST}/artifacts.json"
}

run_verifier() {
    bash "${VERIFIER}" "$@" "${DIST}" >/dev/null
}

expect_failure() {
    local description="$1"
    shift

    if run_verifier "$@" 2>/dev/null; then
        fail "verifier accepted ${description}"
    fi
}

# The verifier locates the Dockerfiles through its own repository root, so the
# Dockerfile checks run against a staged copy of the repository that the test
# can mutate freely.
readonly STAGED_REPO="${TEST_ROOT}/repo"
readonly STAGED_VERIFIER="${STAGED_REPO}/.github/scripts/verify-goreleaser-snapshot.sh"

stage_repo() {
    local dockerfile

    rm -rf "${STAGED_REPO}"
    mkdir -p "${STAGED_REPO}/.github/scripts" "${STAGED_REPO}/.github/release-parity"
    cp "${VERIFIER}" "${STAGED_VERIFIER}"
    cp "${TARGETS}" "${STAGED_REPO}/.github/release-parity/targets.json"
    cp "${REPO_ROOT}/.goreleaser.yaml" "${STAGED_REPO}/.goreleaser.yaml"
    while IFS= read -r dockerfile; do
        mkdir -p "${STAGED_REPO}/$(dirname "${dockerfile}")"
        cp "${REPO_ROOT}/${dockerfile}" "${STAGED_REPO}/${dockerfile}"
    done < <(cd "${REPO_ROOT}" && ls deploy/images/*/Dockerfile deploy/images/*/Dockerfile.goreleaser)
}

expect_staged_failure() {
    local description="$1"

    if bash "${STAGED_VERIFIER}" "${DIST}" >/dev/null 2>&1; then
        fail "verifier accepted ${description}"
    fi
}

write_fixture
run_verifier

# A snapshot built with --skip=docker has no images; that is acceptable only
# when the caller says so.
WITHOUT_IMAGES="true"
write_fixture
run_verifier --skip-images
expect_failure "a snapshot without built images"
WITHOUT_IMAGES="false"

OMIT_ASSET="rad_linux_arm"
write_fixture
expect_failure "a missing CLI asset"
OMIT_ASSET=""

CORRUPT_CHECKSUM="rad_windows_amd64.exe"
write_fixture
expect_failure "a checksum sidecar that does not match its binary"
CORRUPT_CHECKSUM=""

EXTRA_TARGET="true"
write_fixture
expect_failure "a rad build outside the parity contract"
EXTRA_TARGET="false"

OMIT_PLATFORM="ucpd:linux/arm/v7"
write_fixture
expect_failure "a production image missing a required platform"
OMIT_PLATFORM=""

OMIT_SBOM="rad_darwin_arm64"
write_fixture
expect_failure "a CLI asset without an SBOM"
OMIT_SBOM=""

INVALID_SBOM="rad_linux_amd64"
write_fixture
expect_failure "a CLI SBOM that is not an SPDX document"
INVALID_SBOM=""

# Configuration drift is caught before any artifact is inspected. The copy is
# exploded first so editing one image cannot silently follow a YAML anchor.
write_fixture
export GORELEASER_CONFIG_FILE="${TEST_ROOT}/goreleaser.yaml"
cp "${REPO_ROOT}/.goreleaser.yaml" "${GORELEASER_CONFIG_FILE}"
yq -i '
    explode(.)
    | (.dockers_v2[] | select(.id == "controller") | .platforms)
        |= map(select(. != "linux/arm/v7"))
' "${GORELEASER_CONFIG_FILE}"
expect_failure "a configuration that drops a required image platform"

cp "${REPO_ROOT}/.goreleaser.yaml" "${GORELEASER_CONFIG_FILE}"
yq -i 'del(.checksum.split)' "${GORELEASER_CONFIG_FILE}"
expect_failure "a configuration without split checksum sidecars"
unset GORELEASER_CONFIG_FILE

# Each Dockerfile.goreleaser must keep the runtime contract of its production
# Dockerfile: base image, packages, user, working directory, ports, entrypoint.
stage_repo
bash "${STAGED_VERIFIER}" "${DIST}" >/dev/null

stage_repo
printf '\nUSER 0\n' >>"${STAGED_REPO}/deploy/images/ucpd/Dockerfile.goreleaser"
expect_staged_failure "a GoReleaser Dockerfile that changes the runtime user"

stage_repo
sed -i.bak 's/^FROM alpine:3.21.3$/FROM alpine:3.22/' \
    "${STAGED_REPO}/deploy/images/applications-rp/Dockerfile.goreleaser"
grep -q '^FROM alpine:3.22$' \
    "${STAGED_REPO}/deploy/images/applications-rp/Dockerfile.goreleaser" ||
    fail "fixture did not change the applications-rp base image"
expect_staged_failure "a GoReleaser Dockerfile with a different base image"

stage_repo
rm "${STAGED_REPO}/deploy/images/controller/Dockerfile.goreleaser"
expect_staged_failure "a production image without a GoReleaser Dockerfile"

echo "GoReleaser snapshot verifier tests passed"
