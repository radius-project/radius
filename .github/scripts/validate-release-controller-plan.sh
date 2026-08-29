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

PLAN_FILE=""
VERSION=""
SOURCE_PR_COMMIT=""
RELEASE_COMMIT=""
TRIGGER=""
OUTPUT_DIR=""
TEMP_DIR=""

cleanup() {
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
Usage: validate-release-controller-plan.sh --plan-file <path> \
    --version <vX.Y.Z[-rc.N]> --source-pr-commit <sha> \
    --release-commit <sha> --trigger <release-pr|release-backport|dispatch> \
    --output-dir <path>
EOF
}

plan_value() {
    yq -r "$1" "${TEMP_DIR}/release-plan.yaml"
}

load_plan() {
    cp "${PLAN_FILE}" "${TEMP_DIR}/release-plan.yaml"
    yq -e '.' "${TEMP_DIR}/release-plan.yaml" >/dev/null ||
        fail "release plan is not valid YAML"
}

commit_parent() {
    local commit="$1"
    local -a revision=()

    read -r -a revision <<<"$(git rev-list --parents -n 1 "${commit}")"
    ((${#revision[@]} == 2)) ||
        fail "release metadata commit ${commit} must have one parent"
    printf '%s' "${revision[1]}"
}

validate_changed_files() {
    local commit="$1"
    local version="$2"
    local plan_path="$3"
    local require_legacy_cleanup="${4:-false}"
    local expected="${TEMP_DIR}/expected-files.txt"
    local actual="${TEMP_DIR}/actual-files.txt"

    {
        echo 'CHANGELOG.md'
        echo "${plan_path}"
        echo "docs/release-notes/${version}.md"
        echo 'versions.yaml'
        if [[ "${require_legacy_cleanup}" == "true" ]] &&
            git cat-file -e \
                "${commit}^:.github/workflows/release.yaml" \
                2>/dev/null; then
            echo '.github/workflows/release.yaml'
            if git cat-file -e \
                "${commit}:.github/workflows/release.yaml" 2>/dev/null; then
                fail "release metadata backport kept the legacy release workflow"
            fi
        fi
    } | sort >"${expected}"
    git diff --name-only "${commit}^" "${commit}" | sort >"${actual}"
    if ! diff -u "${expected}" "${actual}"; then
        fail "release metadata commit changes files outside the plan contract"
    fi
}

validate_release_metadata() {
    local commit="$1"
    local version="$2"
    local channel="$3"
    local plan_path="$4"
    local versions_file="${TEMP_DIR}/versions.yaml"
    local matches

    git show "${commit}:versions.yaml" >"${versions_file}" ||
        fail "release commit has no versions.yaml"
    matches="$(
        VERSION="${version}" CHANNEL="${channel}" yq -r '
            [.supported[] | select(
                .version == strenv(VERSION) and
                .channel == strenv(CHANNEL)
            )] | length
        ' "${versions_file}"
    )"
    [[ "${matches}" == "1" ]] ||
        fail "versions.yaml does not contain ${version} for ${channel}"
    git cat-file -e "${commit}:CHANGELOG.md" ||
        fail "release commit has no CHANGELOG.md"
    git cat-file -e "${commit}:docs/release-notes/${version}.md" ||
        fail "release commit has no release notes for ${version}"
    git cat-file -e "${commit}:${plan_path}" ||
        fail "release commit has no immutable release plan"
}

validate_committed_plan() {
    local commit="$1"
    local plan_path="$2"
    local committed="${TEMP_DIR}/committed-plan.yaml"

    git show "${commit}:${plan_path}" >"${committed}"
    if ! diff -u \
        <(yq -o=json -I=0 '.' "${TEMP_DIR}/release-plan.yaml" | jq -S .) \
        <(yq -o=json -I=0 '.' "${committed}" | jq -S .); then
        fail "committed release plan differs from approved plan"
    fi
}

remote_tag_commit() {
    local tag="$1"
    local refs sha ref direct="" peeled=""

    refs="$(
        git ls-remote origin "refs/tags/${tag}" \
            "refs/tags/${tag}^{}"
    )" || fail "could not query Radius tag ${tag}"
    while read -r sha ref; do
        case "${ref}" in
            "refs/tags/${tag}^{}") peeled="${sha}" ;;
            "refs/tags/${tag}") direct="${sha}" ;;
        esac
    done <<<"${refs}"

    printf '%s' "${peeled:-${direct}}"
}

