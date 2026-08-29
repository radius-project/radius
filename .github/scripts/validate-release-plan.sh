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

BODY_FILE=""
FILES_FILE=""
BASE_SHA=""
HEAD_DIR=""
REPOSITORY=""
TEMP_DIR=""
GENERATED_WORKTREE=""
PREPARE_RELEASE_SCRIPT="${PREPARE_RELEASE_SCRIPT:-}"
COLLECT_BACKPORTS_SCRIPT="${COLLECT_BACKPORTS_SCRIPT:-}"
CAPTURE_SIBLINGS_SCRIPT="${CAPTURE_SIBLINGS_SCRIPT:-}"
if [[ -z "${PREPARE_RELEASE_SCRIPT}" ]]; then
    PREPARE_RELEASE_SCRIPT="${SCRIPT_DIR}/prepare-release.sh"
fi
if [[ -z "${COLLECT_BACKPORTS_SCRIPT}" ]]; then
    COLLECT_BACKPORTS_SCRIPT="${SCRIPT_DIR}/collect-release-backports.sh"
fi
if [[ -z "${CAPTURE_SIBLINGS_SCRIPT}" ]]; then
    CAPTURE_SIBLINGS_SCRIPT="${SCRIPT_DIR}/capture-release-sibling-commits.sh"
fi

cleanup() {
    if [[ -n "${GENERATED_WORKTREE}" && -d "${GENERATED_WORKTREE}" ]]; then
        git worktree remove --force "${GENERATED_WORKTREE}" \
            >/dev/null 2>&1 || true
    fi
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: validate-release-plan.sh --body-file <path> --files-file <path> \
    --base-sha <commit> --head-dir <path> --repository <owner/repo>
EOF
}

plan_value() {
    yq -r "$1" "${TEMP_DIR}/release-plan.yaml"
}

