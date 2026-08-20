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
readonly DEFAULT_TARGETS="${REPO_ROOT}/.github/release-parity/targets.json"

TARGETS_FILE="${RELEASE_PARITY_TARGETS:-${DEFAULT_TARGETS}}"
OBSERVED_AT="${RELEASE_PARITY_OBSERVED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
IMAGE_DATA_DIR="${RELEASE_PARITY_IMAGE_DATA_DIR:-}"
VERSION=""
OUTPUT_PATH=""
TEMP_DIR=""

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}

trap cleanup EXIT

usage() {
    echo "Usage: $0 --version <version> --output <path>"
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

find_python() {
    if command -v python3 >/dev/null 2>&1; then
        command -v python3
    elif command -v python >/dev/null 2>&1; then
        command -v python
    else
        fail "required command not found: python3 or python"
    fi
}

sha256_file() {
    sha256sum "$1" | cut -d ' ' -f 1
}

sha256_text_file() {
    sha256_file "$1"
}

resolve_tag_commit() {
    local repository="$1"
    local tag="$2"
    local response_file="${TEMP_DIR}/tag-response.json"
    local object_type
    local object_sha
    local attempts=0

    gh api "/repos/${repository}/git/ref/tags/${tag}" >"${response_file}"
    object_type="$(jq -r '.object.type' "${response_file}")"
    object_sha="$(jq -r '.object.sha' "${response_file}")"

    while [[ "${object_type}" == "tag" ]]; do
        ((attempts += 1))
        ((attempts <= 5)) || fail "tag ${tag} has too many indirections"
        gh api "/repos/${repository}/git/tags/${object_sha}" \
            >"${response_file}"
        object_type="$(jq -r '.object.type' "${response_file}")"
        object_sha="$(jq -r '.object.sha' "${response_file}")"
    done

    [[ "${object_type}" == "commit" ]] ||
        fail "tag ${tag} resolves to ${object_type}, not a commit"
    printf '%s' "${object_sha}"
}

extract_build_info() {
    local binary_path="$1"
    local output_path="$2"

    go version -m -json "${binary_path}" |
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
                module: {
                    path: ($build.Main.Path // ""),
                    version: ($build.Main.Version // ""),
                    sum: ($build.Main.Sum // "")
                },
                settings: ($settings | with_entries(select(
                    .key == "-buildmode"
                    or .key == "-compiler"
                    or .key == "CGO_ENABLED"
                    or .key == "GOOS"
                    or .key == "GOARCH"
                    or .key == "GOARM"
                    or .key == "GOAMD64"
                    or .key == "vcs"
                    or .key == "vcs.revision"
                    or .key == "vcs.time"
                    or .key == "vcs.modified"
                ))),
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
        ' >"${output_path}"
}

collect_cli_assets() {
    local release_json="$1"
    local source_commit="$2"
    local output_path="$3"
    local assets_dir="${TEMP_DIR}/assets"
    local entries_file="${TEMP_DIR}/cli-entries.jsonl"
    local asset
    local name
    local os
    local arch
    local binary_path
    local sidecar_path
    local actual_sha
    local declared_sha
    local declared_name
    local checksum_line
    local api_digest
    local content_type
    local checksum_api_digest
    local checksum_content_type
    local checksum_size
    local build_info_path
    local expected_assets
    local actual_assets

    mkdir -p "${assets_dir}"
    expected_assets="$(
        jq -c '[
            .cliAssets[]
            | .name, (.name + ".sha256")
        ] | sort' "${TARGETS_FILE}"
    )"
    actual_assets="$(jq -c '[.assets[].name] | sort' "${release_json}")"
    [[ "${actual_assets}" == "${expected_assets}" ]] ||
        fail "GitHub release assets do not match the expected set"

    gh release download "v${VERSION}" \
        --repo "$(jq -r '.repository' "${TARGETS_FILE}")" \
        --dir "${assets_dir}"
    : >"${entries_file}"

    while IFS= read -r asset; do
        name="$(jq -r '.name' <<<"${asset}")"
        os="$(jq -r '.os' <<<"${asset}")"
        arch="$(jq -r '.arch' <<<"${asset}")"
        binary_path="${assets_dir}/${name}"
        sidecar_path="${binary_path}.sha256"

        [[ -f "${binary_path}" ]] || fail "missing release asset: ${name}"
        [[ -f "${sidecar_path}" ]] ||
            fail "missing checksum asset: ${name}.sha256"

        checksum_line="$(tr -d '\r\n' <"${sidecar_path}")"
        if [[ ! "${checksum_line}" =~ ^([0-9a-f]{64})\ \*(.+)$ ]]; then
            fail "invalid checksum format for ${name}.sha256"
        fi
        declared_sha="${BASH_REMATCH[1]}"
        declared_name="${BASH_REMATCH[2]}"
        [[ "${declared_name}" == "${name}" ]] ||
            fail "checksum asset ${name}.sha256 names ${declared_name}"

        actual_sha="$(sha256_file "${binary_path}")"
        [[ "${declared_sha}" == "${actual_sha}" ]] ||
            fail "checksum mismatch for ${name}"

        api_digest="$(
            jq -r --arg name "${name}" \
                '.assets[] | select(.name == $name) | .digest // ""' \
                "${release_json}"
        )"
        content_type="$(
            jq -r --arg name "${name}" '
                .assets[]
                | select(.name == $name)
                | .content_type // ""
            ' "${release_json}"
        )"
        checksum_api_digest="$(
            jq -r --arg name "${name}.sha256" '
                .assets[]
                | select(.name == $name)
                | .digest // ""
            ' "${release_json}"
        )"
        checksum_content_type="$(
            jq -r --arg name "${name}.sha256" '
                .assets[]
                | select(.name == $name)
                | .content_type // ""
            ' "${release_json}"
        )"
        checksum_size="$(wc -c <"${sidecar_path}")"
        if [[ -n "${api_digest}" ]]; then
            [[ "${api_digest}" == "sha256:${actual_sha}" ]] ||
                fail "GitHub digest mismatch for ${name}"
        fi
        if [[ -n "${checksum_api_digest}" ]]; then
            [[ "${checksum_api_digest}" == \
                "sha256:$(sha256_file "${sidecar_path}")" ]] ||
                fail "GitHub digest mismatch for ${name}.sha256"
        fi

        build_info_path="${TEMP_DIR}/build-${os}-${arch}.json"
        extract_build_info "${binary_path}" "${build_info_path}"
        jq -e --arg commit "${source_commit}" \
            '.linkerMetadata.commit == $commit' "${build_info_path}" \
            >/dev/null || fail "linker commit mismatch for ${name}"

        jq -n \
            --arg name "${name}" \
            --arg os "${os}" \
            --arg arch "${arch}" \
            --arg sha256 "${actual_sha}" \
            --arg checksum_name "${name}.sha256" \
            --arg checksum_sha256 "$(sha256_file "${sidecar_path}")" \
            --arg github_digest "${api_digest}" \
            --arg content_type "${content_type}" \
            --arg checksum_github_digest "${checksum_api_digest}" \
            --arg checksum_content_type "${checksum_content_type}" \
            --argjson size "$(wc -c <"${binary_path}")" \
            --argjson checksum_size "${checksum_size}" \
            --slurpfile build "${build_info_path}" \
            '{
                name: $name,
                os: $os,
                arch: $arch,
                size: $size,
                sha256: $sha256,
                githubDigest: $github_digest,
                contentType: $content_type,
                checksum: {
                    name: $checksum_name,
                    format: "sha256sum-binary",
                    size: $checksum_size,
                    sha256: $checksum_sha256,
                    githubDigest: $checksum_github_digest,
                    contentType: $checksum_content_type,
                    declaredSha256: $sha256,
                    target: $name,
                    valid: true
                },
                build: $build[0]
            }' >>"${entries_file}"
    done < <(jq -c '.cliAssets[]' "${TARGETS_FILE}")

    jq -s 'sort_by(.name)' "${entries_file}" >"${output_path}"
}

