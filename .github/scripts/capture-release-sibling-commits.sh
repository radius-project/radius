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

CHANNEL=""
OUTPUT_FILE=""
REPOSITORY_ROOT="${SIBLING_REPOSITORY_ROOT:-}"
ENTRIES_FILE=""
readonly REPOSITORIES=(recipes dashboard bicep-types-aws)

cleanup() {
    if [[ -n "${ENTRIES_FILE}" ]]; then
        rm -f "${ENTRIES_FILE}"
    fi
}
trap cleanup EXIT

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: capture-release-sibling-commits.sh --channel <X.Y> --output <path>
EOF
}

repository_url() {
    local name="$1"

    if [[ -n "${REPOSITORY_ROOT}" ]]; then
        printf '%s/%s.git' "${REPOSITORY_ROOT%/}" "${name}"
    else
        printf 'https://github.com/radius-project/%s.git' "${name}"
    fi
}

remote_ref_commit() {
    local repository="$1"
    local ref="$2"
    local output status

    set +e
    output="$(git ls-remote --exit-code "${repository}" "${ref}")"
    status=$?
    set -e
    case "${status}" in
        0) cut -f1 <<<"${output}" ;;
        2) return 2 ;;
        *) fail "could not query ${repository} ${ref}" ;;
    esac
}

main() {
    local name repository source_ref source_commit release_ref

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --channel)
                CHANNEL="${2:-}"
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

    if [[ ! "${CHANNEL}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        fail "channel must use X.Y format"
    fi
    [[ -n "${OUTPUT_FILE}" ]] || fail "output path is required"
    command -v git >/dev/null || fail "required command not found: git"
    command -v jq >/dev/null || fail "required command not found: jq"

    ENTRIES_FILE="$(mktemp "${TMPDIR:-/tmp}/release-siblings-XXXXXX")"
    : >"${ENTRIES_FILE}"
    release_ref="refs/heads/release/${CHANNEL}"
    for name in "${REPOSITORIES[@]}"; do
        repository="$(repository_url "${name}")"
        source_ref="release/${CHANNEL}"
        if ! source_commit="$(
            remote_ref_commit "${repository}" "${release_ref}"
        )"; then
            source_ref="main"
            source_commit="$(
                remote_ref_commit "${repository}" refs/heads/main
            )" || fail "${repository} has no main branch"
        fi
        [[ "${source_commit}" =~ ^[0-9a-f]{40}$ ]] ||
            fail "${repository} returned an invalid commit"
        jq -n --arg name "${name}" \
            --arg repository "radius-project/${name}" \
            --arg sourceRef "${source_ref}" \
            --arg sourceCommit "${source_commit}" '{
                name: $name,
                repository: $repository,
                sourceRef: $sourceRef,
                sourceCommit: $sourceCommit
            }' >>"${ENTRIES_FILE}"
    done

    mkdir -p "$(dirname "${OUTPUT_FILE}")"
    jq -S -s '.' "${ENTRIES_FILE}" >"${OUTPUT_FILE}"
    echo "Captured sibling release commits in ${OUTPUT_FILE}."
}

main "$@"