extract_plan() {
    local start_count end_count

    start_count="$(
        grep -Fc '<!-- radius-release-plan:start -->' "${BODY_FILE}"
    )"
    end_count="$(grep -Fc '<!-- radius-release-plan:end -->' "${BODY_FILE}")"
    if [[ "${start_count}" != "1" || "${end_count}" != "1" ]]; then
        fail "pull request body must contain exactly one release plan"
    fi

    awk '
        /<!-- radius-release-plan:start -->/ { active = 1; next }
        /<!-- radius-release-plan:end -->/ { active = 0; exit }
        active && /^```(yaml)?$/ { next }
        active { print }
    ' "${BODY_FILE}" >"${TEMP_DIR}/release-plan.yaml"
    if ! yq -e '.' "${TEMP_DIR}/release-plan.yaml" >/dev/null; then
        fail "release plan is not valid YAML"
    fi
}

validate_files() {
    local version="$1"
    local plan_path="$2"
    local expected="${TEMP_DIR}/expected-files.txt"
    local actual="${TEMP_DIR}/actual-files.txt"

    {
        echo 'CHANGELOG.md'
        echo "${plan_path}"
        echo "docs/release-notes/${version}.md"
        echo 'versions.yaml'
    } | sort >"${expected}"
    jq -e 'type == "array" and all(.[]; type == "string")' \
        "${FILES_FILE}" >/dev/null || fail "changed-files input is invalid"
    jq -r '.[]' "${FILES_FILE}" | tr -d '\r' | sort >"${actual}"
    if ! diff -u "${expected}" "${actual}"; then
        fail "release PR changes files outside the generated contract"
    fi
}

canonical_json() {
    local input="$1"
    local expression="$2"
    local output="$3"

    yq -o=json -I=0 "${expression}" "${input}" >"${output}.raw"
    tr -d '\r' <"${output}.raw" | jq -S -c . >"${output}"
    rm "${output}.raw"
}

normalize_text() {
    tr -d '\r' <"$1" >"$2"
}

normalize_release_notes() {
    local input="$1"
    local output="$2"

    tr -d '\r' <"${input}" | awk '
        /^## Highlights$/ || /^## Upgrading to Radius / {
            print
            print "<!-- curated by maintainer -->"
            curated = 1
            next
        }
        curated && /^## / { curated = 0 }
        curated { next }
        { print }
    ' >"${output}"
}

collect_expected_backports() {
    local channel="$1"
    local output="${TEMP_DIR}/expected-backports.json"

    if [[ -n "${EXPECTED_BACKPORTS_FILE:-}" ]]; then
        cp "${EXPECTED_BACKPORTS_FILE}" "${output}"
    else
        bash "${COLLECT_BACKPORTS_SCRIPT}" --repository "${REPOSITORY}" \
            --channel "${channel}" --output "${output}"
    fi
    printf '%s\n' "${output}"
}

collect_expected_siblings() {
    local channel="$1"
    local output="${TEMP_DIR}/expected-siblings.json"

    if [[ -n "${EXPECTED_SIBLING_REPOSITORIES_FILE:-}" ]]; then
        cp "${EXPECTED_SIBLING_REPOSITORIES_FILE}" "${output}"
    else
        bash "${CAPTURE_SIBLINGS_SCRIPT}" --channel "${channel}" \
            --output "${output}" >/dev/null
    fi
    printf '%s\n' "${output}"
}

regenerate_release() {
    local release_type="$1"
    local channel="$2"
    local release_date="$3"
    local backports_file="$4"
    local siblings_file="$5"
    local output_dir="${TEMP_DIR}/generated-output"

    GENERATED_WORKTREE="${TEMP_DIR}/generated-worktree"
    git worktree add --quiet --detach "${GENERATED_WORKTREE}" "${BASE_SHA}"
    pushd "${GENERATED_WORKTREE}" >/dev/null
    GITHUB_TOKEN="${GITHUB_TOKEN:-}" GITHUB_REPO="${REPOSITORY}" \
        bash "${PREPARE_RELEASE_SCRIPT}" \
        --release-type "${release_type}" --channel "${channel}" \
        --release-date "${release_date}" \
        --backports-file "${backports_file}" \
        --sibling-repositories-file "${siblings_file}" \
        --output-dir "${output_dir}" >/dev/null
    popd >/dev/null
}

validate_generated_contents() {
    local version="$1"
    local release_type="$2"
    local channel="$3"
    local release_date="$4"
    local plan_path="$5"
    local expected_backports expected_siblings expected_notes actual_notes

    expected_backports="$(collect_expected_backports "${channel}")"
    canonical_json "${TEMP_DIR}/release-plan.yaml" '.includedBackports' \
        "${TEMP_DIR}/planned-backports.json"
    jq -S -c . "${expected_backports}" >"${TEMP_DIR}/live-backports.json"
    if ! diff -u "${TEMP_DIR}/live-backports.json" \
        "${TEMP_DIR}/planned-backports.json"; then
        fail "included backports no longer match repository state"
    fi

    expected_siblings="$(collect_expected_siblings "${channel}")"
    canonical_json "${TEMP_DIR}/release-plan.yaml" \
        '.siblingRepositories' "${TEMP_DIR}/planned-siblings.json"
    jq -S -c . "${expected_siblings}" >"${TEMP_DIR}/live-siblings.json"
    if ! diff -u "${TEMP_DIR}/live-siblings.json" \
        "${TEMP_DIR}/planned-siblings.json"; then
        fail "sibling repository commits no longer match live branches"
    fi

    regenerate_release "${release_type}" "${channel}" "${release_date}" \
        "${expected_backports}" "${expected_siblings}"

    canonical_json "${TEMP_DIR}/release-plan.yaml" '.' \
        "${TEMP_DIR}/actual-plan.json"
    canonical_json "${TEMP_DIR}/generated-output/release-plan.yaml" '.' \
        "${TEMP_DIR}/expected-plan.json"
    if ! diff -u "${TEMP_DIR}/expected-plan.json" \
        "${TEMP_DIR}/actual-plan.json"; then
        fail "release plan differs from trusted regeneration"
    fi

    canonical_json "${HEAD_DIR}/${plan_path}" '.' \
        "${TEMP_DIR}/committed-plan.json"
    if ! diff -u "${TEMP_DIR}/actual-plan.json" \
        "${TEMP_DIR}/committed-plan.json"; then
        fail "committed release plan differs from the pull request body"
    fi
    canonical_json "${GENERATED_WORKTREE}/${plan_path}" '.' \
        "${TEMP_DIR}/generated-tracked-plan.json"
    if ! diff -u "${TEMP_DIR}/expected-plan.json" \
        "${TEMP_DIR}/generated-tracked-plan.json"; then
        fail "tracked release plan differs from trusted regeneration"
    fi

    canonical_json "${HEAD_DIR}/versions.yaml" '.' \
        "${TEMP_DIR}/actual-versions.json"
    canonical_json "${GENERATED_WORKTREE}/versions.yaml" '.' \
        "${TEMP_DIR}/expected-versions.json"
    if ! diff -u "${TEMP_DIR}/expected-versions.json" \
        "${TEMP_DIR}/actual-versions.json"; then
        fail "versions.yaml differs from trusted regeneration"
    fi

    normalize_text "${HEAD_DIR}/CHANGELOG.md" \
        "${TEMP_DIR}/actual-changelog.md"
    normalize_text "${GENERATED_WORKTREE}/CHANGELOG.md" \
        "${TEMP_DIR}/expected-changelog.md"
    if ! diff -u "${TEMP_DIR}/expected-changelog.md" \
        "${TEMP_DIR}/actual-changelog.md"; then
        fail "CHANGELOG.md differs from trusted regeneration"
    fi

    expected_notes="${GENERATED_WORKTREE}/docs/release-notes/${version}.md"
    actual_notes="${HEAD_DIR}/docs/release-notes/${version}.md"
    [[ -f "${actual_notes}" ]] || fail "generated release notes are missing"
    normalize_release_notes "${expected_notes}" \
        "${TEMP_DIR}/expected-notes.md"
    normalize_release_notes "${actual_notes}" "${TEMP_DIR}/actual-notes.md"
    if ! diff -u "${TEMP_DIR}/expected-notes.md" \
        "${TEMP_DIR}/actual-notes.md"; then
        fail "release notes differ outside Highlights or Upgrading"
    fi
}

validate_source() {
    local channel="$1"
    local product_ref product_commit release_commit expected_ref actual_commit

    product_ref="$(plan_value '.source.productRef')"
    product_commit="$(plan_value '.source.productCommit')"
    release_commit="$(plan_value '.source.releaseCommit')"
    if [[ "${release_commit}" != "null" ]]; then
        fail "releaseCommit must remain unresolved during preparation"
    fi
    if [[ "$(plan_value '.source.releaseCommitResolution')" == "null" ]]; then
        fail "releaseCommitResolution is required"
    fi

    if [[ "${product_ref}" == "HEAD" ]]; then
        if [[ "${product_commit}" != "${BASE_SHA}" ]]; then
            fail "main advanced; rerun release preparation"
        fi
        return
    fi

    expected_ref="refs/remotes/origin/release/${channel}"
    if [[ "${product_ref}" != "${expected_ref}" ]]; then
        fail "productRef must be HEAD or ${expected_ref}"
    fi
    if ! actual_commit="$(git rev-parse "${expected_ref}^{commit}")"; then
        fail "release/${channel} is not available"
    fi
    if [[ "${actual_commit}" != "${product_commit}" ]]; then
        fail "release/${channel} advanced beyond the approved commit"
    fi
}

validate_policy() {
    local release_type="$1"
    local channel="$2"
    local version="$3"
    local policy_output="${TEMP_DIR}/policy"
    local expected_version

    bash "${SCRIPT_DIR}/prepare-release.sh" \
        --release-type "${release_type}" --channel "${channel}" \
        --output-dir "${policy_output}" --version-only >/dev/null
    expected_version="$(<"${policy_output}/version.txt")"
    if [[ "${version}" != "${expected_version}" ]]; then
        fail "planned version ${version} no longer matches ${expected_version}"
    fi
}

main() {
    local version release_type channel release_date chart_version release_branch
    local release_plan_path canonical_version_pattern expected_repository

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --body-file)
                BODY_FILE="${2:-}"
                shift 2
                ;;
            --files-file)
                FILES_FILE="${2:-}"
                shift 2
                ;;
            --base-sha)
                BASE_SHA="${2:-}"
                shift 2
                ;;
            --head-dir)
                HEAD_DIR="${2:-}"
                shift 2
                ;;
            --repository)
                REPOSITORY="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    [[ -f "${BODY_FILE}" ]] || fail "pull request body file not found"
    [[ -f "${FILES_FILE}" ]] || fail "changed-files file not found"
    [[ -d "${HEAD_DIR}" ]] || fail "pull request head directory not found"
    if [[ ! "${REPOSITORY}" =~ ^[^/]+/[^/]+$ ]]; then
        fail "repository must use owner/name format"
    fi
    if ! git rev-parse --verify --quiet \
        "${BASE_SHA}^{commit}" >/dev/null; then
        fail "base SHA is not a commit"
    fi
    command -v jq >/dev/null || fail "required command not found: jq"
    command -v yq >/dev/null || fail "required command not found: yq"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-plan-test-XXXXXX")"
    extract_plan

    if [[ "$(plan_value '.schemaVersion')" != "2" ]]; then
        fail "unsupported release plan schema"
    fi
    version="$(plan_value '.version')"
    release_type="$(plan_value '.releaseType')"
    channel="$(plan_value '.channel')"
    release_date="$(plan_value '.releaseDate')"
    chart_version="$(plan_value '.chartVersion')"
    release_branch="$(plan_value '.releaseBranch')"
    release_plan_path="$(plan_value '.releasePlanPath')"
    # Numeric identifiers reject leading zeros, matching the semver policy in
    # .github/scripts/release-version.sh.
    canonical_version_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
    canonical_version_pattern+='\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$'
    if [[ ! "${version}" =~ ${canonical_version_pattern} ]]; then
        fail "planned version is not canonical"
    fi
    if [[ ! "${release_type}" =~ ^(rc|final|patch)$ ]]; then
        fail "planned release type is invalid"
    fi
    if [[ ! "${channel}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        fail "planned channel is invalid"
    fi
    if [[ ! "${release_date}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        fail "planned release date is invalid"
    fi
    if [[ "${chart_version}" != "${version#v}" ]]; then
        fail "chart version does not match release version"
    fi
    if [[ "${release_branch}" != "release/${channel}" ]]; then
        fail "release branch does not match the channel"
    fi
    if [[ "${release_plan_path}" != ".github/release-plans/${version}.yaml" ]]; then
        fail "release plan path does not match the version"
    fi
    expected_repository="$(plan_value '.expectedOutputs.repository')"
    if [[ "${expected_repository}" != "radius-project/radius" ]]; then
        fail "expected output contract is invalid"
    fi
    if [[ "$(plan_value '.includedBackports | type')" != "!!seq" ]]; then
        fail "includedBackports must be an array"
    fi
    if [[ "$(
        plan_value '[.includedBackports[].backport_merged] | all'
    )" != "true" ]]; then
        fail "release plan contains an incomplete backport"
    fi

    validate_policy "${release_type}" "${channel}" "${version}"
    validate_source "${channel}"
    if [[ "$(plan_value '.siblingRepositories | type')" != "!!seq" ]]; then
        fail "siblingRepositories must be an array"
    fi
    validate_files "${version}" "${release_plan_path}"
    validate_generated_contents "${version}" "${release_type}" "${channel}" \
        "${release_date}" "${release_plan_path}"
    echo "Release plan ${version} is valid."
}

main "$@"