collect_release_notes() {
    local repository="$1"
    local release_json="$2"
    local source_commit="$3"
    local output_path="$4"
    local body_path="${TEMP_DIR}/release-body.md"
    local source_path="${TEMP_DIR}/release-note-source.md"
    local prerelease
    local source_type
    local repository_path=""
    local source_sha=""
    local matches_source="false"

    jq -j '.body // ""' "${release_json}" >"${body_path}"
    prerelease="$(jq -r '.prerelease' "${release_json}")"

    if [[ "${prerelease}" == "true" ]]; then
        if grep -q 'Release notes generated using configuration' "${body_path}"; then
            source_type="github-generated"
        else
            source_type="github-release-body"
        fi
    else
        source_type="repository-file"
        repository_path="docs/release-notes/v${VERSION}.md"
        gh api \
            -H "Accept: application/vnd.github.raw+json" \
            "/repos/${repository}/contents/${repository_path}?ref=${source_commit}" \
            >"${source_path}"
        source_sha="$(sha256_text_file "${source_path}")"
        if cmp -s "${body_path}" "${source_path}"; then
            matches_source="true"
        fi
    fi

    jq -n \
        --arg type "${source_type}" \
        --arg path "${repository_path}" \
        --arg body_sha "$(sha256_text_file "${body_path}")" \
        --arg source_sha "${source_sha}" \
        --argjson matches_source "${matches_source}" \
        '{
            source: {
                type: $type,
                path: $path,
                sha256: $source_sha
            },
            bodySha256: $body_sha,
            matchesSource: $matches_source
        }' >"${output_path}"
}

