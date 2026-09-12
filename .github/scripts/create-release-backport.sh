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

SOURCE_PR=""
SOURCE_COMMIT=""
SOURCE_TITLE=""
SOURCE_URL=""
CHANNEL=""
OUTPUT_DIR=""
EXPECTED_BASE=""

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: create-release-backport.sh --source-pr <number> --source-commit <sha> \
    --source-title <title> --source-url <url> --channel <X.Y> \
    --output-dir <path> [--expected-base <sha>]
EOF
}

write_outputs() {
    local status="$1"
    local branch="$2"
    local body_file="$3"
    local commit_message_file="${OUTPUT_DIR}/commit-message.txt"

    if [[ "${status}" == "conflict" ]]; then
        printf 'chore(backport): hand off #%s conflict\n' "${SOURCE_PR}" \
            >"${commit_message_file}"
        # The placeholder is the bot's own work, so it keeps the bot as author
        # and the caller substitutes the bot identity for this empty file.
        : >"${OUTPUT_DIR}/author.txt"
    else
        {
            git show -s --format=%B "${SOURCE_COMMIT}"
            echo
            echo "(cherry picked from commit ${SOURCE_COMMIT})"
        } >"${commit_message_file}"
        git show -s --format='%an <%ae>' "${SOURCE_COMMIT}" \
            >"${OUTPUT_DIR}/author.txt"
    fi

    printf '%s\n' "${status}" >"${OUTPUT_DIR}/status.txt"
    printf '%s\n' "${branch}" >"${OUTPUT_DIR}/branch.txt"
    printf '%s\n' "${SOURCE_TITLE}" >"${OUTPUT_DIR}/title.txt"
    printf '%s\n' "${body_file}" >"${OUTPUT_DIR}/body-path.txt"
}

write_pr_body() {
    local body_file="$1"
    local status="$2"
    local release_branch="$3"
    local conflict_file="${4:-}"
    local base_commit="$5"

    {
        echo "<!-- radius-backport-source: #${SOURCE_PR} -->"
        echo "<!-- radius-backport-base: ${base_commit} -->"
        echo "<!-- radius-backport-commit: ${SOURCE_COMMIT} -->"
        echo
        echo "Backport of [#${SOURCE_PR}](${SOURCE_URL})"
        echo "to \`${release_branch}\`."
        echo
        echo "Source commit: \`${SOURCE_COMMIT}\`"
        echo
        if [[ "${status}" == "conflict" ]]; then
            echo "Conflict handoff: \`${conflict_file}\`."
            echo "Follow that file's commands, force-push the resolved branch,"
            echo "and mark this pull request ready for review."
        else
            echo "The source squash commit was cherry-picked with \`-x\`."
            echo "Rebase-merge this pull request to preserve its commit title."
        fi
    } >"${body_file}"
}

is_release_metadata_commit() {
    git diff-tree --no-commit-id --name-only -r "${SOURCE_COMMIT}" -- \
        '.github/release-plans/*.yaml' | grep -q .
}

remove_legacy_release_workflow() {
    local base_commit="$1"

    if is_release_metadata_commit &&
        git cat-file -e "${base_commit}:.github/workflows/release.yaml" \
            2>/dev/null; then
        git rm -q .github/workflows/release.yaml
    fi
}

write_conflict_handoff() {
    local handoff_file="$1"
    local branch="$2"
    local release_branch="$3"
    local base_commit="$4"
    shift 4
    local -a conflicts=("$@")

    mkdir -p "$(dirname "${handoff_file}")"
    {
        echo "# Backport conflict for #${SOURCE_PR}"
        echo
        echo "The automated cherry-pick of \`${SOURCE_COMMIT}\` onto"
        echo "\`${release_branch}\` conflicted in:"
        echo
        printf -- "- \`%s\`\n" "${conflicts[@]}"
        echo
        echo "Resolve the backport with:"
        echo
        echo '```bash'
        printf 'git fetch origin %s \\\n' "${SOURCE_COMMIT}"
        printf '  refs/heads/%s:refs/remotes/origin/%s \\\n' \
            "${branch}" "${branch}"
        printf '  refs/heads/%s:refs/remotes/origin/%s\n' \
            "${release_branch}" "${release_branch}"
        echo "git checkout -B ${branch} origin/${branch}"
        echo "git reset --hard ${base_commit}"
        echo "git cherry-pick -x ${SOURCE_COMMIT}"
        if is_release_metadata_commit &&
            git cat-file -e \
                "${base_commit}:.github/workflows/release.yaml" \
                2>/dev/null; then
            echo "git rm .github/workflows/release.yaml"
        fi
        echo "# Resolve the files listed by git, then:"
        echo "git add <resolved-files>"
        echo "git cherry-pick --continue"
        echo "git push --force-with-lease origin ${branch}"
        echo '```'
        echo
        echo "The hard reset removes this handoff commit before applying the"
        echo "real backport. Do not merge this placeholder commit."
    } >"${handoff_file}"
}

