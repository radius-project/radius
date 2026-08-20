#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly DIST_DIR="${1:-${REPO_ROOT}/dist/goreleaser}"
readonly TARGETS_FILE="${REPO_ROOT}/.github/release-parity/targets.json"
readonly CONFIG_FILE="${REPO_ROOT}/.goreleaser.yaml"

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

main() {
    require_command jq
    require_command sha256sum
    require_command yq

    [[ -f "${DIST_DIR}/artifacts.json" ]] ||
        fail "missing GoReleaser artifacts metadata"
    verify_native_checksum_config
    verify_release_config
    verify_cli_assets "${DIST_DIR}/artifacts.json"
    verify_build_matrix
    verify_image_definitions
    echo "GoReleaser snapshot matches the release parity contract"
}

main "$@"
