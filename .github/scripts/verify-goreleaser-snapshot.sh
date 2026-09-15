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
DIST_DIR="${REPO_ROOT}/dist/goreleaser"
SKIP_IMAGES=0
readonly TARGETS_FILE="${REPO_ROOT}/.github/release-parity/targets.json"
readonly CONFIG_FILE="${GORELEASER_CONFIG_FILE:-${REPO_ROOT}/.goreleaser.yaml}"

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_any_command() {
    local candidate
    for candidate in "$@"; do
        command -v "${candidate}" >/dev/null 2>&1 && return 0
    done
    fail "required command not found: one of $*"
}

# macOS ships shasum and openssl rather than GNU sha256sum.
sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d ' ' -f 1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d ' ' -f 1
    else
        openssl dgst -sha256 "$1" | awk '{ print $NF }'
    fi
}

# GoReleaser records artifact paths relative to its working directory, which is
# the repository root for every Make target. Absolute paths pass through so the
# verifier can also read metadata that was produced elsewhere.
resolve_path() {
    local path="$1"

    if [[ "${path}" == /* ]]; then
        printf '%s' "${path}"
    else
        printf '%s/%s' "${REPO_ROOT}" "${path}"
    fi
}

assert_json_equal() {
    local actual="$1"
    local expected="$2"
    local description="$3"

    jq -e -n --argjson actual "${actual}" --argjson expected "${expected}" \
        '$actual == $expected' >/dev/null && return 0
    echo "expected ${description}: ${expected}" >&2
    echo "actual ${description}: ${actual}" >&2
    fail "${description} do not match the parity contract"
}

platform_name() {
    local os="$1"
    local arch="$2"
    local arm="$3"

    if [[ "${arch}" == "arm" ]]; then
        printf '%s/%s/v%s' "${os}" "${arch}" "${arm}"
    else
        printf '%s/%s' "${os}" "${arch}"
    fi
}

verify_native_checksum_config() {
    yq -e '
        ((.checksum.name_template == "{{ .ArtifactName }}.sha256")
          and (.checksum.algorithm == "sha256")
          and (.checksum.split == true)
          and ((.checksum.ids | length) == 1)
          and (.checksum.ids[0] == "rad"))
    ' "${CONFIG_FILE}" >/dev/null ||
        fail "native GoReleaser checksum configuration is not enabled"
}

verify_release_config() {
    yq -e '
        (((.release.ids | length) == 1)
          and (.release.ids[0] == "rad"))
    ' "${CONFIG_FILE}" >/dev/null ||
        fail "GoReleaser release artifact selection is not rad-only"
}

verify_cli_assets() {
    local artifacts_file="$1"
    local expected_names
    local actual_names
    local asset
    local artifact_path
    local checksum_artifact_path
    local checksum_path
    local declared_hash
    local actual_hash

    expected_names="$(jq -c '[.cliAssets[].name] | sort' "${TARGETS_FILE}")"
    actual_names="$(jq -c '[
        .[]
        | select(
            .type == "Binary"
            and .extra.ID == "rad"
            and (.name | startswith("rad_"))
        )
        | .name
    ] | sort' "${artifacts_file}")"
    assert_json_equal "${actual_names}" "${expected_names}" \
        "CLI asset names"

    while IFS= read -r asset; do
        artifact_path="$(jq -r --arg asset "${asset}" '
            .[]
            | select(
                .name == $asset
                and .type == "Binary"
                and .extra.ID == "rad"
            )
            | .path
        ' "${artifacts_file}")"
        [[ -f "$(resolve_path "${artifact_path}")" ]] ||
            fail "missing CLI artifact: ${artifact_path}"

        checksum_artifact_path="$(jq -r \
            --arg name "${asset}.sha256" \
            --arg binary_path "${artifact_path}" '
            [
                .[]
                | select(
                    .name == $name
                    and .type == "Checksum"
                    and .extra.ChecksumOf == $binary_path
                )
            ]
            | if length == 1 then .[0].path else empty end
        ' "${artifacts_file}")"
        [[ -n "${checksum_artifact_path}" ]] ||
            fail "missing native GoReleaser checksum artifact: ${asset}.sha256"

        checksum_path="$(resolve_path "${checksum_artifact_path}")"
        [[ -f "${checksum_path}" ]] ||
            fail "missing checksum sidecar: ${asset}.sha256"
        declared_hash="$(tr -d '\r\n' <"${checksum_path}")"
        if [[ ! "${declared_hash}" =~ ^[0-9a-f]{64}$ ]]; then
            fail "invalid checksum format for ${asset}.sha256"
        fi
        actual_hash="$(sha256_file "$(resolve_path "${artifact_path}")")"
        [[ "${declared_hash}" == "${actual_hash}" ]] ||
            fail "checksum mismatch for ${asset}"
    done < <(jq -r '.cliAssets[].name' "${TARGETS_FILE}")
}

verify_build_matrix() {
    local expected_builds
    local actual_builds
    local expected_rad_targets
    local actual_rad_targets

    expected_builds='[
        "applications-rp",
        "controller",
        "dynamic-rp",
        "pre-upgrade",
        "rad",
        "ucpd"
    ]'
    actual_builds="$(
        yq -o=json '.builds | map(.id) | sort' "${CONFIG_FILE}"
    )"
    assert_json_equal "${actual_builds}" "${expected_builds}" "build IDs"

    expected_rad_targets="$(jq -c '[
        .cliAssets[]
        | if .arch == "arm"
            then .os + "/" + .arch + "/v7"
            else .os + "/" + .arch
          end
    ] | sort' "${TARGETS_FILE}")"
    actual_rad_targets="$(jq -c '[
        .[]
        | select(
            .type == "Binary"
            and .extra.ID == "rad"
            and (.name | startswith("rad_"))
        )
        | if .goarch == "arm"
            then .goos + "/" + .goarch + "/v" + .goarm
            else .goos + "/" + .goarch
          end
    ] | sort' "${DIST_DIR}/artifacts.json")"
    assert_json_equal "${actual_rad_targets}" "${expected_rad_targets}" \
        "rad build targets"
}

verify_image_definitions() {
    local expected_images
    local actual_images
    local image
    local expected_platforms
    local actual_platforms
    local dockerfile

    expected_images="$(jq -c '[
        .images[]
        | select(.category == "production")
        | .name
    ] | sort' "${TARGETS_FILE}")"
    actual_images="$(
        yq -o=json '.dockers_v2 | map(.id) | sort' "${CONFIG_FILE}"
    )"
    assert_json_equal "${actual_images}" "${expected_images}" \
        "production image names"

    while IFS= read -r image; do
        expected_platforms="$(jq -c --arg image "${image}" '
            .images[]
            | select(.name == $image)
            | .requiredPlatforms
            | sort
        ' "${TARGETS_FILE}")"
        actual_platforms="$(IMAGE="${image}" yq -o=json '
            explode(.)
            | .dockers_v2[]
            | select(.id == env(IMAGE))
            | .platforms
            | sort
        ' "${CONFIG_FILE}")"
        assert_json_equal "${actual_platforms}" "${expected_platforms}" \
            "${image} image platforms"

        IMAGE="${image}" \
            IMAGE_REPOSITORY="ghcr.io/radius-project/${image}" yq -e '
            .dockers_v2[]
                        | select(.id == strenv(IMAGE))
                        | (((.images | length) == 1)
                            and (.images[0] == strenv(IMAGE_REPOSITORY))
                            and ((.ids | length) == 1)
                            and (.ids[0] == strenv(IMAGE))
                            and ((.tags | length) == 1)
                            and (.tags[0] == "{{ .Version }}")
                            and (.sbom == false)
                            and (.labels."org.opencontainers.image.description"
                                == strenv(IMAGE))
                            and (.labels."org.opencontainers.image.source"
                                == "https://github.com/radius-project/radius")
                            and ((.labels."org.opencontainers.image.version" | length) > 0)
                            and ((.labels."org.opencontainers.image.revision" | length) > 0))
        ' "${CONFIG_FILE}" >/dev/null ||
            fail "${image} image metadata does not match the parity contract"

        dockerfile="$(IMAGE="${image}" yq -r '
            .dockers_v2[]
            | select(.id == env(IMAGE))
            | .dockerfile
        ' "${CONFIG_FILE}")"
        [[ -f "${REPO_ROOT}/${dockerfile}" ]] ||
            fail "missing Dockerfile for ${image}: ${dockerfile}"
    done < <(jq -r '
        .images[]
        | select(.category == "production")
        | .name
    ' "${TARGETS_FILE}")

    yq -e '
        .dockers_v2[]
        | select(.id == "ucpd")
                | (((.extra_files | length) == 1)
                    and (.extra_files[0]
                        == "deploy/manifest/built-in-providers/self-hosted"))
    ' "${CONFIG_FILE}" >/dev/null ||
        fail "ucpd image does not include the built-in provider manifests"
}

# Directives that define the image runtime contract. COPY and ARG are excluded
# because the production and GoReleaser build contexts expose the binaries at
# different paths by design.
readonly DOCKERFILE_DIRECTIVES='FROM|RUN|ENV|USER|WORKDIR|EXPOSE|ENTRYPOINT|CMD'

# Emit the runtime directives of a Dockerfile with comments removed, line
# continuations joined, and whitespace collapsed, so that two Dockerfiles can be
# compared on meaning rather than formatting.
normalize_dockerfile() {
    local file="$1"

    awk '
        /^[[:space:]]*#/ { next }
        /^[[:space:]]*$/ { next }
        {
            line = $0
            sub(/^[[:space:]]+/, "", line)
            buffer = buffer line
            if (buffer ~ /\\$/) {
                sub(/\\$/, " ", buffer)
                next
            }
            print buffer
            buffer = ""
        }
        END { if (buffer != "") print buffer }
    ' "${file}" |
        sed -E 's/[[:space:]]+/ /g; s/ $//' |
        { grep -E "^(${DOCKERFILE_DIRECTIVES}) " || true; }
}

# GoReleaser uses separate Dockerfiles, so this static check prevents their
# runtime contract from drifting away from the development image path.
verify_dockerfile_parity() {
    local image
    local production
    local shadow

    while IFS= read -r image; do
        production="${REPO_ROOT}/deploy/images/${image}/Dockerfile"
        shadow="${production}.goreleaser"

        [[ -f "${production}" ]] ||
            fail "missing production Dockerfile for ${image}"
        [[ -f "${shadow}" ]] ||
            fail "missing Dockerfile.goreleaser for ${image}"

        diff -u \
            <(normalize_dockerfile "${production}") \
            <(normalize_dockerfile "${shadow}") ||
            fail "Dockerfile.goreleaser for ${image} does not match the" \
                "production runtime contract"
    done < <(jq -r '
        .images[]
        | select(.category == "production")
        | .name
    ' "${TARGETS_FILE}")
}

# A snapshot loads one image per platform with a platform suffix on the tag,
# while a release pushes one multi-platform manifest per image. In both cases
# the artifact metadata must cover exactly the platforms the contract requires,
# in the repository the configuration declares.
verify_built_images() {
    local artifacts_file="$1"
    local registry="${GORELEASER_IMAGE_REGISTRY:-ghcr.io/radius-project}"
    local image
    local expected_platforms
    local actual_platforms
    local foreign_names

    while IFS= read -r image; do
        expected_platforms="$(jq -c --arg image "${image}" '
            .images[]
            | select(.name == $image)
            | .requiredPlatforms
            | sort
        ' "${TARGETS_FILE}")"
        actual_platforms="$(jq -c --arg image "${image}" '[
            .[]
            | select(.type == "Docker Image" and .extra.ID == $image)
            | .extra.Platforms[]
        ] | unique' "${artifacts_file}")"
        [[ "${actual_platforms}" != "[]" ]] ||
            fail "no built images found for ${image}" \
                "(pass --skip-images when the snapshot ran with --skip=docker)"
        assert_json_equal "${actual_platforms}" "${expected_platforms}" \
            "${image} built image platforms"

        foreign_names="$(jq -c --arg image "${image}" \
            --arg prefix "${registry}/${image}:" '[
            .[]
            | select(.type == "Docker Image" and .extra.ID == $image)
            | .name
            | select(startswith($prefix) | not)
        ]' "${artifacts_file}")"
        [[ "${foreign_names}" == "[]" ]] ||
            fail "${image} images were built for an unexpected repository:" \
                "${foreign_names}"
    done < <(jq -r '
        .images[]
        | select(.category == "production")
        | .name
    ' "${TARGETS_FILE}")
}

main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-images) SKIP_IMAGES=1 ;;
            -*) fail "unknown argument: $1" ;;
            *) DIST_DIR="$1" ;;
        esac
        shift
    done

    require_command jq
    require_command yq
    require_any_command sha256sum shasum openssl

    [[ -f "${DIST_DIR}/artifacts.json" ]] ||
        fail "missing GoReleaser artifacts metadata"
    verify_native_checksum_config
    verify_release_config
    verify_cli_assets "${DIST_DIR}/artifacts.json"
    verify_build_matrix
    verify_image_definitions
    verify_dockerfile_parity
    if [[ "${SKIP_IMAGES}" -eq 1 ]]; then
        echo "skipping built image verification: the snapshot ran without Docker"
    else
        verify_built_images "${DIST_DIR}/artifacts.json"
    fi
    echo "GoReleaser snapshot matches the release parity contract"
}

main "$@"