main() {
    local release_branch branch body_file handoff_file base_commit
    local status conflict
    local -a conflicts=()

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --source-pr)
                SOURCE_PR="${2:-}"
                shift 2
                ;;
            --source-commit)
                SOURCE_COMMIT="${2:-}"
                shift 2
                ;;
            --source-title)
                SOURCE_TITLE="${2:-}"
                shift 2
                ;;
            --source-url)
                SOURCE_URL="${2:-}"
                shift 2
                ;;
            --channel)
                CHANNEL="${2:-}"
                shift 2
                ;;
            --output-dir)
                OUTPUT_DIR="${2:-}"
                shift 2
                ;;
            --expected-base)
                EXPECTED_BASE="${2:-}"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *) fail "unknown option: $1" ;;
        esac
    done

    if [[ ! "${SOURCE_PR}" =~ ^[1-9][0-9]*$ ]]; then
        fail "source PR must be a positive number"
    fi
    if ! git rev-parse --verify --quiet \
        "${SOURCE_COMMIT}^{commit}" >/dev/null; then
        fail "source commit does not exist: ${SOURCE_COMMIT}"
    fi
    [[ -n "${SOURCE_TITLE}" ]] || fail "source title is required"
    [[ "${SOURCE_URL}" =~ ^https:// ]] || fail "source URL must use HTTPS"
    if [[ ! "${CHANNEL}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        fail "channel must use X.Y format"
    fi
    [[ -n "${OUTPUT_DIR}" ]] || fail "output directory is required"
    [[ -z "$(git status --porcelain)" ]] || fail "working tree is not clean"

    release_branch="release/${CHANNEL}"
    if ! git rev-parse --verify --quiet \
        "refs/remotes/origin/${release_branch}^{commit}" \
        >/dev/null; then
        fail "remote release branch does not exist: ${release_branch}"
    fi
    base_commit="$(
        git rev-parse "refs/remotes/origin/${release_branch}^{commit}"
    )"
    if [[ -n "${EXPECTED_BASE}" ]]; then
        if [[ ! "${EXPECTED_BASE}" =~ ^[0-9a-f]{40}$ ]]; then
            fail "expected base must be a full commit SHA"
        fi
        if [[ "${base_commit}" != "${EXPECTED_BASE}" ]]; then
            fail "release/${CHANNEL} advanced beyond its approved base"
        fi
        base_commit="${EXPECTED_BASE}"
    fi

    branch="automation/backport-${SOURCE_PR}-to-${CHANNEL}"
    mkdir -p "${OUTPUT_DIR}"
    body_file="${OUTPUT_DIR}/pull-request-body.md"
    git checkout -q --detach "${base_commit}"

    set +e
    git cherry-pick --no-commit "${SOURCE_COMMIT}" >/dev/null 2>&1
    status=$?
    set -e
    if ((status == 0)); then
        remove_legacy_release_workflow "${base_commit}"
        write_pr_body "${body_file}" success "${release_branch}" "" \
            "${base_commit}"
        write_outputs success "${branch}" "${body_file}"
        return
    fi

    while IFS= read -r conflict; do
        [[ -n "${conflict}" ]] && conflicts+=("${conflict}")
    done < <(git diff --name-only --diff-filter=U)
    git cherry-pick --abort >/dev/null 2>&1 || true
    git reset --hard -q "${base_commit}"
    if ((${#conflicts[@]} == 0)); then
        fail "cherry-pick failed without conflicts: ${SOURCE_COMMIT}"
    fi

    handoff_file=".github/backport-conflicts/${SOURCE_PR}-to-${CHANNEL}.md"
    write_conflict_handoff "${handoff_file}" "${branch}" \
        "${release_branch}" "${base_commit}" "${conflicts[@]}"
    git add "${handoff_file}"
    write_pr_body "${body_file}" conflict "${release_branch}" \
        "${handoff_file}" "${base_commit}"
    cp "${handoff_file}" "${OUTPUT_DIR}/conflict-handoff.md"
    write_outputs conflict "${branch}" "${body_file}"
}

main "$@"
