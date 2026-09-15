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

# Renders cliff.toml against a throwaway repository that holds one commit of
# every kind the pull request title policy allows, plus a legacy subject, and
# checks that each one lands in the section the release process documents.
# The render is offline, so the GitHub integration is not exercised here; the
# changelog-preview workflow covers that on real history.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly CONFIG="${REPO_ROOT}/cliff.toml"
GIT_CLIFF="${GIT_CLIFF:-git-cliff}"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly FIXTURE="${TEST_ROOT}/repo"
readonly UNRELEASED="${TEST_ROOT}/unreleased.md"
readonly RELEASE="${TEST_ROOT}/release.md"

fail() {
    echo "Error: $*" >&2
    exit 1
}

command -v "${GIT_CLIFF}" >/dev/null 2>&1 ||
    fail "git-cliff not found; run 'make install-git-cliff' or set GIT_CLIFF"
[[ -f "${CONFIG}" ]] || fail "missing ${CONFIG}"

commit() {
    git -C "${FIXTURE}" commit -q --allow-empty -m "$1"
}

build_fixture() {
    mkdir -p "${FIXTURE}"
    git -C "${FIXTURE}" init -q -b main
    git -C "${FIXTURE}" config user.name "Radius Test"
    git -C "${FIXTURE}" config user.email "test@example.com"
    git -C "${FIXTURE}" config commit.gpgsign false

    commit "initial"
    git -C "${FIXTURE}" tag v0.60.0
    commit "feat(cli): add recipe validation (#101)"
    commit "fix: preserve resource status (#102)"
    commit "perf: speed up graph walks (#103)"
    commit "refactor(api)!: remove the legacy response field (#104)"
    commit "chore!: drop the python build path (#105)"
    commit "chore(deps): bump the go-deps group (#106)"
    commit "ci(deps): bump the github-actions group (#107)"
    commit "deps: bump helm (#108)"
    commit "revert: revert the graph walk change (#109)"
    commit "docs: update the guide (#110)"
    commit "test: add tests (#111)"
    commit "build: tweak make (#112)"
    commit "ci: tweak a workflow (#113)"
    commit "chore: tidy (#114)"
    commit "Legacy subject without a type (#115)"
}

# Render the fixture's unreleased range with the repository configuration.
render() {
    (
        cd "${FIXTURE}" &&
            env -u GITHUB_TOKEN "${GIT_CLIFF}" --config "${CONFIG}" --offline \
                "$@" v0.60.0..HEAD 2>/dev/null
    )
}

# Print the entries under one "### <heading>" section of a rendered file.
section() {
    awk -v heading="### $1" '
        $0 == heading { in_section = 1; next }
        /^### / { in_section = 0 }
        in_section && /^- / { print }
    ' "$2"
}

expect_under() {
    local heading="$1"
    local needle="$2"
    local file="$3"

    section "${heading}" "${file}" | grep -Fq -- "${needle}" ||
        fail "expected '${needle}' under '${heading}'"
}

expect_absent() {
    local needle="$1"
    local file="$2"

    if grep -Fq -- "${needle}" "${file}"; then
        fail "'${needle}' must not be rendered"
    fi
}

build_fixture

render --strip all >"${UNRELEASED}"
[[ -s "${UNRELEASED}" ]] || fail "git-cliff rendered nothing"

# Sections appear in the documented order and nothing else is rendered.
expected_headings="$(printf '%s\n' "### Breaking changes" "### Added" \
    "### Fixed" "### Changed" "### Dependencies" "### Reverted changes" \
    "### Other changes")"
actual_headings="$(grep '^### ' "${UNRELEASED}")"
[[ "${actual_headings}" == "${expected_headings}" ]] ||
    fail "unexpected section order:"$'\n'"${actual_headings}"

expect_under "Breaking changes" "Remove the legacy response field (#104)" "${UNRELEASED}"
# protect_breaking_commits keeps a breaking commit whose type is otherwise skipped.
expect_under "Breaking changes" "Drop the python build path (#105)" "${UNRELEASED}"
expect_under "Added" "Add recipe validation (#101)" "${UNRELEASED}"
expect_under "Fixed" "Preserve resource status (#102)" "${UNRELEASED}"
expect_under "Changed" "Speed up graph walks (#103)" "${UNRELEASED}"
expect_under "Dependencies" "Bump the go-deps group (#106)" "${UNRELEASED}"
expect_under "Dependencies" "Bump the github-actions group (#107)" "${UNRELEASED}"
expect_under "Dependencies" "Bump helm (#108)" "${UNRELEASED}"
expect_under "Reverted changes" "Revert the graph walk change (#109)" "${UNRELEASED}"
expect_under "Other changes" "Legacy subject without a type (#115)" "${UNRELEASED}"

# Excluded types never reach the changelog.
for excluded in "(#110)" "(#111)" "(#112)" "(#113)" "(#114)"; do
    expect_absent "${excluded}" "${UNRELEASED}"
done

# A breaking commit is listed once, under Breaking changes only.
[[ "$(grep -c '(#104)' "${UNRELEASED}")" -eq 1 ]] ||
    fail "a breaking commit was rendered more than once"

# A tagged render carries the Keep a Changelog heading and comparison link.
render --tag v0.61.0 >"${RELEASE}"
grep -Eq '^## \[0\.61\.0\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$' "${RELEASE}" ||
    fail "release heading is missing or has no ISO date"
grep -Fq '[0.61.0]: https://github.com/radius-project/radius/compare/v0.60.0...v0.61.0' "${RELEASE}" ||
    fail "release comparison link is missing"
grep -Fq '<!-- generated by git-cliff -->' "${RELEASE}" ||
    fail "generated-by marker is missing"

echo "changelog configuration tests passed"
