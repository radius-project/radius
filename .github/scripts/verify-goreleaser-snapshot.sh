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
readonly TARGETS_FILE="${REPO_ROOT}/.github/release-parity/targets.json"
readonly CONFIG_FILE="${GORELEASER_CONFIG_FILE:-${REPO_ROOT}/.goreleaser.yaml}"

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

assert_json_equal() {
    local actual="$1"
    local expected="$2"
    local description="$3"

    jq -e -n --argjson actual "${actual}" --argjson expected "${expected}" \
        '$actual == $expected' >/dev/null ||
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
    local global_environment
    local expected_disable='{{ .Env.GORELEASER_RELEASE_DISABLE }}'

    yq -e '
        ((.release.ids | length) == 1)
        and (.release.ids[0] == "rad")
    ' "${CONFIG_FILE}" >/dev/null ||
        fail "GoReleaser release settings do not match the parity contract"
    [[ "$(yq -r '.release.disable' "${CONFIG_FILE}")" == "${expected_disable}" ]] ||
        fail "GoReleaser release disable switch is not parameterized"

    global_environment="$(yq -r '.env[]' "${CONFIG_FILE}")"
    if ! grep -Fq 'GORELEASER_IMAGE_REGISTRY=' <<<"${global_environment}" ||
        ! grep -Fq 'ghcr.io/radius-project' <<<"${global_environment}"; then
        fail "GoReleaser image registry does not have a production default"
    fi
    if ! grep -Fq 'GORELEASER_RELEASE_DISABLE=' <<<"${global_environment}" ||
        ! grep -Fq 'else }}false{{ end }}' <<<"${global_environment}"; then
        fail "GoReleaser release disable switch does not default to false"
    fi
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
        [[ -f "${REPO_ROOT}/${artifact_path}" ]] ||
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

        checksum_path="${REPO_ROOT}/${checksum_artifact_path}"
        [[ -f "${checksum_path}" ]] ||
            fail "missing checksum sidecar: ${asset}.sha256"
        declared_hash="$(tr -d '\r\n' <"${checksum_path}")"
        if [[ ! "${declared_hash}" =~ ^[0-9a-f]{64}$ ]]; then
            fail "invalid checksum format for ${asset}.sha256"
        fi
        actual_hash="$(
            sha256sum "${REPO_ROOT}/${artifact_path}" | cut -d ' ' -f 1
        )"
        [[ "${declared_hash}" == "${actual_hash}" ]] ||
            fail "checksum mismatch for ${asset}"
    done < <(jq -r '.cliAssets[].name' "${TARGETS_FILE}")
}

verify_build_matrix() {
    local mode="${1:-}"
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

    if [[ "${mode}" == "--config-only" ]]; then
        return
    fi

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
    local image_repository
    local image_repository_count
    local expected_repository

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

        IMAGE="${image}" yq -e '
            .dockers_v2[]
                        | select(.id == strenv(IMAGE))
                        | (((.ids | length) == 1)
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

        image_repository="$(IMAGE="${image}" yq -r '
            .dockers_v2[]
            | select(.id == strenv(IMAGE))
            | .images[0]
        ' "${CONFIG_FILE}")"
        image_repository_count="$(IMAGE="${image}" yq -r '
            .dockers_v2[]
            | select(.id == strenv(IMAGE))
            | .images
            | length
        ' "${CONFIG_FILE}")"
        [[ "${image_repository_count}" == "1" ]] ||
            fail "${image} must publish to exactly one image repository"
        expected_repository="{{ .Env.GORELEASER_IMAGE_REGISTRY }}/${image}"
        [[ "${image_repository}" == "${expected_repository}" ]] ||
            fail "${image} image repository is not parameterized"

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

# The shadow images are built from a separate Dockerfile, so the base image,
# package installs, and runtime identity can drift away from production without
# anything noticing. The shadow parity job only compares Radius-owned files
# inside the images, so this static check is what keeps the rest honest.
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

main() {
    local config_only=0

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --config-only) config_only=1 ;;
            -*) fail "unknown argument: $1" ;;
            *) DIST_DIR="$1" ;;
        esac
        shift
    done

    require_command jq
    require_command yq

    verify_native_checksum_config
    verify_release_config
    verify_build_matrix --config-only
    verify_image_definitions
    verify_dockerfile_parity
    if [[ "${config_only}" -eq 1 ]]; then
        echo "GoReleaser configuration matches the release parity contract"
        return
    fi

    require_command sha256sum

    [[ -f "${DIST_DIR}/artifacts.json" ]] ||
        fail "missing GoReleaser artifacts metadata"
    verify_cli_assets "${DIST_DIR}/artifacts.json"
    verify_build_matrix
    echo "GoReleaser snapshot matches the release parity contract"
}

main "$@"
