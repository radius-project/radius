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
# shellcheck source=.github/scripts/release-version.sh
source "${SCRIPT_DIR}/release-version.sh"

RELEASE_TYPE=""
CHANNEL=""
BACKPORTS_FILE=""
OUTPUT_DIR=""
VERSIONS_FILE="versions.yaml"
CHANGELOG_FILE="CHANGELOG.md"
RELEASE_NOTES_TEMPLATE="docs/release-notes/template.md"
PATCH_NOTES_TEMPLATE="docs/release-notes/template_patch.md"
TARGETS_FILE=".github/release-parity/targets.json"
CLIFF_CONFIG="cliff.toml"
GIT_CLIFF="${GIT_CLIFF:-git-cliff}"
CHANGELOG_RANGE_SCRIPT="${CHANGELOG_RANGE_SCRIPT:-}"
if [[ -z "${CHANGELOG_RANGE_SCRIPT}" ]]; then
    CHANGELOG_RANGE_SCRIPT="${SCRIPT_DIR}/changelog-range.sh"
fi
VERSION_ONLY=false
RELEASE_DATE="${RELEASE_DATE:-$(date -u +%F)}"

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat << 'EOF'
Usage: prepare-release.sh --release-type <rc|final|patch> --channel <X.Y> \
        --backports-file <path> --output-dir <path> [file options]

File options:
    --versions-file <path>
    --changelog-file <path>
    --release-notes-template <path>
    --patch-notes-template <path>
    --targets-file <path>
    --cliff-config <path>
    --release-date <YYYY-MM-DD>

Modes:
    --version-only  Calculate the policy version without changing files.
EOF
}

require_command() {
    command -v "$1" > /dev/null || fail "required command not found: $1"
}

channel_version() {
    yq -r ".supported[] | select(.channel == \"${CHANNEL}\") | .version" \
        "${VERSIONS_FILE}" | head -1
}

latest_supported_version() {
    yq -r '.supported[0].version // ""' "${VERSIONS_FILE}"
}

release_branch_ref() {
    local branch="release/${CHANNEL}"

    if git rev-parse --verify --quiet "refs/remotes/origin/${branch}^{commit}" \
        > /dev/null; then
        printf 'refs/remotes/origin/%s\n' "${branch}"
        return 0
    fi
    if git rev-parse --verify --quiet "refs/heads/${branch}^{commit}" \
        > /dev/null; then
        printf 'refs/heads/%s\n' "${branch}"
        return 0
    fi
    return 1
}

