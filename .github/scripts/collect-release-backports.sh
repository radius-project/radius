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

GH="${GH:-gh}"
REPOSITORY=""
CHANNEL=""
EXPLICIT_PRS=""
OUTPUT_FILE=""
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
    cat << 'EOF'
Usage: collect-release-backports.sh --repository <owner/repo> --channel <X.Y> \
    --output <path> [--explicit-prs <csv>]
EOF
}

collect_explicit_prs() {
    local output="$1"
    local pr metadata
    local -a numbers=()

    printf '[]\n' > "${output}"
    [[ -n "${EXPLICIT_PRS}" ]] || return

    IFS=',' read -r -a numbers <<< "${EXPLICIT_PRS}"
    for pr in "${numbers[@]}"; do
        pr="${pr//[[:space:]]/}"
        if [[ ! "${pr}" =~ ^[1-9][0-9]*$ ]]; then
            fail "invalid explicit pull request number: ${pr}"
        fi
        metadata="$(
            "${GH}" pr view "${pr}" --repo "${REPOSITORY}" \
                --json number,title,url,mergeCommit,mergedAt,baseRefName
        )"
        if ! jq -e '.mergedAt != null and .baseRefName == "main"' \
            <<< "${metadata}" > /dev/null; then
            fail "explicit pull request #${pr} is not merged into main"
        fi
        jq --argjson item "${metadata}" '. + [$item]' "${output}" \
            > "${output}.tmp"
        mv "${output}.tmp" "${output}"
    done
}

main() {
    local label release_branch
    local labeled_file explicit_file sources_file backports_file

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --repository)
                REPOSITORY="${2:-}"
                shift 2
                ;;
            --channel)
                CHANNEL="${2:-}"
                shift 2
                ;;
            --explicit-prs)
                EXPLICIT_PRS="${2:-}"
                shift 2
                ;;
            --output)
                OUTPUT_FILE="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    if [[ ! "${REPOSITORY}" =~ ^[^/]+/[^/]+$ ]]; then
        fail "repository must use owner/name format"
    fi
    if [[ ! "${CHANNEL}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        fail "channel must use X.Y format"
    fi
    [[ -n "${OUTPUT_FILE}" ]] || fail "output path is required"
    command -v "${GH}" > /dev/null || fail "required command not found: ${GH}"
    command -v jq > /dev/null || fail "required command not found: jq"

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/release-backports-XXXXXX")"
    labeled_file="${TEMP_DIR}/labeled.json"
    explicit_file="${TEMP_DIR}/explicit.json"
    sources_file="${TEMP_DIR}/sources.json"
    backports_file="${TEMP_DIR}/backports.json"
    label="backport release/${CHANNEL}"
    release_branch="release/${CHANNEL}"

    "${GH}" pr list --repo "${REPOSITORY}" --state merged --base main \
        --label "${label}" --limit 1000 \
        --json number,title,url,mergeCommit,mergedAt,baseRefName \
        > "${labeled_file}"
    collect_explicit_prs "${explicit_file}"
    jq -s 'add | unique_by(.number) | sort_by(.number)' \
        "${labeled_file}" "${explicit_file}" > "${sources_file}"

    "${GH}" pr list --repo "${REPOSITORY}" --state all \
        --base "${release_branch}" --limit 1000 \
        --json number,url,body,state,mergedAt,mergeCommit,commits \
        > "${backports_file}"

    mkdir -p "$(dirname "${OUTPUT_FILE}")"
    jq --slurpfile backports "${backports_file}" '
        map(
            . as $source |
            ($backports[0] |
                map(select(
                    # A body naming more than one source cannot identify
                    # which backport it is, so it is never trusted.
                    ((.body // "") |
                        [scan("<!-- radius-backport-source: #[0-9]+ -->")] |
                        length) == 1 and
                    ((.body // "") |
                        contains(
                            "<!-- radius-backport-source: #" +
                            "\($source.number) -->"
                        ))
                )) |
                sort_by([(.mergedAt != null), .number]) |
                last
            ) as $backport |
            (($backport.commits // []) |
                any(.[];
                    ((.messageBody // "") | split("\n")) |
                    any(.[];
                        gsub("\\r$"; "") ==
                            "(cherry picked from commit " +
                            "\($source.mergeCommit.oid))"
                    )
                )
            ) as $has_source_trailer |
            {
                source_pr: $source.number,
                source_commit: $source.mergeCommit.oid,
                source_title: $source.title,
                source_url: $source.url,
                backport_pr: ($backport.number // null),
                backport_url: ($backport.url // null),
                backport_merged: (
                    (($backport.mergedAt // null) != null) and
                    $has_source_trailer
                ),
                backport_commit: ($backport.mergeCommit.oid // null)
            }
        )
    ' "${sources_file}" > "${OUTPUT_FILE}"
}

main "$@"
