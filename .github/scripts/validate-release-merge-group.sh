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

CANDIDATES_FILE=""
MERGE_GROUP_SHA=""
BASE_SHA=""
OUTPUT_FILE=""

fail() {
    echo "Error: $*" >&2
    exit 1
}

tree_entry() {
    local commit="$1"
    local path="$2"

    git rev-parse "${commit}:${path}" 2> /dev/null \
                                                   || printf '%s\n' '<missing>'
}

main() {
    local candidate_sha candidate_number candidate_blob merge_blob file
    local candidates_tsv changed_files candidate_files
    local matches_paths touches_release=false
    local -a matches=()

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --candidates-file)
                CANDIDATES_FILE="${2:-}"
                shift 2
                ;;
            --merge-group-sha)
                MERGE_GROUP_SHA="${2:-}"
                shift 2
                ;;
            --base-sha)
                BASE_SHA="${2:-}"
                shift 2
                ;;
            --output-file)
                OUTPUT_FILE="${2:-}"
                shift 2
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    [[ -f "${CANDIDATES_FILE}" ]] || fail "candidates file not found"
    [[ -n "${OUTPUT_FILE}" ]] || fail "output file is required"
    if ! git rev-parse --verify --quiet \
        "${MERGE_GROUP_SHA}^{commit}" > /dev/null; then
        fail "merge group SHA is not a commit"
    fi
    if ! git rev-parse --verify --quiet \
        "${BASE_SHA}^{commit}" > /dev/null; then
        fail "merge group base SHA is not a commit"
    fi
    jq -e 'type == "array" and all(.[];
        (.number | type == "number") and
        (.head_sha | test("^[0-9a-f]{40}$")) and
        (.files | type == "array") and
        all(.files[]; type == "string"))' \
        "${CANDIDATES_FILE}" > /dev/null || fail "candidate input is invalid"

    changed_files="$(mktemp "${TMPDIR:-/tmp}/release-changes-XXXXXX")"
    git diff --name-only "${BASE_SHA}" "${MERGE_GROUP_SHA}" \
        > "${changed_files}.raw"
    sort "${changed_files}.raw" > "${changed_files}"
    rm "${changed_files}.raw"
    if grep -Eq \
        '^(CHANGELOG\.md|versions\.yaml|docs/release-notes/.+\.md)$' \
        "${changed_files}"; then
        touches_release=true
    fi

    candidates_tsv="$(mktemp "${TMPDIR:-/tmp}/release-candidates-XXXXXX")"
    jq -r '.[] | [.number, .head_sha] | @tsv' "${CANDIDATES_FILE}" \
        > "${candidates_tsv}.raw"
    tr -d '\r' < "${candidates_tsv}.raw" > "${candidates_tsv}"
    rm "${candidates_tsv}.raw"
    while IFS=$'\t' read -r candidate_number candidate_sha; do
        if ! git rev-parse --verify --quiet \
            "${candidate_sha}^{commit}" > /dev/null; then
            fail "release PR #${candidate_number} head is unavailable"
        fi
        matches_paths=true
        candidate_files="${candidates_tsv}.${candidate_number}.files"
        jq -r --argjson number "${candidate_number}" \
            '.[] | select(.number == $number) | .files[]' \
            "${CANDIDATES_FILE}" | tr -d '\r' | sort \
            > "${candidate_files}"
        if ! diff -q "${changed_files}" "${candidate_files}" \
            > /dev/null; then
            matches_paths=false
        fi
        while IFS= read -r file; do
            [[ "${matches_paths}" == "true" ]] || break
            candidate_blob="$(tree_entry "${candidate_sha}" "${file}")"
            merge_blob="$(tree_entry "${MERGE_GROUP_SHA}" "${file}")"
            if [[ "${candidate_blob}" != "${merge_blob}" ]]; then
                matches_paths=false
                break
            fi
        done < "${candidate_files}"
        rm "${candidate_files}"
        if [[ "${matches_paths}" == "true" ]]; then
            matches+=("${candidate_number}:${candidate_sha}")
        fi
    done < "${candidates_tsv}"
    rm "${candidates_tsv}" "${changed_files}"

    if ((${#matches[@]} == 0)); then
        if [[ "${touches_release}" == "true" ]]; then
            fail "release metadata changed without a matching release plan"
        fi
        : > "${OUTPUT_FILE}"
        echo "Merge group contains no generated release pull request."
        return
    fi
    if ((${#matches[@]} != 1)); then
        fail "merge group contains multiple release pull requests"
    fi

    candidate_number="${matches[0]%%:*}"
    printf '%s\n' "${candidate_number}" > "${OUTPUT_FILE}"
    echo "Merge group contains only release PR #${candidate_number}."
}

main "$@"