write_outputs() {
    local ready="$1"
    local channel="$2"
    local release_type="$3"
    local release_branch="$4"
    local de_image_tag="${VERSION#v}"
    local recipes_commit dashboard_commit aws_commit
    local resolved_plan="${OUTPUT_DIR}/release-plan.yaml"

    mkdir -p "${OUTPUT_DIR}"
    if [[ "${ready}" == "true" ]]; then
        RELEASE_COMMIT="${RELEASE_COMMIT}" yq \
            '.source.releaseCommit = strenv(RELEASE_COMMIT)' \
            "${TEMP_DIR}/release-plan.yaml" >"${resolved_plan}"
    else
        cp "${TEMP_DIR}/release-plan.yaml" "${resolved_plan}"
    fi
    printf '%s\n' "${ready}" >"${OUTPUT_DIR}/ready.txt"
    printf '%s\n' "${VERSION}" >"${OUTPUT_DIR}/version.txt"
    printf '%s\n' "${channel}" >"${OUTPUT_DIR}/channel.txt"
    printf '%s\n' "${release_type}" >"${OUTPUT_DIR}/release-type.txt"
    printf '%s\n' "${release_branch}" \
        >"${OUTPUT_DIR}/release-branch.txt"
    if [[ "${release_type}" != "rc" ]]; then
        de_image_tag="${channel}"
    fi
    recipes_commit="$(
        plan_value '.siblingRepositories[] | select(.name == "recipes") |
            .sourceCommit'
    )"
    dashboard_commit="$(
        plan_value '.siblingRepositories[] | select(.name == "dashboard") |
            .sourceCommit'
    )"
    aws_commit="$(
        plan_value '.siblingRepositories[] |
            select(.name == "bicep-types-aws") | .sourceCommit'
    )"
    printf '%s\n' "${de_image_tag}" >"${OUTPUT_DIR}/de-image-tag.txt"
    printf '%s\n' "${recipes_commit}" >"${OUTPUT_DIR}/recipes-commit.txt"
    printf '%s\n' "${dashboard_commit}" \
        >"${OUTPUT_DIR}/dashboard-commit.txt"
    printf '%s\n' "${aws_commit}" \
        >"${OUTPUT_DIR}/bicep-types-aws-commit.txt"

    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        {
            echo "ready=${ready}"
            echo "version=${VERSION}"
            echo "channel=${channel}"
            echo "release-type=${release_type}"
            echo "release-branch=${release_branch}"
            echo "release-identifier=${VERSION}-${RELEASE_COMMIT}"
            echo "de-image-tag=${de_image_tag}"
            echo "recipes-commit=${recipes_commit}"
            echo "dashboard-commit=${dashboard_commit}"
            echo "bicep-types-aws-commit=${aws_commit}"
        } >>"${GITHUB_OUTPUT}"
    fi
}

