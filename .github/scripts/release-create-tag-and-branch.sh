#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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

# Reconcile the release branch and release tag for one repository.
#
# Reconciliation rules, from the release lifecycle design:
#   - An absent release branch is created at the checked-out commit.
#   - An existing release branch is adopted and its head is the release commit.
#   - An absent tag is created and pushed by name, never with --tags.
#   - A tag already reachable from the release branch is success.
#   - A tag anywhere else is a conflict; it is never moved or force-pushed.
#   - Every destination is re-read from the remote after it is mutated.
#
# Re-running against an already reconciled repository is a no-op, so a release
# that failed part way through can be resumed.

set -euo pipefail

usage() {
    cat <<EOF
Usage: $0 <repository-directory> <tag-name> <release-branch-name>

Example: $0 radius v0.61.0 release/0.61
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

# Commit a remote tag resolves to, dereferencing annotated tags to their commit.
remote_tag_commit() {
    local tag="$1"
    local refs sha ref direct="" peeled=""

    refs="$(git ls-remote origin "refs/tags/${tag}" "refs/tags/${tag}^{}")"
    while read -r sha ref; do
        case "${ref}" in
            "refs/tags/${tag}^{}") peeled="${sha}" ;;
            "refs/tags/${tag}") direct="${sha}" ;;
        esac
    done <<<"${refs}"

    printf '%s' "${peeled:-${direct}}"
}

remote_branch_commit() {
    local branch="$1"

    git ls-remote origin "refs/heads/${branch}" | cut -f1
}

# Reconcile the release branch and print the commit that the release tags.
reconcile_branch() {
    local branch="$1"
    local existing release_commit observed

    existing="$(remote_branch_commit "${branch}")"
    if [[ -n "${existing}" ]]; then
        git fetch --quiet origin "refs/heads/${branch}"
        release_commit="$(git rev-parse FETCH_HEAD)"
        echo "Release branch ${branch} already exists at ${release_commit}" >&2
        printf '%s' "${release_commit}"
        return
    fi

    release_commit="$(git rev-parse HEAD)"
    echo "Creating release branch ${branch} at ${release_commit}" >&2
    git push origin "${release_commit}:refs/heads/${branch}"

    observed="$(remote_branch_commit "${branch}")"
    [[ "${observed}" == "${release_commit}" ]] ||
        fail "release branch ${branch} is at ${observed:-<missing>} after push, expected ${release_commit}"

    printf '%s' "${release_commit}"
}

reconcile_tag() {
    local tag="$1"
    local release_commit="$2"
    local existing observed

    existing="$(remote_tag_commit "${tag}")"
    if [[ -n "${existing}" ]]; then
        git fetch --quiet origin "+refs/tags/${tag}:refs/tags/${tag}"
        if ! git merge-base --is-ancestor "${existing}" "${release_commit}"; then
            fail "tag ${tag} points at ${existing}, which is not reachable from the release branch at ${release_commit}; refusing to move an existing tag"
        fi
        echo "Tag ${tag} already released at ${existing}"
        return
    fi

    echo "Creating tag ${tag} at ${release_commit}"
    # --force rewrites only the local ref; the push below is never forced, so a
    # tag that reaches the remote in the meantime still fails the push.
    git tag --force "${tag}" "${release_commit}"
    git push origin "refs/tags/${tag}"

    observed="$(remote_tag_commit "${tag}")"
    [[ "${observed}" == "${release_commit}" ]] ||
        fail "tag ${tag} is at ${observed:-<missing>} after push, expected ${release_commit}"
}

main() {
    local repository="${1:-}"
    local tag="${2:-}"
    local branch="${3:-}"
    local release_commit

    if [[ -z "${repository}" || -z "${tag}" || -z "${branch}" ]]; then
        usage >&2
        fail "repository directory, tag name, and release branch name are required"
    fi
    [[ -d "${repository}" ]] || fail "repository directory not found: ${repository}"

    cd "${repository}"

    echo "Reconciling ${tag} on ${branch} for ${repository}"
    release_commit="$(reconcile_branch "${branch}")"
    reconcile_tag "${tag}" "${release_commit}"
}

main "$@"
