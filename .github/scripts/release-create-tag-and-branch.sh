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
#   - An existing release branch must contain the planned commit.
#   - An absent tag is created and pushed by name, never with --tags.
#   - A tag at the planned commit is success; any other target is a conflict.
#   - Every destination is re-read from the remote after it is mutated.
#
# Re-running against an already reconciled repository is a no-op, so a release
# that failed part way through can be resumed.

set -euo pipefail

CHECK_ONLY="${RELEASE_RECONCILE_CHECK_ONLY:-false}"

usage() {
    cat <<EOF
Usage: $0 <repository-directory> <tag-name> <release-branch-name> [planned-commit]

Example: $0 radius v0.61.0 release/0.61 abc123
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

    refs="$(git ls-remote origin "refs/tags/${tag}" "refs/tags/${tag}^{}")" || return
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
    local refs

    refs="$(git ls-remote origin "refs/heads/${branch}")" || return
    cut -f1 <<<"${refs}"
}

fetch_branch() {
    local branch="$1"

    git fetch --quiet origin "refs/heads/${branch}" ||
        fail "could not fetch release branch ${branch}"
}

# Reconcile the release branch and print the commit that the release tags.
reconcile_branch() {
    local branch="$1"
    local planned_commit="$2"
    local existing release_commit observed

    if ! existing="$(remote_branch_commit "${branch}")"; then
        fail "could not query release branch ${branch}"
    fi
    if [[ -n "${existing}" ]]; then
        fetch_branch "${branch}"
        existing="$(git rev-parse FETCH_HEAD)"
        if [[ -n "${planned_commit}" ]] &&
            ! git merge-base --is-ancestor "${planned_commit}" "${existing}"; then
            fail "planned commit ${planned_commit} is not reachable from release branch ${branch} at ${existing}"
        fi

        if ! observed="$(remote_branch_commit "${branch}")"; then
            fail "could not verify release branch ${branch}"
        fi
        [[ "${observed}" == "${existing}" ]] ||
            fail "release branch ${branch} changed from ${existing} to ${observed} during reconciliation"

        release_commit="${planned_commit:-${existing}}"
        echo "Release branch ${branch} already exists at ${existing}; release commit is ${release_commit}" >&2
        printf '%s' "${release_commit}"
        return
    fi

    release_commit="${planned_commit:-$(git rev-parse HEAD)}"
    if [[ "${CHECK_ONLY}" == "true" ]]; then
        echo "Release branch ${branch} is absent and can be created at ${release_commit}" >&2
        printf '%s' "${release_commit}"
        return
    fi
    echo "Creating release branch ${branch} at ${release_commit}" >&2
    if ! git push --force-with-lease="refs/heads/${branch}:" \
        origin "${release_commit}:refs/heads/${branch}"; then
        if ! observed="$(remote_branch_commit "${branch}")"; then
            fail "could not query release branch ${branch} after push failure"
        fi
        if [[ -z "${observed}" ]]; then
            fail "could not create release branch ${branch}; planned commit ${release_commit} is not reachable from ${observed:-<missing>}"
        fi
        fetch_branch "${branch}"
        if ! git merge-base --is-ancestor "${release_commit}" FETCH_HEAD; then
            fail "could not create release branch ${branch}; planned commit ${release_commit} is not reachable from ${observed}"
        fi
        echo "Release branch ${branch} appeared concurrently at ${observed} and contains ${release_commit}" >&2
        printf '%s' "${release_commit}"
        return
    fi

    if ! observed="$(remote_branch_commit "${branch}")"; then
        fail "could not verify release branch ${branch}"
    fi
    [[ -n "${observed}" ]] ||
        fail "release branch ${branch} is missing after push"
    fetch_branch "${branch}"
    if ! git merge-base --is-ancestor "${release_commit}" FETCH_HEAD; then
        fail "release branch ${branch} does not contain planned commit ${release_commit} after push; remote is at ${observed:-<missing>}"
    fi

    printf '%s' "${release_commit}"
}

reconcile_tag() {
    local tag="$1"
    local release_commit="$2"
    local existing observed

    if ! existing="$(remote_tag_commit "${tag}")"; then
        fail "could not query tag ${tag}"
    fi
    if [[ -n "${existing}" ]]; then
        [[ "${existing}" == "${release_commit}" ]] ||
            fail "tag ${tag} points at ${existing}, expected planned commit ${release_commit}; refusing to move an existing tag"
        echo "Tag ${tag} already released at ${existing}"
        return
    fi

    if [[ "${CHECK_ONLY}" == "true" ]]; then
        echo "Tag ${tag} is absent and can be created at ${release_commit}"
        return
    fi

    echo "Creating tag ${tag} at ${release_commit}"
    if ! git push --force-with-lease="refs/tags/${tag}:" \
        origin "${release_commit}:refs/tags/${tag}"; then
        if ! observed="$(remote_tag_commit "${tag}")"; then
            fail "could not query tag ${tag} after push failure"
        fi
        [[ "${observed}" == "${release_commit}" ]] ||
            fail "could not create tag ${tag}; remote is at ${observed:-<missing>}, expected ${release_commit}"
        echo "Tag ${tag} appeared concurrently at ${observed}"
        return
    fi

    if ! observed="$(remote_tag_commit "${tag}")"; then
        fail "could not verify tag ${tag}"
    fi
    [[ "${observed}" == "${release_commit}" ]] ||
        fail "tag ${tag} is at ${observed:-<missing>} after push, expected ${release_commit}"
}

main() {
    local repository="${1:-}"
    local tag="${2:-}"
    local branch="${3:-}"
    local planned_commit="${4:-}"
    local release_commit

    if [[ -z "${repository}" || -z "${tag}" || -z "${branch}" ]]; then
        usage >&2
        fail "repository directory, tag name, and release branch name are required"
    fi
    [[ -d "${repository}" ]] || fail "repository directory not found: ${repository}"
    [[ "${CHECK_ONLY}" =~ ^(true|false)$ ]] ||
        fail "RELEASE_RECONCILE_CHECK_ONLY must be true or false"

    cd "${repository}"

    if [[ -z "${planned_commit}" ]]; then
        if ! planned_commit="$(remote_tag_commit "${tag}")"; then
            fail "could not query tag ${tag}"
        fi
    fi
    if [[ -n "${planned_commit}" ]]; then
        if ! git cat-file -e "${planned_commit}^{commit}" 2>/dev/null; then
            git fetch --quiet origin "${planned_commit}"
        fi
        planned_commit="$(git rev-parse "${planned_commit}^{commit}")"
    fi

    echo "Reconciling ${tag} on ${branch} for ${repository}"
    release_commit="$(reconcile_branch "${branch}" "${planned_commit}")"
    reconcile_tag "${tag}" "${release_commit}"
}

main "$@"
