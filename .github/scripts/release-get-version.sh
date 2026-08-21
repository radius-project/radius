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

set -euo pipefail

fail() {
    echo "Error: $*" >&2
    exit 1
}

tag_exists() {
    local repository="$1"
    local tag="$2"
    local status

    set +e
    git -C "${repository}" ls-remote --exit-code origin "refs/tags/${tag}" >/dev/null
    status=$?
    set -e

    case "${status}" in
        0) return 0 ;;
        2) return 1 ;;
        *) fail "could not query tag ${tag} in repository ${repository}" ;;
    esac
}

main() {
    local versions_csv="${1:-}"
    shift || true
    local -a repositories=("$@")
    local -a versions missing
    local version repository version_number stable_version major minor release_channel
    local release_version=""
    local release_branch_name=""

    [[ -n "${versions_csv}" ]] || fail "versions are required"
    [[ "${#repositories[@]}" -gt 0 ]] || fail "at least one repository directory is required"
    [[ -n "${GITHUB_OUTPUT:-}" ]] || fail "GITHUB_OUTPUT is not set"

    for repository in "${repositories[@]}"; do
        [[ -d "${repository}" ]] || fail "repository directory not found: ${repository}"
    done

    IFS=',' read -r -a versions <<<"${versions_csv}"
    for version in "${versions[@]}"; do
        missing=()
        for repository in "${repositories[@]}"; do
            if ! tag_exists "${repository}" "${version}"; then
                missing+=("${repository}")
            fi
        done

        if [[ "${#missing[@]}" -eq 0 ]]; then
            echo "Tag ${version} exists in every release repository. Skipping..."
            continue
        fi
        [[ -z "${release_version}" ]] ||
            fail "updating multiple versions at once is not supported"

        release_version="${version}"
        version_number="${version#v}"
        stable_version="${version_number%%-*}"
        IFS='.' read -r major minor _ <<<"${stable_version}"
        [[ -n "${major}" && -n "${minor}" ]] ||
            fail "version does not contain a major and minor component: ${version}"
        release_branch_name="release/${major}.${minor}"
        echo "Selecting ${version}; tag is missing from: ${missing[*]}"
    done

    [[ -n "${release_version}" ]] || fail "no release version found"

    version_number="${release_version#v}"
    if [[ "${version_number}" == *-rc* ]]; then
        release_channel="${version_number}"
    else
        release_channel="${version_number%.*}"
    fi

    echo "Release version: ${release_version}"
    echo "Release branch name: ${release_branch_name}"
    echo "Release channel: ${release_channel}"
    {
        echo "release-version=${release_version}"
        echo "release-branch-name=${release_branch_name}"
        echo "release-channel=${release_channel}"
    } >>"${GITHUB_OUTPUT}"
}

main "$@"
