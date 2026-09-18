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

# ============================================================================
# Print the commit range holding the unreleased changes of the current release
# channel, for git-cliff to render.
#
# Radius tags stable releases on release/* branches, so the newest stable tag
# is normally not reachable from main. Two boundaries are possible:
#
#   * the newest stable tag reachable from the ref - a release branch's own
#     release, such as v0.59.0 on release/0.59
#   * the commit where the newest stable tag repo-wide left the ref - the point
#     the current channel branched off main
#
# Whichever of the two is later in history is the channel boundary. Picking the
# newest tag repo-wide unconditionally would replay another channel's history
# on a release branch; picking the newest reachable tag unconditionally would
# reach back many releases on main.
#
# Usage:
#   changelog-range.sh [--ref <ref>]
# ============================================================================

set -euo pipefail

REF="HEAD"

usage() {
    echo "Usage: $0 [--ref <ref>]" >&2
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

# Print the newest stable (non-RC) release tag. Extra arguments are passed to
# `git tag --list`, so callers can restrict it to tags reachable from a ref.
newest_stable_tag() {
    local tag
    while IFS= read -r tag; do
        if [[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            printf '%s\n' "${tag}"
            return 0
        fi
    done < <(git tag --list "v*" --sort=-version:refname "$@")
    return 1
}

main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ref)
                [[ $# -ge 2 && -n "${2}" ]] || fail "--ref requires a value"
                REF="$2"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *)
                fail "unknown option: $1"
                ;;
        esac
    done

    git rev-parse --verify --quiet "${REF}^{commit}" >/dev/null ||
        fail "unknown ref: ${REF}"

    local newest boundary reachable candidate
    newest="$(newest_stable_tag || true)"
    [[ -n "${newest}" ]] || fail "no stable Radius release tag was found"

    if git merge-base --is-ancestor "${newest}" "${REF}"; then
        boundary="$(git rev-parse "${newest}^{commit}")"
        echo "Rendering changes since ${newest}." >&2
    else
        boundary="$(git merge-base "${newest}" "${REF}")"
        echo "Rendering changes since ${newest} branched at ${boundary}." >&2

        reachable="$(newest_stable_tag --merged "${REF}" || true)"
        if [[ -n "${reachable}" ]]; then
            candidate="$(git rev-parse "${reachable}^{commit}")"
            if git merge-base --is-ancestor "${boundary}" "${candidate}"; then
                boundary="${candidate}"
                echo "Using ${reachable}, the newest release on ${REF}." >&2
            fi
        fi
    fi

    printf '%s..%s\n' "${boundary}" "${REF}"
}

main "$@"
