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
readonly SCRIPT="${BACKPORT_SCRIPT_DIR:-${SCRIPT_DIR}}/collect-release-backports.sh"

TEST_ROOT=""
PASS=0
FAIL=0

cleanup() {
    if [[ -n "${TEST_ROOT}" && -d "${TEST_ROOT}" ]]; then
        rm -rf "${TEST_ROOT}"
    fi
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

setup_fake_gh() {
    cat > "${TEST_ROOT}/gh" << 'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$1" == api ]]; then
    case "$2" in
        */git/matching-refs/heads/release/0.60)
            if [[ "${MISSING_RELEASE_BRANCH:-false}" == true ]]; then
                printf '[]\n'
            else
                printf '%s\n' '[{"ref":"refs/heads/release/0.60","object":{"sha":"release-head"}}]'
            fi
            ;;
        */compare/source-102...release-head*)
            if [[ "${FAIL_COMPARISON:-false}" == true ]]; then
                echo 'injected ancestry lookup failure' >&2
                exit 42
            fi
            printf '%s\n' "${SOURCE_102_RELATION:-diverged}"
            ;;
        */compare/*...release-head*) printf 'diverged\n' ;;
        *) echo "Unexpected API request: $2" >&2; exit 1 ;;
    esac
    exit 0
fi

if [[ "$1 $2" == "pr view" && " $* " == *" --json commits "* ]]; then
    if [[ -n "${COMMIT_LOOKUPS:-}" ]]; then
        printf '%s\n' "$3" >> "${COMMIT_LOOKUPS}"
    fi
    case "$3" in
        201) printf '%s\n' '[{"messageBody":"(cherry picked from commit source-101)"}]' ;;
        202) printf '[]\n' ;;
        204) printf '%s\n' '[{"messageBody":"conflict handoff only"}]' ;;
        205) printf '%s\n' '[{"messageBody":"(cherry picked from commit source-105)"}]' ;;
        *) echo "Unexpected commit lookup: $3" >&2; exit 1 ;;
    esac
    exit 0
fi

if [[ "$1 $2" == "pr view" ]]; then
    case "$3" in
        102)
            printf '%s\n' '{"number":102,"title":"feat: explicit",'\
'"url":"https://example.test/102","mergeCommit":{"oid":"source-102"},'\
'"mergedAt":"2026-08-20T00:00:00Z","baseRefName":"main"}'
            ;;
        103)
            printf '%s\n' '{"number":103,"title":"fix: open",'\
'"url":"https://example.test/103","mergeCommit":null,'\
'"mergedAt":null,"baseRefName":"main"}'
            ;;
        104)
            printf '%s\n' '{"number":104,"title":"fix: placeholder",'\
'"url":"https://example.test/104","mergeCommit":{"oid":"source-104"},'\
'"mergedAt":"2026-08-20T00:00:00Z","baseRefName":"main"}'
            ;;
        105)
            printf '%s\n' '{"number":105,"title":"fix: ambiguous",'\
'"url":"https://example.test/105","mergeCommit":{"oid":"source-105"},'\
'"mergedAt":"2026-08-20T00:00:00Z","baseRefName":"main"}'
            ;;
    esac
    exit 0
fi

base=""
fields=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --base) base="$2"; shift 2 ;;
        --json) fields="$2"; shift 2 ;;
        *) shift ;;
    esac
done

if [[ "${base}" == "main" ]]; then
    if [[ "${NO_LABELED:-false}" == true ]]; then
        printf '[]\n'
        exit 0
    fi
    printf '%s\n' '[{"number":101,"title":"fix: labeled",'\
'"url":"https://example.test/101","mergeCommit":{"oid":"source-101"},'\
'"mergedAt":"2026-08-19T00:00:00Z","baseRefName":"main"}]'
else
    # GitHub rejects commits on a page of pull requests: 100 pull requests x
    # 100 commits x 100 authors exceeds its GraphQL node limit.
    if [[ ",${fields}," == *",commits,"* ]]; then
        echo 'GraphQL: requesting up to 1,000,000 possible nodes which exceeds the maximum limit of 500,000.' >&2
        exit 1
    fi
    printf '%s\n' '[{"number":201,"url":"https://example.test/201",'\
'"body":"<!-- radius-backport-source: #101 -->",'\
'"state":"MERGED","mergedAt":"2026-08-21T00:00:00Z",'\
'"mergeCommit":{"oid":"backport-101"}},'\
'{"number":202,"url":"https://example.test/202",'\
'"body":"<!-- radius-backport-source: #102 -->",'\
'"state":"OPEN","mergedAt":null,"mergeCommit":null},'\
'{"number":204,"url":"https://example.test/204",'\
'"body":"<!-- radius-backport-source: #104 -->",'\
'"state":"MERGED","mergedAt":"2026-08-21T00:00:00Z",'\
'"mergeCommit":{"oid":"placeholder-104"}},'\
'{"number":205,"url":"https://example.test/205",'\
'"body":"<!-- radius-backport-source: #105 -->\n'\
'<!-- radius-backport-source: #999 -->",'\
'"state":"MERGED","mergedAt":"2026-08-21T00:00:00Z",'\
'"mergeCommit":{"oid":"backport-105"}}]'
fi
EOF
    chmod +x "${TEST_ROOT}/gh"
}

test_collects_labeled_and_explicit_prs() {
    local output="${TEST_ROOT}/backports.json"

    GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 \
        --explicit-prs '102,104' --output "${output}"

    if [[ "$(jq 'length' "${output}")" != "3" ]]; then
        fail_test "expected three selected pull requests"
        return
    fi
    if [[ "$(jq -c '[.[].source_pr]' "${output}")" != "[104,101,102]" ]]; then
        fail_test "selected PRs must follow main history, not input or PR-number order"
        return
    fi
    if [[ "$(jq -r '.[] | select(.source_pr == 101) | .backport_merged' \
        "${output}")" != "true" ]]; then
        fail_test "labeled pull request should have a merged backport"
        return
    fi
    if [[ "$(jq -r '.[] | select(.source_pr == 102) | .backport_merged' \
        "${output}")" != "false" ]]; then
        fail_test "explicit pull request should report its open backport"
        return
    fi
    if [[ "$(jq -r '.[] | select(.source_pr == 104) | .backport_merged' \
        "${output}")" != "false" ]]; then
        fail_test "merged conflict placeholder must not satisfy the backport"
        return
    fi
    ((++PASS))
}

test_collects_labels_without_explicit_prs() {
    local output="${TEST_ROOT}/labeled-only.json"

    GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 --output "${output}"
    if ! jq -e 'length == 1 and .[0].source_pr == 101' "${output}" > /dev/null; then
        fail_test "labeled fixes must be collected without explicit PR numbers"
        return
    fi
    ((++PASS))
}

test_already_present_sources_are_satisfied() {
    local relation="$1"
    local expected="$2"
    local output="${TEST_ROOT}/present-${relation}.json"

    SOURCE_102_RELATION="${relation}" GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 \
        --explicit-prs 102 --output "${output}"
    if ! jq -e --argjson expected "${expected}" '
        .[] | select(.source_pr == 102) |
        .backport_merged == $expected and .already_present == $expected
    ' "${output}" > /dev/null; then
        fail_test "source ancestry ${relation} produced incorrect completion state"
        return
    fi
    if [[ "${expected}" == true ]] && ! jq -e '
        .[] | select(.source_pr == 102) |
        .backport_pr == null and .backport_commit == "source-102"
    ' "${output}" > /dev/null; then
        fail_test "direct inclusion must bind completion to the original source commit"
        return
    fi
    ((++PASS))
}

test_missing_release_branch_does_not_satisfy_sources() {
    local output="${TEST_ROOT}/missing-branch.json"

    MISSING_RELEASE_BRANCH=true SOURCE_102_RELATION=ahead GH="${TEST_ROOT}/gh" \
        bash "${SCRIPT}" --repository radius-project/radius --channel 0.60 \
        --explicit-prs 102 --output "${output}"
    if [[ "$(jq -r '.[] | select(.source_pr == 102) | .already_present' "${output}")" != false ]]; then
        fail_test "a missing release branch cannot contain the source"
        return
    fi
    ((++PASS))
}

test_ancestry_lookup_failure_stops_collection() {
    if FAIL_COMPARISON=true GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 --explicit-prs 102 \
        --output "${TEST_ROOT}/failed-comparison.json" > /dev/null 2>&1; then
        fail_test "an ancestry API failure must not be treated as completion"
        return
    fi
    if [[ -e "${TEST_ROOT}/failed-comparison.json" ]]; then
        fail_test "failed ancestry lookup must not emit a backport plan"
        return
    fi
    ((++PASS))
}

test_rejects_unmerged_explicit_pr() {
    if GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 \
        --explicit-prs '103' --output "${TEST_ROOT}/invalid.json" \
        > /dev/null 2>&1; then
        fail_test "expected an unmerged explicit pull request to fail"
        return
    fi
    ((++PASS))
}

test_ambiguous_backport_body_is_not_trusted() {
    local output="${TEST_ROOT}/ambiguous.json"

    GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 \
        --explicit-prs '105' --output "${output}"

    if [[ "$(jq -r '.[] | select(.source_pr == 105) | .backport_merged' \
        "${output}")" != "false" ]]; then
        fail_test "a body naming two sources must not satisfy the backport"
        return
    fi
    ((++PASS))
}

test_reads_commits_only_for_backport_candidates() {
    local lookups="${TEST_ROOT}/commit-lookups.txt"

    : > "${lookups}"
    COMMIT_LOOKUPS="${lookups}" GH="${TEST_ROOT}/gh" bash "${SCRIPT}" \
        --repository radius-project/radius --channel 0.60 \
        --explicit-prs '102,104,105' --output "${TEST_ROOT}/candidates.json"
    if [[ "$(sort -n "${lookups}" | paste -sd, -)" != "201,202,204" ]]; then
        fail_test "commits must be read only for single-source backport candidates"
        return
    fi
    ((++PASS))
}

test_release_without_selected_fixes_reads_no_commits() {
    local lookups="${TEST_ROOT}/first-rc-lookups.txt"
    local output="${TEST_ROOT}/first-rc.json"

    : > "${lookups}"
    NO_LABELED=true COMMIT_LOOKUPS="${lookups}" GH="${TEST_ROOT}/gh" \
        bash "${SCRIPT}" --repository radius-project/radius --channel 0.60 \
        --output "${output}"
    if [[ -s "${lookups}" ]] || ! jq -e '. == []' "${output}" > /dev/null; then
        fail_test "a release without selected fixes must not read pull request commits"
        return
    fi
    ((++PASS))
}

main() {
    TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/collect-backports-test-XXXXXX")"
    setup_fake_gh
    cat > "${TEST_ROOT}/git" << 'EOF'
#!/bin/bash
set -euo pipefail
[[ "$*" == "rev-list --first-parent --reverse refs/remotes/origin/main" ]] || {
    echo "Unexpected git arguments: $*" >&2
    exit 1
}
printf '%s\n' source-104 source-101 source-102 source-105
EOF
    chmod +x "${TEST_ROOT}/git"
    export PATH="${TEST_ROOT}:${PATH}"

    test_collects_labeled_and_explicit_prs
    test_collects_labels_without_explicit_prs
    test_already_present_sources_are_satisfied ahead true
    test_already_present_sources_are_satisfied identical true
    test_already_present_sources_are_satisfied behind false
    test_already_present_sources_are_satisfied diverged false
    test_missing_release_branch_does_not_satisfy_sources
    test_ancestry_lookup_failure_stops_collection
    test_rejects_unmerged_explicit_pr
    test_ambiguous_backport_body_is_not_trusted
    test_reads_commits_only_for_backport_candidates
    test_release_without_selected_fixes_reads_no_commits

    if ((FAIL > 0)); then
        echo "collect release backports tests failed: ${PASS} passed, ${FAIL} failed"
        exit 1
    fi

    echo "collect release backports tests passed (${PASS} tests)"
}

main "$@"