validate_plan_fields() {
    local version="$1"
    local release_type="$2"
    local channel="$3"
    local release_branch="$4"
    local plan_path="$5"
    local canonical_version_pattern expected_repository expected_resolution

    [[ "$(plan_value '.schemaVersion')" == "2" ]] ||
        fail "unsupported release plan schema"
    canonical_version_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
    canonical_version_pattern+='\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$'
    [[ "${version}" =~ ${canonical_version_pattern} ]] ||
        fail "planned version is not canonical"
    [[ "${release_type}" =~ ^(rc|final|patch)$ ]] ||
        fail "planned release type is invalid"
    [[ "${channel}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] ||
        fail "planned channel is invalid"
    [[ "$(plan_value '.chartVersion')" == "${version#v}" ]] ||
        fail "chart version does not match release version"
    [[ "${release_branch}" == "release/${channel}" ]] ||
        fail "release branch does not match the channel"
    [[ "${plan_path}" == ".github/release-plans/${version}.yaml" ]] ||
        fail "release plan path does not match the version"
    expected_repository="$(plan_value '.expectedOutputs.repository')"
    [[ "${expected_repository}" == "radius-project/radius" ]] ||
        fail "expected output contract is invalid"
    [[ "$(plan_value '.source.releaseCommit')" == "null" ]] ||
        fail "approved plan already contains a release commit"
    if [[ "$(plan_value '.source.productRef')" == "HEAD" ]]; then
        expected_resolution="release PR squash commit on main"
    else
        expected_resolution="generated release backport commit on ${release_branch}"
    fi
    [[ "$(plan_value '.source.releaseCommitResolution')" == "${expected_resolution}" ]] || fail "release commit resolution is invalid"
    [[ "$(plan_value '.siblingRepositories | type')" == "!!seq" ]] ||
        fail "siblingRepositories must be an array"
    CHANNEL="${channel}" yq -o=json -I=0 '.' \
        "${TEMP_DIR}/release-plan.yaml" | jq -e '
        .siblingRepositories | type == "array" and
        map(.name) == ["recipes", "dashboard", "bicep-types-aws"] and
        all(.[].sourceCommit; test("^[0-9a-f]{40}$")) and
        all(.[].sourceRef;
            . == "main" or . == ("release/" + env.CHANNEL)) and
        all(.[]; .repository == ("radius-project/" + .name))
    ' >/dev/null ||
        fail "sibling repository state is invalid"
    [[ "$(plan_value '.includedBackports | type')" == "!!seq" ]] ||
        fail "includedBackports must be an array"
    [[ "$(plan_value '[.includedBackports[].backport_merged] | all')" == "true" ]] || fail "release plan contains an incomplete backport"
}

validate_backport_commit() {
    local product_commit="$1"
    local release_branch="$2"
    local branch_ref="refs/remotes/origin/${release_branch}"
    local parent branch_commit message

    git show-ref --verify --quiet "${branch_ref}" ||
        fail "${release_branch} is not available"
    branch_commit="$(git rev-parse "${branch_ref}^{commit}")"
    [[ "${branch_commit}" == "${RELEASE_COMMIT}" ]] ||
        fail "${release_branch} is at ${branch_commit}, expected ${RELEASE_COMMIT}"
    parent="$(commit_parent "${RELEASE_COMMIT}")"
    [[ "${parent}" == "${product_commit}" ]] ||
        fail "release branch advanced beyond approved commit ${product_commit}"
    message="$(git show -s --format=%B "${RELEASE_COMMIT}")"
    grep -Fq "(cherry picked from commit ${SOURCE_PR_COMMIT})" \
        <<<"${message}" ||
        fail "release commit is not the approved release PR backport"
}

main() {
    local version release_type channel release_branch product_ref
    local product_commit expected_ref source_parent existing_tag ready="true"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --plan-file)
                PLAN_FILE="${2:-}"
                shift 2
                ;;
            --version)
                VERSION="${2:-}"
                shift 2
                ;;
            --source-pr-commit)
                SOURCE_PR_COMMIT="${2:-}"
                shift 2
                ;;
            --release-commit)
                RELEASE_COMMIT="${2:-}"
                shift 2
                ;;
            --trigger)
                TRIGGER="${2:-}"
                shift 2
                ;;
            --output-dir)
                OUTPUT_DIR="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    [[ -f "${PLAN_FILE}" ]] || fail "release plan file not found"
    [[ -n "${OUTPUT_DIR}" ]] || fail "output directory is required"
    [[ "${TRIGGER}" =~ ^(release-pr|release-backport|dispatch)$ ]] ||
        fail "trigger is invalid"
    [[ "${SOURCE_PR_COMMIT}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "source PR commit must be a full commit SHA"
    [[ "${RELEASE_COMMIT}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "release commit must be a full commit SHA"
    command -v yq >/dev/null || fail "required command not found: yq"
    command -v jq >/dev/null || fail "required command not found: jq"

    git cat-file -e "${SOURCE_PR_COMMIT}^{commit}" ||
        fail "source PR commit does not exist: ${SOURCE_PR_COMMIT}"
    git cat-file -e "${RELEASE_COMMIT}^{commit}" ||
        fail "release commit does not exist: ${RELEASE_COMMIT}"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-controller-XXXXXX")"
    load_plan
    version="$(plan_value '.version')"
    release_type="$(plan_value '.releaseType')"
    channel="$(plan_value '.channel')"
    release_branch="$(plan_value '.releaseBranch')"
    local plan_path
    plan_path="$(plan_value '.releasePlanPath')"
    product_ref="$(plan_value '.source.productRef')"
    product_commit="$(plan_value '.source.productCommit')"

    [[ "${VERSION}" == "${version}" ]] ||
        fail "requested version ${VERSION} does not match plan ${version}"
    [[ "${product_commit}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "product commit must be a full commit SHA"
    validate_plan_fields "${version}" "${release_type}" "${channel}" \
        "${release_branch}" "${plan_path}"
    validate_changed_files "${SOURCE_PR_COMMIT}" "${version}" \
        "${plan_path}" false
    validate_release_metadata "${SOURCE_PR_COMMIT}" "${version}" \
        "${channel}" "${plan_path}"
    validate_committed_plan "${SOURCE_PR_COMMIT}" "${plan_path}"

    expected_ref="refs/remotes/origin/${release_branch}"
    if [[ "${product_ref}" == "HEAD" ]]; then
        [[ "${release_type}" == "rc" && "${version}" == *-rc.1 ]] ||
            fail "a new release branch must start with rc.1"
        source_parent="$(commit_parent "${SOURCE_PR_COMMIT}")"
        [[ "${source_parent}" == "${product_commit}" ]] ||
            fail "release PR was not merged on its approved product commit"
        [[ "${RELEASE_COMMIT}" == "${SOURCE_PR_COMMIT}" ]] ||
            fail "new-channel release commit must be the release PR commit"
        [[ "${TRIGGER}" != "release-backport" ]] ||
            fail "new-channel plans do not use a release backport"
    else
        [[ "${product_ref}" == "${expected_ref}" ]] ||
            fail "productRef must be HEAD or ${expected_ref}"
        if [[ "${TRIGGER}" == "release-pr" ]]; then
            [[ "${RELEASE_COMMIT}" == "${SOURCE_PR_COMMIT}" ]] ||
                fail "release PR trigger has an unexpected source commit"
            ready="false"
        else
            validate_backport_commit "${product_commit}" \
                "${release_branch}"
            validate_changed_files "${RELEASE_COMMIT}" "${version}" \
                "${plan_path}" true
            validate_release_metadata "${RELEASE_COMMIT}" "${version}" \
                "${channel}" "${plan_path}"
            validate_committed_plan "${RELEASE_COMMIT}" "${plan_path}"
        fi
    fi

    if [[ "${ready}" == "true" ]]; then
        existing_tag="$(remote_tag_commit "${version}")"
        if [[ -n "${existing_tag}" &&
            "${existing_tag}" != "${RELEASE_COMMIT}" ]]; then
            fail "Radius tag ${version} points at ${existing_tag}, expected ${RELEASE_COMMIT}"
        fi
    fi

    write_outputs "${ready}" "${channel}" "${release_type}" \
        "${release_branch}"
    if [[ "${ready}" == "true" ]]; then
        echo "Release plan ${version} resolves to ${RELEASE_COMMIT}."
    else
        echo "Release plan ${version} is waiting for its generated backport."
    fi
}

main "$@"
