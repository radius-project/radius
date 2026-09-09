#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIRECTORY="${RELEASE_VERIFY_DIR:-${ROOT}/dist/verification}"
MODE="${1:-verify}"
VERSION="${REL_VERSION:?REL_VERSION is required}"
SOURCE_SHA="${RELEASE_SOURCE_SHA:-$(git -C "${ROOT}" rev-parse HEAD)}"
PLAN="${ROOT}/.github/release-plans/v${VERSION}.yaml"
[[ "${MODE}" == "verify" || "${MODE}" == "recheck" ]] || exit 1
[[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc\.[1-9][0-9]*)?$ ]] || exit 1
[[ "${SOURCE_SHA}" =~ ^[a-f0-9]{40}$ ]] || exit 1
[[ "$(git -C "${ROOT}" rev-parse HEAD)" == "${SOURCE_SHA}" ]] || exit 1
mkdir -p "${DIRECTORY}"
DIRECTORY="$(cd "${DIRECTORY}" && pwd)"
yq -o=json '.' "${PLAN}" > "${DIRECTORY}/plan.json"
yq -o=json '.' "${ROOT}/versions.yaml" > "${DIRECTORY}/versions.json"
cp "${ROOT}/.github/release-parity/targets.json" "${DIRECTORY}/targets.json"
jq -e --slurpfile targets "${DIRECTORY}/targets.json" \
    '.expectedOutputs == $targets[0]' "${DIRECTORY}/plan.json" > /dev/null
jq -n --arg sourceSha "${SOURCE_SHA}" \
    --arg parentSha "$(git -C "${ROOT}" rev-parse HEAD^)" \
    --arg goVersion "$(go env GOVERSION)" \
    --arg terraformVersion "${TERRAFORM_VERSION:?TERRAFORM_VERSION is required}" \
    '{sourceSha:$sourceSha,parentSha:$parentSha,goVersion:$goVersion,
      terraformVersion:$terraformVersion}' > "${DIRECTORY}/context.json"

RELEASE_PARITY_STAGED=true RELEASE_PARITY_TARGETS="${DIRECTORY}/targets.json" \
    RELEASE_PARITY_ASSETS_DIR="${DIRECTORY}/assets" \
    bash "${ROOT}/.github/scripts/release-parity-manifest.sh" \
    --version "${VERSION}" --output "${DIRECTORY}/observed.json"

lock_files=()
while IFS= read -r name; do
    lock_files+=("${DIRECTORY}/assets/${name}-image-digests.json")
done < <(jq -r '.images[] | select(.radiusBuild and .category != "production") | .name' "${DIRECTORY}/targets.json")
jq -s 'add | sort_by(.name)' \
    "${DIRECTORY}/assets/production-image-digests.json" "${lock_files[@]}" \
    > "${DIRECTORY}/release-image-digests.json"
cp "${DIRECTORY}/assets/release-cli-oci.json" "${DIRECTORY}/release-cli-oci.json"
GORELEASER_PARITY_TARGETS="${DIRECTORY}/targets.json" \
    bash "${ROOT}/.github/scripts/release-oci-artifacts.sh" verify \
    --version "${VERSION}" --source-sha "${SOURCE_SHA}" \
    --image-lock "${DIRECTORY}/release-image-digests.json" \
    --cli-lock "${DIRECTORY}/release-cli-oci.json" --sboms

gh api -H 'Accept: application/vnd.github.raw+json' \
    "repos/radius-project/radius/contents/.github/release-state/deployment-engine.json?ref=automation/release-state-${VERSION}" \
    > "${DIRECTORY}/controller-lock.json"
bash "${ROOT}/.github/scripts/verify-deployment-engine-image.sh" \
    --tag "$(jq -er '.deploymentEngine.imageTag' "${DIRECTORY}/controller-lock.json")" \
    --signed-tag "v${VERSION}" \
    --source-commit "$(jq -er '.deploymentEngine.sourceCommit' "${DIRECTORY}/controller-lock.json")" \
    --expected-digest "$(jq -er '.deploymentEngine.digest' "${DIRECTORY}/controller-lock.json")" \
    --output "${DIRECTORY}/deployment-engine.json"
node "${ROOT}/.github/scripts/verify-release-manifest.mjs" "${DIRECTORY}" "${MODE}"

if [[ "${MODE}" == "verify" ]]; then
    helm pull "$(jq -er '.helm.ociReference' "${DIRECTORY}/targets.json")" \
        --version "${VERSION}" --destination "${DIRECTORY}"
    RELEASE_VERIFY_CLI="${DIRECTORY}/assets/rad_linux_amd64" \
        RELEASE_VERIFY_CHART="${DIRECTORY}/radius-${VERSION}.tgz" \
        RELEASE_VERIFY_MANIFEST="${DIRECTORY}/release-manifest.json" \
        bash "${ROOT}/.github/scripts/release-verification.sh" "${VERSION}"
fi
