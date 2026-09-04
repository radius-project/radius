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

SHADOW_DIR="${GORELEASER_SHADOW_DIR:-${REPO_ROOT}/dist/goreleaser}"
PRODUCTION_DIR="${GORELEASER_PRODUCTION_DIR:-${REPO_ROOT}/release}"
SHADOW_REGISTRY="${GORELEASER_SHADOW_REGISTRY:-ghcr.io/radius-project/dev}"
PRODUCTION_REGISTRY="${GORELEASER_PRODUCTION_REGISTRY:-ghcr.io/radius-project}"
PRODUCTION_IMAGE_LOCK="${GORELEASER_PRODUCTION_IMAGE_LOCK:-}"
TARGETS_FILE="${GORELEASER_PARITY_TARGETS:-${REPO_ROOT}/.github/release-parity/targets.json}"
BASELINE_DIR="${GORELEASER_PARITY_BASELINES:-${REPO_ROOT}/.github/release-parity/baselines}"
REPORT_PATH="${GORELEASER_PARITY_REPORT:-${REPO_ROOT}/dist/goreleaser-shadow-parity.json}"
BUILD_INFO_DIR="${GORELEASER_BUILD_INFO_DIR:-}"
SHADOW_IMAGE_DATA_DIR="${GORELEASER_SHADOW_IMAGE_DATA_DIR:-}"
PRODUCTION_IMAGE_DATA_DIR="${GORELEASER_PRODUCTION_IMAGE_DATA_DIR:-}"
SHADOW_PAYLOAD_DIR="${GORELEASER_SHADOW_PAYLOAD_DIR:-}"
PRODUCTION_PAYLOAD_DIR="${GORELEASER_PRODUCTION_PAYLOAD_DIR:-}"
PAYLOAD_MANIFEST_PACKAGE="./.github/scripts/image-payload-manifest"
VERSION="${REL_VERSION:-}"
CHANNEL="${REL_CHANNEL:-}"
CHART_VERSION_VALUE="${CHART_VERSION:-}"
SOURCE_COMMIT="${GIT_COMMIT:-}"
TEMP_DIR=""
declare -a CONTAINERS=()

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Options:
  --shadow-dir <path>          Downloaded GoReleaser output directory
  --production-dir <path>      Downloaded production CLI artifact directory
  --shadow-registry <registry> Shadow image registry namespace
  --production-registry <reg>  Production image registry namespace
  --version <version>          Full release version without the v prefix
  --channel <channel>          Production image channel tag
  --chart-version <version>    Expected embedded Helm chart version
  --commit <sha>               Expected source commit
  --report <path>              JSON parity report output path
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

