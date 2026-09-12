#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly DIST_DIR="${GORELEASER_DIST_DIR:-dist/goreleaser}"
readonly TARGETS="${RELEASE_PARITY_TARGETS:-${SCRIPT_DIR}/../release-parity/targets.json}"
readonly REGISTRY="${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
readonly TAG="${DOCKER_TAG_VERSION:?DOCKER_TAG_VERSION is required}"
readonly IMAGES_DIR="${SNAPSHOT_IMAGES_DIR:-dist/images}"
readonly MODE="${1:?Expected save or push-edge}"

case "${MODE}" in
    save) ;;
    push-edge)
        [[ "${TAG}" == edge ]] || {
            echo "push-edge publishes only the edge tag" >&2
            exit 1
        }
        ;;
    *)
        echo "Expected save or push-edge" >&2
        exit 1
        ;;
esac

version="$(jq -er '.version' "${DIST_DIR}/metadata.json")"
[[ "${version}" =~ ^0\.0\.0-snapshot-[a-f0-9]+$ ]] || {
    echo "Expected GoReleaser snapshot metadata" >&2
    exit 1
}

jq -e --slurpfile targets "${TARGETS}" --arg registry "${REGISTRY}" --arg version "${version}" '
    [.[] | select(.type == "Docker Image")] as $images |
    [$targets[0].images[] | select(.category == "production") |
        .name as $id | .requiredPlatforms[] | {id: $id, platform: .}] as $expected |
    ($images | length > 0) and
    ([$images[] | {id: .extra.ID, platform: .extra.Platforms[0]}] | sort) == ($expected | sort) and
    all($images[]; . as $image |
        (.extra.Platforms | length == 1) and .name == .path and
        (.name | startswith($registry + "/" + $image.extra.ID + ":" + $version + "-")))
' "${DIST_DIR}/artifacts.json" >/dev/null

mkdir -p "${IMAGES_DIR}"
mapfile -t image_ids < <(jq -r '[.[] | select(.type == "Docker Image") | .extra.ID] | unique[]' "${DIST_DIR}/artifacts.json")
for image_id in "${image_ids[@]}"; do
    mapfile -t images < <(jq -r --arg id "${image_id}" '
        .[] | select(.type == "Docker Image" and .extra.ID == $id) | .path
    ' "${DIST_DIR}/artifacts.json")
    if [[ "${MODE}" == save ]]; then
        native_image="$(jq -er --arg id "${image_id}" '
            .[] | select(.type == "Docker Image" and .extra.ID == $id and .extra.Platforms == ["linux/amd64"]) | .path
        ' "${DIST_DIR}/artifacts.json")"
        docker tag "${native_image}" "${REGISTRY}/${image_id}:${TAG}"
        docker save --output "${IMAGES_DIR}/${image_id}.tar" "${images[@]}" "${REGISTRY}/${image_id}:${TAG}"
    else
        sources=()
        for image in "${images[@]}"; do
            platform="$(jq -er --arg path "${image}" '
                .[] | select(.type == "Docker Image" and .path == $path) | .extra.Platforms[0]
            ' "${DIST_DIR}/artifacts.json")"
            # A moving per-platform tag replaces one set of snapshot-version tags
            # per main build; the edge index references the manifests by digest.
            reference="${REGISTRY}/${image_id}:edge-${platform//\//-}"
            docker tag "${image}" "${reference}"
            docker push "${reference}"
            digest="$(oras resolve "${reference}")"
            [[ "${digest}" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 1
            sources+=("${REGISTRY}/${image_id}@${digest}")
        done
        docker buildx imagetools create --tag "${REGISTRY}/${image_id}:edge" "${sources[@]}"
    fi
done
