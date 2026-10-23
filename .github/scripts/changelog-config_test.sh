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
# Offline GitHub context fixtures also check contributor rendering without
# requiring API access; changelog-preview covers fetching real metadata.

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
readonly CONTEXT="${TEST_ROOT}/context.json"
readonly LINKED="${TEST_ROOT}/linked.md"

fail() {
    echo "Error: $*" >&2
    exit 1
}

command -v "${GIT_CLIFF}" >/dev/null 2>&1 ||
    fail "git-cliff not found; run 'make install-git-cliff' or set GIT_CLIFF"
command -v jq >/dev/null 2>&1 ||
    fail "jq not found; run 'make install-jq'"
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
    git -C "${FIXTURE}" config tag.gpgsign false

    commit "initial"
    git -C "${FIXTURE}" tag v0.60.0
    commit "feat(cli): add recipe validation (#101)"
    commit "fix: preserve resource status (#102)"
    commit "perf: speed up graph walks (#103)"
    commit "refactor(api)!: remove the legacy response field (#104)"
    commit "chore!: drop the python build path (#105)"
    commit "chore(deps): bump the go-deps group (#106)"
    commit "ci(deps): bump the github-actions group (#107)"
    commit "style: format shell scripts (#108)"
    commit "revert: revert the graph walk change (#109)"
    commit "docs: update the guide (#110)"
    commit "test: add tests (#111)"
    commit "build: tweak make (#112)"
    commit "ci: tweak a workflow (#113)"
    commit "chore: tidy (#114)"
    commit "Legacy subject without a type (#115)"
    commit 'fix: keep [@linked](https://github.com/linked) intact (#116)'
    commit "fix: keep dev@example.com, \`@code\`, @scope/pkg and pkg@v2 (#117)"
    git -C "${FIXTURE}" tag legacy-start
    commit "Preserve legacy client behavior (#118)"
    for type in feat fix perf refactor revert docs style test build ci chore; do
        commit "${type}(legacy):"
    done
    commit "chore(legacy)!:"
    commit "ci(deps) update legacy automation (#119)"
}

render() {
    local range="$1"
    shift

    (
        cd "${FIXTURE}" &&
            env -u GITHUB_TOKEN "${GIT_CLIFF}" --config "${CONFIG}" --offline \
                "$@" "${range}"
    )
}

render_context() {
    env -u GITHUB_TOKEN "${GIT_CLIFF}" --config "${CONFIG}" --offline \
        --from-context "$1" --strip all
}