normalize_image() {
    local source_path="$1"
    local reference="$2"
    local name="$3"
    local category="$4"
    local output_path="$5"

    jq -S \
        --arg reference "${reference}" \
        --arg name "${name}" \
        --arg category "${category}" '
        def platform_name($platform):
            $platform.os + "/" + $platform.architecture
            + (if ($platform.variant // "") == ""
                then ""
                else "/" + $platform.variant
            end);
        def normalized_config($platform; $digest; $image):
            {
                platform: $platform,
                digest: $digest,
                config: {
                    user: ($image.config.User // ""),
                    entrypoint: ($image.config.Entrypoint // []),
                    command: ($image.config.Cmd // []),
                    workingDirectory: ($image.config.WorkingDir // ""),
                    environment: ($image.config.Env // []),
                    labels: ($image.config.Labels // {})
                }
            };
        if (.manifest.manifests // null) != null then
            . as $root
            | [
                $root.manifest.manifests[]
                | select(.platform.os != "unknown")
                | . as $manifest
                | (platform_name($manifest.platform)) as $platform
                | normalized_config(
                    $platform;
                    $manifest.digest;
                    $root.image[$platform]
                )
            ] as $platforms
            | [
                $root.manifest.manifests[]
                | select(.platform.os == "unknown")
                | {
                    digest,
                    mediaType,
                    size,
                    annotations: (.annotations // {}),
                    platform: (platform_name(.platform))
                }
            ] as $auxiliary
            | {
                name: $name,
                category: $category,
                reference: $reference,
                digest: $root.manifest.digest,
                mediaType: $root.manifest.mediaType,
                platforms: ($platforms | sort_by(.platform)),
                auxiliaryManifests: ($auxiliary | sort_by(.digest))
            }
        else
            . as $root
            | (platform_name($root.image)) as $platform
            | {
                name: $name,
                category: $category,
                reference: $reference,
                digest: $root.manifest.digest,
                mediaType: $root.manifest.mediaType,
                platforms: [
                    normalized_config(
                        $platform;
                        $root.manifest.digest;
                        $root.image
                    )
                ],
                auxiliaryManifests: []
            }
        end
    ' "${source_path}" >"${output_path}"
}

collect_images() {
    local channel="$1"
    local source_commit="$2"
    local output_path="$3"
    local entries_file="${TEMP_DIR}/image-entries.jsonl"
    local registry
    local target
    local name
    local category
    local radius_build
    local reference
    local raw_path
    local normalized_path
    local expected_platforms

    registry="$(jq -r '.imageRegistry' "${TARGETS_FILE}")"
    : >"${entries_file}"

    while IFS= read -r target; do
        name="$(jq -r '.name' <<<"${target}")"
        category="$(jq -r '.category' <<<"${target}")"
        radius_build="$(jq -r '.radiusBuild' <<<"${target}")"
        reference="${registry}/${name}:${channel}"
        raw_path="${TEMP_DIR}/image-${name}-raw.json"
        normalized_path="${TEMP_DIR}/image-${name}.json"
        expected_platforms="$(
            jq -c '.requiredPlatforms | sort' <<<"${target}"
        )"

        if [[ -n "${IMAGE_DATA_DIR}" ]]; then
            [[ -f "${IMAGE_DATA_DIR}/${name}.json" ]] ||
                fail "missing image data for ${name}"
            cp "${IMAGE_DATA_DIR}/${name}.json" "${raw_path}"
        else
            docker buildx imagetools inspect --format '{{json .}}' \
                "${reference}" >"${raw_path}"
        fi
        normalize_image "${raw_path}" "${reference}" "${name}" \
            "${category}" "${normalized_path}"

        jq -e --argjson expected "${expected_platforms}" '
            ([.platforms[].platform] | sort) == $expected
        ' "${normalized_path}" >/dev/null ||
            fail "unexpected platform set for ${reference}"

        if [[ "${radius_build}" == "true" ]]; then
            jq -e \
                --arg name "${name}" \
                --arg version "${VERSION}" \
                --arg commit "${source_commit}" '
                all(.platforms[];
                    .config.labels[
                        "org.opencontainers.image.description"
                    ] == $name
                    and .config.labels[
                        "org.opencontainers.image.source"
                    ] == "https://github.com/radius-project/radius"
                    and .config.labels[
                        "org.opencontainers.image.version"
                    ] == $version
                    and .config.labels[
                        "org.opencontainers.image.revision"
                    ] == $commit
                )
            ' "${normalized_path}" >/dev/null ||
                fail "Radius image labels do not match ${reference}"
        fi

        cat "${normalized_path}" >>"${entries_file}"
    done < <(jq -c '.images[]' "${TARGETS_FILE}")

    jq -s 'sort_by(.name)' "${entries_file}" >"${output_path}"
}

collect_helm_chart() {
    local channel="$1"
    local output_path="$2"
    local oci_reference
    local oras_repository
    local oras_reference
    local descriptor_path="${TEMP_DIR}/helm-descriptor.json"
    local manifest_path="${TEMP_DIR}/helm-manifest.json"
    local metadata_yaml="${TEMP_DIR}/helm-metadata.yaml"
    local metadata_json="${TEMP_DIR}/helm-metadata.json"
    local rendered_yaml="${TEMP_DIR}/helm-rendered.yaml"
    local rendered_json="${TEMP_DIR}/helm-rendered.json"
    local rendered_images="${TEMP_DIR}/helm-images.json"
    local expected_images

    oci_reference="$(jq -r '.helm.ociReference' "${TARGETS_FILE}")"
    oras_repository="$(jq -r '.helm.orasReference' "${TARGETS_FILE}")"
    oras_reference="${oras_repository}:${VERSION}"

    oras manifest fetch --descriptor "${oras_reference}" \
        >"${descriptor_path}"
    oras manifest fetch "${oras_reference}" >"${manifest_path}"
    helm show chart "${oci_reference}" --version "${VERSION}" \
        >"${metadata_yaml}"
    yq eval -o=json '.' "${metadata_yaml}" >"${metadata_json}"

    jq -e --arg version "${VERSION}" '
        .name == "radius"
        and .version == $version
        and .appVersion == $version
    ' "${metadata_json}" >/dev/null ||
        fail "Helm metadata does not match release ${VERSION}"

    helm template radius "${oci_reference}" \
        --version "${VERSION}" \
        --namespace radius-system \
        --set preupgrade.enabled=true \
        --set-string "preupgrade.targetVersion=${VERSION}" \
        >"${rendered_yaml}"
    yq eval-all -o=json '[.]' "${rendered_yaml}" >"${rendered_json}"
    jq -S '[
        .[]
        | ..
        | objects
        | .image? // empty
        | select(type == "string")
    ] | unique' "${rendered_json}" >"${rendered_images}"

    expected_images="$(jq -c '.helm.expectedImages | sort' "${TARGETS_FILE}")"
    jq -e --argjson expected "${expected_images}" '
        [
            .[]
            | split("/")[-1]
            | split("@")[0]
            | split(":")[0]
        ]
        | unique
        | sort
        | . == $expected
    ' "${rendered_images}" >/dev/null ||
        fail "Helm chart rendered an unexpected image set"

    jq -S -n \
        --arg reference "${oras_reference}" \
        --arg channel "${channel}" \
        --slurpfile descriptor "${descriptor_path}" \
        --slurpfile manifest "${manifest_path}" \
        --slurpfile metadata "${metadata_json}" \
        --slurpfile images "${rendered_images}" '
        {
            reference: $reference,
            releaseChannel: $channel,
            descriptor: $descriptor[0],
            manifest: $manifest[0],
            metadata: $metadata[0],
            renderedImages: $images[0]
        }
    ' >"${output_path}"
}

collect_sibling_repositories() {
    local tag="$1"
    local output_path="$2"
    local entries_file="${TEMP_DIR}/repository-entries.jsonl"
    local repository
    local commit

    : >"${entries_file}"
    while IFS= read -r repository; do
        commit="$(resolve_tag_commit "${repository}" "${tag}")"
        jq -n \
            --arg repository "${repository}" \
            --arg tag "${tag}" \
            --arg commit "${commit}" \
            '{repository: $repository, tag: $tag, commit: $commit}' \
            >>"${entries_file}"
    done < <(jq -r '.siblingRepositories[]' "${TARGETS_FILE}")

    jq -s 'sort_by(.repository)' "${entries_file}" >"${output_path}"
}

collect_oci_artifacts() {
    local channel="$1"
    local output_path="$2"
    local entries_file="${TEMP_DIR}/oci-artifact-entries.jsonl"
    local target
    local name
    local repository
    local expected_type
    local reference
    local descriptor_path
    local manifest_path

    : >"${entries_file}"
    while IFS= read -r target; do
        name="$(jq -r '.name' <<<"${target}")"
        repository="$(jq -r '.repository' <<<"${target}")"
        expected_type="$(jq -r '.artifactType' <<<"${target}")"
        reference="${repository}:${channel}"
        descriptor_path="${TEMP_DIR}/${name}-descriptor.json"
        manifest_path="${TEMP_DIR}/${name}-manifest.json"

        oras manifest fetch --descriptor "${reference}" \
            >"${descriptor_path}"
        oras manifest fetch "${reference}" >"${manifest_path}"
        jq -e --arg expected "${expected_type}" \
            '.artifactType == $expected' "${manifest_path}" >/dev/null ||
            fail "unexpected artifact type for ${reference}"

        jq -S -n \
            --arg name "${name}" \
            --arg reference "${reference}" \
            --slurpfile descriptor "${descriptor_path}" \
            --slurpfile manifest "${manifest_path}" '
            {
                name: $name,
                reference: $reference,
                descriptor: $descriptor[0],
                manifest: $manifest[0]
            }
        ' >>"${entries_file}"
    done < <(jq -c '.ociArtifacts[]' "${TARGETS_FILE}")

    jq -s 'sort_by(.name)' "${entries_file}" >"${output_path}"
}

main() {
    local repository
    local tag
    local release_json
    local source_commit
    local cli_assets_json
    local release_notes_json
    local runtime_version_json
    local linux_amd64_path
    local python_command
    local channel
    local images_json
    local helm_json
    local sibling_repositories_json
    local oci_artifacts_json

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --version)
                VERSION="${2:-}"
                shift 2
                ;;
            --output)
                OUTPUT_PATH="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                return 0
                ;;
            *)
                fail "unknown argument: $1"
                ;;
        esac
    done

    [[ -n "${VERSION}" ]] || fail "--version is required"
    [[ -n "${OUTPUT_PATH}" ]] || fail "--output is required"
    VERSION="${VERSION#v}"

    require_command gh
    require_command go
    require_command jq
    if [[ -z "${IMAGE_DATA_DIR}" ]]; then
        require_command docker
    fi
    require_command helm
    require_command oras
    require_command sha256sum
    require_command yq
    [[ -f "${TARGETS_FILE}" ]] || fail "targets file not found: ${TARGETS_FILE}"
    python_command="$(find_python)"
    "${python_command}" "${REPO_ROOT}/.github/scripts/validate_semver.py" \
        "${VERSION}"

    TEMP_DIR="$(mktemp -d)"
    repository="$(jq -r '.repository' "${TARGETS_FILE}")"
    tag="v${VERSION}"
    release_json="${TEMP_DIR}/release.json"
    cli_assets_json="${TEMP_DIR}/cli-assets.json"
    release_notes_json="${TEMP_DIR}/release-notes.json"
    runtime_version_json="${TEMP_DIR}/runtime-version.json"
    images_json="${TEMP_DIR}/images.json"
    helm_json="${TEMP_DIR}/helm.json"
    sibling_repositories_json="${TEMP_DIR}/sibling-repositories.json"
    oci_artifacts_json="${TEMP_DIR}/oci-artifacts.json"

    gh api "/repos/${repository}/releases/tags/${tag}" >"${release_json}"
    jq -e '.draft == false' "${release_json}" >/dev/null ||
        fail "release ${tag} is still a draft"
    source_commit="$(resolve_tag_commit "${repository}" "${tag}")"

    collect_cli_assets "${release_json}" "${source_commit}" \
        "${cli_assets_json}"
    collect_release_notes "${repository}" "${release_json}" \
        "${source_commit}" "${release_notes_json}"

    linux_amd64_path="${TEMP_DIR}/assets/rad_linux_amd64"
    [[ -f "${linux_amd64_path}" ]] ||
        fail "rad_linux_amd64 is required for runtime metadata"
    chmod +x "${linux_amd64_path}"
    "${linux_amd64_path}" version --cli -o json >"${runtime_version_json}"
    jq -e --arg commit "${source_commit}" '.commit == $commit' \
        "${runtime_version_json}" >/dev/null ||
        fail "runtime version commit does not match release tag"
    channel="$(
        jq -r '.[0].build.linkerMetadata.channel' "${cli_assets_json}"
    )"
    [[ -n "${channel}" ]] || fail "release channel is missing from CLI metadata"

    collect_images "${channel}" "${source_commit}" "${images_json}"
    collect_helm_chart "${channel}" "${helm_json}"
    collect_sibling_repositories "${tag}" "${sibling_repositories_json}"
    collect_oci_artifacts "${channel}" "${oci_artifacts_json}"

    mkdir -p "$(dirname "${OUTPUT_PATH}")"
    jq -S -n \
        --arg observed_at "${OBSERVED_AT}" \
        --arg repository "${repository}" \
        --arg tag "${tag}" \
        --arg version "${VERSION}" \
        --arg source_commit "${source_commit}" \
        --arg title "$(jq -r '.name // ""' "${release_json}")" \
        --arg published_at "$(jq -r '.published_at // ""' "${release_json}")" \
        --arg html_url "$(jq -r '.html_url // ""' "${release_json}")" \
        --argjson prerelease "$(jq -r '.prerelease' "${release_json}")" \
        --slurpfile notes "${release_notes_json}" \
        --slurpfile assets "${cli_assets_json}" \
        --slurpfile runtime "${runtime_version_json}" \
        --slurpfile images "${images_json}" \
        --slurpfile helm "${helm_json}" \
        --slurpfile repositories "${sibling_repositories_json}" \
        --slurpfile oci_artifacts "${oci_artifacts_json}" \
        '{
            schemaVersion: 1,
            observedAt: $observed_at,
            release: {
                repository: $repository,
                tag: $tag,
                version: $version,
                sourceCommit: $source_commit,
                title: $title,
                publishedAt: $published_at,
                htmlUrl: $html_url,
                draft: false,
                prerelease: $prerelease,
                notes: $notes[0]
            },
            cli: {
                assets: $assets[0],
                runtimeVersion: $runtime[0]
            },
            images: $images[0],
            helm: $helm[0],
            downstream: {
                repositories: $repositories[0],
                ociArtifacts: $oci_artifacts[0]
            }
        }' >"${OUTPUT_PATH}"

    echo "Wrote release parity manifest to ${OUTPUT_PATH}"
}

main "$@"