cleanup() {
    local container

    for container in "${CONTAINERS[@]:-}"; do
        docker rm -f "${container}" >/dev/null 2>&1 || true
    done
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

sha256_file() {
    local path="$1"

    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${path}" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${path}" | awk '{print $1}'
    else
        openssl dgst -sha256 "${path}" | awk '{print $NF}'
    fi
}

assert_json_equal() {
    local actual="$1"
    local expected="$2"
    local description="$3"

    jq -e -n --argjson actual "${actual}" --argjson expected "${expected}" \
        '$actual == $expected' >/dev/null ||
        fail "${description} do not match"
}

select_baseline() {
    local output="$1"
    local -a baselines=("${BASELINE_DIR}"/*.json)

    [[ -f "${baselines[0]}" ]] ||
        fail "no release parity baselines found in ${BASELINE_DIR}"
    jq -S -s '
        map(select(.release.prerelease == false))
        | sort_by(.release.version | split(".") | map(tonumber))
        | last // error("no final-release baseline found")
    ' "${baselines[@]}" >"${output}"
}

verify_contract() {
    local baseline="$1"
    local expected_assets
    local baseline_assets
    local expected_images
    local baseline_images

    expected_assets="$(jq -c '[.cliAssets[].name] | sort' "${TARGETS_FILE}")"
    baseline_assets="$(jq -c '[.cli.assets[].name] | sort' "${baseline}")"
    assert_json_equal "${baseline_assets}" "${expected_assets}" \
        "baseline and target CLI asset sets"

    expected_images="$(jq -c '[
        .images[] | select(.category == "production")
        | {name, platforms: (.requiredPlatforms | sort)}
    ] | sort_by(.name)' "${TARGETS_FILE}")"
    baseline_images="$(jq -c '[
        .images[] | select(.category == "production")
        | {name, platforms: ([.platforms[].platform] | sort)}
    ] | sort_by(.name)' "${baseline}")"
    assert_json_equal "${baseline_images}" "${expected_images}" \
        "baseline and target production image sets"
}

shadow_artifact_path() {
    local name="$1"
    local artifact_path
    local relative_path

    artifact_path="$(jq -er --arg name "${name}" '
        [
            .[]
            | select(
                .name == $name
                and .type == "Binary"
                and .extra.ID == "rad"
            )
        ]
        | if length == 1 then .[0].path else error(
            "expected one GoReleaser binary named " + $name
          ) end
    ' "${SHADOW_DIR}/artifacts.json")"
    relative_path="${artifact_path#dist/goreleaser/}"
    [[ "${relative_path}" != "${artifact_path}" ]] ||
        fail "unexpected GoReleaser artifact path: ${artifact_path}"
    printf '%s/%s' "${SHADOW_DIR}" "${relative_path}"
}

extract_build_info() {
    local kind="$1"
    local asset="$2"
    local binary="$3"
    local output="$4"
    local fixture="${BUILD_INFO_DIR}/${kind}-${asset}.json"

    if [[ -n "${BUILD_INFO_DIR}" ]]; then
        [[ -f "${fixture}" ]] || fail "missing build-info fixture: ${fixture}"
        cp "${fixture}" "${output}"
        return
    fi

    go version -m -json "${binary}" |
        jq '
            def settings:
                reduce (.Settings // [])[] as $setting
                    ({}; .[$setting.Key] = $setting.Value);
            . as $build
            | (settings) as $settings
            | ($settings["-ldflags"] // "") as $ldflags
            | ([
                $ldflags
                | scan("-X ([^= ]+)=([^ ]+)")
                | {key: .[0], value: .[1]}
            ] | from_entries) as $linker
            | {
                goVersion: $build.GoVersion,
                path: $build.Path,
                module: $build.Main,
                settings: $settings,
                linkerMetadata: {
                    channel: ($linker[
                        "github.com/radius-project/radius/pkg/version.channel"
                    ] // ""),
                    release: ($linker[
                        "github.com/radius-project/radius/pkg/version.release"
                    ] // ""),
                    commit: ($linker[
                        "github.com/radius-project/radius/pkg/version.commit"
                    ] // ""),
                    version: ($linker[
                        "github.com/radius-project/radius/pkg/version.version"
                    ] // ""),
                    chartVersion: ($linker[
                        "github.com/radius-project/radius/pkg/version.chartVersion"
                    ] // ""),
                    terraformVersion: ($linker[
                        "github.com/radius-project/radius/pkg/recipes/terraform.terraformVersion"
                    ] // "")
                }
            }
        ' >"${output}"
}

runtime_version() {
    local kind="$1"
    local binary="$2"
    local output="$3"
    local fixture="${BUILD_INFO_DIR}/${kind}-runtime.json"

    if [[ -n "${BUILD_INFO_DIR}" ]]; then
        [[ -f "${fixture}" ]] || fail "missing runtime fixture: ${fixture}"
        cp "${fixture}" "${output}"
        return
    fi

    chmod +x "${binary}"
    "${binary}" version --cli -o json >"${output}"
}

verify_cli_artifacts() {
    local entries_file="$1"
    local baseline="$2"
    local asset
    local name
    local shadow_binary
    local production_binary
    local shadow_hash
    local production_hash
    local checksum_path
    local declared_hash
    local shadow_build_info
    local production_build_info
    local expected_names
    local actual_names
    local shadow_runtime
    local production_runtime
    local baseline_build_contract
    local shadow_build_contract

    expected_names="$(jq -c '[.cliAssets[].name] | sort' "${TARGETS_FILE}")"
    actual_names="$(jq -c '[
        .[]
        | select(
            .type == "Binary"
            and .extra.ID == "rad"
            and (.name | startswith("rad_"))
        )
        | .name
    ] | sort' "${SHADOW_DIR}/artifacts.json")"
    assert_json_equal "${actual_names}" "${expected_names}" \
        "shadow CLI asset names"

    while IFS= read -r asset; do
        name="$(jq -r '.name' <<<"${asset}")"
        shadow_binary="$(shadow_artifact_path "${name}")"
        production_binary="${PRODUCTION_DIR}/${name}"
        checksum_path="${SHADOW_DIR}/${name}.sha256"

        [[ -f "${shadow_binary}" ]] || fail "missing shadow binary: ${name}"
        [[ -f "${production_binary}" ]] ||
            fail "missing production binary: ${name}"
        [[ -f "${checksum_path}" ]] ||
            fail "missing shadow checksum: ${name}.sha256"

        shadow_hash="$(sha256_file "${shadow_binary}")"
        production_hash="$(sha256_file "${production_binary}")"
        [[ "${shadow_hash}" == "${production_hash}" ]] ||
            fail "binary digest mismatch for ${name}"

        declared_hash="$(tr -d '\r\n' <"${checksum_path}")"
        [[ "${declared_hash}" =~ ^[0-9a-f]{64}$ ]] ||
            fail "unexpected native checksum format for ${name}.sha256"
        [[ "${declared_hash}" == "${shadow_hash}" ]] ||
            fail "shadow checksum mismatch for ${name}"

        shadow_build_info="${TEMP_DIR}/shadow-${name}.json"
        production_build_info="${TEMP_DIR}/production-${name}.json"
        extract_build_info shadow "${name}" "${shadow_binary}" \
            "${shadow_build_info}"
        extract_build_info production "${name}" "${production_binary}" \
            "${production_build_info}"
        assert_json_equal \
            "$(jq -cS . "${shadow_build_info}")" \
            "$(jq -cS . "${production_build_info}")" \
            "embedded build metadata for ${name}"

        jq -e \
            --arg channel "${CHANNEL}" \
            --arg version "${VERSION}" \
            --arg commit "${SOURCE_COMMIT}" \
            --arg chart "${CHART_VERSION_VALUE}" '
            .linkerMetadata.channel == $channel
            and .linkerMetadata.release == $version
            and .linkerMetadata.commit == $commit
            and .linkerMetadata.version == ("v" + $version)
            and .linkerMetadata.chartVersion == $chart
            and .settings.CGO_ENABLED == "0"
        ' "${shadow_build_info}" >/dev/null ||
            fail "embedded release metadata is incorrect for ${name}"

        if jq -e --arg name "${name}" '
            any(.cli.assets[]; .name == $name and (.build // null) != null)
        ' "${baseline}" >/dev/null; then
            baseline_build_contract="$(jq -cS --arg name "${name}" '
                def stable_settings: with_entries(select(
                    .key == "-buildmode"
                    or .key == "-compiler"
                    or .key == "CGO_ENABLED"
                    or .key == "GOOS"
                    or .key == "GOARCH"
                    or .key == "GOARM"
                    or .key == "GOAMD64"
                ));
                .cli.assets[]
                | select(.name == $name)
                | {
                    path: .build.path,
                    modulePath: .build.module.path,
                    settings: (.build.settings | stable_settings)
                }
            ' "${baseline}")"
            shadow_build_contract="$(jq -cS '
                def stable_settings: with_entries(select(
                    .key == "-buildmode"
                    or .key == "-compiler"
                    or .key == "CGO_ENABLED"
                    or .key == "GOOS"
                    or .key == "GOARCH"
                    or .key == "GOARM"
                    or .key == "GOAMD64"
                ));
                {
                    path,
                    modulePath: (.module.Path // .module.path // ""),
                    settings: (.settings | stable_settings)
                }
            ' "${shadow_build_info}")"
            assert_json_equal "${shadow_build_contract}" \
                "${baseline_build_contract}" \
                "baseline build contract for ${name}"
        fi

        jq -n \
            --arg name "${name}" \
            --arg sha256 "${shadow_hash}" \
            '{name: $name, sha256: $sha256}' >>"${entries_file}"
    done < <(jq -c '.cliAssets[]' "${TARGETS_FILE}")

    shadow_binary="$(shadow_artifact_path "rad_linux_amd64")"
    production_binary="${PRODUCTION_DIR}/rad_linux_amd64"
    shadow_runtime="${TEMP_DIR}/shadow-runtime.json"
    production_runtime="${TEMP_DIR}/production-runtime.json"
    runtime_version shadow "${shadow_binary}" "${shadow_runtime}"
    runtime_version production "${production_binary}" "${production_runtime}"
    assert_json_equal \
        "$(jq -cS . "${shadow_runtime}")" \
        "$(jq -cS . "${production_runtime}")" \
        "runtime CLI version output"
}

verify_image_baseline_contract() {
    local baseline="$1"
    local name="$2"
    local current="$3"
    local baseline_config
    local current_config
    local baseline_auxiliary
    local current_auxiliary

    if ! jq -e --arg name "${name}" '
        any(.images[];
            .name == $name
            and any(.platforms[]; (.config // null) != null))
    ' "${baseline}" >/dev/null; then
        return
    fi

    baseline_config="$(jq -cS --arg name "${name}" '
        def stable_labels: with_entries(
            if .key == "org.opencontainers.image.revision"
                then .value = "<revision>"
            elif .key == "org.opencontainers.image.version"
                then .value = "<version>"
            else .
            end
        );
        [.images[]
            | select(.name == $name)
            | .platforms[]
            | {
                platform,
                config: {
                    User: (.config.user // ""),
                    Entrypoint: (.config.entrypoint // []),
                    Cmd: (.config.command // []),
                    WorkingDir: (.config.workingDirectory // ""),
                    Env: (.config.environment // []),
                    Labels: ((.config.labels // {}) | stable_labels)
                }
            }
        ] | sort_by(.platform)
    ' "${baseline}")"
    current_config="$(jq -cS '
        def stable_labels: with_entries(
            if .key == "org.opencontainers.image.revision"
                then .value = "<revision>"
            elif .key == "org.opencontainers.image.version"
                then .value = "<version>"
            else .
            end
        );
        [.platforms[]
            | {
                platform,
                config: {
                    User: (.config.User // ""),
                    Entrypoint: (.config.Entrypoint // []),
                    Cmd: (.config.Cmd // []),
                    WorkingDir: (.config.WorkingDir // ""),
                    Env: (.config.Env // []),
                    Labels: ((.config.Labels // {}) | stable_labels)
                }
            }
        ] | sort_by(.platform)
    ' "${current}")"
    assert_json_equal "${current_config}" "${baseline_config}" \
        "baseline runtime image contract for ${name}"

    if jq -e --arg name "${name}" '
        any(.images[];
            .name == $name
            and (.auxiliaryManifests // null) != null)
    ' "${baseline}" >/dev/null; then
        baseline_auxiliary="$(jq -cS --arg name "${name}" '
            [.images[]
                | select(.name == $name)
                | .auxiliaryManifests[]
                | {
                    mediaType,
                    platform,
                    type: (.annotations[
                        "vnd.docker.reference.type"
                    ] // "")
                }
            ] | sort_by([.mediaType, .platform, .type])
        ' "${baseline}")"
        current_auxiliary="$(jq -cS '.auxiliary' "${current}")"
        assert_json_equal "${current_auxiliary}" "${baseline_auxiliary}" \
            "baseline auxiliary manifest contract for ${name}"
    fi
}

normalize_image() {
    local source="$1"
    local output="$2"

    jq -S '
        def platform_name($platform):
            $platform.os + "/" + $platform.architecture
            + (if ($platform.variant // "") == ""
                then ""
                else "/" + $platform.variant
              end);
        def config($image): ($image.config // {});
        if (.manifest.manifests // null) != null then
            . as $root
            | {
                digest: $root.manifest.digest,
                mediaType: $root.manifest.mediaType,
                platforms: [
                    $root.manifest.manifests[]
                    | select(.platform.os != "unknown")
                    | (platform_name(.platform)) as $platform
                    | {
                        platform: $platform,
                        digest: .digest,
                        config: config($root.image[$platform])
                    }
                ] | sort_by(.platform),
                auxiliary: [
                    $root.manifest.manifests[]
                    | select(.platform.os == "unknown")
                    | {
                        mediaType,
                        platform: platform_name(.platform),
                        type: (.annotations[
                            "vnd.docker.reference.type"
                        ] // "")
                    }
                ] | sort_by([.mediaType, .platform, .type])
            }
        else
            {
                digest: .manifest.digest,
                mediaType: .manifest.mediaType,
                platforms: [{
                    platform: platform_name(.image),
                    digest: .manifest.digest,
                    config: config(.image)
                }],
                auxiliary: []
            }
        end
    ' "${source}" >"${output}"
}

inspect_image() {
    local kind="$1"
    local name="$2"
    local reference="$3"
    local output="$4"
    local fixture_dir

    if [[ "${kind}" == "shadow" ]]; then
        fixture_dir="${SHADOW_IMAGE_DATA_DIR}"
    else
        fixture_dir="${PRODUCTION_IMAGE_DATA_DIR}"
    fi

    if [[ -n "${fixture_dir}" ]]; then
        [[ -f "${fixture_dir}/${name}.json" ]] ||
            fail "missing ${kind} image fixture for ${name}"
        cp "${fixture_dir}/${name}.json" "${output}"
        return
    fi

    docker buildx imagetools inspect --format '{{json .}}' \
        "${reference}" >"${output}"
}

# Radius-owned payload paths inside each image, relative to the image root.
#
# Only these paths are compared between the production and shadow images.
# Everything else in the root filesystem comes from the shared base image and
# its package installs. The production build restores those layers from the
# BuildKit GitHub Actions cache while the shadow build resolves them fresh, so
# comparing them would report upstream package drift rather than a GoReleaser
# packaging difference. Base image and package parity is enforced statically
# instead, by the Dockerfile contract check in verify-goreleaser-snapshot.sh.
payload_paths() {
    local name="$1"

    printf '%s\n' "${name}"
    if [[ "${name}" == "ucpd" ]]; then
        printf '%s\n' "manifest"
    fi
}

capture_payload() {
    local kind="$1"
    local name="$2"
    local repository="$3"
    local digest="$4"
    local platform="$5"
    local manifest_output="$6"
    local hash_output="$7"
    local fixture_dir
    local platform_slug
    local container
    local reference
    local archive
    local -a prefixes=()

    platform_slug="$(tr '/' '_' <<<"${platform}")"
    reference="${repository}@${digest}"
    archive="${TEMP_DIR}/${kind}-${name}-${platform_slug}.tar"

    if [[ "${kind}" == "shadow" ]]; then
        fixture_dir="${SHADOW_PAYLOAD_DIR}"
    else
        fixture_dir="${PRODUCTION_PAYLOAD_DIR}"
    fi

    if [[ -n "${fixture_dir}" ]]; then
        [[ -f "${fixture_dir}/${name}-${platform_slug}.payload.jsonl" ]] ||
            fail "missing ${kind} payload fixture for ${name} ${platform}"
        [[ -f "${fixture_dir}/${name}-${platform_slug}.binary.sha256" ]] ||
            fail "missing ${kind} binary hash fixture for ${name} ${platform}"
        sort -u "${fixture_dir}/${name}-${platform_slug}.payload.jsonl" \
            >"${manifest_output}"
        cp "${fixture_dir}/${name}-${platform_slug}.binary.sha256" \
            "${hash_output}"
        return
    fi

    docker pull --quiet --platform "${platform}" "${reference}" >/dev/null
    container="$(docker create --platform "${platform}" "${reference}")"
    CONTAINERS+=("${container}")
    docker export --output "${archive}" "${container}"
    docker rm "${container}" >/dev/null
    CONTAINERS=("${CONTAINERS[@]/${container}/}")

    mapfile -t prefixes < <(payload_paths "${name}" | sed 's/^/--payload=/')
    (cd "${REPO_ROOT}" && go run "${PAYLOAD_MANIFEST_PACKAGE}" \
        --archive="${archive}" \
        --binary="${name}" \
        --manifest="${manifest_output}" \
        --hash="${hash_output}" \
        "${prefixes[@]}")
}

verify_images() {
    local entries_file="$1"
    local baseline="$2"
    local target
    local name
    local shadow_reference
    local production_reference
    local shadow_raw
    local production_raw
    local shadow_normalized
    local production_normalized
    local expected_platforms
    local actual_platforms
    local shadow_config
    local production_config
    local shadow_auxiliary
    local production_auxiliary
    local platform
    local shadow_platform_digest
    local production_platform_digest
    local shadow_payload
    local production_payload
    local shadow_binary_hash
    local production_binary_hash
    local platform_slug
    local locked_digest

    if [[ -n "${PRODUCTION_IMAGE_LOCK}" ]]; then
        [[ -f "${PRODUCTION_IMAGE_LOCK}" ]] ||
            fail "production image lock not found: ${PRODUCTION_IMAGE_LOCK}"
        assert_json_equal \
            "$(jq -c '[.[].name] | sort' "${PRODUCTION_IMAGE_LOCK}")" \
            "$(jq -c '[
                .images[] | select(.category == "production") | .name
            ] | sort' "${TARGETS_FILE}")" \
            "production image lock names"
    fi

    while IFS= read -r target; do
        name="$(jq -r '.name' <<<"${target}")"
        shadow_reference="${SHADOW_REGISTRY}/${name}:${VERSION}"
        if [[ -n "${PRODUCTION_IMAGE_LOCK}" ]]; then
            locked_digest="$(jq -er --arg name "${name}" '
                .[]
                | select(.name == $name)
                | .digest
                | select(test("^sha256:[0-9a-f]{64}$"))
            ' "${PRODUCTION_IMAGE_LOCK}")" ||
                fail "production image lock is invalid for ${name}"
            production_reference="${PRODUCTION_REGISTRY}/${name}@${locked_digest}"
        else
            production_reference="${PRODUCTION_REGISTRY}/${name}:${CHANNEL}"
        fi
        shadow_raw="${TEMP_DIR}/shadow-${name}-raw.json"
        production_raw="${TEMP_DIR}/production-${name}-raw.json"
        shadow_normalized="${TEMP_DIR}/shadow-${name}.json"
        production_normalized="${TEMP_DIR}/production-${name}.json"

        inspect_image shadow "${name}" "${shadow_reference}" "${shadow_raw}"
        inspect_image production "${name}" "${production_reference}" \
            "${production_raw}"
        normalize_image "${shadow_raw}" "${shadow_normalized}"
        normalize_image "${production_raw}" "${production_normalized}"
        if [[ -n "${PRODUCTION_IMAGE_LOCK}" ]]; then
            [[ "$(jq -r '.digest' "${production_normalized}")" == "${locked_digest}" ]] ||
                fail "production image digest does not match the lock for ${name}"
        fi

        expected_platforms="$(jq -c '.requiredPlatforms | sort' <<<"${target}")"
        actual_platforms="$(jq -c '[.platforms[].platform] | sort' \
            "${shadow_normalized}")"
        assert_json_equal "${actual_platforms}" "${expected_platforms}" \
            "shadow platforms for ${name}"
        assert_json_equal \
            "$(jq -c '[.platforms[].platform] | sort' \
                "${production_normalized}")" \
            "${expected_platforms}" \
            "production platforms for ${name}"

        shadow_config="$(jq -cS '[.platforms[] | {platform, config}]' \
            "${shadow_normalized}")"
        production_config="$(jq -cS '[.platforms[] | {platform, config}]' \
            "${production_normalized}")"
        assert_json_equal "${shadow_config}" "${production_config}" \
            "runtime image configuration for ${name}"

        shadow_auxiliary="$(jq -cS '.auxiliary' "${shadow_normalized}")"
        production_auxiliary="$(jq -cS '.auxiliary' \
            "${production_normalized}")"
        assert_json_equal "${shadow_auxiliary}" "${production_auxiliary}" \
            "auxiliary manifest shape for ${name}"
        verify_image_baseline_contract "${baseline}" "${name}" \
            "${production_normalized}"

        while IFS= read -r platform; do
            platform_slug="$(tr '/' '_' <<<"${platform}")"
            shadow_platform_digest="$(jq -r --arg platform "${platform}" '
                .platforms[]
                | select(.platform == $platform)
                | .digest
            ' "${shadow_normalized}")"
            production_platform_digest="$(jq -r --arg platform "${platform}" '
                .platforms[]
                | select(.platform == $platform)
                | .digest
            ' "${production_normalized}")"
            shadow_payload="${TEMP_DIR}/shadow-${name}-${platform_slug}.payload"
            production_payload="${TEMP_DIR}/production-${name}-${platform_slug}.payload"
            shadow_binary_hash="${TEMP_DIR}/shadow-${name}-${platform_slug}.binary.sha256"
            production_binary_hash="${TEMP_DIR}/production-${name}-${platform_slug}.binary.sha256"

            capture_payload shadow "${name}" \
                "${SHADOW_REGISTRY}/${name}" "${shadow_platform_digest}" \
                "${platform}" "${shadow_payload}" "${shadow_binary_hash}"
            capture_payload production "${name}" \
                "${PRODUCTION_REGISTRY}/${name}" \
                "${production_platform_digest}" "${platform}" \
                "${production_payload}" "${production_binary_hash}"
            cmp -s "${shadow_payload}" "${production_payload}" ||
                fail "runtime payload mismatch for ${name} ${platform}"
            cmp -s "${shadow_binary_hash}" "${production_binary_hash}" ||
                fail "embedded server binary mismatch for ${name} ${platform}"
        done < <(jq -r '.requiredPlatforms[]' <<<"${target}")

        jq -n \
            --arg name "${name}" \
            --arg productionReference "${production_reference}" \
            --arg productionDigest "$(jq -r '.digest' \
                "${production_normalized}")" \
            --arg shadowReference "${shadow_reference}" \
            --arg shadowDigest "$(jq -r '.digest' \
                "${shadow_normalized}")" '
            {
                name: $name,
                productionReference: $productionReference,
                productionDigest: $productionDigest,
                shadowReference: $shadowReference,
                shadowDigest: $shadowDigest
            }
        ' >>"${entries_file}"
    done < <(jq -c '
        .images[] | select(.category == "production")
    ' "${TARGETS_FILE}")
}

write_report() {
    local baseline="$1"
    local cli_entries="$2"
    local image_entries="$3"

    mkdir -p "$(dirname "${REPORT_PATH}")"
    jq -S -n \
        --arg baselineVersion "$(jq -r '.release.version' "${baseline}")" \
        --arg version "${VERSION}" \
        --arg channel "${CHANNEL}" \
        --arg commit "${SOURCE_COMMIT}" \
        --arg shadowRegistry "${SHADOW_REGISTRY}" \
        --slurpfile cli "${cli_entries}" \
        --slurpfile images "${image_entries}" '
        {
            schemaVersion: 1,
            baselineVersion: $baselineVersion,
            shadowVersion: $version,
            productionChannel: $channel,
            sourceCommit: $commit,
            shadowRegistry: $shadowRegistry,
            checks: {
                cliAssetNames: "match",
                cliBinaryDigests: "match",
                binaryMetadata: "match",
                imageNamesAndPlatforms: "match",
                imageRuntimeConfiguration: "match",
                baselineRuntimeContract: "match",
                imageRuntimeFilesystem: "match",
                embeddedServerBinaries: "match",
                scmRelease: "disabled"
            },
            knownDifferences: [{
                path: "cli.assets[].checksum.content",
                production: "<sha256> *<asset>",
                shadow: "<sha256>",
                reason: "GoReleaser v2 native split-checksum format; sidecar names and ChecksumOf metadata preserve the target association."
            }],
            cliAssets: ($cli | sort_by(.name)),
            images: ($images | sort_by(.name))
        }
    ' >"${REPORT_PATH}"
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --shadow-dir)
                SHADOW_DIR="$2"
                shift 2
                ;;
            --production-dir)
                PRODUCTION_DIR="$2"
                shift 2
                ;;
            --shadow-registry)
                SHADOW_REGISTRY="$2"
                shift 2
                ;;
            --production-registry)
                PRODUCTION_REGISTRY="$2"
                shift 2
                ;;
            --version)
                VERSION="$2"
                shift 2
                ;;
            --channel)
                CHANNEL="$2"
                shift 2
                ;;
            --chart-version)
                CHART_VERSION_VALUE="$2"
                shift 2
                ;;
            --commit)
                SOURCE_COMMIT="$2"
                shift 2
                ;;
            --report)
                REPORT_PATH="$2"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown argument: $1" ;;
        esac
    done
}

main() {
    local baseline
    local cli_entries
    local image_entries

    parse_args "$@"
    require_command jq
    if [[ -z "${BUILD_INFO_DIR}" ]]; then
        require_command go
    fi
    if [[ -z "${SHADOW_IMAGE_DATA_DIR}" ||
        -z "${PRODUCTION_IMAGE_DATA_DIR}" ||
        -z "${SHADOW_PAYLOAD_DIR}" ||
        -z "${PRODUCTION_PAYLOAD_DIR}" ]]; then
        require_command docker
        require_command tar
        require_command go
        [[ -d "${REPO_ROOT}/.github/scripts/image-payload-manifest" ]] ||
            fail "payload manifest program not found"
    fi

    [[ -n "${VERSION}" ]] || fail "release version is required"
    [[ -n "${CHANNEL}" ]] || fail "release channel is required"
    [[ -n "${CHART_VERSION_VALUE}" ]] || fail "chart version is required"
    [[ "${SOURCE_COMMIT}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "source commit must be a full Git SHA"
    [[ -f "${TARGETS_FILE}" ]] || fail "targets file not found: ${TARGETS_FILE}"
    [[ -f "${SHADOW_DIR}/artifacts.json" ]] ||
        fail "shadow artifacts.json not found"
    [[ -d "${PRODUCTION_DIR}" ]] ||
        fail "production artifact directory not found: ${PRODUCTION_DIR}"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/goreleaser-shadow-XXXXXX")"
    baseline="${TEMP_DIR}/baseline.json"
    cli_entries="${TEMP_DIR}/cli.jsonl"
    image_entries="${TEMP_DIR}/images.jsonl"
    : >"${cli_entries}"
    : >"${image_entries}"

    select_baseline "${baseline}"
    verify_contract "${baseline}"
    verify_cli_artifacts "${cli_entries}" "${baseline}"
    verify_images "${image_entries}" "${baseline}"
    write_report "${baseline}" "${cli_entries}" "${image_entries}"

    echo "GoReleaser shadow output matches the production parity contract"
    echo "Parity report: ${REPORT_PATH}"
}

main "$@"