highest_rc_number() {
    local tag
    local highest=0
    local pattern="^v${CHANNEL//./\\.}\\.0-rc\\.?([1-9][0-9]*)$"

    while IFS= read -r tag; do
        if [[ "${tag}" =~ ${pattern} ]]; then
            if ((10#${BASH_REMATCH[1]} > highest)); then
                highest=$((10#${BASH_REMATCH[1]}))
            fi
        fi
    done < <(git tag --list "v${CHANNEL}.0-rc*" --sort=version:refname)

    printf '%s\n' "${highest}"
}

newest_stable_tag() {
    local tag

    while IFS= read -r tag; do
        if [[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            printf '%s\n' "${tag}"
            return 0
        fi
    done < <(git tag --list 'v*' --sort=-version:refname)
    return 1
}

next_channel() {
    local version="$1"
    local stable="${version#v}"
    local major minor

    stable="${stable%%-*}"
    IFS='.' read -r major minor _ <<< "${stable}"
    printf '%s.%s\n' "${major}" "$((10#${minor} + 1))"
}

calculate_version() {
    local current latest rc_number branch_ref current_rc_pattern

    current="$(channel_version)"
    latest="$(latest_supported_version)"
    branch_ref="$(release_branch_ref || true)"

    case "${RELEASE_TYPE}" in
        rc)
            rc_number="$(highest_rc_number)"
            if ((rc_number == 0)); then
                if [[ -n "${branch_ref}" ]]; then
                    fail "release/${CHANNEL} exists but has no RC tags"
                fi
                if [[ ! "${latest}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                    fail "latest supported version must be stable"
                fi
                if [[ "${CHANNEL}" != "$(next_channel "${latest}")" ]]; then
                    fail "first RC channel must follow ${latest}"
                fi
                printf 'v%s.0-rc.1\n' "${CHANNEL}"
                return
            fi
            if [[ -z "${branch_ref}" ]]; then
                fail "release/${CHANNEL} is required for another RC"
            fi
            current_rc_pattern="^v${CHANNEL//./\.}\\.0-rc\\.?([1-9][0-9]*)$"
            if [[ ! "${current}" =~ ${current_rc_pattern} ]]; then
                fail "versions.yaml has no current RC for ${CHANNEL}"
            fi
            if ((10#${BASH_REMATCH[1]} != rc_number)); then
                fail "versions.yaml RC does not match the highest tag"
            fi
            if ! git merge-base --is-ancestor "refs/tags/${current}" \
                "${branch_ref}"; then
                fail "${current} is not reachable from release/${CHANNEL}"
            fi
            printf 'v%s.0-rc.%s\n' "${CHANNEL}" "$((rc_number + 1))"
            ;;
        final)
            if [[ -z "${branch_ref}" ]]; then
                fail "release/${CHANNEL} is required for a final release"
            fi
            if [[ ! "${current}" =~ ^v${CHANNEL//./\.}\.0-rc\.?[0-9]+$ ]]; then
                fail "versions.yaml has no RC for ${CHANNEL}"
            fi
            local rc_commit branch_commit
            if ! rc_commit="$(
                git rev-parse --verify "refs/tags/${current}^{commit}"
            )"; then
                fail "RC tag does not exist: ${current}"
            fi
            branch_commit="$(git rev-parse "${branch_ref}^{commit}")"
            if [[ "${branch_commit}" != "${rc_commit}" ]]; then
                fail "release/${CHANNEL} advanced; validate another RC"
            fi
            printf 'v%s.0\n' "${CHANNEL}"
            ;;
        patch)
            if [[ -z "${branch_ref}" ]]; then
                fail "release/${CHANNEL} is required for a patch"
            fi
            if [[ ! "${current}" =~ ^v${CHANNEL//./\.}\.([0-9]+)$ ]]; then
                fail "versions.yaml has no stable ${CHANNEL} release"
            fi
            if ! git rev-parse --verify --quiet \
                "refs/tags/${current}^{commit}" > /dev/null; then
                fail "stable tag does not exist: ${current}"
            fi
            if ! git merge-base --is-ancestor "refs/tags/${current}" \
                "${branch_ref}"; then
                fail "${current} is not reachable from release/${CHANNEL}"
            fi
            printf 'v%s.%s\n' "${CHANNEL}" \
                "$((10#${BASH_REMATCH[1]} + 1))"
            ;;
    esac
}

validate_backports() {
    local branch_ref="$1"
    local missing

    if ! jq -e 'type == "array"' "${BACKPORTS_FILE}" > /dev/null; then
        fail "backports file must contain a JSON array"
    fi

    if [[ -z "${branch_ref}" ]]; then
        if [[ "$(jq 'length' "${BACKPORTS_FILE}")" != "0" ]]; then
            fail "a first RC already contains main; omit backports"
        fi
        return
    fi

    missing="$(jq -r \
        '.[] | select(.backport_merged != true) | "#\(.source_pr)"' \
        "${BACKPORTS_FILE}" | paste -sd, -)"
    if [[ -n "${missing}" ]]; then
        fail "backports missing from release/${CHANNEL}: ${missing}"
    fi
}

replace_marker() {
    local document="$1"
    local marker="$2"
    local replacement="$3"
    local output="${document}.tmp"

    awk -v marker="${marker}" -v replacement="${replacement}" '
        $0 == marker {
            while ((getline line < replacement) > 0) {
                print line
            }
            close(replacement)
            next
        }
        { print }
    ' "${document}" > "${output}"
    mv "${output}" "${document}"
}

extract_section() {
    local document="$1"
    local heading="$2"
    local output="$3"

    awk -v heading="${heading}" '
        $0 == heading { active = 1; next }
        active && /^### / { exit }
        active { print }
    ' "${document}" > "${output}"

    if [[ ! -s "${output}" ]]; then
        printf 'None.\n' > "${output}"
    fi
}

render_changelog() {
    local version="$1"
    local ref="$2"
    local body_file="$3"
    local range
    local -a remote_options=()

    range="$(bash "${CHANGELOG_RANGE_SCRIPT}" --ref "${ref}")"
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        remote_options+=(--offline)
    fi

    "${GIT_CLIFF}" --config "${CLIFF_CONFIG}" --tag "${version}" \
        --strip all --output "${body_file}" "${remote_options[@]}" \
        "${range}"
    [[ -s "${body_file}" ]] || fail "git-cliff rendered an empty changelog"

    awk -v heading="## [${version#v}] - ${RELEASE_DATE}" '
        NR == 1 && /^## \[/ { print heading; next }
        { print }
    ' "${body_file}" > "${body_file}.tmp"
    mv "${body_file}.tmp" "${body_file}"
}

update_versions() {
    local version="$1"
    local current="$2"

    if [[ -z "${current}" ]]; then
        # yq reads these environment variables through strenv().
        # shellcheck disable=SC2016
        CHANNEL="${CHANNEL}" VERSION="${version}" yq -i '
            .supported as $supported |
            .supported = (
                [{"channel": strenv(CHANNEL),
                  "version": strenv(VERSION)}] + $supported[0:-1]
            ) |
            .deprecated = ([$supported[-1]] + .deprecated)
        ' "${VERSIONS_FILE}"
    else
        CHANNEL="${CHANNEL}" VERSION="${version}" yq -i '
            (.supported[] | select(.channel == strenv(CHANNEL)) | .version) =
                strenv(VERSION)
        ' "${VERSIONS_FILE}"
    fi
}

update_changelog() {
    local version="$1"
    local previous_version="$2"
    local body_file="$3"
    local display_version="${version#v}"
    local output="${CHANGELOG_FILE}.tmp"

    if ! grep -Fqx '## [Unreleased]' "${CHANGELOG_FILE}"; then
        fail "CHANGELOG.md has no Unreleased section"
    fi
    if grep -Fq "## [${display_version}] -" "${CHANGELOG_FILE}"; then
        fail "CHANGELOG.md already contains ${version}"
    fi

    awk -v body="${body_file}" '
        { print }
        $0 == "## [Unreleased]" {
            print ""
            while ((getline line < body) > 0) {
                print line
            }
            close(body)
        }
    ' "${CHANGELOG_FILE}" > "${output}"
    mv "${output}" "${CHANGELOG_FILE}"

    awk -v current="${display_version}" -v version="${version}" \
        -v previous="${previous_version}" \
        -v base="https://github.com/radius-project/radius/compare/" '
        /^\[Unreleased\]:/ {
            print "[Unreleased]: " base version "...HEAD"
            print "[" current "]: " base previous "..." version
            next
        }
        { print }
    ' "${CHANGELOG_FILE}" > "${output}"
    mv "${output}" "${CHANGELOG_FILE}"
}

generate_release_notes() {
    local version="$1"
    local body_file="$2"
    local notes_file="$3"
    local template="${RELEASE_NOTES_TEMPLATE}"
    local generated="${OUTPUT_DIR}/notes-changelog.md"
    local breaking="${OUTPUT_DIR}/breaking-changes.md"
    local contributors="${OUTPUT_DIR}/new-contributors.md"

    if [[ "${RELEASE_TYPE}" == "patch" ]]; then
        template="${PATCH_NOTES_TEMPLATE}"
    fi
    cp "${template}" "${notes_file}"
    sed -i "s/vX\.Y\.Z/${version}/g" "${notes_file}"
    sed -i '/REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT/d' \
        "${notes_file}"

    awk 'NR == 1 { next } { lines[++count] = $0 }
        END {
            start = 1
            while (start <= count && lines[start] == "") { start++ }
            for (cursor = start; cursor <= count; cursor++) {
                print lines[cursor]
            }
        }
    ' "${body_file}" > "${generated}"
    replace_marker "${notes_file}" \
        '<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->' \
        "${generated}"

    if [[ "${RELEASE_TYPE}" != "patch" ]]; then
        extract_section "${body_file}" '### Breaking changes' "${breaking}"
        extract_section "${body_file}" '### New contributors' \
            "${contributors}"
        replace_marker "${notes_file}" \
            '<!-- ADD ANY BREAKING CHANGES HERE, IF ANY -->' "${breaking}"
        replace_marker "${notes_file}" \
            '<!-- PASTE THE OUTPUT OF THE GENERATED CONTRIBUTOR LIST HERE -->' \
            "${contributors}"
    fi
}

write_release_plan() {
    local version="$1"
    local previous_version="$2"
    local source_ref="$3"
    local source_commit="$4"
    local plan_file="${OUTPUT_DIR}/release-plan.yaml"
    local body_file="${OUTPUT_DIR}/release-pr-body.md"
    local requires_backport="$5"
    local release_commit_resolution="release PR squash commit on main"

    if [[ "${source_ref}" != "HEAD" ]]; then
        release_commit_resolution="generated release backport commit"
        release_commit_resolution+=" on release/${CHANNEL}"
    fi

    VERSION="${version}" RELEASE_TYPE="${RELEASE_TYPE}" \
        CHANNEL="${CHANNEL}" RELEASE_DATE="${RELEASE_DATE}" \
        PREVIOUS_VERSION="${previous_version}" \
        SOURCE_REF="${source_ref}" SOURCE_COMMIT="${source_commit}" \
        RELEASE_COMMIT_RESOLUTION="${release_commit_resolution}" \
        TARGETS_FILE="${TARGETS_FILE}" BACKPORTS_FILE="${BACKPORTS_FILE}" \
        yq -n '
            {
                "schemaVersion": 1,
                "version": strenv(VERSION),
                "releaseType": strenv(RELEASE_TYPE),
                "channel": strenv(CHANNEL),
                "releaseDate": strenv(RELEASE_DATE),
                "chartVersion": (strenv(VERSION) | sub("^v"; "")),
                "source": {
                    "productRef": strenv(SOURCE_REF),
                    "productCommit": strenv(SOURCE_COMMIT),
                    "releaseCommit": null,
                    "releaseCommitResolution":
                        strenv(RELEASE_COMMIT_RESOLUTION)
                },
                "releaseBranch": "release/" + strenv(CHANNEL),
                "previousVersion": strenv(PREVIOUS_VERSION),
                "expectedOutputs": load(strenv(TARGETS_FILE)),
                "includedBackports": load(strenv(BACKPORTS_FILE))
            } | ... style = ""
        ' > "${plan_file}"

    {
        echo "## Release plan"
        echo
        echo "This pull request was generated by the Prepare Release workflow."
        echo "The plan below is the reviewable release-controller input."
        echo
        echo '<!-- radius-release-plan:start -->'
        echo '```yaml'
        cat "${plan_file}"
        echo '```'
        echo '<!-- radius-release-plan:end -->'
        echo
        echo "## Maintainer review"
        echo
        echo "- [ ] Curate Highlights in the generated release notes."
        echo "- [ ] Review and update the Upgrading guidance."
        echo "- [ ] Verify the source commit and included backports."
        if [[ "${requires_backport}" == "true" ]]; then
            echo "- [ ] Rebase-merge this PR's generated backport after merge."
        fi
        echo
        echo "Generated for #12814."
    } > "${body_file}"

    printf 'chore(release): prepare %s\n' "${version}" \
        > "${OUTPUT_DIR}/pr-title.txt"
    printf 'automation/prepare-release-%s\n' "${version#v}" \
        > "${OUTPUT_DIR}/pr-branch.txt"
    printf '%s\n' "${requires_backport}" \
        > "${OUTPUT_DIR}/requires-backport.txt"
}

main() {
    local version branch_ref

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --release-type)
                RELEASE_TYPE="${2:-}"
                shift 2
                ;;
            --channel)
                CHANNEL="${2:-}"
                shift 2
                ;;
            --backports-file)
                BACKPORTS_FILE="${2:-}"
                shift 2
                ;;
            --output-dir)
                OUTPUT_DIR="${2:-}"
                shift 2
                ;;
            --versions-file)
                VERSIONS_FILE="${2:-}"
                shift 2
                ;;
            --changelog-file)
                CHANGELOG_FILE="${2:-}"
                shift 2
                ;;
            --release-notes-template)
                RELEASE_NOTES_TEMPLATE="${2:-}"
                shift 2
                ;;
            --patch-notes-template)
                PATCH_NOTES_TEMPLATE="${2:-}"
                shift 2
                ;;
            --targets-file)
                TARGETS_FILE="${2:-}"
                shift 2
                ;;
            --cliff-config)
                CLIFF_CONFIG="${2:-}"
                shift 2
                ;;
            --release-date)
                RELEASE_DATE="${2:-}"
                shift 2
                ;;
            --version-only)
                VERSION_ONLY=true
                shift
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    if [[ ! "${RELEASE_TYPE}" =~ ^(rc|final|patch)$ ]]; then
        fail "release type must be rc, final, or patch"
    fi
    if [[ ! "${CHANNEL}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        fail "channel must use X.Y format"
    fi
    if [[ ! "${RELEASE_DATE}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        fail "release date must use YYYY-MM-DD format"
    fi
    [[ -n "${OUTPUT_DIR}" ]] || fail "output directory is required"
    [[ -f "${VERSIONS_FILE}" ]] || fail "versions file not found"
    require_command git
    require_command yq

    branch_ref="$(release_branch_ref || true)"
    version="$(calculate_version)"
    mkdir -p "${OUTPUT_DIR}"
    printf '%s\n' "${version}" > "${OUTPUT_DIR}/version.txt"
    printf 'automation/prepare-release-%s\n' "${version#v}" \
        > "${OUTPUT_DIR}/pr-branch.txt"
    if [[ "${VERSION_ONLY}" == "true" ]]; then
        printf 'Selected %s (%s) for release/%s.\n' \
            "${version}" "${RELEASE_TYPE}" "${CHANNEL}"
        return
    fi

    [[ -f "${BACKPORTS_FILE}" ]] || fail "backports file not found"
    [[ -f "${CHANGELOG_FILE}" ]] || fail "changelog file not found"
    if [[ ! -f "${RELEASE_NOTES_TEMPLATE}" ]]; then
        fail "release notes template not found"
    fi
    [[ -f "${PATCH_NOTES_TEMPLATE}" ]] \
                                       || fail "patch notes template not found"
    [[ -f "${TARGETS_FILE}" ]] || fail "release targets file not found"
    [[ -f "${CLIFF_CONFIG}" ]] || fail "git-cliff config not found"
    require_command jq
    require_command "${GIT_CLIFF}"
    validate_backports "${branch_ref}"

    local current previous_version source_ref source_commit requires_backport
    local changelog_base_version changelog_body notes_file
    current="$(channel_version)"
    previous_version="${current:-$(latest_supported_version)}"
    if [[ "${RELEASE_TYPE}" == "patch" ]]; then
        changelog_base_version="${current}"
    else
        if ! changelog_base_version="$(newest_stable_tag)"; then
            fail "no stable Radius release tag was found"
        fi
    fi
    source_ref="${branch_ref:-HEAD}"
    source_commit="$(git rev-parse "${source_ref}^{commit}")"
    requires_backport="false"
    [[ -z "${branch_ref}" ]] || requires_backport="true"
    changelog_body="${OUTPUT_DIR}/changelog-section.md"
    notes_file="docs/release-notes/${version}.md"

    render_changelog "${version}" "${source_ref}" "${changelog_body}"
    update_versions "${version}" "${current}"
    update_changelog "${version}" "${changelog_base_version}" \
        "${changelog_body}"
    generate_release_notes "${version}" "${changelog_body}" "${notes_file}"
    write_release_plan "${version}" "${previous_version}" "${source_ref}" \
        "${source_commit}" "${requires_backport}"
    printf 'Prepared %s (%s) for release/%s.\n' \
        "${version}" "${RELEASE_TYPE}" "${CHANNEL}"
}

main "$@"