# Print the entries under one "### <heading>" section of a rendered file.
section() {
    awk -v heading="### $1" '
        $0 == heading { in_section = 1; next }
        /^### / { in_section = 0 }
        in_section && NF { print }
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

render v0.60.0..HEAD --strip all >"${UNRELEASED}"
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
expect_under "Reverted changes" "Revert the graph walk change (#109)" "${UNRELEASED}"
expect_under "Other changes" "Legacy subject without a type (#115)" "${UNRELEASED}"

render legacy-start..HEAD --strip all >"${TEST_ROOT}/legacy.md"
[[ "$(grep '^### ' "${TEST_ROOT}/legacy.md")" == "### Other changes" ]] ||
    fail "nonconventional-only history must use the fallback group"
while IFS= read -r subject; do
    for output in "${UNRELEASED}" "${TEST_ROOT}/legacy.md"; do
        expect_under "Other changes" "${subject^}" "${output}"
        [[ "$(grep -Fc -- "- ${subject^}" "${output}")" -eq 1 ]] ||
            fail "nonconventional commit must appear exactly once: ${subject}"
    done
done < <(git -C "${FIXTURE}" log --format=%s legacy-start..HEAD)

# Excluded types never reach the changelog.
for excluded in "(#108)" "(#110)" "(#111)" "(#112)" "(#113)" "(#114)"; do
    expect_absent "${excluded}" "${UNRELEASED}"
done

# A breaking commit is listed once, under Breaking changes only.
[[ "$(grep -c '(#104)' "${UNRELEASED}")" -eq 1 ]] ||
    fail "a breaking commit was rendered more than once"

# A tagged render carries the Keep a Changelog heading and comparison link.
render v0.60.0..HEAD --tag v0.61.0 >"${RELEASE}"
grep -Eq '^## \[0\.61\.0\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$' "${RELEASE}" ||
    fail "release heading is missing or has no ISO date"
grep -Fq '[0.61.0]: https://github.com/radius-project/radius/compare/v0.60.0...v0.61.0' "${RELEASE}" ||
    fail "release comparison link is missing"
grep -Fq '<!-- generated by git-cliff -->' "${RELEASE}" ||
    fail "generated-by marker is missing"

render v0.60.0..HEAD --context | jq '
    .[0].commits |= map(.github += (
        if .message | endswith("(#101)") then
            {username: "merger", pr_author: "DariuszPorowski", pr_number: 101}
        elif .message | endswith("(#102)") then
            {username: "fallback-author", pr_number: 102}
        elif .message | endswith("(#104)") then
            {username: "merger", pr_author: "DariuszPorowski", pr_number: 104}
        elif .message | endswith("(#106)") then
            {username: "dependabot[bot]", pr_number: 106}
        elif .message | endswith("(#115)") then
            {username: "itpropro", pr_number: 115}
        else {} end
    ) | .remote = .github) |
    .[0].github.contributors = ([
        {username: "DariuszPorowski", pr_number: 101, is_first_time: false},
        {username: "dependabot[bot]", pr_number: 106, is_first_time: false},
        {username: "itpropro", pr_number: 115, is_first_time: true},
        {username: "fallback-author", pr_number: 102, is_first_time: false}
    ] | map(. + {pr_labels: []}))
' >"${CONTEXT}"
render_context "${CONTEXT}" >"${LINKED}"

profile='[@DariuszPorowski](https://github.com/DariuszPorowski)'
bot='[@dependabot[bot]](https://github.com/apps/dependabot)'
new_contributor='[@itpropro](https://github.com/itpropro)'
fallback='[@fallback-author](https://github.com/fallback-author)'
pulls='https://github.com/radius-project/radius/pull'
expect_under "Added" \
    "Add recipe validation by ${profile} in [#101](${pulls}/101)" "${LINKED}"
expect_under "Breaking changes" \
    "Remove the legacy response field by ${profile}" "${LINKED}"
expect_under "Fixed" \
    "Preserve resource status by ${fallback}" "${LINKED}"
expect_under "Dependencies" \
    "Bump the go-deps group by ${bot}" "${LINKED}"
expect_absent 'by @' "${LINKED}"
expect_absent '[@merger]' "${LINKED}"
expect_absent '(#101)' "${LINKED}"
expect_under "Fixed" \
    'Keep [@linked](https://github.com/linked) intact (#116)' "${LINKED}"
expect_under "Fixed" \
    "Keep dev@example.com, \`@code\`, @scope/pkg and pkg@v2 (#117)" "${LINKED}"

expected="${profile}, ${bot}, ${new_contributor}, ${fallback}"
[[ "$(section "Contributors" "${LINKED}")" == "${expected}" ]] ||
    fail "contributors must be one comma-separated line in metadata order"
expected="- ${new_contributor} made their first contribution"
expected+=" in [#115](${pulls}/115)"
[[ "$(section "New contributors" "${LINKED}")" == "${expected}" ]] ||
    fail "new contributors must keep their linked first contribution"

render_context "${CONTEXT}" >"${TEST_ROOT}/rerendered.md"
cmp "${LINKED}" "${TEST_ROOT}/rerendered.md" ||
    fail "rendering the same metadata must be deterministic"

jq '
    .[0].github.contributors |=
        [{username: null, pr_labels: [], is_first_time: true}] + .[:2] +
        [{pr_labels: [], is_first_time: false}] + .[2:] +
        [{username: null, pr_labels: [], is_first_time: false}]
' "${CONTEXT}" >"${TEST_ROOT}/missing-usernames.json"
render_context "${TEST_ROOT}/missing-usernames.json" \
    >"${TEST_ROOT}/missing-usernames.md"
cmp "${LINKED}" "${TEST_ROOT}/missing-usernames.md" ||
    fail "missing usernames must not change links, separators, or headings"

jq '.[0].github.contributors |= map(select(.username == null))' \
    "${TEST_ROOT}/missing-usernames.json" >"${TEST_ROOT}/anonymous.json"
render_context "${TEST_ROOT}/anonymous.json" >"${TEST_ROOT}/anonymous.md"
expect_absent '### Contributors' "${TEST_ROOT}/anonymous.md"
expect_absent '### New contributors' "${TEST_ROOT}/anonymous.md"
expect_under "Added" \
    "Add recipe validation by ${profile}" "${TEST_ROOT}/anonymous.md"

jq '.[0].github.contributors = []' "${CONTEXT}" \
    >"${TEST_ROOT}/empty.json"
render_context "${TEST_ROOT}/empty.json" >"${TEST_ROOT}/empty.md"
cmp "${TEST_ROOT}/anonymous.md" "${TEST_ROOT}/empty.md" ||
    fail "empty and anonymous contributor lists must render identically"

jq '.[0].github.contributors |= map(
    select(.username == "dependabot[bot]") | .is_first_time = true
)' "${CONTEXT}" >"${TEST_ROOT}/bot.json"
render_context "${TEST_ROOT}/bot.json" >"${TEST_ROOT}/bot.md"
[[ "$(section "Contributors" "${TEST_ROOT}/bot.md")" == "${bot}" ]] ||
    fail "a bot-only contributor list must retain the app profile link"
expect_under "New contributors" \
    "- ${bot} made their first contribution in [#106](${pulls}/106)" \
    "${TEST_ROOT}/bot.md"

jq '.[0].github.contributors |= .[:1]' "${CONTEXT}" \
    >"${TEST_ROOT}/single.json"
render_context "${TEST_ROOT}/single.json" >"${LINKED}"
[[ "$(section "Contributors" "${LINKED}")" == "${profile}" ]] ||
    fail "a single contributor must have no comma or bullet"
expect_absent '### New contributors' "${LINKED}"

git -C "${FIXTURE}" tag v0.61.0
commit "fix: preserve later release ordering (#120)"
render v0.60.0..HEAD --tag v0.62.0 >"${TEST_ROOT}/history.md"
expected="$(printf '%s\n' '[0.62.0]' '[0.61.0]')"
[[ "$(awk '/^## \[/ { print $2 }' "${TEST_ROOT}/history.md")" == "${expected}" ]] ||
    fail "release sections must put the newest version first"
grep -Fq '[0.62.0]: https://github.com/radius-project/radius/compare/v0.61.0...v0.62.0' \
    "${TEST_ROOT}/history.md" || fail "later release comparison is missing"

echo "changelog configuration tests passed"
